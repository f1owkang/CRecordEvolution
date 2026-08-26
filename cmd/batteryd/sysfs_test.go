package main

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func newTestSysFS(t *testing.T) SysFS {
	t.Helper()
	base := t.TempDir()
	return SysFS{Base: base, devices: filepath.Join(base, "devices")}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestSysFSFindNodeFixedPath(t *testing.T) {
	s := newTestSysFS(t)
	want := filepath.Join(s.Base, "battery", "temp")
	writeFile(t, want, "285\n")

	got, err := s.FindNode("temp")
	if err != nil {
		t.Fatalf("固定路径命中却返回错误: %v", err)
	}
	if got != want {
		t.Fatalf("固定路径命中: want %q, got %q", want, got)
	}
}

func TestSysFSFindNodeFallback(t *testing.T) {
	s := newTestSysFS(t)
	want := filepath.Join(s.devices, "power_supply", "bms", "charge_full")
	writeFile(t, want, "5000000\n")

	got, err := s.FindNode("charge_full")
	if err != nil {
		t.Fatalf("兜底命中却返回错误: %v", err)
	}
	if got != want {
		t.Fatalf("兜底命中: want %q, got %q", want, got)
	}
}

func TestSysFSFindNodeNotFound(t *testing.T) {
	s := newTestSysFS(t)

	_, err := s.FindNode("no_such_node")
	if !errors.Is(err, ErrNodeNotFound) {
		t.Fatalf("双失败应返回 ErrNodeNotFound, got %v", err)
	}
}

func TestSysFSReadInt(t *testing.T) {
	s := newTestSysFS(t)
	cases := []struct {
		file    string
		content string
		want    int64
		wantErr bool
	}{
		{"normal", "285\n", 285, false},
		{"spaces", " 5000000\r\n", 5000000, false},
		{"empty", "\n", 0, true},
		{"alpha", "abc\n", 0, true},
	}
	for _, tc := range cases {
		t.Run(tc.file, func(t *testing.T) {
			p := filepath.Join(s.Base, tc.file+".txt")
			writeFile(t, p, tc.content)

			got, err := s.ReadInt(p)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("内容 %q 应报错, got %d", tc.content, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("内容 %q 不应报错: %v", tc.content, err)
			}
			if got != tc.want {
				t.Fatalf("want %d, got %d", tc.want, got)
			}
		})
	}
}

func TestSysFSNormCurrentUA(t *testing.T) {
	if got := NormCurrentUA(500); got != 500000 {
		t.Fatalf("NormCurrentUA(500) = %d, want 500000", got)
	}
	if got := NormCurrentUA(1500000); got != 1500000 {
		t.Fatalf("NormCurrentUA(1500000) = %d, want 1500000", got)
	}
}

func TestSysFSNormTempC(t *testing.T) {
	if got := NormTempC(285); got != 28.5 {
		t.Fatalf("NormTempC(285) = %v, want 28.5", got)
	}
	if got := NormTempC(28); got != 28 {
		t.Fatalf("NormTempC(28) = %v, want 28", got)
	}
}
