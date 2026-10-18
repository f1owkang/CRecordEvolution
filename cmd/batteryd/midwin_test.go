package main

import (
	"os"
	"testing"
	"time"
)

// mkMidPasses 构造 n 次穿越 [lo,hi) 的诚实映射充电样本：容量 mah mAh、
// 电流 iUA µA、步长 dt 秒（1% 所需 tick 数 = mah·3600·1000/100/iUA/dt）。
// 段间以 UA=0 + 超过 midSegMaxDt 的间隔断段。
func mkMidPasses(base int64, passes, lo, hi int, mah, iUA, dt int64) []SampleRow {
	perPctTicks := mah * 3600_000 / 100 / iUA / dt
	var rows []SampleRow
	ts := base
	for p := 0; p < passes; p++ {
		cap, tick := lo, int64(0)
		for cap < hi {
			rows = append(rows, SampleRow{TS: ts, UA: iUA, UV: 3_900_000, Cap: int64(cap)})
			ts += dt
			tick++
			if tick%perPctTicks == 0 {
				cap++
			}
		}
		rows = append(rows, SampleRow{TS: ts, UA: 0, UV: 3_900_000, Cap: int64(hi)})
		ts += midSegMaxDt + 60
	}
	return rows
}

func TestMidImpliedHonestMapping(t *testing.T) {
	// 3 次穿越 30~90%，真实容量 5000 mAh 的诚实映射 ⇒ 隐含 ≈5000
	rows := mkMidPasses(1_700_000_000, 3, 30, 90, 5000, 4_000_000, 15)
	got, ok := MidImplied(rows, midWinLo, midWinHi)
	if !ok {
		t.Fatal("3 次穿越应产出隐含容量")
	}
	if d := got - 5_000_000; d < -30_000 || d > 30_000 {
		t.Fatalf("隐含应约 5000 mAh（误差 0.6%% 内）, got %d", got)
	}
}

func TestMidImpliedImmuneToTopCompression(t *testing.T) {
	// 每次穿越后附加 99% 长停滞大电流（顶部压缩 + CV 蓄水池形态），隐含不变
	rows := mkMidPasses(1_700_000_000, 3, 30, 90, 5000, 4_000_000, 15)
	ts := rows[len(rows)-1].TS
	for p := 0; p < 3; p++ {
		for k := 0; k < 60; k++ {
			rows = append(rows, SampleRow{TS: ts, UA: 2_000_000, UV: 4_350_000, Cap: 99})
			ts += 15
		}
		ts += midSegMaxDt + 60
	}
	got, ok := MidImplied(rows, midWinLo, midWinHi)
	if !ok {
		t.Fatal("应有隐含容量")
	}
	if d := got - 5_000_000; d < -30_000 || d > 30_000 {
		t.Fatalf("顶部停滞不应影响中段隐含: got %d", got)
	}
}

func TestMidImpliedCoverageGates(t *testing.T) {
	// 每格穿越 <3 次 ⇒ 拒绝
	if _, ok := MidImplied(mkMidPasses(1_700_000_000, 2, 30, 90, 5000, 4_000_000, 15), midWinLo, midWinHi); ok {
		t.Fatal("穿越次数不足应拒绝")
	}
	// 只穿 40~90（30~39 无覆盖）⇒ 拒绝
	if _, ok := MidImplied(mkMidPasses(1_700_000_000, 4, 40, 90, 5000, 4_000_000, 15), midWinLo, midWinHi); ok {
		t.Fatal("窗格覆盖不全应拒绝")
	}
	// 少量电量计回修（cap 下降）区间被跳过，不影响整体（稀疏注入 ~0.7%）
	rows := mkMidPasses(1_700_000_000, 3, 30, 90, 5000, 4_000_000, 15)
	for i := range rows {
		if i%137 == 0 && rows[i].Cap > 31 {
			rows[i].Cap--
		}
	}
	if got, ok := MidImplied(rows, midWinLo, midWinHi); !ok {
		t.Fatal("少量回修不应导致整体拒绝")
	} else if d := got - 5_000_000; d < -100_000 || d > 100_000 {
		t.Fatalf("回修跳过后隐含应仍约 5000, got %d", got)
	}
}

// 环境变量 CRE_MIDWIN_DB 指向真机 battery.db 时回放；CI 无此文件自动跳过。
// 2026-10-18 实测（stable v1.3.4，cc 覆盖最近 9 天）：[30,90) pass 归一 ≈5747 mAh；
// 修复前旧行回退电流积分只有 5381 mAh。
func TestMidImpliedReplayRealDeviceData(t *testing.T) {
	path := os.Getenv("CRE_MIDWIN_DB")
	if path == "" {
		t.Skip("未设置 CRE_MIDWIN_DB，跳过真机数据回放")
	}
	src, err := os.ReadFile(path)
	if err != nil {
		t.Skipf("读取 %s 失败: %v", path, err)
	}
	tmp := t.TempDir() + "/replay.db"
	if err := os.WriteFile(tmp, src, 0o600); err != nil {
		t.Fatal(err)
	}
	st, err := OpenStore(tmp)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	rows, err := st.SamplesRange(0, time.Now().Unix())
	if err != nil {
		t.Fatal(err)
	}
	got, ok := MidImplied(rows, midWinLo, midWinHi)
	if !ok {
		t.Fatalf("真机 %d 样本应满足覆盖门槛", len(rows))
	}
	t.Logf("真机回放: %d 样本 → 中段隐含 %.0f mAh", len(rows), float64(got)/1000)
	if mah := float64(got) / 1000; mah < 5600 || mah > 5900 {
		t.Fatalf("真机回放期望 5600~5900 mAh（离线基准 5747），got %.0f", mah)
	}
}

// 中段隐含口径：有电量计读数的区间按 cc 增量归因（µAh 直读），无 cc 的旧行
// 回退电流积分。构造「cc 口径 5000mAh / 积分口径 7500mAh」的对照数据，校验
// MidImplied 优先采信 cc、清掉 cc 后确实回退积分。
func TestMidImpliedPrefersCounterDeltas(t *testing.T) {
	// 3 段穿越 30~90%，每 1% 用 3 拍（15s）；cc 每 1% 恰好 +50000µAh（⇒5000mAh）
	// UA 固定 6A：积分口径每 1% = 6e6×45/3600 = 75000µAh（⇒7500mAh）
	var rows []SampleRow
	ts := int64(1_700_000_000)
	cc := int64(10_000_000)
	for p := 0; p < 3; p++ {
		for cap := 30; cap < 90; cap++ {
			for k := 0; k < 3; k++ {
				rows = append(rows, SampleRow{TS: ts, UA: 6_000_000, UV: 3_900_000, Cap: int64(cap), CC: cc})
				ts += 15
				cc += 50_000 / 3
			}
		}
		rows = append(rows, SampleRow{TS: ts, UA: 0, UV: 3_900_000, Cap: 90, CC: cc})
		ts += midSegMaxDt + 60
	}
	got, ok := MidImplied(rows, midWinLo, midWinHi)
	if !ok {
		t.Fatal("三段覆盖应可计算")
	}
	if d := got - 5_000_000; d < -60_000 || d > 60_000 {
		t.Fatalf("应采信电量计口径 ≈5000mAh，got %d", got)
	}
	for i := range rows {
		rows[i].CC = 0
	}
	gotUA, ok := MidImplied(rows, midWinLo, midWinHi)
	if !ok {
		t.Fatal("回退积分口径后仍应可计算")
	}
	if d := gotUA - 7_500_000; d < -90_000 || d > 90_000 {
		t.Fatalf("无 cc 时应回退积分口径 ≈7500mAh，got %d", gotUA)
	}
}

// 有电量计时，缺 cc 的旧区间必须跳过而不是回退积分：构造 3 段 cc（5000mAh）
// + 3 段无 cc 旧行（积分口径 3000mAh）。修复前按 pass 混合会得到 ≈4000mAh，
// 修复后只采信 cc 段（≈5000mAh）。
func TestMidImpliedSkipsLegacyRowsWhenCounterAvailable(t *testing.T) {
	var rows []SampleRow
	ts := int64(1_700_000_000)
	cc := int64(10_000_000)
	for p := 0; p < 3; p++ {
		for cap := 30; cap < 90; cap++ {
			for k := 0; k < 3; k++ {
				rows = append(rows, SampleRow{TS: ts, UA: 6_000_000, UV: 3_900_000, Cap: int64(cap), CC: cc})
				ts += 15
				cc += 50_000 / 3
			}
		}
		rows = append(rows, SampleRow{TS: ts, UA: 0, UV: 3_900_000, Cap: 90, CC: cc})
		ts += midSegMaxDt + 60
	}
	rows = append(rows, mkMidPasses(ts, 3, 30, 90, 3000, 4_000_000, 27)...)

	got, ok := MidImplied(rows, midWinLo, midWinHi)
	if !ok {
		t.Fatal("cc 段覆盖应满足门槛")
	}
	if d := got - 5_000_000; d < -60_000 || d > 60_000 {
		t.Fatalf("旧行应被跳过，仅采信 cc 口径 ≈5000mAh，got %d", got)
	}
}

// 有电量计但 cc 段覆盖不足：宁可返回 ok=false，也不回退旧行的偏低积分——
// 旧行积分若参与，覆盖门槛会假性满足，中段值又被拉低。
func TestMidImpliedCounterCoverageInsufficient(t *testing.T) {
	var rows []SampleRow
	ts := int64(1_700_000_000)
	cc := int64(10_000_000)
	for p := 0; p < 2; p++ { // 仅 2 次穿越，低于 midMinBinPass=3
		for cap := 30; cap < 90; cap++ {
			for k := 0; k < 3; k++ {
				rows = append(rows, SampleRow{TS: ts, UA: 6_000_000, UV: 3_900_000, Cap: int64(cap), CC: cc})
				ts += 15
				cc += 50_000 / 3
			}
		}
		rows = append(rows, SampleRow{TS: ts, UA: 0, UV: 3_900_000, Cap: 90, CC: cc})
		ts += midSegMaxDt + 60
	}
	rows = append(rows, mkMidPasses(ts, 3, 30, 90, 3000, 4_000_000, 27)...)
	if _, ok := MidImplied(rows, midWinLo, midWinHi); ok {
		t.Fatal("cc 段覆盖不足时应返回 ok=false，不得回退旧行积分")
	}
}

// 同一段里某窗格只有一部分区间有 cc：该窗格必须 Q 与 pass 一起丢弃，
// 不能拿部分 Q 去撞一整次穿越（否则隐含值会被拉偏）。
func TestMidImpliedDropsPartiallyCoveredBin(t *testing.T) {
	var rows []SampleRow
	ts := int64(1_700_000_000)
	cc := int64(10_000_000)
	for p := 0; p < 3; p++ { // 3 段完整 cc 穿越（5000mAh）
		for cap := 30; cap < 90; cap++ {
			for k := 0; k < 3; k++ {
				rows = append(rows, SampleRow{TS: ts, UA: 6_000_000, UV: 3_900_000, Cap: int64(cap), CC: cc})
				ts += 15
				cc += 50_000 / 3
			}
		}
		rows = append(rows, SampleRow{TS: ts, UA: 0, UV: 3_900_000, Cap: 90, CC: cc})
		ts += midSegMaxDt + 60
	}
	// 一段 30→31 的部分覆盖：跨 30→31 的那拍缺 cc，30 格只剩部分 Q
	rows = append(rows,
		SampleRow{TS: ts, UA: 6_000_000, UV: 3_900_000, Cap: 30, CC: 1_000},
		SampleRow{TS: ts + 15, UA: 6_000_000, UV: 3_900_000, Cap: 30, CC: 151_000},
		SampleRow{TS: ts + 30, UA: 6_000_000, UV: 3_900_000, Cap: 31},
		SampleRow{TS: ts + 45, UA: 0, UV: 3_900_000, Cap: 31},
	)
	got, ok := MidImplied(rows, midWinLo, midWinHi)
	if !ok {
		t.Fatal("完整段覆盖应达标")
	}
	if d := got - 5_000_000; d < -30_000 || d > 30_000 {
		t.Fatalf("部分覆盖的窗格应整格丢弃，隐含应约 5000mAh，got %d", got)
	}
}

// 中段校准与会话通道同口径的窗口门控：隐含满容量越界不得混入 EMA——这条旁路
// 直接驱动用户可见的「实测」值，量纲误判把电流积分放大 1000 倍时它是唯一出口。
func TestCalibrateMidRejectsOutOfWindowImplied(t *testing.T) {
	r := newPipeRig(t)
	// 8000mAh 的映射 ⇒ 隐含 ≈8000000µAh，超出设计容量 4000000 的 1.5 倍上界
	for _, row := range mkMidPasses(tickBaseTs-20*86400, 3, 30, 90, 8000, 4_000_000, 15) {
		if err := r.st.InsertSample(row.TS, row.UA, row.UV, row.Cap, 0); err != nil {
			t.Fatal(err)
		}
	}
	if err := r.st.KVSet(kvKeyEmaUA, "4000000"); err != nil {
		t.Fatal(err)
	}
	r.p.calibrateMid(tickBaseTs)
	if got := kvInt(r.st, kvKeyEmaUA); got != 4_000_000 {
		t.Fatalf("越界隐含值不应混入 EMA, ema = %d", got)
	}
	var n int
	if err := r.st.db.QueryRow(`SELECT count(*) FROM events WHERE kind='mid_cal_skip'`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("越界应落 1 条 mid_cal_skip 事件, got %d", n)
	}
}

func TestSettleAppliesMidCalibrationOncePerDay(t *testing.T) {
	r := newPipeRig(t)
	// 预置 EMA=5300 与 3 次诚实穿越（5000 mAh），埋在会话时间之前
	r.st.KVSet(kvKeyEmaUA, "5300000")
	r.st.KVSet(kvKeySamples, "5")
	for _, row := range mkMidPasses(tickBaseTs-20*86400, 3, 30, 90, 5000, 4_000_000, 15) {
		if err := r.st.InsertSample(row.TS, row.UA, row.UV, row.Cap, 0); err != nil {
			t.Fatal(err)
		}
	}
	// 完整充电会话 20→80：rig 的 step() 每拍自带 60s，3000mA×60s = 180000µA·s
	// 恰为 5000mAh 的 1%，每拍 +1%，诚实映射
	runSession := func(startCap int64) {
		r.put(startCap, 3_000_000, 3_700_000)
		r.step("Charging")
		for i := 1; i < 60; i++ {
			r.put(startCap+int64(i), 3_000_000, 3_700_000)
			r.step("Charging")
		}
		r.put(startCap+60, 0, 4_100_000)
		for k := 0; k < 3; k++ {
			if out := r.step("Discharging"); out.SessionSettled {
				return
			}
		}
		r.t.Fatal("三次静息拍内未结算")
	}
	runSession(20)

	calTs := kvInt(r.st, kvMidCalTs)
	if calTs == 0 {
		t.Fatal("采信结算后应记录 mid_cal_ts")
	}
	ema := kvInt(r.st, kvKeyEmaUA)
	// 会话结算(5300→5270)后校准再拉向 5000：期望 ≈5243；留 ±2% 容差
	if ema < 5_140_000 || ema > 5_340_000 || ema >= 5_270_000 {
		t.Fatalf("校准后 EMA 应位于 (5270000 之下且趋向 5000000), got %d", ema)
	}

	// 同日第二次采信结算：mid_cal_ts 不被刷新（<24h 门控）
	r.cur = r.cur.Add(2 * time.Hour)
	runSession(25)
	if got := kvInt(r.st, kvMidCalTs); got != calTs {
		t.Fatalf("同日第二次结算不应再次校准: mid_cal_ts %d → %d", calTs, got)
	}
}
