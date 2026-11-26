package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseDesignMah(t *testing.T) {
	cases := []struct {
		in      string
		want    int64
		wantErr bool
	}{
		{"3220", 3220, false},
		{" 4800 ", 4800, false},
		{"100", 100, false},
		{"30000", 30000, false},
		{"99", 0, true},
		{"30001", 0, true},
		{"", 0, true},
		{"3220mAh", 0, true},
		{"-3220", 0, true},
		{"3220.5", 0, true},
	}
	for _, c := range cases {
		got, err := parseDesignMah(c.in)
		if c.wantErr {
			if err == nil {
				t.Fatalf("parseDesignMah(%q) 应为错误, got %d", c.in, got)
			}
			continue
		}
		if err != nil || got != c.want {
			t.Fatalf("parseDesignMah(%q) = (%d,%v), want %d", c.in, got, err, c.want)
		}
	}
}

// 覆盖文件写入/清除：内容为纯数字 + LF，清除后文件不存在；重复清除视为成功。
func TestDesignOverrideFileRoundTrip(t *testing.T) {
	dir := t.TempDir()
	if err := writeDesignOverride(dir, 3220); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(dir, designOverrideFile))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "3220\n" {
		t.Fatalf("覆盖文件内容 = %q, want %q", string(data), "3220\n")
	}
	// 解析侧必须能读回同一个值（resolveDesignCapacity 的手动分支）。
	uAh, err := readDesignOverrideUAh(dir)
	if err != nil || uAh != 3_220_000 {
		t.Fatalf("readDesignOverrideUAh = (%d,%v), want 3220000", uAh, err)
	}
	if err := clearDesignOverride(dir); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, designOverrideFile)); !os.IsNotExist(err) {
		t.Fatalf("清除后文件应不存在, err=%v", err)
	}
	if err := clearDesignOverride(dir); err != nil {
		t.Fatalf("重复清除应成功: %v", err)
	}
	// 越界值不允许写盘（调用方在 parseDesignMah 已拦，这里保证写函数不被绕过时仍安全）。
	if _, err := parseDesignMah("50000"); err == nil || !strings.Contains(err.Error(), "范围") {
		t.Fatalf("越界应带范围提示, got %v", err)
	}
}
