package main

import (
	"math"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

// icaSynthRows 构造恒流升压合成线：电压 3.70→4.30 V 每 tick 匀升 10 mV，
// 电流 baseUA；落在 [boostLoUV, boostHiUV) 的样本 tick 换用 boostUA，模拟
// 该电压域内注入的 dq 尖峰（brief Step 1：21-tick 局部增强）。
func icaSynthRows(boostLoUV, boostHiUV, baseUA, boostUA int64) []SampleRow {
	var rows []SampleRow
	for i := int64(0); i <= 600; i++ {
		uv := 3_700_000 + i*icaBinUV
		ua := baseUA
		if uv >= boostLoUV && uv < boostHiUV {
			ua = boostUA
		}
		rows = append(rows, SampleRow{TS: i * tickSeconds, UA: ua, UV: uv})
	}
	return rows
}

// 高斯形峰注入：电压 3.70→4.30 V 每 tick 匀升 10 mV，恒流 500 mA 基线上，
// 对电流叠加以 3.95 V 为中心的高斯放大（幅度 4 倍基线、σ=30 mV）模拟 dq 尖峰。
// 主峰应落于注入中心附近，peakUV ∈ [3_935_000, 3_965_000]。
func TestFindPeakGaussianInjectionAtThreePointNineFive(t *testing.T) {
	const (
		baseUA    = 500_000
		centerUV  = 3_950_000.0
		sigmaUV   = 30_000.0
		ampFactor = 4.0
	)
	var rows []SampleRow
	for i := int64(0); i <= 600; i++ {
		uv := 3_700_000 + i*icaBinUV
		d := float64(uv) - centerUV
		factor := 1.0 + (ampFactor-1.0)*math.Exp(-d*d/(2*sigmaUV*sigmaUV))
		rows = append(rows, SampleRow{TS: i * tickSeconds, UA: int64(math.Round(baseUA * factor)), UV: uv})
	}
	uv, h, ok := FindPeak(rows)
	if !ok {
		t.Fatal("含峰注入线应检出主峰")
	}
	if h <= 0 {
		t.Fatalf("绝对峰高应为正, got %v", h)
	}
	if uv < 3_935_000 || uv > 3_965_000 {
		t.Fatalf("peakUV = %d, want ∈ [3_935_000, 3_965_000]", uv)
	}
}

// 平滑无峰线：全程恒流匀升，ΔQ 各桶近似均一，无显著突出主峰，ok=false。
func TestFindPeakSmoothRampNoPeak(t *testing.T) {
	if _, _, ok := FindPeak(icaSynthRows(0, 0, 500_000, 500_000)); ok {
		t.Fatal("平滑无峰线不应给出主峰")
	}
}

func TestFindPeakInsufficientInput(t *testing.T) {
	if _, _, ok := FindPeak(nil); ok {
		t.Fatal("空输入应 ok=false")
	}
	one := []SampleRow{{TS: 0, UA: 500_000, UV: 3_950_000}}
	if _, _, ok := FindPeak(one); ok {
		t.Fatal("单点不足判峰，应 ok=false")
	}
}

// 全程电压都在搜索域之外：无可参与搜索的桶，ok=false。
func TestFindPeakAllBinsOutsideSearchDomain(t *testing.T) {
	var rows []SampleRow
	for i := int64(0); i < 20; i++ {
		rows = append(rows, SampleRow{TS: i * tickSeconds, UA: 500_000, UV: 3_400_000 + i*tickSeconds})
	}
	if _, _, ok := FindPeak(rows); ok {
		t.Fatal("搜索域外无峰可寻，应 ok=false")
	}
}

// ica_peaks 表 roundtrip 与 90 天清理白名单。
func TestICAPeaksRoundtripAndPrune(t *testing.T) {
	s := openTestStore(t, filepath.Join(t.TempDir(), "battery.db"))
	defer func() { _ = s.Close() }()

	if err := s.InsertICAPeak(100, 3_950_000, 1.0); err != nil {
		t.Fatalf("InsertICAPeak: %v", err)
	}
	if err := s.InsertICAPeak(200, 3_960_000, 0.98); err != nil {
		t.Fatalf("InsertICAPeak: %v", err)
	}
	got, err := s.RecentICAPeaks(1)
	if err != nil {
		t.Fatalf("RecentICAPeaks: %v", err)
	}
	want := ICAPeakRow{TS: 200, PeakUV: 3_960_000, PeakHRel: 0.98}
	if len(got) != 1 || got[0] != want {
		t.Fatalf("RecentICAPeaks(1) = %+v, want %+v", got, want)
	}

	if err := s.PruneBefore(150); err != nil {
		t.Fatalf("PruneBefore: %v", err)
	}
	if n := countRows(t, s, "ica_peaks"); n != 1 {
		t.Fatalf("清理后 ica_peaks 行数 = %d, want 1", n)
	}
}

const (
	icaTestEndTs   = int64(1777000000)
	icaTestBaseUA  = int64(800_000)
	icaTestBoostUA = int64(1_400_000)
)

// icaGatedRows 构造一条覆盖 3.85~4.05 V、中部 14 tick 以增强电流模拟峰的样本行。
func icaGatedRows() []SampleRow {
	return icaSynthRows(3_930_000, 4_070_000, icaTestBaseUA, icaTestBoostUA)
}

// 首个合格会话：Missing 基准时写 kv ica_peak_base 并落 rel=1 的行。
func TestRecordICAFirstSessionWritesBaseAndRelOne(t *testing.T) {
	r := newPipeRig(t)
	r.p.recordICA(icaTestEndTs, icaGatedRows())

	got, err := r.st.RecentICAPeaks(1)
	if err != nil {
		t.Fatalf("RecentICAPeaks: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("ica_peaks 行数 = %d, want 1", len(got))
	}
	if got[0].TS != icaTestEndTs {
		t.Fatalf("session_end_ts = %d, want %d", got[0].TS, icaTestEndTs)
	}
	if got[0].PeakHRel != 1.0 {
		t.Fatalf("首个会话 rel = %v, want 1", got[0].PeakHRel)
	}
	baseTxt, ok := kvString(t, r.st, kvICAPeakBase)
	if !ok {
		t.Fatal("首个合格会话应写入 kv ica_peak_base")
	}
	base, perr := strconv.ParseFloat(baseTxt, 64)
	if perr != nil || base <= 0 {
		t.Fatalf("kv ica_peak_base = %q 应为正数", baseTxt)
	}
}

// 已有基准时换算 rel=h_abs/base：基准加倍则同形状曲线的 rel 减半。
func TestRecordICARelScalesWithPresetBase(t *testing.T) {
	r1 := newPipeRig(t)
	r1.p.recordICA(icaTestEndTs, icaGatedRows())
	base1, _ := kvString(t, r1.st, kvICAPeakBase)

	r2 := newPipeRig(t)
	b, err := strconv.ParseFloat(base1, 64)
	if err != nil {
		t.Fatalf("前置：解析基准 %q: %v", base1, err)
	}
	if err := r2.st.KVSet(kvICAPeakBase, strconv.FormatFloat(b*2, 'f', -1, 64)); err != nil {
		t.Fatalf("预置基准: %v", err)
	}
	r2.p.recordICA(icaTestEndTs, icaGatedRows())

	got, err := r2.st.RecentICAPeaks(1)
	if err != nil {
		t.Fatalf("RecentICAPeaks: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("ica_peaks 行数 = %d, want 1", len(got))
	}
	const eps = 1e-9
	if diff := got[0].PeakHRel - 0.5; diff > eps || diff < -eps {
		t.Fatalf("rel = %v, want ≈ 0.5", got[0].PeakHRel)
	}
}

// 基准异常(<0)：跳过本会话，不落行。
func TestRecordICASkipsOnNegativeBase(t *testing.T) {
	r := newPipeRig(t)
	if err := r.st.KVSet(kvICAPeakBase, "-3"); err != nil {
		t.Fatalf("预置负基准: %v", err)
	}
	r.p.recordICA(icaTestEndTs, icaGatedRows())

	if n := countRows(t, r.st, "ica_peaks"); n != 0 {
		t.Fatalf("基准异常应跳过本会话, rows = %d", n)
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
	if len(kinds) == 0 {
		t.Fatal("跳过会话应在 events 留痕")
	}
	for _, k := range kinds {
		if !strings.HasPrefix(k, "ica") {
			t.Fatalf("事件 kind = %q 应为 ica 前缀", k)
		}
	}
}

// 基准损坏 "0"：ParseFloat 成功但除零得 +Inf，若不拦截将随 json.Marshal
// 整体报错使 batteryd json 输出失效——应与本会话跳过并 events 留痕。
func TestRecordICASkipsOnZeroBase(t *testing.T) {
	r := newPipeRig(t)
	if err := r.st.KVSet(kvICAPeakBase, "0"); err != nil {
		t.Fatalf("预置零基准: %v", err)
	}
	r.p.recordICA(icaTestEndTs, icaGatedRows())

	if n := countRows(t, r.st, "ica_peaks"); n != 0 {
		t.Fatalf("零基准应跳过本会话, rows = %d", n)
	}
	queryEventOnce(t, r.st, "ica_skip")
}

// 基准损坏 "NaN"：ParseFloat 成功且 NaN<0 为 false，原判定绕过——
// 应按异常降级：本会话不落行、events 留痕。
func TestRecordICASkipsOnNaNBase(t *testing.T) {
	r := newPipeRig(t)
	if err := r.st.KVSet(kvICAPeakBase, "NaN"); err != nil {
		t.Fatalf("预置 NaN 基准: %v", err)
	}
	r.p.recordICA(icaTestEndTs, icaGatedRows())

	if n := countRows(t, r.st, "ica_peaks"); n != 0 {
		t.Fatalf("NaN 基准应跳过本会话, rows = %d", n)
	}
	queryEventOnce(t, r.st, "ica_skip")
}

// 空门控行集合（如全部段倍率超限）：静默无痕返回。
func TestRecordICAEmptyGatedRowsNoop(t *testing.T) {
	r := newPipeRig(t)
	r.p.recordICA(icaTestEndTs, nil)
	if n := countRows(t, r.st, "ica_peaks"); n != 0 {
		t.Fatalf("空输入不应落行, rows = %d", n)
	}
	if n := countRows(t, r.st, "events"); n != 0 {
		t.Fatalf("空输入应静默, events = %d", n)
	}
}

// 有效结算挂接：中部适度增强电流仍低于 C/2 门控，结算后同步落 ICA 行并写基准。
func TestSettleRecordsICAWithinGatedRate(t *testing.T) {
	r := newPipeRig(t)

	boostLoK, boostHiK := 21, 35 // 到达电压 3_910_000..4_050_000，跨 3.95V 邻域
	r.put(76, icaTestBaseUA, 3_700_000+1*icaBinUV)
	for k := 2; k <= 37; k++ {
		ua := icaTestBaseUA
		if k > boostLoK && k <= boostHiK {
			ua = icaTestBoostUA // 1_400_000×2 = 2.8M < designUA 4M，门控通过
		}
		capV := int64(76 + k/4)
		if k == 37 {
			capV = 100
		}
		r.put(capV, ua, 3_700_000+int64(k)*icaBinUV)
		out := r.step("Charging")
		if k == 37 && !out.SessionSettled {
			t.Fatal("cap=100 应封账结算")
		}
	}

	sess := onlySession(t, r.st)
	if !sess.Valid {
		t.Fatalf("前置：会话应有效(端点 cap=%d/%d)", sess.StartCap, sess.EndCap)
	}
	if n := countRows(t, r.st, "ica_peaks"); n != 1 {
		t.Fatalf("有效且倍率达标的会话应落 1 条 ICA 记录, rows = %d", n)
	}
	got, err := r.st.RecentICAPeaks(1)
	if err != nil {
		t.Fatalf("RecentICAPeaks: %v", err)
	}
	if got[0].PeakHRel != 1.0 {
		t.Fatalf("首个会话 rel = %v, want 1", got[0].PeakHRel)
	}
	if got[0].PeakUV < 3_900_000 || got[0].PeakUV > 4_010_000 {
		t.Fatalf("peakUV = %d, want 注入域 [3_900_000, 4_010_000]", got[0].PeakUV)
	}
	if txt, ok := kvString(t, r.st, kvICAPeakBase); !ok {
		t.Fatal("settle 后应写 kv ica_peak_base")
	} else if b, perr := strconv.ParseFloat(txt, 64); perr != nil || b <= 0 {
		t.Fatalf("kv ica_peak_base = %q 应为正数", txt)
	}
	if n := countRows(t, r.st, "events"); n != 0 {
		t.Fatalf("正常路径不应记事件, rows = %d", n)
	}
}

// 段均值 ×2 超过设计容量：与 CCCT 共用同一门控，ICA 一并不落行。
func TestSettleSkipsICAWhenRateExceedsDesign(t *testing.T) {
	r := newPipeRig(t)

	r.put(76, 3_000_000, 3_850_000+6_250)
	r.step("Charging")
	for k := 2; k <= 18; k++ {
		r.put(90, 3_000_000, 3_850_000+int64(k)*6_250)
		r.step("Charging")
	}
	r.put(100, 3_000_000, 4_100_000)
	if out := r.step("Charging"); !out.SessionSettled {
		t.Fatal("cap=100 应封账结算")
	}
	sess := onlySession(t, r.st)
	if !sess.Valid {
		t.Fatal("前置：会话应有效")
	}
	if n := countRows(t, r.st, "ica_peaks"); n != 0 {
		t.Fatalf("倍率超标不应落 ICA 行, rows = %d", n)
	}
}

func TestJSONIncludesIcaPeaksEntries(t *testing.T) {
	ica := []icaEntry{{TS: 900, PeakUV: 3_950_000, PeakHRel: 1}, {TS: 1500, PeakUV: 3_960_000, PeakHRel: 0.97}}
	b, err := RenderJSON("stable", Design{}, Snapshot{}, nil, nil, nil, nil, ica, 0, time.Unix(0, 0))
	if err != nil {
		t.Fatal(err)
	}
	if want := `"ica_peaks":[{"ts":900,"peak_uv":3950000,"peak_h_rel":1},{"ts":1500,"peak_uv":3960000,"peak_h_rel":0.97}]`; !strings.Contains(string(b), want) {
		t.Fatalf("ica_peaks 序列化不符, got %s", b)
	}
	b2, _ := RenderJSON("stable", Design{}, Snapshot{}, nil, nil, nil, nil, []icaEntry{}, 0, time.Unix(0, 0))
	if !strings.Contains(string(b2), `"ica_peaks":[]`) {
		t.Fatalf("空切片应输出 [] 而非 null: %s", b2)
	}
}
