package main

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
)

// history 子命令给 WebUI 历史页提供可分页、可按时间范围/类型筛选的数据。
// 与 json 子命令分开：json 仍只输出概览所需的最近 10 条，历史页不受其限制。
const (
	historyDefaultDays  = 90
	historyDefaultLimit = 20
	historyMaxLimit     = 50
	historyMaxDays      = 90
	historyTrendLimit   = 120
)

// historyOptions 为 history 子命令的解析后参数（范围/分页/类型筛选）。
type historyOptions struct {
	Days   int
	Limit  int
	Before int64
	Type   string // all | charge | discharge
}

// historyEvent 为历史页时间轴的一行：充电或放电会话二选一。
type historyEvent struct {
	Kind      string          `json:"kind"`
	TS        int64           `json:"ts"`
	Charge    *sessionEntry   `json:"charge,omitempty"`
	Discharge *dischargeEntry `json:"discharge,omitempty"`
}

// historySummary 为范围内摘要统计（会话数/有效会话数/放电数/样本数）。
type historySummary struct {
	Days          int   `json:"days"`
	Since         int64 `json:"since"`
	Until         int64 `json:"until"`
	Sessions      int   `json:"sessions"`
	ValidSessions int   `json:"valid_sessions"`
	Discharges    int   `json:"discharges"`
	Samples       int64 `json:"samples"`
	OldestTs      int64 `json:"oldest_ts,omitempty"`
	NewestTs      int64 `json:"newest_ts,omitempty"`
}

// historyTrends 为历史页趋势视图的数据集（各表最多 historyTrendLimit 点）。
type historyTrends struct {
	Sessions   []sessionEntry   `json:"sessions"`
	Discharge  []dischargeEntry `json:"discharge"`
	RestPoints []restEntry      `json:"rest_points"`
	Ccct       []ccctEntry      `json:"ccct"`
	IcaPeaks   []icaEntry       `json:"ica_peaks"`
	Estimates  []recentEntry    `json:"estimates"`
}

// historyDoc 为 `batteryd history` 的 JSON 输出结构。
type historyDoc struct {
	Events     []historyEvent `json:"events"`
	HasMore    bool           `json:"has_more"`
	NextBefore int64          `json:"next_before,omitempty"`
	Summary    historySummary `json:"summary"`
	Trends     historyTrends  `json:"trends"`
}

// parseHistoryArgs 解析 history 子命令参数并校验取值范围。
func parseHistoryArgs(args []string) (historyOptions, error) {
	opt := historyOptions{Days: historyDefaultDays, Limit: historyDefaultLimit, Type: "all"}
	for i := 0; i < len(args); i++ {
		arg := args[i]
		val := ""
		if strings.Contains(arg, "=") {
			arg, val, _ = strings.Cut(arg, "=")
		} else {
			if i+1 >= len(args) {
				return opt, fmt.Errorf("history: 参数 %s 缺少取值", arg)
			}
			i++
			val = args[i]
		}
		switch arg {
		case "--days":
			n, err := strconv.Atoi(val)
			if err != nil || n < 1 || n > historyMaxDays {
				return opt, fmt.Errorf("history: --days 必须是 1-%d", historyMaxDays)
			}
			opt.Days = n
		case "--limit":
			n, err := strconv.Atoi(val)
			if err != nil || n < 1 || n > historyMaxLimit {
				return opt, fmt.Errorf("history: --limit 必须是 1-%d", historyMaxLimit)
			}
			opt.Limit = n
		case "--before":
			n, err := strconv.ParseInt(val, 10, 64)
			if err != nil || n < 0 {
				return opt, fmt.Errorf("history: --before 必须是非负整数")
			}
			opt.Before = n
		case "--type":
			if val != "all" && val != "charge" && val != "discharge" {
				return opt, fmt.Errorf("history: --type 必须是 all/charge/discharge")
			}
			opt.Type = val
		default:
			return opt, fmt.Errorf("history: 未知参数 %s", arg)
		}
	}
	return opt, nil
}

// runHistory 执行 history 子命令：组装并输出历史页 JSON。
func runHistory(args []string) error {
	opt, err := parseHistoryArgs(args)
	if err != nil {
		return err
	}
	a, err := newApp()
	if err != nil {
		return err
	}
	defer a.st.Close()

	doc, err := buildHistoryDoc(a.st, opt, localNow())
	if err != nil {
		return err
	}
	b, err := json.Marshal(doc)
	if err != nil {
		return err
	}
	fmt.Println(string(b))
	return nil
}

// buildHistoryDoc 合并充电/放电事件，按时间倒序分页并附带摘要与趋势点。
func buildHistoryDoc(st *Store, opt historyOptions, now time.Time) (*historyDoc, error) {
	since := now.AddDate(0, 0, -opt.Days).Unix()
	until := now.Unix()

	sessRows, err := st.SessionsSince(since)
	if err != nil {
		return nil, err
	}
	disRows, err := st.DischargeSince(since)
	if err != nil {
		return nil, err
	}
	samples, err := st.CountSamplesSince(since)
	if err != nil {
		return nil, err
	}

	all := make([]historyEvent, 0, len(sessRows)+len(disRows))
	validSessions := 0
	for _, se := range sessRows {
		ce := convSession(se)
		all = append(all, historyEvent{Kind: "charge", TS: ce.EndTs, Charge: &ce})
		if se.Valid {
			validSessions++
		}
	}
	for _, dr := range disRows {
		de := dischargeEntry{TS: dr.TS, Secs: dr.Secs, Mah: dr.UAh / 1000,
			StartCap: dr.StartCap, EndCap: dr.EndCap, Implied: dr.Implied,
			InvalidReason: dr.InvalidReason}
		if de.Implied != nil {
			v := *de.Implied / 1000
			de.Implied = &v
		}
		all = append(all, historyEvent{Kind: "discharge", TS: de.TS, Discharge: &de})
	}
	// 同一秒内按 kind 固定排序，保证分页游标稳定。
	sort.Slice(all, func(i, j int) bool {
		if all[i].TS != all[j].TS {
			return all[i].TS > all[j].TS
		}
		return all[i].Kind < all[j].Kind
	})

	summary := historySummary{
		Days: opt.Days, Since: since, Until: until,
		Sessions: len(sessRows), ValidSessions: validSessions,
		Discharges: len(disRows), Samples: samples,
	}
	if len(all) > 0 {
		summary.NewestTs = all[0].TS
		summary.OldestTs = all[len(all)-1].TS
	}

	filtered := all
	if opt.Type != "all" {
		filtered = make([]historyEvent, 0, len(all))
		for _, ev := range all {
			if ev.Kind == opt.Type {
				filtered = append(filtered, ev)
			}
		}
	}
	page, hasMore, nextBefore := paginateHistory(filtered, opt.Before, opt.Limit)

	trends, err := loadHistoryTrends(st, since)
	if err != nil {
		return nil, err
	}

	return &historyDoc{
		Events:     page,
		HasMore:    hasMore,
		NextBefore: nextBefore,
		Summary:    summary,
		Trends:     trends,
	}, nil
}

// paginateHistory 按 --before 游标分页；同秒事件并入本页保证游标稳定。
func paginateHistory(events []historyEvent, before int64, limit int) ([]historyEvent, bool, int64) {
	if len(events) == 0 || limit < 1 {
		return []historyEvent{}, false, 0
	}
	start := 0
	if before > 0 {
		for start < len(events) && events[start].TS >= before {
			start++
		}
	}
	if start >= len(events) {
		return []historyEvent{}, false, 0
	}
	end := start + limit
	if end > len(events) {
		end = len(events)
	}
	// 同一秒的事件可能跨页；把尾时间戳相同的全部收进本页，下一页用严格 < 游标。
	for end < len(events) && events[end].TS == events[end-1].TS {
		end++
	}
	hasMore := end < len(events)
	next := int64(0)
	if hasMore {
		next = events[end-1].TS
	}
	return events[start:end], hasMore, next
}

// loadHistoryTrends 读取各类趋势点并统一为时间升序。
func loadHistoryTrends(st *Store, since int64) (historyTrends, error) {
	tr := historyTrends{
		Sessions:   []sessionEntry{},
		Discharge:  []dischargeEntry{},
		RestPoints: []restEntry{},
		Ccct:       []ccctEntry{},
		IcaPeaks:   []icaEntry{},
		Estimates:  []recentEntry{},
	}

	sessRows, err := st.SessionsSince(since)
	if err != nil {
		return tr, err
	}
	for i, se := range sessRows {
		if i >= historyTrendLimit {
			break
		}
		tr.Sessions = append(tr.Sessions, convSession(se))
	}
	reverseSessions(tr.Sessions)

	disRows, err := st.DischargeSince(since)
	if err != nil {
		return tr, err
	}
	for i, dr := range disRows {
		if i >= historyTrendLimit {
			break
		}
		de := dischargeEntry{TS: dr.TS, Secs: dr.Secs, Mah: dr.UAh / 1000,
			StartCap: dr.StartCap, EndCap: dr.EndCap, Implied: dr.Implied,
			InvalidReason: dr.InvalidReason}
		if de.Implied != nil {
			v := *de.Implied / 1000
			de.Implied = &v
		}
		tr.Discharge = append(tr.Discharge, de)
	}
	reverseDischarge(tr.Discharge)

	restRows, err := st.RestPointsSince(since, historyTrendLimit)
	if err != nil {
		return tr, err
	}
	for i := len(restRows) - 1; i >= 0; i-- {
		rp := restRows[i]
		tr.RestPoints = append(tr.RestPoints, restEntry{TS: rp.TS, UV: rp.UV, Cap: rp.Cap})
	}

	ccctRows, err := st.CCCTSince(since, historyTrendLimit)
	if err != nil {
		return tr, err
	}
	for i := len(ccctRows) - 1; i >= 0; i-- {
		c := ccctRows[i]
		tr.Ccct = append(tr.Ccct, ccctEntry{TS: c.TS, Secs: c.Secs, VwLo: c.VwLo, VwHi: c.VwHi})
	}

	icaRows, err := st.ICAPeaksSince(since, historyTrendLimit)
	if err != nil {
		return tr, err
	}
	for i := len(icaRows) - 1; i >= 0; i-- {
		ip := icaRows[i]
		if h := finitePtr(&ip.PeakHRel); h != nil {
			tr.IcaPeaks = append(tr.IcaPeaks, icaEntry{TS: ip.TS, PeakUV: ip.PeakUV, PeakHRel: h})
		}
	}

	estRows, err := st.EstimatesSince(since, historyTrendLimit)
	if err != nil {
		return tr, err
	}
	for i := len(estRows) - 1; i >= 0; i-- {
		tv := estRows[i]
		tr.Estimates = append(tr.Estimates, recentEntry{TS: tv.TS, Mah: tv.V / 1000})
	}

	return tr, nil
}

// reverseSessions 原地把会话数组反转为时间升序（旧→新），与 WebUI 图表 helper 一致。
func reverseSessions(rows []sessionEntry) {
	for i, j := 0, len(rows)-1; i < j; i, j = i+1, j-1 {
		rows[i], rows[j] = rows[j], rows[i]
	}
}

// reverseDischarge 原地把放电数组反转为时间升序（旧→新），与 WebUI 图表 helper 一致。
func reverseDischarge(rows []dischargeEntry) {
	for i, j := 0, len(rows)-1; i < j; i, j = i+1, j-1 {
		rows[i], rows[j] = rows[j], rows[i]
	}
}
