package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
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
	// 负值（放电方向）与正值同口径：判据必须取绝对值——写 raw > 10000 会让
	// 负值一律落进 ×1000 分支，-1.5A 被放大成 -1500A。
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
// 用例覆盖 µA 机型（6050mAh 手机，current_now 报 µA）：拿 charge_full/10
// 当幅值阈值会把 0.6A 以下的读数整批判成 mA 再 ×1000，读数被放成百安级，
// 充电积分与中段校准全线失真。
func TestNormCurrentUAWithFull(t *testing.T) {
	const fullUAh = 6_050_000 // 6050mAh 以 µAh 计 ⇒ 同驱动的 current_now 是 µA
	const fullMah = 6_050     // 同容量以 mAh 计 ⇒ current_now 是 mA
	cases := []struct {
		name    string
		raw     int64
		fullUAh int64
		want    int64
	}{
		{"µA 小电流直通（实测 40mA）", 40_000, fullUAh, 40_000},
		{"µA 中等电流直通（实测 297mA）", 297_000, fullUAh, 297_000},
		{"µA 快充直通（2.6A）", 2_655_000, fullUAh, 2_655_000},
		{"µA 负值同口径", -40_000, fullUAh, -40_000},
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
		if got := NormCurrentUAWithFull(c.raw, c.fullUAh); got != c.want {
			t.Errorf("%s: NormCurrentUAWithFull(%d, %d) = %d, want %d", c.name, c.raw, c.fullUAh, got, c.want)
		}
	}
}

// 量纲锚点分界：charge_full ≥ 100000 只可能是 µAh（100Ah 电池不存在），
// 以下只可能是 mAh；两侧各取一例钉住边界。
func TestNormCurrentUAWithFullUnitAnchorBoundary(t *testing.T) {
	if got := NormCurrentUAWithFull(15_000, microUnitChargeMinUAh); got != 15_000 {
		t.Fatalf("边界值及以上按 µAh 量纲直通, got %d", got)
	}
	if got := NormCurrentUAWithFull(15_000, microUnitChargeMinUAh-1); got != 15_000_000 {
		t.Fatalf("边界值以下按 mAh 量纲 ×1000, got %d", got)
	}
}

// charge_counter 量纲归一：以 charge_full（µAh）+ 显示电量双锚点判定。mAh 机型的
// 绝对读数比满容量小三个数量级（如 37% 时 1874，charge_full=5,226,000µAh，且
// 1874×1000≈37%×full）；µAh 机型即使低电量也在阈值之上（1% ≈ full/100），
// 显示电量为 0/缺失时一律不判，避免把低电量边缘的 µAh 读数放大 1000 倍。
func TestNormCounterUAh(t *testing.T) {
	const fullUAh = 5_226_000
	cases := []struct {
		name string
		raw  int64
		full int64
		cap  int64
		want int64
	}{
		{"µAh 机型直通（实测 1874mAh 报 1874000）", 1_874_000, fullUAh, 37, 1_874_000},
		{"mAh 机型 ×1000（issue #7：37% 报 1874）", 1874, fullUAh, 37, 1_874_000},
		{"mAh 机型满充 ×1000", 5226, fullUAh, 100, 5_226_000},
		{"mAh 机型但显示电量对不上不缩放", 1874, fullUAh, 80, 1874},
		{"显示电量为 0 时不缩放（低电量边缘）", 522, fullUAh, 0, 522},
		{"µAh 机型 1% 电量仍在阈值上", 52_260, fullUAh, 1, 52_260},
		{"charge_full 缺失不猜", 1874, 0, 37, 1874},
		{"charge_full 本身是 mAh 量纲时不改", 1874, 5226, 37, 1874},
		{"非正读数不变", 0, fullUAh, 37, 0},
		{"超出 mAh 合理上界不乘", 200_000, fullUAh, 37, 200_000},
	}
	for _, c := range cases {
		if got := normCounterUAh(c.raw, c.full, c.cap); got != c.want {
			t.Errorf("%s: normCounterUAh(%d, %d, %d) = %d, want %d",
				c.name, c.raw, c.full, c.cap, got, c.want)
		}
	}
}

// 历史迁移判定必须「已确定」才动手：µAh 机型判为 1，无法判定时 decided=false，
// 调用方放弃迁移而不是猜一个倍数去改库。
func TestCounterUnitDecided(t *testing.T) {
	const fullUAh = 5_226_000
	cases := []struct {
		name        string
		raw         int64
		full        int64
		cap         int64
		wantScale   int64
		wantDecided bool
	}{
		{"issue #7 读数与 37% 对应", 1874, fullUAh, 37, 1000, true},
		{"µAh 机型判为 µAh", 1_874_000, fullUAh, 37, 1, true},
		{"mAh 读数与显示电量对不上", 1874, fullUAh, 80, 1, false},
		{"显示电量缺失", 1874, fullUAh, 0, 1, false},
		{"charge_full 是 mAh 量纲", 1874, 5226, 37, 1, false},
		{"读数超上界按 µAh 判", 200_000, fullUAh, 37, 1, true},
		{"charge_full 缺失", 1874, 0, 37, 1, false},
	}
	for _, c := range cases {
		scale, decided := counterUnitDecided(c.raw, c.full, c.cap)
		if scale != c.wantScale || decided != c.wantDecided {
			t.Errorf("%s: counterUnitDecided(%d, %d, %d) = (%d,%v), want (%d,%v)",
				c.name, c.raw, c.full, c.cap, scale, decided, c.wantScale, c.wantDecided)
		}
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

// stubDTBFind 替换 find 执行器，记录每次调用的参数（含 bin）。
func stubDTBFind(t *testing.T, out map[string]string) *[]string {
	t.Helper()
	old := dtbFind
	var calls []string
	dtbFind = func(bin string, args ...string) ([]byte, error) {
		calls = append(calls, bin+" "+strings.Join(args, " "))
		// args = [-H, dir, -name, pattern] 或 [dir, -name, pattern]
		dir := args[0]
		if dir == "-H" {
			dir = args[1]
		}
		if p, ok := out[dir]; ok {
			return []byte(p), nil
		}
		return nil, errStub{}
	}
	t.Cleanup(func() { dtbFind = old })
	return &calls
}

// issue #14 根因之一：/proc/device-tree 在多数内核上只是指向
// /sys/firmware/devicetree/base 的符号链接，find 不跟随起始链接会返回空。
// 必须优先搜真实目录，且 find 带 -H 兜底。
func TestReadDTBatteryCapacityPrefersFirmwareDir(t *testing.T) {
	base := t.TempDir()
	realDir := filepath.Join(base, "firmware", "devicetree", "base")
	prop := filepath.Join(realDir, "soc", "i2c@89c000", "battery@55", "vivo,bat-capacity-mah")
	writeFile(t, prop, string([]byte{0x00, 0x00, 0x0c, 0x94})) // 0x0C94 = 3220mAh

	oldDirs := dtbCapacityDirs
	dtbCapacityDirs = []string{realDir, filepath.Join(base, "missing")}
	t.Cleanup(func() { dtbCapacityDirs = oldDirs })
	calls := stubDTBFind(t, map[string]string{realDir: prop + "\n"})

	v, err := readDTBatteryCapacity()
	if err != nil || v != 3_220_000 {
		t.Fatalf("应读到设备树 3220000µAh, got (%d,%v)", v, err)
	}
	if len(*calls) != 1 {
		t.Fatalf("命中真实目录后不应再试其它根目录, calls=%v", *calls)
	}
	if !strings.Contains((*calls)[0], "-H "+realDir) {
		t.Fatalf("find 应带 -H 且搜索真实目录, got %q", (*calls)[0])
	}
}

// 真实目录不存在时跳过它，回退到符号链接路径（老内核只有 /proc/device-tree）。
func TestReadDTBatteryCapacityFallsBackToProcTree(t *testing.T) {
	base := t.TempDir()
	procDir := filepath.Join(base, "device-tree")
	prop := filepath.Join(procDir, "soc", "battery@55", "vivo,bat-capacity-mah")
	writeFile(t, prop, string([]byte{0x00, 0x00, 0x0c, 0x94}))

	oldDirs := dtbCapacityDirs
	dtbCapacityDirs = []string{filepath.Join(base, "absent"), procDir}
	t.Cleanup(func() { dtbCapacityDirs = oldDirs })
	calls := stubDTBFind(t, map[string]string{procDir: prop + "\n"})

	v, err := readDTBatteryCapacity()
	if err != nil || v != 3_220_000 {
		t.Fatalf("应回退到第二根目录读到 3220000µAh, got (%d,%v)", v, err)
	}
	if len(*calls) == 0 || !strings.Contains((*calls)[0], procDir) {
		t.Fatalf("不存在的根目录不应触发 find, calls=%v", *calls)
	}
}

// find 不认 -H 的老 ROM/find 也要能读到：回退不带 -H 的调用。
func TestReadDTBatteryCapacityWithoutFindH(t *testing.T) {
	base := t.TempDir()
	realDir := filepath.Join(base, "base")
	prop := filepath.Join(realDir, "battery@55", "vivo,bat-capacity-mah")
	writeFile(t, prop, string([]byte{0x00, 0x00, 0x0c, 0x94}))

	oldDirs, oldFind := dtbCapacityDirs, dtbFind
	dtbCapacityDirs = []string{realDir}
	t.Cleanup(func() {
		dtbCapacityDirs, dtbFind = oldDirs, oldFind
	})
	seen := 0
	dtbFind = func(bin string, args ...string) ([]byte, error) {
		for _, a := range args {
			if a == "-H" {
				seen++
				return nil, errStub{}
			}
		}
		return []byte(prop + "\n"), nil
	}

	v, err := readDTBatteryCapacity()
	if err != nil || v != 3_220_000 {
		t.Fatalf("不带 -H 的回退调用应成功, got (%d,%v)", v, err)
	}
	if seen == 0 {
		t.Fatal("应先尝试带 -H 的调用")
	}
}

// 设备树里没有该属性时返回错误（调用方按设计容量缺失降级）。
func TestReadDTBatteryCapacityNotFound(t *testing.T) {
	oldDirs := dtbCapacityDirs
	dtbCapacityDirs = []string{t.TempDir()}
	t.Cleanup(func() { dtbCapacityDirs = oldDirs })
	stubDTBFind(t, nil)

	if v, err := readDTBatteryCapacity(); err == nil || v != 0 {
		t.Fatalf("应返回错误与 0, got (%d,%v)", v, err)
	}
}

// 大端 32 位、单位 mAh；0 值、短文件与非数值属性跳过，继续看下一个候选路径。
func TestReadDTBCapacityPaths(t *testing.T) {
	base := t.TempDir()
	zero := filepath.Join(base, "zero")
	short := filepath.Join(base, "short")
	junk := filepath.Join(base, "junk")
	good := filepath.Join(base, "good")
	writeFile(t, zero, string([]byte{0, 0, 0, 0}))
	writeFile(t, short, string([]byte{0, 0}))
	// 字符串属性 "3220" 按 big-endian 解出 0x33323230，必须被范围门控拒绝。
	writeFile(t, junk, "3220")
	writeFile(t, good, string([]byte{0x00, 0x00, 0x0c, 0x94}))

	got, ok := readDTBCapacityPaths([]string{filepath.Join(base, "absent"), zero, short, junk, good})
	if !ok || got != 3_220_000 {
		t.Fatalf("应跳过无效候选读到 3220000µAh, got (%d,%v)", got, ok)
	}
}
