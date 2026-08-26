package main

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestDeriveModdir(t *testing.T) {
	root := t.TempDir()
	exe := filepath.Join(root, "bin", "batteryd")
	if err := os.MkdirAll(filepath.Dir(exe), 0o755); err != nil {
		t.Fatalf("创建 bin 布局: %v", err)
	}
	if err := os.WriteFile(exe, []byte("#!/system/bin/sh\n"), 0o755); err != nil {
		t.Fatalf("写占位二进制: %v", err)
	}

	moddir := deriveModdir(exe)
	if moddir != root {
		t.Fatalf("deriveModdir = %q, want %q", moddir, root)
	}
	if got, want := filepath.Join(moddir, "module.prop"), filepath.Join(root, "module.prop"); got != want {
		t.Fatalf("propPath 拼接 = %q, want %q", got, want)
	}
	if got, want := filepath.Join(moddir, "data", "battery.db"), filepath.Join(root, "data", "battery.db"); got != want {
		t.Fatalf("dataDir 拼接 = %q, want %q", got, want)
	}
}

func TestTickStrikeToleratesThenFatalAtFive(t *testing.T) {
	transient := errors.New("sysfs 抖动")
	streak, dead := 0, false
	for i := 1; i <= 4; i++ {
		streak, dead = tickStrike(streak, transient)
		if dead {
			t.Fatalf("第 %d 次瞬时失败不应致命", i)
		}
	}
	streak, dead = tickStrike(streak, transient)
	if !dead || streak != tickFailMax {
		t.Fatalf("第 %d 次连续失败应致命, streak=%d dead=%v", tickFailMax, streak, dead)
	}
}

func TestTickStrikeSettleErrorImmediate(t *testing.T) {
	streak, dead := tickStrike(2, &SettleError{Err: errors.New("db 损坏")})
	if !dead {
		t.Fatal("结算落库错误应立即致命")
	}
	if streak != 2 {
		t.Fatalf("结算错误不应推进计数, streak=%d", streak)
	}
}

func TestTickStoreFailureWrappedAsSettleError(t *testing.T) {
	r := newPipeRig(t)

	r.put(30, 6000000, 4200000)
	r.step("Charging")

	_ = r.st.Close()
	r.put(30, 500000, 4300000)
	_, err := r.p.Tick("Discharging")
	var se *SettleError
	if !errors.As(err, &se) {
		t.Fatalf("落库失败应包成 SettleError, got %v", err)
	}
}
