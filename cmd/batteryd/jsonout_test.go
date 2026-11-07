package main

import (
	"bytes"
	"encoding/json"
	"math"
	"strings"
	"testing"
	"time"
)

func TestRenderJSONGoldenWithNulls(t *testing.T) {
	d := Design{DesignMah: 4000, HasDesign: true, FullMah: 3850, HasFull: true, Cycles: 210, HasCycles: true, Pct: 96, HasPct: true}
	snap := Snapshot{}
	now := time.Date(2026, 8, 26, 14, 30, 5, 0, time.UTC)
	want := `{"channel":"stable","design_mah":4000,"full_mah":3850,"cycles":210,"pct":96,"est_mah":null,"est_mah_sigma":null,"samples":0,"cycle_equiv":0,"r_moh":null,"temp_c":null,"trend_mah_per_week":null,"updated":"2026-08-26 14:30:05","recent":[],"sessions":null,"rest_points":null,"ccct":null,"ica_peaks":null,"samples_n":0,"sessions_n":0}`
	got, err := RenderJSON("stable", d, snap, []TsVal{}, nil, nil, nil, nil, nil, nil, nil, 0, now)
	if err != nil {
		t.Fatalf("RenderJSON: %v", err)
	}
	if string(got) != want {
		t.Fatalf("got  %s\nwant %s", got, want)
	}
}

func TestRenderJSONGoldenPartialNulls(t *testing.T) {
	// 缺设计容量与循环次数 → 对应字段渲染为 null
	d := Design{FullMah: 3850, HasFull: true}
	now := time.Date(2026, 8, 26, 14, 30, 5, 0, time.UTC)
	want := `{"channel":"stable","design_mah":null,"full_mah":3850,"cycles":null,"pct":null,"est_mah":null,"est_mah_sigma":null,"samples":0,"cycle_equiv":0,"r_moh":null,"temp_c":null,"trend_mah_per_week":null,"updated":"2026-08-26 14:30:05","recent":[],"sessions":null,"rest_points":null,"ccct":null,"ica_peaks":null,"samples_n":0,"sessions_n":0}`
	got, err := RenderJSON("stable", d, Snapshot{}, []TsVal{}, nil, nil, nil, nil, nil, nil, nil, 0, now)
	if err != nil {
		t.Fatalf("RenderJSON: %v", err)
	}
	if string(got) != want {
		t.Fatalf("got  %s\nwant %s", got, want)
	}
}

func TestRenderJSONGoldenFull(t *testing.T) {
	estUAh := int64(4501234)
	rMoh := 0.85
	tempC := 28.5
	sigma := 12.3
	d := Design{DesignMah: 5000, HasDesign: true, FullMah: 4600, HasFull: true, Cycles: 322, HasCycles: true, Pct: 92, HasPct: true}
	snap := Snapshot{EstUAh: &estUAh, Samples: 42, CycleEquiv: 12.5, RMoh: &rMoh, TempC: &tempC, SigmaMah: &sigma}
	// estimates 表存 µAh，RenderJSON 负责 /1000 转 mAh
	recent := []TsVal{{TS: 1778000000, V: 4501000}, {TS: 1777999940, V: 4498000}}
	now := time.Date(2026, 8, 26, 9, 5, 1, 0, time.UTC)
	want := `{"channel":"ml","design_mah":5000,"full_mah":4600,"cycles":322,"pct":92,"est_mah":4501,"est_mah_sigma":12.3,"samples":42,"cycle_equiv":12.5,"r_moh":0.85,"temp_c":28.5,"trend_mah_per_week":null,"updated":"2026-08-26 09:05:01","recent":[{"ts":1778000000,"mah":4501},{"ts":1777999940,"mah":4498}],"sessions":null,"rest_points":null,"ccct":null,"ica_peaks":null,"samples_n":0,"sessions_n":0}`
	got, err := RenderJSON("ml", d, snap, recent, nil, nil, nil, nil, nil, nil, nil, 0, now)
	if err != nil {
		t.Fatalf("RenderJSON: %v", err)
	}
	if string(got) != want {
		t.Fatalf("got  %s\nwant %s", got, want)
	}
}

func TestRenderJSONSanitizesNonFiniteFloats(t *testing.T) {
	d := Design{DesignMah: 4000, HasDesign: true, FullMah: 3850, HasFull: true, Cycles: 210, HasCycles: true, Pct: 96, HasPct: true}
	now := time.Date(2026, 8, 26, 14, 30, 5, 0, time.UTC)

	nanR := math.NaN()
	infTemp := math.Inf(1)
	snapNaN := Snapshot{CycleEquiv: 3.5, RMoh: &nanR}
	out, err := RenderJSON("stable", d, snapNaN, nil, nil, nil, nil, nil, nil, nil, nil, 0, now)
	if err != nil {
		t.Fatalf("RenderJSON: %v", err)
	}
	if len(out) == 0 {
		t.Fatal("输出为空：Marshal 错误被静默吞掉")
	}
	if !json.Valid(out) {
		t.Fatalf("输出非法 JSON: %s", out)
	}
	if want := `"r_moh":null`; !strings.Contains(string(out), want) {
		t.Fatalf("NaN 的 r_moh 应渲染为 %s，got %s", want, out)
	}

	infCycle := math.Inf(-1)
	snapInf := Snapshot{CycleEquiv: infCycle, TempC: &infTemp}
	out, err = RenderJSON("stable", d, snapInf, nil, nil, nil, nil, nil, nil, nil, nil, 0, now)
	if err != nil {
		t.Fatalf("RenderJSON: %v", err)
	}
	if len(out) == 0 {
		t.Fatal("输出为空：Marshal 错误被静默吞掉")
	}
	if !json.Valid(out) {
		t.Fatalf("输出非法 JSON: %s", out)
	}
	for _, want := range []string{`"cycle_equiv":null`, `"temp_c":null`} {
		if !strings.Contains(string(out), want) {
			t.Fatalf("%s 缺失，got %s", want, out)
		}
	}
	if bytes.Contains(out, []byte("Infinity")) || bytes.Contains(out, []byte("NaN")) {
		t.Fatalf("非有限值泄漏进输出: %s", out)
	}
}

func TestRenderJSONNewFields(t *testing.T) {
	d := Design{HasDesign: true, DesignMah: 4500, HasFull: true, FullMah: 4450, HasCycles: true, Cycles: 123}
	snap := Snapshot{}
	recent := []TsVal{{TS: 1, V: 4_400_000}}
	sess := []sessionEntry{{StartTs: 100, EndTs: 200, StartCap: 20, EndCap: 80,
		DeltaPct: 60, EstMah: intPtr(4500), Valid: true}}
	rests := []restEntry{{TS: 300, UV: 3_900_000, Cap: 60}}
	b, err := RenderJSON("stable", d, snap, recent, sess, rests, nil, nil, nil, nil, nil, 42, time.Unix(0, 0))
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	for _, want := range []string{`"samples_n":42`, `"valid":true`, `"est_mah":4500`, `"uv":3900000`, `"delta_pct":60`,
		`"start_cap":20`, `"end_cap":80`} {
		if !strings.Contains(s, want) {
			t.Fatalf("缺字段 %s: %s", want, s)
		}
	}
	b2, _ := RenderJSON("stable", Design{}, Snapshot{}, nil, []sessionEntry{}, []restEntry{}, []ccctEntry{}, []icaEntry{}, nil, nil, nil, 0, time.Unix(0, 0))
	if strings.Contains(string(b2), `"sessions":[{`) {
		t.Fatalf("空数据不应出会话数组元素: %s", b2)
	}
	for _, want := range []string{`"sessions":[]`, `"rest_points":[]`, `"ccct":[]`, `"ica_peaks":[]`} {
		if !strings.Contains(string(b2), want) {
			t.Fatalf("空切片应输出 %s 而非 null: %s", want, b2)
		}
	}
}

func TestRenderJSONCcctEntries(t *testing.T) {
	ccct := []ccctEntry{{TS: 500, Secs: 960, VwLo: 4200000, VwHi: 4300000}, {TS: 1460, Secs: 480}}
	b, err := RenderJSON("stable", Design{}, Snapshot{}, nil, nil, nil, ccct, nil, nil, nil, nil, 0, time.Unix(0, 0))
	if err != nil {
		t.Fatal(err)
	}
	if want := `"ccct":[{"ts":500,"secs":960,"vw_lo":4200000,"vw_hi":4300000},{"ts":1460,"secs":480}]`; !strings.Contains(string(b), want) {
		t.Fatalf("ccct 序列化不符（含穿窗电压与缺省降级）, got %s", b)
	}
}

func TestRenderJSONSessionInvalidReason(t *testing.T) {
	sess := []sessionEntry{
		{StartTs: 100, EndTs: 200, DeltaPct: 5, Valid: false, InvalidReason: "delta_lt_20"},
		{StartTs: 300, EndTs: 400, DeltaPct: 60, EstMah: intPtr(4500), Valid: true},
		{StartTs: 500, EndTs: 600, DeltaPct: 8, Valid: false}, // 旧行为无原因（legacy 行）
	}
	b, err := RenderJSON("stable", Design{}, Snapshot{}, nil, sess, nil, nil, nil, nil, nil, nil, 0, time.Unix(0, 0))
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Sessions []map[string]any `json:"sessions"`
	}
	if err := json.Unmarshal(b, &doc); err != nil {
		t.Fatalf("解析输出: %v\n%s", err, b)
	}
	if len(doc.Sessions) != 3 {
		t.Fatalf("sessions 条数 = %d, want 3: %s", len(doc.Sessions), b)
	}
	if got := doc.Sessions[0]["invalid_reason"]; got != "delta_lt_20" {
		t.Fatalf("被拒会话应带 invalid_reason=delta_lt_20, got %v: %s", got, b)
	}
	for _, key := range []int{1, 2} {
		if _, has := doc.Sessions[key]["invalid_reason"]; has {
			t.Fatalf("第 %d 条会话无原因应省略 invalid_reason: %s", key+1, b)
		}
	}
}

func TestRenderJSONTrendStates(t *testing.T) {
	span := int64(16)
	mpw := -1.25
	cases := []struct {
		name string
		snap Snapshot
		want []string
		bad  []string
	}{
		{"insufficient带进度", Snapshot{TrendState: "insufficient", TrendSpanDay: &span},
			[]string{`"trend_state":"insufficient"`, `"trend_span_day":16`}, nil},
		{"stable无进度", Snapshot{TrendState: "stable"},
			[]string{`"trend_state":"stable"`}, []string{`"trend_span_day"`}},
		{"significant带斜率", Snapshot{TrendState: "significant", TrendMahPerWeek: &mpw},
			[]string{`"trend_state":"significant"`, `"trend_mah_per_week":-1.25`}, []string{`"trend_span_day"`}},
		{"无状态整体省略", Snapshot{}, nil, []string{`"trend_state"`, `"trend_span_day"`}},
	}
	for _, c := range cases {
		b, err := RenderJSON("stable", Design{}, c.snap, nil, nil, nil, nil, nil, nil, nil, nil, 0, time.Unix(0, 0))
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		s := string(b)
		for _, w := range c.want {
			if !strings.Contains(s, w) {
				t.Fatalf("%s: 缺 %s: %s", c.name, w, s)
			}
		}
		for _, w := range c.bad {
			if strings.Contains(s, w) {
				t.Fatalf("%s: 不应出现 %s: %s", c.name, w, s)
			}
		}
	}
}

func TestConvSessionCarriesInvalidReason(t *testing.T) {
	in := Session{StartTs: 100, EndTs: 200, StartCap: 30, EndCap: 35,
		AccUAs: 1_800_000_000, Valid: false, InvalidReason: "temp_out_of_range"}
	e := convSession(in)
	if e.InvalidReason != "temp_out_of_range" {
		t.Fatalf("convSession 应透传 invalid_reason, got %q", e.InvalidReason)
	}
	if e := convSession(Session{Valid: true}); e.InvalidReason != "" {
		t.Fatalf("有效会话 invalid_reason 应为空, got %q", e.InvalidReason)
	}
}

func TestRenderJSONDischargeEntries(t *testing.T) {
	implied := int64(5300)
	disch := []dischargeEntry{
		{TS: 500, Secs: 4800, Mah: 2765, StartCap: 78, EndCap: 25, Implied: &implied},
		{TS: 900, Secs: 600, Mah: 300, StartCap: 85, EndCap: 80}, // 无隐含
	}
	b, err := RenderJSON("stable", Design{}, Snapshot{}, nil, nil, nil, nil, nil, disch, nil, nil, 0, time.Unix(0, 0))
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	if want := `"discharge_recent":[{"ts":500,"secs":4800,"uah_mah":2765,"start_cap":78,"end_cap":25,"implied_mah":5300},{"ts":900,"secs":600,"uah_mah":300,"start_cap":85,"end_cap":80}]`; !strings.Contains(s, want) {
		t.Fatalf("discharge_recent 序列化不符, got %s", s)
	}
	b2, _ := RenderJSON("stable", Design{}, Snapshot{}, nil, nil, nil, nil, nil, nil, nil, nil, 0, time.Unix(0, 0))
	if strings.Contains(string(b2), "discharge_recent") {
		t.Fatalf("空放电切片应整体省略: %s", b2)
	}
}

// calib 是顶层诊断对象（设置页「采样节点 / 电流刻度」消费）：字段齐全时
// 全量输出，Snapshot.Calib 为 nil 时整体省略。
func TestRenderJSONCalibInfo(t *testing.T) {
	scale := 1000.0
	raw, ua := int64(3000), int64(3_000_000)
	snap := Snapshot{Calib: &CalibInfo{
		CurrentNode: "/sys/class/power_supply/battery/current_now",
		CounterNode: "/sys/class/power_supply/battery/charge_counter",
		Scale:       &scale, Unit: "ma", Raw: &raw, UA: &ua,
	}}
	b, err := RenderJSON("stable", Design{}, snap, nil, nil, nil, nil, nil, nil, nil, nil, 0, time.Unix(0, 0))
	if err != nil {
		t.Fatal(err)
	}
	want := `"calib":{"current_node":"/sys/class/power_supply/battery/current_now",` +
		`"counter_node":"/sys/class/power_supply/battery/charge_counter",` +
		`"current_scale":1000,"current_unit":"ma","raw_current":3000,"current_ua":3000000}`
	if !strings.Contains(string(b), want) {
		t.Fatalf("calib 序列化不符, want 含 %s, got %s", want, b)
	}
	// 节点名含 / 与引号无需转义，但字段缺失时必须整段省略
	b2, _ := RenderJSON("stable", Design{}, Snapshot{}, nil, nil, nil, nil, nil, nil, nil, nil, 0, time.Unix(0, 0))
	if strings.Contains(string(b2), `"calib"`) {
		t.Fatalf("无诊断数据时不应输出 calib: %s", b2)
	}
}

// version 是 WebUI 关于页展示的模块版本（module.prop）；读不到时省略。
func TestRenderJSONVersion(t *testing.T) {
	b, err := RenderJSON("stable", Design{}, Snapshot{Version: "1.3.6(35)"},
		nil, nil, nil, nil, nil, nil, nil, nil, 0, time.Unix(0, 0))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), `"version":"1.3.6(35)"`) {
		t.Fatalf("缺少 version 字段: %s", b)
	}
	b2, _ := RenderJSON("stable", Design{}, Snapshot{}, nil, nil, nil, nil, nil, nil, nil, nil, 0, time.Unix(0, 0))
	if strings.Contains(string(b2), `"version"`) {
		t.Fatalf("版本缺失时应省略 version: %s", b2)
	}
}

// 概览趋势图：趋势端点由后端按 recent 同一份数据拟合给出，前端直接用
// 端点画虚线；只下发斜率时前端会拿首个实际点当锚点，虚线与数据整体错位。
func TestRenderJSONTrendLineEndpoints(t *testing.T) {
	base := int64(1735689600)
	pts := make([]TsVal, 0, 30)
	for d := 0; d < 30; d++ {
		pts = append(pts, TsVal{TS: base + int64(d)*86400, V: 4_500_000 - int64(d)*3000})
	}
	// store 返回最新在前，RenderJSON 按传入顺序输出 recent
	for i, j := 0, len(pts)-1; i < j; i, j = i+1, j-1 {
		pts[i], pts[j] = pts[j], pts[i]
	}
	mahPerWeek := -21000.0
	snap := Snapshot{TrendMahPerWeek: &mahPerWeek, TrendState: string(TrendSignificant)}
	b, err := RenderJSON("stable", Design{}, snap, pts, nil, nil, nil, nil, nil, nil, nil, 0, time.Unix(base, 0))
	if err != nil {
		t.Fatalf("RenderJSON: %v", err)
	}
	if !strings.Contains(string(b), `"trend_from_mah":4500`) {
		t.Fatalf("缺 trend_from_mah 4500: %s", b)
	}
	if !strings.Contains(string(b), `"trend_to_mah":4413`) {
		t.Fatalf("缺 trend_to_mah 4413: %s", b)
	}
	// 趋势不显著（stats 未下发斜率）时不输出端点，前端不画虚线
	b2, err := RenderJSON("stable", Design{}, Snapshot{}, pts, nil, nil, nil, nil, nil, nil, nil, 0, time.Unix(base, 0))
	if err != nil {
		t.Fatalf("RenderJSON: %v", err)
	}
	if strings.Contains(string(b2), "trend_from_mah") {
		t.Fatalf("不显著时不应输出趋势端点: %s", b2)
	}
}

// live 是顶层实时节点对象（概览页当前状态）：字段齐全时全量输出，单位后缀
// 即量纲（current_ua=µA、voltage_uv=µV、acc_uah=µAh、power_mw=mW）。
func TestRenderJSONLiveInfo(t *testing.T) {
	capPct, curUA := int64(42), int64(3_000_000)
	voltUV, powerMW := int64(4_200_000), int64(12_600)
	startCap, accUAh := int64(20), int64(1_000_000)
	avgUA, etaMin := int64(5_000_000), int64(30)
	live := &liveInfo{
		Status: "Charging", Cap: &capPct, CurrentUA: &curUA, VoltageUV: &voltUV, PowerMW: &powerMW,
		Charging: &liveCharging{StartTs: 1_700_000_000, StartCap: startCap, Cap: &capPct,
			AccUAh: &accUAh, Secs: 600, AvgCurrentUA: &avgUA, EtaMin: &etaMin},
	}
	b, err := RenderJSON("stable", Design{}, Snapshot{}, nil, nil, nil, nil, nil, nil, live, nil, 0, time.Unix(0, 0))
	if err != nil {
		t.Fatal(err)
	}
	want := `"live":{"status":"Charging","cap":42,"current_ua":3000000,"voltage_uv":4200000,` +
		`"power_mw":12600,"charging":{"start_ts":1700000000,"start_cap":20,"cap":42,` +
		`"acc_uah":1000000,"secs":600,"avg_current_ua":5000000,"eta_min":30}}`
	if !strings.Contains(string(b), want) {
		t.Fatalf("live 序列化不符, want 含 %s\ngot %s", want, b)
	}

	// 部分缺失：只读到 status 与容量，且会话尚未采信电量计 ⇒ acc/avg/eta 全部省略
	live2 := &liveInfo{Status: "Discharging",
		Charging: &liveCharging{StartTs: 1_700_000_000, StartCap: 20, Secs: 60}}
	b2, err := RenderJSON("stable", Design{}, Snapshot{}, nil, nil, nil, nil, nil, nil, live2, nil, 0, time.Unix(0, 0))
	if err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{"acc_uah", "avg_current_ua", "eta_min", "power_mw"} {
		if strings.Contains(string(b2), bad) {
			t.Fatalf("缺失字段不应输出 %s: %s", bad, b2)
		}
	}
	if want := `"live":{"status":"Discharging","charging":{"start_ts":1700000000,"start_cap":20,"secs":60}}`; !strings.Contains(string(b2), want) {
		t.Fatalf("部分缺失 live 序列化不符, want 含 %s\ngot %s", want, b2)
	}
}

// daemon 是顶层心跳对象：version/ts/age_sec/stale，文件缺失/损坏时整体省略。
func TestRenderJSONDaemonInfo(t *testing.T) {
	ts, age := int64(1_700_000_000), int64(12)
	d := &daemonInfo{Version: "1.3.8(37)", TS: &ts, AgeSec: &age, Stale: true}
	b, err := RenderJSON("stable", Design{}, Snapshot{}, nil, nil, nil, nil, nil, nil, nil, d, 0, time.Unix(0, 0))
	if err != nil {
		t.Fatal(err)
	}
	want := `"daemon":{"version":"1.3.8(37)","ts":1700000000,"age_sec":12,"stale":true}`
	if !strings.Contains(string(b), want) {
		t.Fatalf("daemon 序列化不符, want 含 %s\ngot %s", want, b)
	}

	// 只有版本（ts 缺失）时省略 ts/age_sec；不 stale
	b2, err := RenderJSON("stable", Design{}, Snapshot{}, nil, nil, nil, nil, nil, nil, nil,
		&daemonInfo{Version: "1.3.8(37)"}, 0, time.Unix(0, 0))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b2), `"daemon":{"version":"1.3.8(37)"}`) {
		t.Fatalf("缺 ts 时应只输出 version: %s", b2)
	}
	if strings.Contains(string(b2), "age_sec") || strings.Contains(string(b2), "stale") {
		t.Fatalf("缺 ts 时不应输出 age_sec/stale: %s", b2)
	}
}

// live/daemon 全部缺失（nil）时两个顶层字段整体省略，旧前端解析不受影响。
func TestRenderJSONLiveDaemonOmitted(t *testing.T) {
	b, err := RenderJSON("stable", Design{}, Snapshot{}, nil, nil, nil, nil, nil, nil, nil, nil, 0, time.Unix(0, 0))
	if err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{`"live"`, `"daemon"`} {
		if strings.Contains(string(b), bad) {
			t.Fatalf("无实时数据/心跳时不应输出 %s: %s", bad, b)
		}
	}
	if !json.Valid(b) {
		t.Fatalf("输出非法 JSON: %s", b)
	}
}
