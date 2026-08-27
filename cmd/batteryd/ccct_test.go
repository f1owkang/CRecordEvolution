package main

import (
	"strings"
	"testing"
	"time"
)

// 构造恒流升压跨窗场景（brief Step 1）：60 样本恒流、电压每 tick 匀升，
// 在窗 [ccctWinLo, ccctWinHi] 内恰好走 16 tick = 960 s。
func TestAnalyzeCCCTCrossesWindowOnceSecs960(t *testing.T) {
	var rows []SampleRow
	base := int64(1000)
	// 步长 6_250 µV/tick：首个 ≥3_900_000 与首个 ≥4_000_000 相差 16 tick。
	for i := 0; i < 60; i++ {
		rows = append(rows, SampleRow{
			TS:  base + int64(i)*tickSeconds,
			UA:  500_000,
			UV:  3_850_000 + int64(i)*6_250,
			Cap: int64(50 + i),
		})
	}
	segs := DetectCCSegs(rows, 5)
	if len(segs) != 1 {
		t.Fatalf("前置：应检出 1 段恒流, got %d", len(segs))
	}
	secs, ok := AnalyzeCCCT(rows, segs)
	if !ok {
		t.Fatal("跨整窗恒流段应为 ok=true")
	}
	if secs != 960 {
		t.Fatalf("secs = %d, want 960", secs)
	}
}

// 断裂样例：电流在段中途大幅跳变（变异系数超门限），前后两段各自不满足
// 「从窗下沿以下起步、越过窗上沿」——前段电压未及下沿、后段不再回落到下沿以下，
// 无可采信跨界段，ok=false。
func TestAnalyzeCCCTBrokenCurrentNoCross(t *testing.T) {
	var rows []SampleRow
	base := int64(2000)
	for i := 0; i < 30; i++ { // 平稳但电压低于窗下沿
		rows = append(rows, SampleRow{
			TS: base + int64(i)*tickSeconds, UA: 500_000,
			UV: 3_600_000 + int64(i)*10_000, Cap: int64(i),
		})
	}
	for i := 30; i < 35; i++ { // 跳变窗口：σ/μ 远超 0.10
		jolt := []int64{200_000, 700_000, 250_000, 650_000, 180_000}[i-30]
		rows = append(rows, SampleRow{
			TS: base + int64(i)*tickSeconds, UA: jolt,
			UV: 3_900_000 + int64(i-30)*10_000, Cap: 30,
		})
	}
	for i := 35; i < 60; i++ { // 恢复平稳，但电压已高于窗下沿且不复回
		rows = append(rows, SampleRow{
			TS: base + int64(i)*tickSeconds, UA: 500_000,
			UV: 3_950_000 + int64(i-35)*10_000, Cap: 35,
		})
	}
	segs := DetectCCSegs(rows, 5)
	if len(segs) != 2 {
		t.Fatalf("前置：跳变后应为两段, got %+v", segs)
	}
	if _, ok := AnalyzeCCCT(rows, segs); ok {
		t.Fatal("断裂电流不应产出可采信 CCCT")
	}
}

// 手工构造两个都完整跨越窗口的段：0 或 1 之外一律拒绝（无法唯一归因 Q=I·t）。
func TestAnalyzeCCCTMultipleCrossingSegmentsRejected(t *testing.T) {
	var rows []SampleRow
	for j := 0; j < 40; j++ {
		uv := 3_850_000 + int64(j)*10_000
		if j >= 16 {
			uv = 3_850_000 + int64(j-16)*10_000 // 回落后二次爬窗
		}
		rows = append(rows, SampleRow{TS: int64(j) * tickSeconds, UA: 500_000, UV: uv, Cap: 30})
	}
	segs := []CCSeg{
		{FromTs: 0, ToTs: 15 * tickSeconds, MeanUA: 500_000},
		{FromTs: 16 * tickSeconds, ToTs: 39 * tickSeconds, MeanUA: 500_000},
	}
	if _, ok := AnalyzeCCCT(rows, segs); ok {
		t.Fatal("两个跨界段应整体拒绝而非任取其一")
	}
}

func TestAnalyzeCCCTInsufficientInput(t *testing.T) {
	if _, ok := AnalyzeCCCT(nil, nil); ok {
		t.Fatal("空输入应 ok=false")
	}
	rows := []SampleRow{{TS: 0, UA: 500_000, UV: 3_950_000}}
	if _, ok := AnalyzeCCCT(rows, []CCSeg{{FromTs: 0, ToTs: 60, MeanUA: 500_000}}); ok {
		t.Fatal("单点不足跨窗，应 ok=false")
	}
}

// designUA==0 时倍率无从判定，视为不采信：记一条 ccct_skip，不写 ccct 表。
func TestRecordCCCTSkipsWithoutDesignCapacity(t *testing.T) {
	r := newPipeRig(t)
	seed := int64(tickBaseTs)
	p := NewPipeline(r.fs, r.st, NewStable(r.st), 0, func() time.Time { return time.Unix(seed, 0) })
	for j := 0; j < 35; j++ {
		if err := r.st.InsertSample(seed+int64(j)*tickSeconds, 500_000,
			3_850_000+int64(j)*6_250, 60); err != nil {
			t.Fatalf("种子样本: %v", err)
		}
	}
	p.recordCCCT(seed, seed+34*tickSeconds)

	if n := countRows(t, r.st, "ccct"); n != 0 {
		t.Fatalf("缺设计容量不应落 ccct 行, rows = %d", n)
	}
	var kinds []string
	dbRows, err := r.st.db.Query(`SELECT kind FROM events`)
	if err != nil {
		t.Fatalf("读取事件: %v", err)
	}
	defer dbRows.Close()
	for dbRows.Next() {
		var k string
		if err := dbRows.Scan(&k); err != nil {
			t.Fatalf("扫描事件: %v", err)
		}
		kinds = append(kinds, k)
	}
	if len(kinds) != 1 || kinds[0] != "ccct_skip" {
		t.Fatalf("应恰记 1 条 ccct_skip, got %v", kinds)
	}
}

// 有效结算 + 倍率达标：settle 后同步分析 samples，命中跨窗恒流段即落
// 一条 ccct 记录（0.1V 格点窗穿窗耗时 960 s）。
func TestSettleRecordsCCCTWithinGatedRate(t *testing.T) {
	r := newPipeRig(t)

	r.put(76, 1800000, 3_856_250) // k=1：uv=3_850_000+6_250
	r.step("Charging")
	for k := 2; k <= 31; k++ {
		r.put(88, 1800000, 3_850_000+int64(k)*6_250)
		r.step("Charging")
	}
	r.put(100, 1800000, 3_850_000+32*6_250)
	if out := r.step("Charging"); !out.SessionSettled {
		t.Fatal("cap=100 应封账结算")
	}

	sess := onlySession(t, r.st)
	if !sess.Valid {
		t.Fatalf("前置：会话应有效(端点 cap=%d/%d)", sess.StartCap, sess.EndCap)
	}
	if n := countRows(t, r.st, "estimates"); n != 1 {
		t.Fatalf("前置：estimates 行数 = %d, want 1", n)
	}
	if n := countRows(t, r.st, "ccct"); n != 1 {
		t.Fatalf("有效且倍率达标的会话应落 1 条 ccct, rows = %d", n)
	}
	var ts, vwLo, vwHi, secs int64
	if err := r.st.db.QueryRow(`SELECT ts,vw_lo,vw_hi,secs FROM ccct`).Scan(&ts, &vwLo, &vwHi, &secs); err != nil {
		t.Fatalf("读取 ccct: %v", err)
	}
	wantLoTs := tickBaseTs + 8*tickSeconds // 第 8 tick 恰为 3_900_000
	wantHiTs := tickBaseTs + 24*tickSeconds
	if ts != wantHiTs || vwLo != 3_900_000 || vwHi != 4_000_000 || secs != wantHiTs-wantLoTs {
		t.Fatalf("ccct = (%d,%d,%d,%d), want (%d,3900000,4000000,960)",
			ts, vwLo, vwHi, secs, wantHiTs)
	}
	if n := countRows(t, r.st, "events"); n != 0 {
		t.Fatalf("正常路径不应记事件, rows = %d", n)
	}
}

// 段均值 ×2 超过设计容量（>C/2）：不采信，记一条 ccct_skip 且不落 ccct 行。
func TestSettleSkipsCCCTWhenRateExceedsDesign(t *testing.T) {
	r := newPipeRig(t)

	r.put(76, 3000000, 3_850_000+6_250)
	r.step("Charging")
	for k := 2; k <= 18; k++ {
		r.put(90, 3000000, 3_850_000+int64(k)*6_250)
		r.step("Charging")
	}
	r.put(100, 3000000, 4_100_000)
	if out := r.step("Charging"); !out.SessionSettled {
		t.Fatal("cap=100 应封账结算")
	}
	sess := onlySession(t, r.st)
	if !sess.Valid {
		t.Fatal("前置：会话应有效")
	}
	if n := countRows(t, r.st, "ccct"); n != 0 {
		t.Fatalf("倍率超标不应落 ccct 行, rows = %d", n)
	}
	detail, kind := queryEventOnce(t, r.st, "ccct_skip")
	if !strings.Contains(detail, "倍率") {
		t.Fatalf("ccct_skip 细节应说明倍率缘由: %q (kind=%s)", detail, kind)
	}
}

// queryEventOnce 断言 events 表恰有一条指定 kind 的事件并返回 (detail, kind)。
func queryEventOnce(t *testing.T, st *Store, kind string) (string, string) {
	t.Helper()
	var d, k string
	if err := st.db.QueryRow(`SELECT detail, kind FROM events WHERE kind = ?`, kind).Scan(&d, &k); err != nil {
		t.Fatalf("读取事件 %q: %v", kind, err)
	}
	return d, k
}
