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

func TestSysFSReadIntSigned(t *testing.T) {
	s := newTestSysFS(t)
	cases := []struct {
		content string
		want    int64
		wantErr bool
	}{
		{"-500000\n", -500000, false},
		{"500000\n", 500000, false},
		{"-0\n", 0, false},
		{"abc\n", 0, true},
		{"\n", 0, true},
	}
	for i, tc := range cases {
		t.Run(string(rune('a'+i)), func(t *testing.T) {
			p := filepath.Join(s.Base, "signed.txt")
			writeFile(t, p, tc.content)
			got, err := s.ReadIntSigned(p)
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
	// 负值（放电方向）与正值同口径：旧实现写的是 raw > 10000，负值一律落进
	// ×1000 分支，-1.5A 会被放大成 -1500A。
	if got := NormCurrentUA(-1500000); got != -1500000 {
		t.Fatalf("NormCurrentUA(-1500000) = %d, want -1500000", got)
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

// 单位交叉校验：以 charge_full 的量纲为锚点区分 mA / µA。
// 用例取自实机回归（6050mAh 手机，current_now 报 µA）：旧口径拿 charge_full/10
// 当幅值阈值，把 0.6A 以下的读数整批判成 mA 再 ×1000，采样库里因此每天多出
// 200+ 条百安级电流，充电积分与中段校准全线失真。
func TestNormCurrentUAWithFull(t *testing.T) {
	const fullUA = 6_050_000 // 6050mAh 以 µAh 计 ⇒ 同驱动的 current_now 是 µA
	const fullMah = 6_050    // 同容量以 mAh 计 ⇒ current_now 是 mA
	cases := []struct {
		name   string
		raw    int64
		fullUA int64
		want   int64
	}{
		{"µA 小电流直通（实测 40mA）", 40_000, fullUA, 40_000},
		{"µA 中等电流直通（实测 297mA）", 297_000, fullUA, 297_000},
		{"µA 快充直通（2.6A）", 2_655_000, fullUA, 2_655_000},
		{"µA 负值同口径", -40_000, fullUA, -40_000},
		{"mA 设备读数×1000", 812, fullMah, 812_000},
		{"mA 设备负值×1000", -812, fullMah, -812_000},
		// 量纲判成 mA 但读数越界（40A 不可能）：回退原值，不产生 1000 倍污染
		{"mA 设备越界读数回退原值", 40_000, fullMah, 40_000},
		// charge_full 不可用（0）时退化为幅值启发式：>10000 视为 µA
		{"无锚点大值直通", 1_500_000, 0, 1_500_000},
		{"无锚点小值×1000", 1500, 0, 1_500_000},
		{"无锚点涓流直通（实测静息 20mA）", 20_000, 0, 20_000},
	}
	for _, c := range cases {
		if got := NormCurrentUAWithFull(c.raw, c.fullUA); got != c.want {
			t.Errorf("%s: NormCurrentUAWithFull(%d, %d) = %d, want %d", c.name, c.raw, c.fullUA, got, c.want)
		}
	}
}

// 量纲锚点分界：charge_full ≥ 100000 只可能是 µAh（100Ah 电池不存在），
// 以下只可能是 mAh；两侧各取一例钉住边界。
func TestNormCurrentUAWithFullUnitAnchorBoundary(t *testing.T) {
	if got := NormCurrentUAWithFull(15_000, microUnitChargeMinUA); got != 15_000 {
		t.Fatalf("边界值及以上按 µAh 量纲直通, got %d", got)
	}
	if got := NormCurrentUAWithFull(15_000, microUnitChargeMinUA-1); got != 15_000_000 {
		t.Fatalf("边界值以下按 mAh 量纲 ×1000, got %d", got)
	}
}

// sysfs 暂时性错误不计入守护进程连续失败计数。
func TestTickStrikeIgnoresSysfsTransient(t *testing.T) {
	prev, kill := tickStrike(3, &SysfsTransient{Err: errStub{}})
	if kill {
		t.Fatal("瞬态 sysfs 错误不应触发 kill")
	}
	if prev != 3 {
		t.Fatalf("瞬态错误不应推进失败计数, got %d", prev)
	}
	// 对照：普通错误照常推进并在达上限时 kill
	if _, kill := tickStrike(tickFailMax-1, errStub{}); !kill {
		t.Fatal("普通错误达上限应 kill")
	}
	// SettleError 走安全侧：不计数、立即终止（结算失败可能脏数据，重启 daemon）
	if _, dead := tickStrike(tickFailMax-1, &SettleError{Err: errStub{}}); !dead {
		t.Fatal("SettleError 应立即终止 daemon")
	}
}

type errStub struct{}

func (errStub) Error() string { return "stub" }
