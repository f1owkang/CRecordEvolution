package main

import (
	"testing"
	"time"
)

func TestParseHistoryArgs(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		want    historyOptions
		wantErr bool
	}{
		{"默认", nil, historyOptions{Days: 90, Limit: 20, Type: "all"}, false},
		{"完整参数", []string{"--days", "30", "--limit", "50", "--before", "123", "--type", "charge"},
			historyOptions{Days: 30, Limit: 50, Before: 123, Type: "charge"}, false},
		{"等号参数", []string{"--days=7", "--type=discharge"}, historyOptions{Days: 7, Limit: 20, Type: "discharge"}, false},
		{"天数越界", []string{"--days", "91"}, historyOptions{}, true},
		{"limit 越界", []string{"--limit", "51"}, historyOptions{}, true},
		{"before 负数", []string{"--before", "-1"}, historyOptions{}, true},
		{"非法类型", []string{"--type", "rest"}, historyOptions{}, true},
		{"缺值", []string{"--days"}, historyOptions{}, true},
		{"未知参数", []string{"--foo", "1"}, historyOptions{}, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseHistoryArgs(tc.args)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("期望错误，实际 %+v", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseHistoryArgs: %v", err)
			}
			if got != tc.want {
				t.Fatalf("got %+v want %+v", got, tc.want)
			}
		})
	}
}

func TestPaginateHistoryKeepsSameTimestampTogether(t *testing.T) {
	ev := []historyEvent{
		{Kind: "charge", TS: 100},
		{Kind: "discharge", TS: 90},
		{Kind: "charge", TS: 90},
		{Kind: "discharge", TS: 80},
	}
	page, more, next := paginateHistory(ev, 0, 2)
	if len(page) != 3 || !more || next != 90 {
		t.Fatalf("第一页=%d more=%v next=%d，期望 3/true/90", len(page), more, next)
	}
	page2, more2, next2 := paginateHistory(ev, next, 2)
	if len(page2) != 1 || more2 || next2 != 0 || page2[0].TS != 80 {
		t.Fatalf("第二页=%+v more=%v next=%d", page2, more2, next2)
	}
}

func TestSummarizeCharges(t *testing.T) {
	rows := []Session{
		// 电量计口径优先：900000µAh（而不是积分 1000000µAh）
		{StartTs: 1000, EndTs: 4600, AccUAs: 3_600_000_000, CounterUAh: 900_000, Valid: true},
		// 无电量计时回退积分：3600000000µA·s ÷ 3600 = 1000000µAh
		{StartTs: 2000, EndTs: 3800, AccUAs: 3_600_000_000, Valid: true},
		// 无效会话不参与任何统计
		{StartTs: 0, EndTs: 9999, AccUAs: 9_999_999, CounterUAh: 123, Valid: false},
	}
	charge, avg, secs := summarizeCharges(rows)
	if charge != 1_900_000 || avg != 950_000 || secs != 2700 {
		t.Fatalf("summarizeCharges=(%d,%d,%d) want (1900000,950000,2700)", charge, avg, secs)
	}
	if charge, avg, secs := summarizeCharges(nil); charge != 0 || avg != 0 || secs != 0 {
		t.Fatalf("空输入应为全零，got (%d,%d,%d)", charge, avg, secs)
	}
}

func TestBuildHistoryDocSummaryFilterAndTrends(t *testing.T) {
	s := openTestStore(t, t.TempDir()+"/battery.db")
	defer func() { _ = s.Close() }()

	now := time.Unix(1_800_000_000, 0)
	day := int64(24 * 3600)
	inRange := now.Unix() - day
	older := now.Unix() - 40*day

	mustInsertSession := func(end int64, valid bool, reason string) {
		t.Helper()
		if _, err := s.InsertSession(Session{
			StartTs: end - 3600, EndTs: end, StartCap: 20, EndCap: 80,
			AccUAs: 2_160_000_000, TempAvg: 30, CRate: 0.5, Valid: valid, InvalidReason: reason,
		}); err != nil {
			t.Fatalf("InsertSession: %v", err)
		}
	}
	mustInsertSession(inRange, true, "")
	mustInsertSession(inRange-3600, false, "delta_lt_20")
	mustInsertSession(older, true, "") // 超出 30 天范围

	if err := s.InsertDischarge(DischargeRow{
		TS: inRange - 1800, Secs: 5400, UAh: 2_100_000,
		StartCap: 80, EndCap: 58,
	}); err != nil {
		t.Fatalf("InsertDischarge: %v", err)
	}
	if err := s.InsertRestPoint(inRange-7200, 3_890_000, 55); err != nil {
		t.Fatalf("InsertRestPoint: %v", err)
	}
	if err := s.InsertCCCT(inRange-7200, 4_200_000, 4_300_000, 820); err != nil {
		t.Fatalf("InsertCCCT: %v", err)
	}
	if err := s.InsertICAPeak(inRange-7200, 4_200_000, 1.12); err != nil {
		t.Fatalf("InsertICAPeak: %v", err)
	}
	if err := s.InsertEstimate(inRange-600, 4_400_000); err != nil { // µAh
		t.Fatalf("InsertEstimate: %v", err)
	}
	for i := 0; i < 2; i++ {
		if err := s.InsertSample(inRange-int64(i*60), 100_000, 3_800_000, 50, 0); err != nil {
			t.Fatalf("InsertSample: %v", err)
		}
	}

	doc, err := buildHistoryDoc(s, historyOptions{Days: 30, Limit: 20, Type: "all"}, now)
	if err != nil {
		t.Fatalf("buildHistoryDoc: %v", err)
	}
	if doc.Summary.Sessions != 2 || doc.Summary.ValidSessions != 1 || doc.Summary.Discharges != 1 {
		t.Fatalf("summary=%+v", doc.Summary)
	}
	if doc.Summary.Samples != 2 {
		t.Fatalf("samples=%d，期望 2", doc.Summary.Samples)
	}
	if len(doc.Events) != 3 {
		t.Fatalf("events=%d，期望 3", len(doc.Events))
	}
	if doc.Events[0].TS < doc.Events[1].TS || doc.Events[1].TS < doc.Events[2].TS {
		t.Fatalf("events 不是时间倒序: %+v", doc.Events)
	}
	if len(doc.Trends.Sessions) != 2 {
		t.Fatalf("trend sessions=%d，期望 2（范围外应排除）", len(doc.Trends.Sessions))
	}
	if doc.Trends.Sessions[0].EndTs > doc.Trends.Sessions[1].EndTs {
		t.Fatalf("趋势会话应升序: %+v", doc.Trends.Sessions)
	}
	if len(doc.Trends.Estimates) != 1 || doc.Trends.Estimates[0].Mah != 4400 {
		t.Fatalf("estimates=%+v，期望 4400 mAh", doc.Trends.Estimates)
	}
	// 长期档案不受范围筛选影响（来自永久保留的 daily 表）。
	if len(doc.Trends.Archive) != 1 || doc.Trends.Archive[0].Mah != 4400 {
		t.Fatalf("archive=%+v，期望 4400 mAh", doc.Trends.Archive)
	}
	// 摘要的充入统计只计有效会话：积分 600000µAh、时长 1 小时。
	if doc.Summary.ChargeMah != 600 || doc.Summary.AvgMah != 600 || doc.Summary.AvgSecs != 3600 {
		t.Fatalf("充入摘要=%+v，期望 600/600/3600", doc.Summary)
	}
	if len(doc.Trends.RestPoints) != 1 || len(doc.Trends.Ccct) != 1 || len(doc.Trends.IcaPeaks) != 1 {
		t.Fatalf("趋势点数不对: rest=%d ccct=%d ica=%d",
			len(doc.Trends.RestPoints), len(doc.Trends.Ccct), len(doc.Trends.IcaPeaks))
	}

	// 类型筛选只影响事件，不影响 summary/trends。
	chargeDoc, err := buildHistoryDoc(s, historyOptions{Days: 30, Limit: 20, Type: "charge"}, now)
	if err != nil {
		t.Fatalf("buildHistoryDoc(charge): %v", err)
	}
	if len(chargeDoc.Events) != 2 || chargeDoc.Events[0].Kind != "charge" {
		t.Fatalf("charge events=%+v", chargeDoc.Events)
	}
	if chargeDoc.Summary.Discharges != 1 {
		t.Fatalf("summary 不应受类型筛选影响: %+v", chargeDoc.Summary)
	}

	// 分页：limit=1 时 next_before 指向下一页起点，且没有重复。
	page1, err := buildHistoryDoc(s, historyOptions{Days: 30, Limit: 1, Type: "all"}, now)
	if err != nil {
		t.Fatalf("buildHistoryDoc(page1): %v", err)
	}
	if !page1.HasMore || page1.NextBefore == 0 || len(page1.Events) != 1 {
		t.Fatalf("page1=%+v", page1)
	}
	page2, err := buildHistoryDoc(s, historyOptions{Days: 30, Limit: 1, Type: "all", Before: page1.NextBefore}, now)
	if err != nil {
		t.Fatalf("buildHistoryDoc(page2): %v", err)
	}
	if len(page2.Events) != 1 || page2.Events[0].TS >= page1.Events[0].TS {
		t.Fatalf("page2=%+v page1=%+v", page2.Events, page1.Events)
	}
}
