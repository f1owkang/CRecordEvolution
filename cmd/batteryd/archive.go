// 长期容量档案：estimates 表按 90 天保留窗口裁剪，而健康度衰减以季度为单位，
// 用户看不到一个季度以前的容量。daily 表把每天的最后一笔采信估算按本地日汇总
// 并永久保留（不进 PruneBefore），是历史页「长期容量档案」的唯一来源。
//
// 档案是只读派生视图：不参与结算、校准与四路互检，写失败也不阻断采样主链路。
package main

import (
	"database/sql"
	"fmt"
	"time"
)

const (
	// kvDailyBackfill 一次性回填标记：升级后把仍在保留窗口内的 estimates 折算
	// 进 daily，避免新功能上线时档案从空白开始。
	kvDailyBackfill = "daily_backfill"

	// archiveQueryLimit 单次读取的档案行数上限（约 11 年每日一行）。
	archiveQueryLimit = 4000
	// historyArchiveLimit 历史页长期档案最多下发的点数：超过后等分聚合为
	// 若干周均点，保证数年档案也保持可读密度与可接受的 JSON 体积。
	historyArchiveLimit = 120
)

// DailyRow 长期档案一行：Day 为本地日 yyyymmdd，TS/UAh 为当日最后一次采信
// 估算的时刻与容量（µAh），N 为当日估算点数量。
type DailyRow struct {
	Day int64
	TS  int64
	UAh int64
	N   int64
}

// dayKeyAt 把 Unix 秒折成给定时区的本地日 yyyymmdd（数值序即日期序）。
// 单独抽成纯函数便于用固定时区测试跨日边界。
func dayKeyAt(ts int64, loc *time.Location) int64 {
	t := time.Unix(ts, 0).In(loc)
	return int64(t.Year())*10000 + int64(t.Month())*100 + int64(t.Day())
}

// localDayKey 用进程解析出的本地时区折算本地日（与 localNow 同源）。
func localDayKey(ts int64) int64 {
	localZoneOnce.Do(func() { localZone = resolveLocalZone() })
	return dayKeyAt(ts, localZone)
}

// upsertDaily 把一笔估算并入当天档案：容量取当天最后一笔，点数累加。
// 接受 *sql.Tx 以便调用方把「estimates 明细 + daily 档案」放进同一事务。
func upsertDaily(tx execer, day, ts, uah int64) error {
	_, err := tx.Exec(`INSERT INTO daily(day,ts,uah,n) VALUES(?,?,?,1)
		ON CONFLICT(day) DO UPDATE SET ts=excluded.ts, uah=excluded.uah, n=daily.n+1`,
		day, ts, uah)
	return err
}

// DailyCount 返回档案表行数（回填判空与测试使用）。
func (s *Store) DailyCount() (int64, error) {
	var n int64
	err := s.db.QueryRow(`SELECT COUNT(*) FROM daily`).Scan(&n)
	return n, err
}

// aggregateDaily 把 estimates 明细按本地日聚合为档案行：同日多笔只保留最后一笔
// 容量（时间升序输入），N 为当日估算点数量。纯函数便于单测跨日边界。
func aggregateDaily(rows []TsVal) []DailyRow {
	days := make([]DailyRow, 0, len(rows))
	idx := make(map[int64]int, len(rows))
	for _, tv := range rows {
		day := localDayKey(tv.TS)
		if i, ok := idx[day]; ok {
			days[i].TS = tv.TS
			days[i].UAh = tv.V
			days[i].N++
			continue
		}
		idx[day] = len(days)
		days = append(days, DailyRow{Day: day, TS: tv.TS, UAh: tv.V, N: 1})
	}
	return days
}

// BackfillDaily 把保留窗口内的 estimates 按本地日回填进 daily。
//
// 幂等：标记已落后直接返回；回填本身用「按 estimates 聚合结果覆盖写」而不是
// 累加，因此可安全重试。不能因为 daily 已有行就认为已完成——首次回填失败（DB
// busy/IO）后，daemon 正常结算会先写出一条 daily，此时若直接落标记，升级前仍在
// estimates 里的历史点就永不进档案，90 天清理后档案出现永久空洞。
// 回填只在升级后的第一次启动执行，会触发一次 getprop 时区解析。
func (s *Store) BackfillDaily() error {
	if v, ok := s.KVGet(kvDailyBackfill); ok && v == "1" {
		return nil
	}
	rows, err := queryAll(s.db, `SELECT `+estimateColumns+` FROM estimates ORDER BY ts ASC`,
		scanTsVal)
	if err != nil {
		return err
	}
	days := aggregateDaily(rows)
	if len(days) == 0 {
		return s.KVSet(kvDailyBackfill, "1")
	}
	if err := s.withTx(func(tx *sql.Tx) error {
		for _, d := range days {
			if _, err := tx.Exec(`INSERT INTO daily(day,ts,uah,n) VALUES(?,?,?,?)
				ON CONFLICT(day) DO UPDATE SET ts=excluded.ts, uah=excluded.uah, n=excluded.n`,
				d.Day, d.TS, d.UAh, d.N); err != nil {
				return err
			}
		}
		_, err := tx.Exec(`INSERT OR REPLACE INTO kv(k, v) VALUES(?, '1')`, kvDailyBackfill)
		return err
	}); err != nil {
		return err
	}
	_ = s.InsertEvent("daily_backfill", fmt.Sprintf("长期容量档案回填 %d 天", len(days)))
	return nil
}

// DailyEstimates 取最近 limit 天的档案，按日期升序返回（老→新，图表方向）。
func (s *Store) DailyEstimates(limit int) ([]DailyRow, error) {
	rows, err := queryAll(s.db, `SELECT `+dailyColumns+` FROM daily ORDER BY day DESC LIMIT ?`,
		scanDaily, limit)
	if err != nil {
		return nil, err
	}
	reverseInPlace(rows)
	return rows, nil
}

// downsampleDaily 把按日升序的档案压到最多 maxPts 个点：等分桶后取桶内
// 容量均值，时间戳取桶内最后一天。档案按天永久保留，数年数据直接下发会让
// 图表与 JSON 体积同时失控，桶均点同时起到平滑作用。
func downsampleDaily(rows []DailyRow, maxPts int) []DailyRow {
	if maxPts < 1 || len(rows) <= maxPts {
		return rows
	}
	out := make([]DailyRow, 0, maxPts)
	n := len(rows)
	for i := 0; i < maxPts; i++ {
		start := i * n / maxPts
		end := (i + 1) * n / maxPts
		if end <= start {
			continue
		}
		var sum int64
		for _, r := range rows[start:end] {
			sum += r.UAh
		}
		last := rows[end-1]
		out = append(out, DailyRow{
			Day: last.Day,
			TS:  last.TS,
			UAh: sum / int64(end-start),
			N:   int64(end - start),
		})
	}
	return out
}
