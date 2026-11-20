package main

import (
	"database/sql"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// 用 issue #12 真机 diag/日志里的原始数值构造回放夹具（2026-11-15 三星 A56）：
//   - 日志 [11-15 21:11:53/54/59] 三个 daemon 各结算了一次同一次拒收会话：
//     cap 86→90、dur 23m45s/23m45s/24m45s、avgI 544/572/580mA、reason delta_lt_20；
//   - 报告者截图里 05:05 / 05:06 两条放电是同一段被结算两次（cap 100→58、
//     released 2060mAh、implied 4904mAh）；
//   - 20:40 的采信会话（cap 58→88）与 20:12 的放电是独立记录，不能被误删。
//
// 复核目标：21:11 三条只留最后写入的一条、05:05/05:06 只留较晚一条，正常记录不动；
// 清理结果计数与事件正确；二次扫描（旧 daemon 又写进重复行）仍能自愈。
func TestIssue12ReplayDedupe(t *testing.T) {
	st := openTestStore(t, filepath.Join(t.TempDir(), "battery.db"))
	defer st.Close()

	loc := time.FixedZone("CST", 8*3600)
	at := func(h, m, s int) int64 { return time.Date(2026, 11, 15, h, m, s, 0, loc).Unix() }

	insertSession := func(startTs, endTs, startCap, endCap int64, valid bool, reason string, accUAs, cc, avgI int64) {
		t.Helper()
		if _, err := st.InsertSession(Session{
			StartTs: startTs, EndTs: endTs, StartCap: startCap, EndCap: endCap,
			AccUAs: accUAs, CounterUAh: cc, AvgI: avgI, Valid: valid, InvalidReason: reason,
		}); err != nil {
			t.Fatalf("InsertSession: %v", err)
		}
	}
	// 16:02 与 20:40 的独立会话（diag 里原样出现），不应被合并。
	insertSession(at(15, 2, 0), at(16, 2, 0), 94, 100, false, "delta_lt_20", 1_404_375_000, 285_000, 683_000)
	insertSession(at(19, 55, 0), at(20, 40, 0), 58, 88, true, "", 5_386_380_000, 1_490_000, 3_235_000)
	// 21:11 的重复三行（日志原始时刻与数值）。
	insertSession(at(20, 48, 8), at(21, 11, 53), 86, 90, false, "delta_lt_20", 775_440_000, 215_000, 544_000)
	insertSession(at(20, 48, 9), at(21, 11, 54), 86, 90, false, "delta_lt_20", 780_000_000, 215_000, 572_000)
	insertSession(at(20, 47, 14), at(21, 11, 59), 86, 90, false, "delta_lt_20", 790_000_000, 215_000, 580_000)

	insertDischarge := func(ts, secs, uah, startCap, endCap int64, implied *int64) {
		t.Helper()
		if err := st.InsertDischarge(DischargeRow{
			TS: ts, Secs: secs, UAh: uah, StartCap: startCap, EndCap: endCap, Implied: implied,
		}); err != nil {
			t.Fatalf("InsertDischarge: %v", err)
		}
	}
	implied := int64(4_904_000) // 4904 mAh
	// 05:05 / 05:06 的重复放电（截图里的那对），以及与它们相隔很远的 20:12 正常行。
	insertDischarge(at(5, 5, 0), 13_100, 2_060_000, 100, 58, &implied)
	insertDischarge(at(5, 6, 0), 13_120, 2_060_000, 100, 58, &implied)
	insertDischarge(at(20, 12, 0), 13_080, 2_060_000, 100, 58, &implied)
	// 另一段起始电量不同的真实放电，不能被判成重复。
	insertDischarge(at(9, 0, 0), 5_000, 900_000, 44, 30, nil)

	now := at(21, 40, 0)
	res, err := st.RepairDuplicateRecords(now)
	if err != nil {
		t.Fatalf("RepairDuplicateRecords: %v", err)
	}
	if res.Sessions != 2 || res.Discharges != 1 {
		t.Fatalf("清理结果 = %+v, want 2 会话 + 1 放电", res)
	}
	if got := queryInt64(t, st, `SELECT COUNT(*) FROM sessions`); got != 3 {
		t.Fatalf("清理后会话数 = %d, want 3", got)
	}
	if got := queryInt64(t, st, `SELECT COUNT(*) FROM sessions WHERE start_cap = 86 AND end_cap = 90`); got != 1 {
		t.Fatalf("21:11 的重复会话应只留 1 行, got %d", got)
	}
	// 保留最后写入（id 最大）的一行：21:11:59、avgI 580mA。
	if got := queryInt64(t, st, `SELECT avg_i FROM sessions WHERE start_cap = 86`); got != 580_000 {
		t.Fatalf("应保留最后写入的会话, avgI = %d", got)
	}
	if got := queryInt64(t, st, `SELECT COUNT(*) FROM sessions WHERE start_cap IN (94, 58)`); got != 2 {
		t.Fatalf("独立会话不应被删, got %d", got)
	}
	if got := queryInt64(t, st, `SELECT COUNT(*) FROM discharge`); got != 3 {
		t.Fatalf("清理后放电数 = %d, want 3", got)
	}
	if got := queryInt64(t, st, `SELECT COUNT(*) FROM discharge WHERE end_ts = `+itoa64(at(5, 5, 0))); got != 0 {
		t.Fatal("较晚的重复放电应保留 05:06，05:05 应删除")
	}
	if got := queryInt64(t, st, `SELECT COUNT(*) FROM discharge WHERE end_ts = `+itoa64(at(5, 6, 0))); got != 1 {
		t.Fatal("05:06 的放电应保留")
	}
	info := st.DupRepairInfo()
	if info == nil || info.Sessions != 2 || info.Discharges != 1 || info.TS != now {
		t.Fatalf("dup_repair 信息 = %+v, want 2/1 ts=%d", info, now)
	}
	if got := queryInt64(t, st, `SELECT COUNT(*) FROM events WHERE kind = 'dup_repair'`); got != 1 {
		t.Fatalf("应恰好落一条 dup_repair 事件, got %d", got)
	}

	// 旧 daemon 在首次清理后又写进一条同会话（issue #12 的升级场景）：重扫必须再收掉。
	insertSession(at(20, 48, 20), at(21, 11, 58), 86, 90, false, "delta_lt_20", 800_000_000, 215_000, 590_000)
	res2, err := st.RepairDuplicateRecords(now + 300)
	if err != nil {
		t.Fatalf("RepairDuplicateRecords(2): %v", err)
	}
	if res2.Sessions != 1 || res2.Discharges != 0 {
		t.Fatalf("二次清理 = %+v, want 1 会话", res2)
	}
	if info2 := st.DupRepairInfo(); info2 == nil || info2.Sessions != 3 || info2.Discharges != 1 {
		t.Fatalf("累计清理计数 = %+v, want sessions=3 discharges=1", info2)
	}
	// 幂等：再扫一次不应有任何删除。
	if res3, err := st.RepairDuplicateRecords(now + 600); err != nil || res3.Removed() {
		t.Fatalf("幂等扫描 = %+v err=%v", res3, err)
	}
}

// 用导出的真机 battery.db 复核重复清理：CRE_REPLAY_DB 指向数据库文件（只读副本，
// 不会改原文件）。没有该文件时跳过，与 CRE_MIDWIN_DB 的回放用例同风格。
func TestRepairDuplicateRecordsReplayExternalDB(t *testing.T) {
	path := os.Getenv("CRE_REPLAY_DB")
	if path == "" {
		t.Skip("未设置 CRE_REPLAY_DB，跳过真机数据复核")
	}
	src, err := os.ReadFile(path)
	if err != nil {
		t.Skipf("读取 %s 失败: %v", path, err)
	}
	tmp := filepath.Join(t.TempDir(), "replay.db")
	if err := os.WriteFile(tmp, src, 0o600); err != nil {
		t.Fatal(err)
	}
	st := openTestStore(t, tmp)
	defer st.Close()

	beforeS := queryInt64(t, st, `SELECT COUNT(*) FROM sessions`)
	beforeD := queryInt64(t, st, `SELECT COUNT(*) FROM discharge`)
	res, err := st.RepairDuplicateRecords(time.Now().Unix())
	if err != nil {
		t.Fatalf("RepairDuplicateRecords: %v", err)
	}
	afterS := queryInt64(t, st, `SELECT COUNT(*) FROM sessions`)
	afterD := queryInt64(t, st, `SELECT COUNT(*) FROM discharge`)
	t.Logf("真机库复核：sessions %d→%d（删 %d）、discharge %d→%d（删 %d）",
		beforeS, afterS, res.Sessions, beforeD, afterD, res.Discharges)
	if res.Sessions != beforeS-afterS || res.Discharges != beforeD-afterD {
		t.Fatalf("统计与实际删除不一致: res=%+v 行数变化 %d/%d",
			res, beforeS-afterS, beforeD-afterD)
	}
	// 清理后再扫一遍必须零删除（可重跑但幂等）。
	if again, err := st.RepairDuplicateRecords(time.Now().Unix() + 1); err != nil || again.Removed() {
		t.Fatalf("二次扫描应零删除, got %+v err=%v", again, err)
	}

	// 在真实数据里注入已知的重复对（复制最后一条会话/放电，微调结束时刻），复核
	// 只删重复、不碰其它真实行。
	var sess Session
	var reason sql.NullString
	err = st.db.QueryRow(`SELECT start_ts,end_ts,start_cap,end_cap,ua,cc,avg_i,c_rate,
		temp_min,temp_max,temp_avg,v_start,duration,valid,invalid_reason
		FROM sessions ORDER BY id DESC LIMIT 1`).Scan(
		&sess.StartTs, &sess.EndTs, &sess.StartCap, &sess.EndCap, &sess.AccUAs, &sess.CounterUAh, &sess.AvgI, &sess.CRate,
		&sess.TempMin, &sess.TempMax, &sess.TempAvg, &sess.VStart, &sess.Duration, &sess.Valid, &reason)
	if err != nil {
		t.Fatalf("读取真实会话: %v", err)
	}
	sess.InvalidReason = reason.String
	dupSess := sess
	dupSess.StartTs += 2
	dupSess.EndTs += 2
	if _, err := st.InsertSession(dupSess); err != nil {
		t.Fatalf("注入重复会话: %v", err)
	}

	rows, err := st.RecentDischarge(1)
	if err != nil || len(rows) != 1 {
		t.Fatalf("读取真实放电: rows=%d err=%v", len(rows), err)
	}
	dupDis := rows[0]
	dupDis.TS += 40
	dupDis.Secs += 20
	if err := st.InsertDischarge(dupDis); err != nil {
		t.Fatalf("注入重复放电: %v", err)
	}

	beforeS2 := queryInt64(t, st, `SELECT COUNT(*) FROM sessions`)
	beforeD2 := queryInt64(t, st, `SELECT COUNT(*) FROM discharge`)
	res2, err := st.RepairDuplicateRecords(time.Now().Unix() + 2)
	if err != nil {
		t.Fatalf("注入后清理: %v", err)
	}
	if res2.Sessions != 1 || res2.Discharges != 1 {
		t.Fatalf("注入后清理结果 = %+v, want 1 会话 + 1 放电", res2)
	}
	if got := queryInt64(t, st, `SELECT COUNT(*) FROM sessions`); got != beforeS2-1 {
		t.Fatalf("真实库注入复核：sessions %d→%d, want 只删 1", beforeS2, got)
	}
	if got := queryInt64(t, st, `SELECT COUNT(*) FROM discharge`); got != beforeD2-1 {
		t.Fatalf("真实库注入复核：discharge %d→%d, want 只删 1", beforeD2, got)
	}
	if again, err := st.RepairDuplicateRecords(time.Now().Unix() + 3); err != nil || again.Removed() {
		t.Fatalf("注入复核后应零删除, got %+v err=%v", again, err)
	}
}

func itoa64(v int64) string {
	if v == 0 {
		return "0"
	}
	neg := v < 0
	if neg {
		v = -v
	}
	var buf [20]byte
	i := len(buf)
	for v > 0 {
		i--
		buf[i] = byte('0' + v%10)
		v /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
