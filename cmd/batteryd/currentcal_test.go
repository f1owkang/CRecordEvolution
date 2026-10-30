package main

import (
	"database/sql"
	"math"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// 自校准刻度学习：正常设备（原始积分 vs 电量计偏差 5~15%）吸附到 ×1；欧加系
// current_now 报 mA 时吸附到 ×1000；异常比值（电量计缺口/非 10 的幂）拒绝。
// 入参是「原始读数·秒」积分（不是被当前刻度缩放过的 accUAs）。
func TestLearnCurrentScale(t *testing.T) {
	const rawUAs = int64(5_000_000) * 3600 // 5A 原始读数持续 1 小时
	cases := []struct {
		name    string
		rawUAs  int64
		counter int64
		want    float64
		wantOK  bool
	}{
		// 原始读数已是 µA：原始积分=电量计
		{"uA 直通", rawUAs, 5_000_000, 1, true},
		// 原始积分相对电量计偏低 15%（正常设备实测范围），仍应吸附到 ×1
		{"uA 偏低 15%", rawUAs, 5_750_000, 1, true},
		// issue #6：原始读数是 mA（5000 表示 5A），电量计差分是 µAh
		{"mA 原始读数", 5000 * 3600, 5_000_000, 1000, true},
		// 电量计差分远大于原始积分（比值约 1.93×10³）：双电芯等效容量口径下
		// 积分只有电量计的一半，仍需吸附到 ×1000
		{"比值 1.93e3", 2_001_720, 1_074_000, 1000, true},
		// 比值 3.5 倍：落在相邻 10 的幂吸附区之间的拒绝带，仍不校准
		{"比值异常 3.5x", rawUAs, 17_500_000, 0, false},
		// 电量计缺口让比值偏小 10 倍：数量级合法，纯函数层按幂次采信；调用方
		// （结算口径裁决）会先剔除不可信电量计
		{"电量计缺口 10x", rawUAs, 500_000, 0.1, true},
		{"无原始积分", 0, 5_000_000, 0, false},
		{"无电量计", rawUAs, 0, 0, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, ratio, ok := learnCurrentScale(c.rawUAs, c.counter)
			if ok != c.wantOK || got != c.want {
				t.Fatalf("learnCurrentScale(%d, %d) = (%v, ratio=%v, %v), want (%v, ratio>0, %v)",
					c.rawUAs, c.counter, got, ratio, ok, c.want, c.wantOK)
			}
			// 比值同时返回给调用方写 cur_cal_fail：合法输入时必须为正数
			if c.rawUAs > 0 && c.counter > 0 && ratio <= 0 {
				t.Fatalf("合法输入应返回正比值, got %v", ratio)
			}
		})
	}
}

// 结算自校准的落库行为：比值落在吸附区内时学到对应刻度并落 cur_cal 事件；
// 同一刻度不重复落盘/落事件；学不出刻度时落 cur_cal_fail 事件并带原始比值，
// 且不覆盖已有刻度。
func TestLearnCurrentScaleFromSessionEvents(t *testing.T) {
	r := newPipeRig(t)
	// 原始积分 2,001,720µA·s、电量计 1,074,000µAh（比值约 1.93×10³）
	scale, learned := r.p.learnCurrentScaleFromSession(2_001_720, 1_074_000)
	if !learned || scale != 1000 {
		t.Fatalf("学到刻度 = (%v, %v), want (1000, true)", scale, learned)
	}
	if got, ok := kvString(t, r.st, kvCurrentScale); !ok || got != "1000" {
		t.Fatalf("kv[cur_scale] = (%q,%v), want (\"1000\",true)", got, ok)
	}
	var n int
	if err := r.st.db.QueryRow(`SELECT count(*) FROM events WHERE kind='cur_cal'`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("cur_cal = %d, want 1", n)
	}

	// 同一刻度再学一次：不重复落盘/落事件，也不触发本会话重算
	if scale, learned = r.p.learnCurrentScaleFromSession(2_001_720, 1_074_000); learned || scale != 1000 {
		t.Fatalf("重复学习 = (%v, %v), want (1000, false)", scale, learned)
	}
	if err := r.st.db.QueryRow(`SELECT count(*) FROM events WHERE kind='cur_cal'`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("刻度未变不应重复落 cur_cal, got %d", n)
	}

	// 比值 3.5 倍：不覆盖已学刻度，只落 cur_cal_fail 留痕
	if scale, learned = r.p.learnCurrentScaleFromSession(5_000_000*3600, 17_500_000); learned || scale != 0 {
		t.Fatalf("异常比值 = (%v, %v), want (0, false)", scale, learned)
	}
	if got, ok := kvString(t, r.st, kvCurrentScale); !ok || got != "1000" {
		t.Fatalf("学不出刻度时 kv[cur_scale] = (%q,%v), want (\"1000\",true)", got, ok)
	}
	var detail sql.NullString
	if err := r.st.db.QueryRow(`SELECT detail FROM events WHERE kind='cur_cal_fail' ORDER BY rowid DESC LIMIT 1`).Scan(&detail); err != nil {
		t.Fatalf("应落 cur_cal_fail 事件: %v", err)
	}
	if !strings.Contains(detail.String, "3.50") {
		t.Fatalf("cur_cal_fail detail 应带原始比值 3.50, got %q", detail.String)
	}
}

// 学到新刻度后本会话积分电量的就地换算：非正输入/刻度/溢出返回 0（调用方保留
// 原值），正常输入按刻度换算成 µA·s。
func TestScaledAccUAs(t *testing.T) {
	cases := []struct {
		name   string
		rawUAs int64
		scale  float64
		want   int64
	}{
		{"×1000", 2_001_720, 1000, 2_001_720_000},
		{"×1", 10_980_000, 1, 10_980_000},
		{"无原始积分", 0, 1000, 0},
		{"未校准", 2_001_720, 0, 0},
		{"溢出", math.MaxInt64/2 + 1, 1000, 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := scaledAccUAs(c.rawUAs, c.scale); got != c.want {
				t.Fatalf("scaledAccUAs(%d, %v) = %d, want %d", c.rawUAs, c.scale, got, c.want)
			}
		})
	}
}

func TestNormalizeCurrentUA(t *testing.T) {
	const fullUAh = int64(5_500_000)
	cases := []struct {
		name  string
		raw   int64
		full  int64
		scale float64
		want  int64
	}{
		// 未校准：旧逻辑把 mA 读数当 µA 直通（issue #6 的根因，保留作回归锚点）
		{"未校准 mA 直通", 3000, fullUAh, 0, 3000},
		{"学习刻度 mA→µA", 3000, fullUAh, 1000, 3_000_000},
		{"学习刻度负值", -3000, fullUAh, 1000, -3_000_000},
		{"无锚点大值启发式", 5_000_000, 0, 0, 5_000_000},
		// 学习刻度把读数推出 25A 物理上界：退回锚点逻辑，堵死 1000 倍误乘
		{"学习刻度越界回退", 5000, fullUAh, 1_000_000, 5000},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := normalizeCurrentUA(c.raw, c.full, c.scale); got != c.want {
				t.Fatalf("normalizeCurrentUA(%d, %d, %v) = %d, want %d",
					c.raw, c.full, c.scale, got, c.want)
			}
		})
	}
}

func TestCurrentUnitLabel(t *testing.T) {
	cases := []struct {
		name  string
		full  int64
		scale float64
		want  string
	}{
		{"µAh 锚点", 5_500_000, 0, "ua"},
		{"mAh 锚点", 5500, 0, "ma"},
		{"无锚点", 0, 0, "auto"},
		{"已校准 µA", 0, 1, "ua"},
		{"已校准 mA", 5_500_000, 1000, "ma"},
		{"非整幂次刻度", 5_500_000, 100, "auto"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := currentUnitLabel(c.full, c.scale); got != c.want {
				t.Fatalf("currentUnitLabel(%d, %v) = %q, want %q", c.full, c.scale, got, c.want)
			}
		})
	}
}

// 口径裁决：只有「电量计隐含容量在窗内、积分在窗外」才翻转旧行为；其余情况
// （积分可信、双方都不可信、掉幅不足、无参考容量）都维持旧口径。
func TestResolveCounterSource(t *testing.T) {
	const ref = int64(4_000_000)
	cases := []struct {
		name          string
		counterUAh    int64
		accUAs        int64
		delta, refUAh int64
		wantUse       bool
		wantReason    string
	}{
		// 60 点涨幅：电量计 3.0Ah ⇒ 隐含 5000mAh（窗内）；积分 3mAh ⇒ 5mAh（窗外）
		{"issue6 保留电量计", 3_000_000, 3000 * 3600, 60, ref, true, "counter_plausible"},
		// 电量计 30mAh ⇒ 隐含 50mAh（窗外）；积分 2.4Ah ⇒ 隐含 4000mAh（窗内）
		{"积分可信回退积分", 30_000, 2_400_000 * 3600, 60, ref, false, "integral_plausible"},
		// 两路都不可信：沿用旧行为（回退积分）
		{"双方不可信", 30_000, 3000 * 3600, 60, ref, false, "ambiguous"},
		// 相差不足 3 倍：直接采信电量计（旧行为）
		{"相差 2 倍", 2_000_000, 1_000_000 * 3600, 60, ref, true, "within_3x"},
		// 无掉幅：无法折算隐含容量，沿用旧行为
		{"无掉幅", 3_000_000, 3000 * 3600, 0, ref, false, "ambiguous"},
		// 无参考容量：沿用旧行为
		{"无参考容量", 3_000_000, 3000 * 3600, 60, 0, false, "ambiguous"},
		// 无电量计/无积分
		{"无电量计", 0, 3000 * 3600, 60, ref, false, "no_counter"},
		{"无积分", 3_000_000, 0, 60, ref, true, "no_integral"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			res := resolveCounterSource(c.counterUAh, c.accUAs, c.delta, c.refUAh)
			if res.UseCounter != c.wantUse || res.Reason != c.wantReason {
				t.Fatalf("resolveCounterSource = (use=%v, reason=%q), want (%v, %q)",
					res.UseCounter, res.Reason, c.wantUse, c.wantReason)
			}
		})
	}
}

func TestParseCurrentScale(t *testing.T) {
	for _, c := range []struct {
		in   string
		want float64
	}{
		{"1000", 1000}, {"1", 1}, {"abc", 0}, {"0", 0}, {"-1", 0}, {"", 0},
	} {
		if got := parseCurrentScale(c.in); got != c.want {
			t.Errorf("parseCurrentScale(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}

// issue #6 回归：一加 Ace 3 的 current_now 报 mA（原始 3000 被当 µA 直通），
// 同节点 charge_full / charge_counter 报 µAh。旧逻辑积分低估 1000 倍、结算时
// 把正确的电量计口径判成异常丢弃 ⇒ 估算 2mAh 且 out_of_window 拒收。修复后：
// 采信电量计、sessions.cc 落库、学习到 ×1000 刻度，且下一会话的采样电流正确。
func TestIssue6MilliampCurrentCalibration(t *testing.T) {
	r := newPipeRig(t)
	// 模拟该机型：charge_full 5.5Ah（µAh 量纲）⇒ 旧锚点认定 current_now 为 µA
	writeFile(t, filepath.Join(r.fs.Base, "battery", "charge_full"), fmtNode(5_500_000))
	r.rebuildPipeline()
	if err := r.st.KVSet(kvKeyEmaUAh, "4800000"); err != nil {
		t.Fatal(err)
	}
	if err := r.st.KVSet(kvKeySamples, "3"); err != nil {
		t.Fatal(err)
	}
	// 预置一个旧刻度下写成的 ICA 基准：学到 ×1000 后基准应同步 ×1000，否则
	// 新刻度算出的峰高相对旧基准会恒偏 1000×。
	if err := r.st.KVSet(kvICAPeakBase, "1234"); err != nil {
		t.Fatal(err)
	}

	cc := int64(6_000_000)
	putCounter(t, r, cc)
	// 61 拍 cap 20→80（涨幅 60 点），电量计每拍 +50000µAh ⇒ 合计 3000mAh，
	// 隐含 5000mAh 在设计窗口内；电流原始 3000、被当 µA 直通 ⇒ 积分隐含仅 5mAh。
	for i := int64(0); i <= 60; i++ {
		r.put(20+i, 3_000, 4_100_000)
		if i > 0 {
			cc += 50_000
			putCounter(t, r, cc)
		}
		r.step("Charging")
	}
	// 会话期内落一条按旧刻度算出的内阻（电流被当 µA 直通 ⇒ mo 偏大 1000 倍），
	// 校准回填后应缩回 1/1000。
	if err := r.st.InsertResistance(r.cur.Unix(), 50); err != nil {
		t.Fatal(err)
	}
	r.put(80, 20_000, 4_100_000)
	for k := 0; k < 3; k++ {
		if out := r.step("Full"); out.SessionSettled {
			break
		}
	}

	sess := onlySession(t, r.st)
	if !sess.Valid {
		t.Fatalf("应采信电量计口径，实际被拒 reason=%s", sess.InvalidReason)
	}
	if sess.CounterUAh != 3_000_000 {
		t.Fatalf("sessions.cc = %d, want 3000000（会话期电量计增量）", sess.CounterUAh)
	}
	// 隐含 = 3000000×100/60 = 5000000µAh；涨幅 60 < 满充封账 99 ⇒ EMA 权重 1/10
	wantEma := (int64(4_800_000)*9 + int64(5_000_000)) / 10
	if got := kvInt(r.st, kvKeyEmaUAh); got != wantEma {
		t.Fatalf("EMA = %d, want %d（电量计隐含 5000mAh）", got, wantEma)
	}
	if got, ok := kvString(t, r.st, kvCurrentScale); !ok || got != "1000" {
		t.Fatalf("kv[cur_scale] = (%q,%v), want (\"1000\",true)", got, ok)
	}
	// 学到刻度的这个会话自身也要按新刻度重算：否则历史里它的 ua 只有几分之一
	// mAh、avgI 显示 0mA、倍率 0.00C，看起来像「没校准」。
	rawUAs := kvInt(r.st, kvSessRawUAs)
	if rawUAs <= 0 {
		t.Fatalf("会话原始积分 = %d, want >0", rawUAs)
	}
	wantAcc := rawUAs * 1000
	if sess.AccUAs != wantAcc {
		t.Fatalf("会话 ua = %d, want %d（原始积分 × 1000）", sess.AccUAs, wantAcc)
	}
	if sess.AvgI != sess.AccUAs/sess.Duration {
		t.Fatalf("会话 avgI = %d, want %d", sess.AvgI, sess.AccUAs/sess.Duration)
	}
	if sess.CRate <= 0 {
		t.Fatalf("会话倍率 = %v, want >0", sess.CRate)
	}
	var n int
	if err := r.st.db.QueryRow(`SELECT count(*) FROM events WHERE kind='counter_scale'`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("应落 1 条 counter_scale 裁决事件, got %d", n)
	}
	if err := r.st.db.QueryRow(`SELECT count(*) FROM events WHERE kind='cur_cal'`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("应落 1 条 cur_cal 事件, got %d", n)
	}

	// 校准会话自身落库的样本必须已按新刻度回填（否则 recordCCCT/recordICA 会用
	// 旧刻度样本，把 1000× 偏小的峰高写进 ica_peak_base）。区间回填不早于首拍、
	// 不晚于结算时刻，全部 61 条样本都应在内。
	firstRows, err := r.st.SamplesRange(sess.StartTs, sess.EndTs)
	if err != nil {
		t.Fatal(err)
	}
	if len(firstRows) < 30 {
		t.Fatalf("校准会话样本数 = %d, 过少", len(firstRows))
	}
	for _, row := range firstRows {
		if row.UA != 3_000_000 {
			t.Fatalf("校准会话样本 ua = %dµA, want 3000000（已按新刻度回填）", row.UA)
		}
	}
	// 内阻按 1/刻度 缩放：R=ΔU/ΔI，电流放大 1000 倍则内阻缩小 1000 倍。
	var mo float64
	if err := r.st.db.QueryRow(`SELECT mo FROM resistance`).Scan(&mo); err != nil {
		t.Fatalf("读取回填后的内阻: %v", err)
	}
	if math.Abs(mo-0.05) > 1e-9 {
		t.Fatalf("回填后内阻 = %v mΩ, want 0.05（旧刻度 50mΩ ÷ 1000）", mo)
	}
	// ICA 基准按同一比例修正，保证新刻度下的 peak_h_rel 与历史基准可比。
	baseTxt, ok := kvString(t, r.st, kvICAPeakBase)
	if !ok {
		t.Fatal("应有 ica_peak_base")
	}
	base, perr := strconv.ParseFloat(baseTxt, 64)
	if perr != nil || math.Abs(base-1_234_000) > 1e-6 {
		t.Fatalf("ica_peak_base = %q, want 1234000（旧基准 1234 × 1000）", baseTxt)
	}
	var backfillN int
	if err := r.st.db.QueryRow(`SELECT count(*) FROM events WHERE kind='cur_backfill'`).Scan(&backfillN); err != nil {
		t.Fatal(err)
	}
	if backfillN != 1 {
		t.Fatalf("应落 1 条 cur_backfill 事件, got %d", backfillN)
	}

	// 学习刻度即时生效：第二会话首拍采样电流已按 ×1000 归一
	secondStart := r.cur.Unix() + 60
	r.put(20, 3_000, 4_100_000)
	r.step("Charging")
	rows, err := r.st.SamplesRange(secondStart, r.cur.Unix())
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("样本数 = %d, want 1", len(rows))
	}
	if rows[0].UA != 3_000_000 {
		t.Fatalf("校准后样本电流 = %dµA, want 3000000（原始 3000 × 1000）", rows[0].UA)
	}

	// 第二个完整会话：已学 ×1000 不得被反推回 ×1（否则刻度在 1000↔1 间震荡，
	// samples/avg_i/吞吐/CCCT 门控会隔会话错一周期）。
	for i := int64(1); i <= 60; i++ {
		r.put(20+i, 3_000, 4_100_000)
		cc += 50_000
		putCounter(t, r, cc)
		r.step("Charging")
	}
	// 已校准后 raw 0 才走去抖结算：raw 20 会被 ×1000 归一成 20A，tailCurrent
	// 误判成尾段充电（真机上满电停流读数为 0/极小值，不触发该分支）。
	r.put(80, 0, 4_100_000)
	for k := 0; k < 3; k++ {
		if out := r.step("Full"); out.SessionSettled {
			break
		}
	}
	if got, ok := kvString(t, r.st, kvCurrentScale); !ok || got != "1000" {
		t.Fatalf("第二会话后 kv[cur_scale] = (%q,%v), want (\"1000\",true)（刻度不得被反推回 ×1）", got, ok)
	}
	if err := r.st.db.QueryRow(`SELECT count(*) FROM events WHERE kind='cur_cal'`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("刻度未变化时不应重复落 cur_cal 事件, got %d", n)
	}
	// 第二会话积分已按 ×1000 归一，与电量计同量级，不再触发口径裁决事件
	if err := r.st.db.QueryRow(`SELECT count(*) FROM events WHERE kind='counter_scale'`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("校准后不应再落 counter_scale 事件, got %d", n)
	}
	var secondUA, secondAvg int64
	if err := r.st.db.QueryRow(`SELECT ua, avg_i FROM sessions ORDER BY id DESC LIMIT 1`).Scan(&secondUA, &secondAvg); err != nil {
		t.Fatal(err)
	}
	if secondUA <= 0 || secondAvg <= 0 {
		t.Fatalf("第二会话积分/平均电流 = (%d, %d), want >0（已按新刻度累计）", secondUA, secondAvg)
	}
	rows, err = r.st.SamplesRange(secondStart, r.cur.Unix())
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) < 30 {
		t.Fatalf("第二会话样本数 = %d, 过少", len(rows))
	}
	for _, row := range rows {
		if row.UA != 3_000_000 {
			t.Fatalf("第二会话样本电流 = %dµA, want 3000000（刻度保持 ×1000）", row.UA)
		}
	}

	// 重启后刻度从 kv 恢复：新 Pipeline 继续按校准值归一
	r.rebuildPipeline()
	if got := r.p.currentUA(3_000); got != 3_000_000 {
		t.Fatalf("重启后 currentUA(3000) = %d, want 3000000", got)
	}
}

// 无锚点机型（无 charge_full、未校准）禁止回填：旧有效刻度无法确定，任何按
// 比例缩放都是二次量纲猜测（AGENTS 明令禁止），会把真实的 µA 样本再放大/缩小。
// 学到刻度只影响之后的会话，本会话样本/内阻保持原样并记事件留痕。
func TestNoAnchorSkipsCurrentScaleBackfill(t *testing.T) {
	r := newPipeRig(t) // 不写 charge_full：fullUAh=0，anchorScaleUA=0
	if err := r.st.KVSet(kvKeyEmaUAh, "4800000"); err != nil {
		t.Fatal(err)
	}
	if err := r.st.KVSet(kvKeySamples, "3"); err != nil {
		t.Fatal(err)
	}

	cc := int64(6_000_000)
	putCounter(t, r, cc)
	// 原始 2400：无锚点时幅值启发式 ×1000 ⇒ 采样 2,400,000µA（本身已是 µA）。
	// 电量计每拍 +40000µAh 共 2400mAh，涨幅 60 点 ⇒ 隐含 4000mAh（设计容量），
	// 原始积分 vs 电量计比值 = 2,400,000×3600/(2400×3600) = 1000 ⇒ 学到 ×1000。
	for i := int64(0); i <= 60; i++ {
		r.put(20+i, 2_400, 4_100_000)
		if i > 0 {
			cc += 40_000
			putCounter(t, r, cc)
		}
		r.step("Charging")
	}
	// 会话期内的旧内阻样本：无锚点不回填，必须保持原值。
	if err := r.st.InsertResistance(r.cur.Unix(), 50); err != nil {
		t.Fatal(err)
	}
	r.put(80, 0, 4_100_000)
	for k := 0; k < 3; k++ {
		if out := r.step("Full"); out.SessionSettled {
			break
		}
	}
	if got, ok := kvString(t, r.st, kvCurrentScale); !ok || got != "1000" {
		t.Fatalf("kv[cur_scale] = (%q,%v), want (\"1000\",true)", got, ok)
	}
	sess := onlySession(t, r.st)
	rows, err := r.st.SamplesRange(sess.StartTs, sess.EndTs)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) < 30 {
		t.Fatalf("样本数 = %d, 过少", len(rows))
	}
	for _, row := range rows {
		if row.UA != 2_400_000 {
			t.Fatalf("无锚点会话样本 ua = %dµA, want 2400000（不回填）", row.UA)
		}
	}
	var mo float64
	if err := r.st.db.QueryRow(`SELECT mo FROM resistance`).Scan(&mo); err != nil {
		t.Fatalf("读取内阻: %v", err)
	}
	if math.Abs(mo-50) > 1e-9 {
		t.Fatalf("无锚点内阻 = %v mΩ, want 50（不回填）", mo)
	}
	var n int
	if err := r.st.db.QueryRow(`SELECT count(*) FROM events WHERE kind='cur_backfill_skip'`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("无锚点应落 1 条 cur_backfill_skip, got %d", n)
	}
	if err := r.st.db.QueryRow(`SELECT count(*) FROM events WHERE kind='cur_backfill'`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("无锚点不应回填, cur_backfill = %d", n)
	}
}

// 正常 µA 设备不能被自校准误伤：电量计与积分一致（比值 ≈1）时刻度应为 ×1，
// 采样电流保持原值，不出现二次 ×1000。
func TestNormalMicroampDeviceStaysCalibratedToOne(t *testing.T) {
	r := newPipeRig(t)
	writeFile(t, filepath.Join(r.fs.Base, "battery", "charge_full"), fmtNode(5_500_000))
	r.rebuildPipeline()
	if err := r.st.KVSet(kvKeyEmaUAh, "4800000"); err != nil {
		t.Fatal(err)
	}
	if err := r.st.KVSet(kvKeySamples, "3"); err != nil {
		t.Fatal(err)
	}

	cc := int64(6_000_000)
	putCounter(t, r, cc)
	// 2.4A 充电 61 拍（60s/拍）⇒ 电量计每拍 +40000µAh，合计 2400mAh；
	// 涨幅 60 点 ⇒ 隐含 4000mAh，两路口径都在设计窗口内。
	for i := int64(0); i <= 60; i++ {
		r.put(20+i, 2_400_000, 4_100_000)
		if i > 0 {
			cc += 40_000
			putCounter(t, r, cc)
		}
		r.step("Charging")
	}
	r.put(80, 20_000, 4_100_000)
	for k := 0; k < 3; k++ {
		if out := r.step("Full"); out.SessionSettled {
			break
		}
	}
	if sess := onlySession(t, r.st); !sess.Valid {
		t.Fatalf("正常设备会话不应被拒，reason=%s", sess.InvalidReason)
	}
	if got, ok := kvString(t, r.st, kvCurrentScale); !ok || got != "1" {
		t.Fatalf("kv[cur_scale] = (%q,%v), want (\"1\",true)", got, ok)
	}
	// 校准后采样电流仍是 µA 原值（不会被再乘 1000）
	r.put(20, 2_400_000, 4_100_000)
	r.step("Charging")
	rows, err := r.st.SamplesRange(r.cur.Unix(), r.cur.Unix())
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].UA != 2_400_000 {
		t.Fatalf("校准后样本 = %+v, want ua=2400000", rows)
	}
}

// 系统 OTA 改变 current_now 单位后，旧刻度必须能被下一个采信会话覆盖（否则会
// 永久按旧量纲放大/缩小 1000 倍）。这里模拟「已存 ×1000，但节点已改回 µA」。
func TestCurrentScaleRecalibratesAfterUnitChange(t *testing.T) {
	r := newPipeRig(t)
	writeFile(t, filepath.Join(r.fs.Base, "battery", "charge_full"), fmtNode(5_500_000))
	if err := r.st.KVSet(kvCurrentScale, "1000"); err != nil {
		t.Fatal(err)
	}
	if err := r.st.KVSet(kvKeyEmaUAh, "4800000"); err != nil {
		t.Fatal(err)
	}
	if err := r.st.KVSet(kvKeySamples, "3"); err != nil {
		t.Fatal(err)
	}
	r.rebuildPipeline() // 加载旧刻度 1000

	cc := int64(6_000_000)
	putCounter(t, r, cc)
	for i := int64(0); i <= 60; i++ {
		r.put(20+i, 2_400_000, 4_100_000)
		if i > 0 {
			cc += 40_000
			putCounter(t, r, cc)
		}
		r.step("Charging")
	}
	// 旧刻度 1000 仍在时，满电停流（raw 0）才能走去抖结算：此时 tailCurrent 不会
	// 把停流误判成尾段充电；结算时按新行为重学刻度。
	r.put(80, 0, 4_100_000)
	for k := 0; k < 3; k++ {
		if out := r.step("Full"); out.SessionSettled {
			break
		}
	}
	if sess := onlySession(t, r.st); !sess.Valid {
		t.Fatalf("单位恢复后会话不应被拒，reason=%s", sess.InvalidReason)
	}
	if got, ok := kvString(t, r.st, kvCurrentScale); !ok || got != "1" {
		t.Fatalf("OTA 改单位后 kv[cur_scale] = (%q,%v), want (\"1\",true)", got, ok)
	}
}

// 展示口径回归：历史页会话「估算」优先由 sessions.cc（电量计）折算，不再用被
// 低估的电流积分显示 2mAh；旧行 cc=0 保留积分口径显示。
func TestConvSessionUsesCounterForDisplay(t *testing.T) {
	cc := int64(3_000_000)
	got := convSession(Session{StartCap: 20, EndCap: 80, CounterUAh: cc})
	if got.EstMah == nil || *got.EstMah != 5000 {
		t.Fatalf("cc 口径 EstMah = %v, want 5000", got.EstMah)
	}
	old := convSession(Session{StartCap: 20, EndCap: 80, AccUAs: 3_000_000 * 3600})
	if old.EstMah == nil || *old.EstMah != 5000 {
		t.Fatalf("旧行积分口径 EstMah = %v, want 5000", old.EstMah)
	}
	// issue #6 设备上的旧行：电流被当 µA 直通（3000µA×3600s）⇒ 显示 2~5mAh。
	// 历史行无法反推电量计口径，保持原值（只修新行），属有意取舍。
	bad := convSession(Session{StartCap: 20, EndCap: 80, AccUAs: 3_000 * 3600})
	if bad.EstMah == nil || *bad.EstMah != 5 {
		t.Fatalf("旧行低估口径 EstMah = %v, want 5（历史行不改写）", bad.EstMah)
	}
}

// 自校准的「原始读数·秒」累计必须跨 daemon 重启（restoreSession）续记，否则
// 重启后的会话只按半段反推、ratio 被带偏或直接学不出刻度。
func TestSessionRawUAsPersistsAcrossRestart(t *testing.T) {
	r := newPipeRig(t)
	writeFile(t, filepath.Join(r.fs.Base, "battery", "charge_full"), fmtNode(5_500_000))
	r.rebuildPipeline()

	const iUA = int64(2_500_000)
	for i := int64(0); i < 5; i++ {
		r.put(50+i, iUA, 4_100_000)
		r.step("Charging")
	}
	// 5 拍 × 60s × 2.5A = 7.5e8 原始单位·秒
	before := kvInt(r.st, kvSessRawUAs)
	if want := int64(5 * 60 * iUA); before != want {
		t.Fatalf("重启前 kv[sess_raw_uas] = %d, want %d", before, want)
	}
	r.rebuildPipeline() // 进程重启：会话由 restoreSession 拉起
	if got := kvInt(r.st, kvSessRawUAs); got != before {
		t.Fatalf("重启后 kv[sess_raw_uas] = %d, want %d（原始读数累计应随会话恢复）", got, before)
	}
	r.put(55, iUA, 4_100_000)
	r.step("Charging")
	if got, want := kvInt(r.st, kvSessRawUAs), before+60*iUA; got != want {
		t.Fatalf("重启后续记 kv[sess_raw_uas] = %d, want %d", got, want)
	}
}

// calibInfo 是 WebUI 设置页/issue 排障的 JSON 出口：节点齐全时给出路径、当前
// 原始/归一读数与单位标签，学到刻度后 current_scale/current_unit 同步变化；
// 节点全缺失且未校准时返回 nil（JSON 省略 calib）。
func TestCalibInfo(t *testing.T) {
	base := t.TempDir()
	fs := SysFS{Base: base, devices: filepath.Join(base, "devices")}
	battery := filepath.Join(base, "battery")
	currentPath := filepath.Join(battery, "current_now")
	counterPath := filepath.Join(battery, "charge_counter")
	writeFile(t, currentPath, "3000\n")
	writeFile(t, filepath.Join(battery, "charge_full"), "5500000\n")
	writeFile(t, counterPath, "6000000\n")

	st := openTestStore(t, filepath.Join(t.TempDir(), "battery.db"))
	defer func() { _ = st.Close() }()
	a := &app{fs: fs, st: st}

	// 未校准：单位按 charge_full 锚点判为 ua，读数直通（诊断信息恰好暴露问题）
	info := a.calibInfo()
	if info == nil {
		t.Fatal("节点齐全时 calibInfo 不应为 nil")
	}
	if info.CurrentNode != currentPath || info.CounterNode != counterPath {
		t.Fatalf("节点路径 = (%q,%q), want (%q,%q)",
			info.CurrentNode, info.CounterNode, currentPath, counterPath)
	}
	if info.Scale != nil || info.Unit != "ua" {
		t.Fatalf("未校准状态 = (scale=%v, unit=%q), want (nil, \"ua\")", info.Scale, info.Unit)
	}
	if info.Raw == nil || *info.Raw != 3000 || info.UA == nil || *info.UA != 3000 {
		t.Fatalf("未校准读数 = (%v,%v), want (3000,3000)", info.Raw, info.UA)
	}

	// 学到 ×1000：unit 变 ma，归一读数 3,000,000µA
	if err := st.KVSet(kvCurrentScale, "1000"); err != nil {
		t.Fatal(err)
	}
	info = a.calibInfo()
	if info.Scale == nil || *info.Scale != 1000 || info.Unit != "ma" {
		t.Fatalf("校准状态 = (scale=%v, unit=%q), want (1000, \"ma\")", info.Scale, info.Unit)
	}
	if info.UA == nil || *info.UA != 3_000_000 {
		t.Fatalf("校准后归一读数 = %v, want 3000000", info.UA)
	}

	// 节点全缺失且未校准 → nil（JSON 省略 calib）
	st2 := openTestStore(t, filepath.Join(t.TempDir(), "battery2.db"))
	defer func() { _ = st2.Close() }()
	missingBase := t.TempDir()
	a2 := &app{fs: SysFS{Base: missingBase, devices: filepath.Join(missingBase, "devices")}, st: st2}
	if got := a2.calibInfo(); got != nil {
		t.Fatalf("节点缺失且未校准时 calibInfo = %+v, want nil", got)
	}
}
