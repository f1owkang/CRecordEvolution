package main

import (
	"errors"
	"math"
	"os"
	"path/filepath"
	"strings"
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
	var se *SettleError
	// 去抖期内不触库；3 拍后进入结算路径才会写库失败
	_, err := r.p.Tick("Discharging")
	if err != nil {
		t.Fatalf("去抖第 1 拍不应触库, got %v", err)
	}
	_, err = r.p.Tick("Discharging")
	if err != nil {
		t.Fatalf("去抖第 2 拍不应触库, got %v", err)
	}
	_, err = r.p.Tick("Discharging")
	if !errors.As(err, &se) {
		t.Fatalf("落库失败应包成 SettleError, got %v", err)
	}
}

func TestStatsSigmaGatedByChannel(t *testing.T) {
	// ML 刷回 stable 且 data 残留时，kv 中过期的 σ 不得泄漏进 stable 的任何出口
	old := channel
	defer func() { channel = old }()
	_, st := newTestStable(t)
	if err := st.KVSet(kvKeyEmaSigma, "12.5"); err != nil {
		t.Fatalf("预置 kv: %v", err)
	}
	a := &app{st: st}

	channel = "stable"
	d, snap, err := a.stats()
	if err != nil {
		t.Fatalf("stats: %v", err)
	}
	if snap.SigmaMah != nil {
		t.Fatalf("stable 通道不应读出 SigmaMah, got %v (design=%v)", *snap.SigmaMah, d.DesignMah)
	}

	channel = "ml"
	_, snap, err = a.stats()
	if err != nil {
		t.Fatalf("stats(ml): %v", err)
	}
	if snap.SigmaMah == nil || *snap.SigmaMah != 12.5 {
		t.Fatalf("ml 通道应读出 σ=12.5, got %v", snap.SigmaMah)
	}
}

func TestSigmaMahRejectsNonFinite(t *testing.T) {
	// ParseFloat("NaN")/("Inf") err=nil 且不满足 <=0，必须显式拦截
	_, st := newTestStable(t)
	for _, val := range []string{"NaN", "Inf", "+Inf", "-Inf"} {
		if err := st.KVSet(kvKeyEmaSigma, val); err != nil {
			t.Fatalf("预置 kv[%s=%q]: %v", kvKeyEmaSigma, val, err)
		}
		a := &app{st: st}
		if got := a.sigmaMah(); got != nil {
			t.Fatalf("σ=%q 应被判 nil, got %v", val, *got)
		}
	}
}

func TestCycleEquivUsesHoursBase(t *testing.T) {
	// charged_ua_total 单位 µA·s，除以 designUA(µAh) 前必须 ÷3600 折算小时：
	// 设备实测值 109516965000 µA·s ÷ 8600000 µAh ≈ 3.54 循环，
	// 旧实现漏 ÷3600 会显示 12734.53
	got := cycleEquiv(109516965000, 8600000)
	want := 109516965000.0 / (8600000.0 * 3600)
	if math.Abs(got-want) > 1e-9 {
		t.Fatalf("cycleEquiv = %v, want %v", got, want)
	}
	if got := cycleEquiv(4320000000, 4000000); got != 0.3 {
		t.Fatalf("cycleEquiv = %v, want 0.3", got)
	}
	if !math.IsInf(cycleEquiv(1000, 0), 1) {
		t.Fatal("designUA=0 应返回 +Inf（出口按非有限值降级）")
	}
}

func TestMedianMohRobust(t *testing.T) {
	// 中位数口径：单个 5~10 倍离群值不得拉动显示值（旧 EMA 实测被踢高 7 倍）
	mos := []float64{11.9, 12.4, 11.1, 246.3, 12.8, 11.5, 12.1}
	got := medianMoh(mos)
	if got == nil || math.Abs(*got-12.1) > 1e-9 {
		t.Fatalf("medianMoh = %v, want 12.1", got)
	}
	even := medianMoh([]float64{10, 20})
	if even == nil || *even != 15 {
		t.Fatalf("偶数个中位数 = %v, want 15", even)
	}
	if medianMoh(nil) != nil {
		t.Fatal("空切片应返回 nil")
	}
	if medianMoh([]float64{0, -1}) != nil {
		t.Fatal("无正值应返回 nil")
	}
}

func TestAppendLogRotate(t *testing.T) {
	dir := t.TempDir()
	a := &app{moddir: dir}
	a.appendLog("第一条 %d", 1)
	for i := 0; i < 20000; i++ { // 超过 maxLogBytes 触发收敛
		a.appendLog("填充行")
	}
	a.appendLog("最新一条")
	data, err := os.ReadFile(filepath.Join(dir, "data", "batteryd.log"))
	if err != nil {
		t.Fatal(err)
	}
	if len(data) > maxLogBytes+512 {
		t.Fatalf("日志未收敛: %d bytes", len(data))
	}
	if !strings.Contains(string(data), "最新一条") {
		t.Fatal("收敛时应保留尾部")
	}
	if !strings.HasPrefix(strings.SplitN(string(data), "\n", 2)[0], "[") {
		t.Fatal("行首应带时间戳")
	}
}
