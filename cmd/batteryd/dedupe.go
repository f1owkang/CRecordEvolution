// 多 daemon 并发写库遗留数据的一次性清理（issue #12）。
//
// 背景：升级后旧 daemon 仍会驻留（旧版本没有单实例锁），只要管理器/软重启又拉起
// 一个新 daemon，两个进程就会各自从 kv 恢复同一次充电会话、各自在 status 翻转时
// 结算一次；放电同理。结果就是历史里同一段会话/放电出现两行，只有 avgI、ua 这类
// 进程内累计值有细微差别。单实例锁（singleton.go）只挡住增量，已经落库的重复行
// 在这里按「起止电量相同 + 起止时刻相近」配对删除，保留最后写入的一行。
//
// 只做一次性修复：kv duplicate_repair 标记落地后不再扫描；清理失败只记事件、
// 下次启动重试，绝不阻断采样。

package main

import (
	"database/sql"
	"fmt"
)

// kvDuplicateRepair 一次性数据修复标记：多 daemon 并发写库的重复会话/放电行已清理。
const kvDuplicateRepair = "duplicate_repair"

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

// sameChargeSession 判两行是否同一次充电会话：起止电量与有效性/拒收原因一致，
// 起止时刻都在容差内。进程内累计的 ua/avgI 不参与比较——重复行本来就只在
// 这些值上有差别。
func sameChargeSession(a, b sessionSig) bool {
	if a.startCap != b.startCap || a.endCap != b.endCap {
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
// id 升序；相邻重复对里保留 id 更大（最后写入、带着最新校准口径）的一行。
func duplicateSessionIDs(rows []sessionSig) []int64 {
	var removed []int64
	for i := 1; i < len(rows); i++ {
		if rows[i].startTs <= 0 {
			continue
		}
		if sameChargeSession(rows[i-1], rows[i]) {
			removed = append(removed, rows[i-1].id)
		}
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

// RepairDuplicateRecords 清理多 daemon 并发写库留下的重复会话/放电行。
// 已执行过（kv 标记已落）直接返回；清理与标记写入在同一个事务里，避免并发
// 子进程（daemon / json / once）重复执行，失败则下次启动用同一逻辑重试。
func (s *Store) RepairDuplicateRecords() error {
	if v, ok := s.KVGet(kvDuplicateRepair); ok && v == "1" {
		return nil
	}
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("重复记录清理开启事务失败：%w", err)
	}
	defer func() { _ = tx.Rollback() }()

	// 会话：90 天窗口内行数不多，取出关键列在 Go 里配对，避免复杂 SQL。
	sessions, err := queryAll(tx, `SELECT id, start_ts, end_ts, start_cap, end_cap, valid, invalid_reason
		FROM sessions ORDER BY start_ts, id`, scanSessionSig)
	if err != nil {
		return fmt.Errorf("读取会话失败：%w", err)
	}
	for _, id := range duplicateSessionIDs(sessions) {
		if _, err := tx.Exec(`DELETE FROM sessions WHERE id = ?`, id); err != nil {
			return fmt.Errorf("删除重复会话 id=%d 失败：%w", id, err)
		}
	}

	discharges, err := queryAll(tx, `SELECT end_ts, secs, uah, start_cap
		FROM discharge ORDER BY end_ts`, scanDischargeSig)
	if err != nil {
		return fmt.Errorf("读取放电记录失败：%w", err)
	}
	for _, ts := range duplicateDischargeEndTs(discharges) {
		if _, err := tx.Exec(`DELETE FROM discharge WHERE end_ts = ?`, ts); err != nil {
			return fmt.Errorf("删除重复放电 end_ts=%d 失败：%w", ts, err)
		}
	}

	if _, err := tx.Exec(`INSERT OR REPLACE INTO kv(k, v) VALUES(?, '1')`, kvDuplicateRepair); err != nil {
		return err
	}
	return tx.Commit()
}
