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

// historySummary 为范围内摘要统计（会话数/有效会话数/放电数/样本数，
// 以及有效会话的充入总量、平均每次充入与平均时长）。
type historySummary struct {
	Days          int   `json:"days"`
	Since         int64 `json:"since"`
	Until         int64 `json:"until"`
	Sessions      int   `json:"sessions"`
	ValidSessions int   `json:"valid_sessions"`
	Discharges    int   `json:"discharges"`
	Samples       int64 `json:"samples"`
	// ChargeMah 有效会话累计充入（mAh，电量计差分优先、缺失回退积分），
	// AvgMah 为平均每次充入，AvgSecs 为平均会话时长（秒）；无有效会话时省略。
	ChargeMah int64 `json:"charge_mah,omitempty"`
	AvgMah    int64 `json:"avg_mah,omitempty"`
	AvgSecs   int64 `json:"avg_secs,omitempty"`
	OldestTs  int64 `json:"oldest_ts,omitempty"`
	NewestTs  int64 `json:"newest_ts,omitempty"`
}

// historyTrends 为历史页趋势视图的数据集（各表最多 historyTrendLimit 点）。
// Archive 不受 --days 影响：它来自永久保留的按日档案，是 90 天以外容量的来源。
type historyTrends struct {
	Sessions   []sessionEntry   `json:"sessions"`
	Discharge  []dischargeEntry `json:"discharge"`
	RestPoints []restEntry      `json:"rest_points"`
	Ccct       []ccctEntry      `json:"ccct"`
	IcaPeaks   []icaEntry       `json:"ica_peaks"`
	Estimates  []recentEntry    `json:"estimates"`
	Archive    []archiveEntry   `json:"archive"`
}

// archiveEntry 为长期容量档案的一个点：TS 为该本地日最后一次估算的时刻，
// Mah 为当日容量（mAh）。
type archiveEntry struct {
	TS  int64 `json:"ts"`
	Mah int64 `json:"mah"`
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
	a.repairDuplicatesIfStale()

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
		de := convDischarge(dr)
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
	chargeUAh, avgUAh, avgSecs := summarizeCharges(sessRows)
	summary.ChargeMah = chargeUAh / 1000
	summary.AvgMah = avgUAh / 1000
	summary.AvgSecs = avgSecs
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

	trends, err := loadHistoryTrends(st, since, sessRows, disRows)
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

// loadHistoryTrends 读取各类趋势点并统一为时间升序。充电/放电行由
// buildHistoryDoc 传入（其摘要与分页已查过同一份范围内全量行）：趋势只取
// 新→旧的前 historyTrendLimit 条，避免同一范围对 sessions/discharge 各扫两遍库。
func loadHistoryTrends(st *Store, since int64, sessRows []Session, disRows []DischargeRow) (historyTrends, error) {
	tr := historyTrends{}

	// 查询结果为新→旧；先截近端上限，再转 JSON 条目并反转为旧→新（图表方向）。
	if len(sessRows) > historyTrendLimit {
		sessRows = sessRows[:historyTrendLimit]
	}
	tr.Sessions = convSessions(sessRows)
	reverseInPlace(tr.Sessions)
	if len(disRows) > historyTrendLimit {
		disRows = disRows[:historyTrendLimit]
	}
	tr.Discharge = convDischarges(disRows)
	reverseInPlace(tr.Discharge)

	restRows, err := st.RestPointsSince(since, historyTrendLimit)
	if err != nil {
		return tr, err
	}
	tr.RestPoints = convRestPoints(restRows)
	reverseInPlace(tr.RestPoints)

	ccctRows, err := st.CCCTSince(since, historyTrendLimit)
	if err != nil {
		return tr, err
	}
	tr.Ccct = convCCCT(ccctRows)
	reverseInPlace(tr.Ccct)

	icaRows, err := st.ICAPeaksSince(since, historyTrendLimit)
	if err != nil {
		return tr, err
	}
	// 趋势图只画有有限峰高的点：非有限值在条目里是「缺失即省略」，直接跳过。
	// 先显式初始化空切片：该字段无 omitempty，nil 会被序列化成 null，破坏
	// 「空数据输出 []」的既有契约（新装/长期无充电会话时必然走到这里）。
	tr.IcaPeaks = []icaEntry{}
	for _, e := range convICAPeaks(icaRows) {
		if e.PeakHRel != nil {
			tr.IcaPeaks = append(tr.IcaPeaks, e)
		}
	}
	reverseInPlace(tr.IcaPeaks)

	estRows, err := st.EstimatesSince(since, historyTrendLimit)
	if err != nil {
		return tr, err
	}
	tr.Estimates = convEstimates(estRows)
	reverseInPlace(tr.Estimates)

	// 长期容量档案：按本地日汇总、永久保留，按需压缩到 historyArchiveLimit 点。
	dailyRows, err := st.DailyEstimates(archiveQueryLimit)
	if err != nil {
		return tr, err
	}
	tr.Archive = convArchive(downsampleDaily(dailyRows, historyArchiveLimit))

	return tr, nil
}

// summarizeCharges 由范围内会话行汇总充入统计（只计有效会话）：累计充入 µAh
// （结算采信的电量计差分优先，缺失/未采信回退电流积分）、平均每次充入 µAh、
// 平均会话时长（秒）。没有任何有效会话时全部返回 0（JSON 省略）。
func summarizeCharges(rows []Session) (chargeUAh, avgUAh, avgSecs int64) {
	var n, secs int64
	for _, se := range rows {
		if !se.Valid {
			continue
		}
		n++
		secs += se.EndTs - se.StartTs
		energy := se.CounterUAh
		if energy <= 0 {
			energy = se.AccUAs / 3600
		}
		if energy > 0 {
			chargeUAh += energy
		}
	}
	if n == 0 {
		return 0, 0, 0
	}
	return chargeUAh, chargeUAh / n, secs / n
}
