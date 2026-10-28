package main

import (
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestDiagClean(t *testing.T) {
	cases := []struct {
		name string
		in   string
		max  int
		want string
	}{
		{"换行制表符折叠", "OnePlus\nPJE110\t(ace3)\r\n", 48, "OnePlus PJE110 (ace3)"},
		{"控制字符剔除", "a\x00b\x1fc", 48, "abc"},
		{"首尾空白", "  value  ", 48, "value"},
		{"超长截断", "abcdefghij", 6, "abcde…"},
		{"空串", " \n\t ", 48, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := diagClean(c.in, c.max); got != c.want {
				t.Fatalf("diagClean(%q, %d) = %q, want %q", c.in, c.max, got, c.want)
			}
		})
	}
}

func TestReadModuleVersion(t *testing.T) {
	path := filepath.Join(t.TempDir(), "module.prop")
	writeFile(t, path, "id=CRecordEvolution\nversion=1.3.6\nversionCode=35\n")
	if got, err := readModuleVersion(path); err != nil || got != "1.3.6(35)" {
		t.Fatalf("readModuleVersion = (%q,%v), want (\"1.3.6(35)\",nil)", got, err)
	}
	path2 := filepath.Join(t.TempDir(), "module.prop")
	writeFile(t, path2, "id=x\n")
	if _, err := readModuleVersion(path2); err == nil {
		t.Fatal("缺 version 应报错")
	}
}

func TestReadLogTail(t *testing.T) {
	path := filepath.Join(t.TempDir(), "batteryd.log")
	writeFile(t, path, "line1\n[10-28 20:00:00] [校准] scale=1000\nline2\n[10-28 20:01:00] [结算] 采信 x\nline3\n[10-28 20:02:00] 普通行\n"+
		"[10-28 20:03:00] 启动 channel=stable\n[10-28 20:04:00] 启动 channel=stable\n")
	got := readLogTail(path, 2)
	if len(got) != 2 {
		t.Fatalf("readLogTail 行数 = %d, want 2 (%v)", len(got), got)
	}
	if !strings.Contains(got[0], "校准") || !strings.Contains(got[1], "结算") {
		t.Fatalf("应优先保留关键词行且按时间排序: %v", got)
	}
	// 多次重启的启动行不得挤掉校准/结算，且同一行只保留一次。
	got = readLogTail(path, 5)
	starts := 0
	for _, line := range got {
		if strings.Contains(line, "启动") {
			starts++
		}
	}
	if starts > 1 {
		t.Fatalf("启动行应去重: %v", got)
	}
	if !strings.Contains(strings.Join(got, "\n"), "校准") {
		t.Fatalf("校准行不应被启动行挤掉: %v", got)
	}
	// 无关键词时退回最后 n 行
	plain := filepath.Join(t.TempDir(), "plain.log")
	writeFile(t, plain, "a\nb\nc very long "+strings.Repeat("x", 300)+"\n")
	got = readLogTail(plain, 2)
	if len(got) != 2 || got[0] != "b" || !strings.HasSuffix(got[1], "…") {
		t.Fatalf("无关键词时尾部行/截断不符: %v", got)
	}
	if readLogTail(filepath.Join(t.TempDir(), "missing.log"), 3) != nil {
		t.Fatal("缺文件应返回 nil")
	}
}

// 隐私白名单：诊断只允许读取固定非唯一属性，序列号/IMEI 等直接拒绝且不执行
// getprop。
func TestDiagPropAllowlist(t *testing.T) {
	for _, key := range []string{
		"ro.product.manufacturer", "ro.product.model", "ro.product.device",
		"ro.build.version.release", "ro.build.version.sdk", "ro.product.cpu.abi",
	} {
		if !diagPropAllowed(key) {
			t.Fatalf("%s 应在诊断白名单内", key)
		}
	}
	for _, key := range []string{
		"ro.serialno", "ro.imei", "persist.sys.imei", "ro.product.serial",
		"net.hostname", "android.os.Build.SERIAL", "ro.android_id",
	} {
		if diagPropAllowed(key) {
			t.Fatalf("%s 不应在诊断白名单内", key)
		}
		if got := diagGetprop(key); got != "" {
			t.Fatalf("diagGetprop(%q) = %q, want empty", key, got)
		}
	}
}

// diagText 示例：固定输入下检查字段齐全、格式单行、长度受限。
func TestRenderDiag(t *testing.T) {
	est := int64(5_273_000)
	sigma := 31.5
	scale := 1000.0
	anchor := 1.0
	raw, ua := int64(7000), int64(7_000_000)
	cap := int64(42)
	v := int64(4_210_000)
	cc := int64(2_341_000)
	ema, mid, dis, full := int64(5273), int64(5300), int64(5280), int64(5500)
	implied := int64(4_531_000)
	in := diagInput{
		Version: "1.3.6(35)", Channel: "stable", Root: "KernelSU",
		Manufacturer: "OnePlus", Model: "PJE110", Device: "aston", Android: "16", SDK: "36", ABI: "arm64",
		Design: Design{HasDesign: true, DesignMah: 5500, HasFull: true, FullMah: 5500, HasCycles: true, Cycles: 210},
		Snap: Snapshot{
			EstUAh: &est, Samples: 12, CycleEquiv: 1.42, TempC: &sigma,
			Calib: &CalibInfo{
				CurrentNode: "/sys/class/power_supply/battery/current_now",
				CounterNode: "/sys/class/power_supply/battery/charge_counter",
				Scale:       &scale, AnchorScale: &anchor, Unit: "ma", Raw: &raw, UA: &ua,
			},
			Quality: &Quality{OK: true, SpreadPct: 15, EmaMah: &ema, MidMah: &mid, DisMah: &dis, FullMah: &full},
		},
		Status: "Charging", Cap: &cap, VoltageUV: &v, CounterUAh: &cc,
		Sessions: []Session{
			{StartTs: 1_760_000_000, EndTs: 1_760_002_040, StartCap: 20, EndCap: 93,
				AccUAs: 14_280_000, CounterUAh: 3_850_011, AvgI: 7_000_000, Valid: true},
			{EndTs: 1_759_900_000, StartCap: 25, EndCap: 98, Valid: false, InvalidReason: "out_of_window", AvgI: 3_000},
		},
		Discharges:   []DischargeRow{{TS: 1_759_800_000, UAh: 3_308_000, StartCap: 100, EndCap: 27, Implied: &implied}},
		EventCounts:  map[string]int64{"cur_cal": 1, "counter_scale": 2},
		LastEvent:    &diagEvent{Kind: "counter_scale", Detail: "counter=3850011µAh 积分=3966µAh 保留电量计", TS: 1_760_002_000},
		LastSampleTS: 1_760_002_000,
		Now:          1_760_002_000 + 3*86400 + 120,
		LogTail:      []string{"[10-28 20:05:12] [结算] 采信 cap=20→93 dur=34m0s avgI=7000mA 口径=电量计 估算=5273mAh"},
		Loc:          time.UTC,
	}
	out := renderDiag(in)
	for _, want := range []string{
		"CRecordEvolution diag", "已脱敏",
		"module=1.3.6(35) channel=stable root=KernelSU android=16(sdk36) abi=arm64",
		"device=OnePlus PJE110 (aston)",
		"calib=x1000 ma anchor=x1 I_node=",
		"est=5273mAh samples=12 last_tick=",
		"quality=ok spread=15% ema/mid/dis/full=5273mAh/5300mAh/5280mAh/5500mAh",
		"live=status=Charging cap=42% Iraw=7000 I=7.00A V=4.21V T=31.5C cc=2341mAh",
		"kernel=full=5500mAh design=5500mAh cycles=210",
		"valid=1", "avgI=7000mA", "ua=14280000µA·s", "cc=3850011", "src=cc", "est=5273mAh",
		"valid=0 reason=out_of_window", "src=?",
		"discharge=", "released=3308mAh implied=4531mAh",
		"events7d=cur_cal=1 counter_scale=2",
		"last_event=", "(3d)", "counter_scale:", "保留电量计",
		"log=[10-28 20:05:12]",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("诊断缺字段 %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "\r") {
		t.Fatal("诊断不应含 CR")
	}
	if n := len([]rune(out)); n > diagMaxChars {
		t.Fatalf("诊断长度 %d 超上限 %d", n, diagMaxChars)
	}

	// 未校准（Scale=nil）时 calib 行应显示 auto，而不是 x?。
	in.Snap.Calib = &CalibInfo{
		CurrentNode: "/sys/class/power_supply/battery/current_now",
		CounterNode: "/sys/class/power_supply/battery/charge_counter",
		AnchorScale: &anchor, Unit: "ua", Raw: &raw, UA: &ua,
	}
	if got := renderDiag(in); !strings.Contains(got, "calib=auto ua anchor=x1 I_node=") {
		t.Fatalf("未校准 calib 行不符:\n%s", got)
	}
	// cc=0 的历史行显示 —，避免误读为“零充入”。
	zeroCC := diagInput{Version: "1.3.6(35)", Loc: time.UTC, Sessions: []Session{
		{EndTs: 1_760_000_000, StartCap: 20, EndCap: 80, AccUAs: 3_600_000_000, Valid: true},
	}}
	if got := renderDiag(zeroCC); !strings.Contains(got, "cc=—") {
		t.Fatalf("cc=0 应显示 —:\n%s", got)
	}
}

// 结构化行本身超长时按 rune 截断并标注；日志行不得突破上限。
func TestRenderDiagTruncates(t *testing.T) {
	in := diagInput{Version: "1.3.6(35)", Loc: time.UTC}
	for i := 0; i < 80; i++ {
		in.Sessions = append(in.Sessions, Session{
			EndTs: int64(1_760_000_000 + i*3600), StartCap: 10, EndCap: 90,
			AccUAs: 1, CounterUAh: 2, AvgI: 3000, Valid: true,
		})
	}
	out := renderDiag(in)
	if n := len([]rune(out)); n > diagMaxChars {
		t.Fatalf("截断后长度 %d 超上限 %d", n, diagMaxChars)
	}
	if !strings.HasSuffix(out, "…(truncated)") {
		t.Fatalf("超长输出应带截断标记, got tail %q", out[len(out)-40:])
	}
}

// 事件统计只取 kind 计数，供诊断摘要使用（不暴露 detail 正文）。
func TestEventCountsSince(t *testing.T) {
	s := openTestStore(t, filepath.Join(t.TempDir(), "battery.db"))
	defer func() { _ = s.Close() }()
	old := time.Now().Unix() - 30*86400
	if _, err := s.db.Exec(`INSERT INTO events(ts, kind, detail) VALUES(?,?,?)`, old, "cur_cal", "old"); err != nil {
		t.Fatal(err)
	}
	for _, kind := range []string{"cur_cal", "counter_scale", "counter_scale", "quality_warn"} {
		if err := s.InsertEvent(kind, "detail-should-not-appear"); err != nil {
			t.Fatal(err)
		}
	}
	counts, err := s.EventCountsSince(time.Now().Unix() - 7*86400)
	if err != nil {
		t.Fatal(err)
	}
	if counts["cur_cal"] != 1 || counts["counter_scale"] != 2 || counts["quality_warn"] != 1 {
		t.Fatalf("事件计数不符: %+v", counts)
	}
	out := diagEventsLine(counts)
	if strings.Contains(out, "detail") {
		t.Fatalf("诊断事件摘要不应包含 detail: %q", out)
	}
}
