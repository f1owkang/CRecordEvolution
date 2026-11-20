// 多 daemon 并发写库遗留数据的可重跑清理（issue #12）。
//
// 背景：升级后旧 daemon 仍会驻留（旧版本没有单实例锁），只要管理器/软重启又拉起
// 一个新 daemon，两个进程就会各自从 kv 恢复同一次充电会话、各自在 status 翻转时
// 结算一次；放电同理。结果就是历史里同一段会话/放电出现两行，只有 avgI、ua 这类
// 进程内累计值有细微差别。单实例锁（singleton.go）只挡住增量，已经落库的重复行
// 在这里按「起止电量相同 + 起止时刻相近」配对删除，保留最后写入的一行。
//
// 这里不再用一次性标记：旧 daemon 在第一次清理之后仍可能继续写进新的重复行
// （issue #12 的升级场景），所以扫描必须可重跑——daemon 启动时强制扫一次，
// 检测到旧 daemon 心跳（stale）时按 dupScanMinInterval 节流重扫。判重本身幂等。
//
// 清理失败只记事件、下次触发时重试，绝不阻断采样。

package main

import (
	"database/sql"
	"fmt"
	"strconv"
)

const (
	// kvDupScanTs 上次扫描时刻（Unix 秒）：WebUI 的 json 每 60s 轮询一次，
	// 扫描要开写事务，用它与 dupScanMinInterval 把重扫节奏压到 ≥5 分钟。
	kvDupScanTs = "dup_scan_ts"
	// kvDupRepairTs / kvDupRemoved* 最近一次清理的时间与累计删除行数，
	// 供 JSON（dup_repair）、diag 与设置页展示；从未清理过时整体省略。
	kvDupRepairTs          = "dup_repair_ts"
	kvDupRemovedSessions   = "dup_removed_sessions"
	kvDupRemovedDischarges = "dup_removed_discharges"

	// dupScanMinInterval 两次重复扫描的最小间隔（秒）。
	dupScanMinInterval int64 = 300
)

// dupResult 是一次重复记录扫描的结果（供事件/日志/JSON 出口）。
type dupResult struct {
	Sessions   int64
	Discharges int64
}

// Removed 本趟是否真的删掉了行。
func (r dupResult) Removed() bool { return r.Sessions > 0 || r.Discharges > 0 }

// dupRepairInfo 是清理结果的外部出口（JSON `dup_repair` 与 diag）；TS 为最近一次
// 删除发生的时刻，Sessions/Discharges 为累计删除行数。
type dupRepairInfo struct {
	TS         int64 `json:"ts"`
	Sessions   int64 `json:"sessions"`
	Discharges int64 `json:"discharges"`
}

// DuplicatesScanDue 判断是否该扫描重复记录：force 用于 daemon 启动（每次都扫），
// 其余调用点（WebUI/action 检测到旧 daemon）按 dupScanMinInterval 节流。
func (s *Store) DuplicatesScanDue(now int64, force bool) bool {
	if force {
		return true
	}
	last := kvInt(s, kvDupScanTs)
	return last <= 0 || now-last >= dupScanMinInterval
}

// DuplicatesScanned 是否扫描过。升级后首次打开 WebUI 时即使没有 stale 心跳也应
// 扫一次，不必非等 daemon 重启才能清掉升级前遗留的重复行。
func (s *Store) DuplicatesScanned() bool { return kvInt(s, kvDupScanTs) > 0 }

// DupRepairInfo 读取历史清理结果；从未清理过返回 nil（JSON 省略该字段）。
func (s *Store) DupRepairInfo() *dupRepairInfo {
	ts := kvInt(s, kvDupRepairTs)
	if ts <= 0 {
		return nil
	}
	return &dupRepairInfo{
		TS:         ts,
		Sessions:   kvInt(s, kvDupRemovedSessions),
		Discharges: kvInt(s, kvDupRemovedDischarges),
	}
}

// sessionSig 是充电会话去重所需的列（id 用于删除与保留「最后写入」的那行）。
type sessionSig struct {
	id               int64
	startTs, endTs   int64
	startCap, endCap int64
	valid            bool
	reason           string
}

// dischargeSig 是放电去重所需的列（end_ts 为主键）。
type dischargeSig struct {
	endTs, secs, uah, startCap int64
}

// dedupeWindowSecs 判「同一次会话/放电段」的起止时刻容差：不同 daemon 各自在
// 自己那一拍看到 status 翻转，结算时刻会差几十秒；而两次真实会话之间至少隔着
// 一次去抖（3 拍 ≈ 3 分钟）和一次充电过程，正常远大于此窗口。
const dedupeWindowSecs int64 = 180

// dedupeDischargeSecs 放电段时长容差：同一段放电在不同进程里累计的时长只差几个
// tick（≤5 分钟），真实相邻两段放电不可能同时满足起始电量、放出电量都一致。
const dedupeDischargeSecs int64 = 300

// dedupeDischargeTs 放电结算时刻容差：重复行只差几十秒；放宽到 10 分钟仍小于
// 「两段真实放电之间必须隔着一次充电」的间隔。
const dedupeDischargeTs int64 = 600

// sameDischargeSession 判两行是否同一次放电被结算了两次：起始电量一致、放出电量
// 相差 ≤0.5%、时长与结束时刻都落在容差内。结束电量允许不同——它由各进程结算时
// 读到的显示电量决定，本来就会差 1 个百分点。
func sameDischargeSession(a, b dischargeSig) bool {
	if a.startCap != b.startCap {
		return false
	}
	if absI64(a.uah-b.uah)*200 > a.uah {
		return false
	}
	if absI64(a.secs-b.secs) > dedupeDischargeSecs {
		return false
	}
	return absI64(a.endTs-b.endTs) <= dedupeDischargeTs
}

// duplicateDischargeEndTs 返回应删除的 end_ts：按结束时刻升序扫描，与上一行同段
// 时保留较晚结束（更完整）的一行。放电段不会相互重叠，重复行在时间轴上是相邻的。
func duplicateDischargeEndTs(rows []dischargeSig) []int64 {
	var removed []int64
	for i := 1; i < len(rows); i++ {
		if sameDischargeSession(rows[i-1], rows[i]) {
			removed = append(removed, rows[i-1].endTs)
		}
	}
	return removed
}

// sameChargeSession 判两行是否同一次充电会话：起止电量一致、有效性/拒收原因一致，
// 起止时刻都在容差内。结束电量允许差 1 个百分点——各进程在相邻拍结算，读到的
// 显示电量可能跨过一格。进程内累计的 ua/avgI 不参与比较。
func sameChargeSession(a, b sessionSig) bool {
	if a.startCap != b.startCap || absI64(a.endCap-b.endCap) > 1 {
		return false
	}
	if a.valid != b.valid || a.reason != b.reason {
		return false
	}
	if absI64(a.startTs-b.startTs) > dedupeWindowSecs {
		return false
	}
	return absI64(a.endTs-b.endTs) <= dedupeWindowSecs
}

// duplicateSessionIDs 返回应删除的会话 id：输入按 start_ts 升序、同 start_ts 按
// id 升序。相邻同类行组成一组，组内保留 id 最大（最后写入、带着最新校准口径）
// 的一行。不能只保留「排序最后」的一行——重复行的起止时刻由各进程结算的那一拍
// 决定，最后写入的未必起止最晚，按排序取会删掉刚写进来的那行。
func duplicateSessionIDs(rows []sessionSig) []int64 {
	var removed []int64
	for i := 0; i < len(rows); {
		if rows[i].startTs <= 0 {
			i++
			continue
		}
		maxID := rows[i].id
		j := i + 1
		for j < len(rows) && sameChargeSession(rows[i], rows[j]) {
			if rows[j].id > maxID {
				maxID = rows[j].id
			}
			j++
		}
		for k := i; k < j; k++ {
			if rows[k].id != maxID {
				removed = append(removed, rows[k].id)
			}
		}
		i = j
	}
	return removed
}

// scanSessionSig / scanDischargeSig 只取去重需要的列。
func scanSessionSig(rows *sql.Rows) (sessionSig, error) {
	var v sessionSig
	var valid int64
	var reason sql.NullString
	if err := rows.Scan(&v.id, &v.startTs, &v.endTs, &v.startCap, &v.endCap, &valid, &reason); err != nil {
		return sessionSig{}, err
	}
	v.valid = valid == 1
	v.reason = reason.String
	return v, nil
}

func scanDischargeSig(rows *sql.Rows) (dischargeSig, error) {
	var v dischargeSig
	err := rows.Scan(&v.endTs, &v.secs, &v.uah, &v.startCap)
	return v, err
}

// RepairDuplicateRecords 清理多 daemon 并发写库留下的重复会话/放电行，可重复执行。
// 删除、扫描时刻与累计计数写在同一个事务里，避免并发子进程（daemon / json / once）
// 重复统计；失败则回滚，下次触发时重试。
func (s *Store) RepairDuplicateRecords(now int64) (dupResult, error) {
	var res dupResult
	prevSessions := kvInt(s, kvDupRemovedSessions)
	prevDischarges := kvInt(s, kvDupRemovedDischarges)

	tx, err := s.db.Begin()
	if err != nil {
		return res, fmt.Errorf("重复记录清理开启事务失败：%w", err)
	}
	defer func() { _ = tx.Rollback() }()

	// 会话：90 天窗口内行数不多，取出关键列在 Go 里配对，避免复杂 SQL。
	sessions, err := queryAll(tx, `SELECT id, start_ts, end_ts, start_cap, end_cap, valid, invalid_reason
		FROM sessions ORDER BY start_ts, id`, scanSessionSig)
	if err != nil {
		return res, fmt.Errorf("读取会话失败：%w", err)
	}
	removeIDs := duplicateSessionIDs(sessions)
	for _, id := range removeIDs {
		if _, err := tx.Exec(`DELETE FROM sessions WHERE id = ?`, id); err != nil {
			return res, fmt.Errorf("删除重复会话 id=%d 失败：%w", id, err)
		}
	}
	res.Sessions = int64(len(removeIDs))

	discharges, err := queryAll(tx, `SELECT end_ts, secs, uah, start_cap
		FROM discharge ORDER BY end_ts`, scanDischargeSig)
	if err != nil {
		return res, fmt.Errorf("读取放电记录失败：%w", err)
	}
	removeTs := duplicateDischargeEndTs(discharges)
	for _, ts := range removeTs {
		if _, err := tx.Exec(`DELETE FROM discharge WHERE end_ts = ?`, ts); err != nil {
			return res, fmt.Errorf("删除重复放电 end_ts=%d 失败：%w", ts, err)
		}
	}
	res.Discharges = int64(len(removeTs))

	if _, err := tx.Exec(`INSERT OR REPLACE INTO kv(k, v) VALUES(?, ?)`,
		kvDupScanTs, strconv.FormatInt(now, 10)); err != nil {
		return res, err
	}
	if res.Removed() {
		for k, v := range map[string]int64{
			kvDupRepairTs:          now,
			kvDupRemovedSessions:   prevSessions + res.Sessions,
			kvDupRemovedDischarges: prevDischarges + res.Discharges,
		} {
			if _, err := tx.Exec(`INSERT OR REPLACE INTO kv(k, v) VALUES(?, ?)`, k, strconv.FormatInt(v, 10)); err != nil {
				return res, err
			}
		}
	}
	if err := tx.Commit(); err != nil {
		return res, err
	}
	if res.Removed() {
		_ = s.InsertEvent("dup_repair", fmt.Sprintf("会话 %d 条 / 放电 %d 条", res.Sessions, res.Discharges))
	}
	return res, nil
}
