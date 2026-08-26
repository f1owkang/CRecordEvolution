package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBuildDescriptionBase(t *testing.T) {
	d := Design{DesignMah: 4000, FullMah: 3850, Cycles: 210}
	snap := Snapshot{Pct: 96}
	want := "出厂设计容量为：4000 mAh，当前电池容量为：3850 mAh，电池循环次数为：210次，估算剩余容量百分比为：96%"
	if got := BuildDescription(d, snap); got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestBuildDescriptionWithMeasured(t *testing.T) {
	estUA := int64(3820000)
	d := Design{DesignMah: 4000, FullMah: 3850, Cycles: 210}
	snap := Snapshot{Pct: 96, EstUA: &estUA}
	want := "出厂设计容量为：4000 mAh，当前电池容量为：3850 mAh，电池循环次数为：210次，估算剩余容量百分比为：96%，实测估算容量为：3820 mAh"
	if got := BuildDescription(d, snap); got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func writeTestProp(t *testing.T, content string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "module.prop")
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatalf("写测试文件: %v", err)
	}
	return p
}

func readTestProp(t *testing.T, p string) string {
	t.Helper()
	out, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("读回文件: %v", err)
	}
	return string(out)
}

func TestWriteModulePropReplacesExistingLine(t *testing.T) {
	p := writeTestProp(t, "id=Charging_Record\nname=Charging Record\nversion=1.1\ndescription=旧的描述\nauthor=f1owkang\n")
	if err := WriteModuleProp(p, "新的描述"); err != nil {
		t.Fatalf("WriteModuleProp: %v", err)
	}
	want := "id=Charging_Record\nname=Charging Record\nversion=1.1\ndescription=新的描述\nauthor=f1owkang\n"
	if got := readTestProp(t, p); got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
	if _, err := os.Stat(p + ".tmp"); !os.IsNotExist(err) {
		t.Fatalf("临时文件应已被 rename 消费, stat err=%v", err)
	}
}

func TestWriteModulePropAppendsWhenMissing(t *testing.T) {
	p := writeTestProp(t, "id=Charging_Record\nname=Charging Record\nversion=1.1\n")
	if err := WriteModuleProp(p, "新的描述"); err != nil {
		t.Fatalf("WriteModuleProp: %v", err)
	}
	want := "id=Charging_Record\nname=Charging Record\nversion=1.1\ndescription=新的描述\n"
	if got := readTestProp(t, p); got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestWriteModulePropKeepsLFAndLineCount(t *testing.T) {
	p := writeTestProp(t, "a=1\nb=2\ndescription=x\nc=3\n")
	if err := WriteModuleProp(p, "y"); err != nil {
		t.Fatalf("WriteModuleProp: %v", err)
	}
	out, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("读回文件: %v", err)
	}
	if bytes.ContainsRune(out, '\r') {
		t.Fatalf("输出中出现 CR: %q", out)
	}
	if got, want := bytes.Count(out, []byte("\n")), 4; got != want {
		t.Fatalf("换行数 got %d, want %d", got, want)
	}
	if !strings.HasSuffix(string(out), "\n") {
		t.Fatalf("尾随换行丢失: %q", out)
	}
}
