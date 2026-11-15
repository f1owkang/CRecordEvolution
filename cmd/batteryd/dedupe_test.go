package main

import (
	"path/filepath"
	"testing"
)

// 同一次充电会话被两个 daemon 各结算一次（issue #12）：起止电量、有效性一致，
// 只在起止时刻差几十秒、avgI/ua 不同。清理后应只留最后写入的一行，正常会话不动。
func TestRepairDuplicateSessions(t *testing.T) {
	st := openTestStore(t, filepath.Join(t.TempDir(), "battery.db"))
	defer st.Close()

	insert := func(startTs, endTs, startCap, endCap int64, reason string, ua int64) {
		t.Helper()
		if _, err := st.InsertSession(Session{
			StartTs: startTs, EndTs: endTs, StartCap: startCap, EndCap: endCap,
			AccUAs: ua, AvgI: ua / 600, Valid: false, InvalidReason: reason,
		}); err != nil {
			t.Fatalf("InsertSession: %v", err)
		}
	}
	// 正常相邻会话：27 分钟前结束、本次 28→31，不应该被合并。
	insert(1000, 1600, 36, 47, "delta_lt_20", 1924245000)
	// 重复对：同一会话，两次结算相差 2 秒、ua 不同。
	insert(3000, 3600, 28, 31, "delta_lt_20", 572325000)
	insert(3000, 3602, 28, 31, "delta_lt_20", 613380000)
	// 同起始时刻但结束电量不同：不是同一段，保留两行（宁可少删）。
	insert(5000, 5600, 30, 40, "", 111)
	insert(5000, 5600, 30, 41, "", 222)

	// openTestStore 已经落过标记；测试里手工回滚标记以触发清理。
	if err := st.KVSet(kvDuplicateRepair, "0"); err != nil {
		t.Fatal(err)
	}
	if err := st.RepairDuplicateRecords(); err != nil {
		t.Fatalf("RepairDuplicateRecords: %v", err)
	}

	if got := queryInt64(t, st, `SELECT COUNT(*) FROM sessions`); got != 4 {
		t.Fatalf("清理后会话数 = %d, want 4（删掉 1 条重复）", got)
	}
	if got := queryInt64(t, st, `SELECT COUNT(*) FROM sessions WHERE start_ts = 3000`); got != 1 {
		t.Fatalf("同一会话应只留 1 行, got %d", got)
	}
	// 保留的是后写入（id 更大）的那行：ua 取第二次结算的值。
	if got := queryInt64(t, st, `SELECT ua FROM sessions WHERE start_ts = 3000`); got != 613380000 {
		t.Fatalf("应保留最后写入的会话行, ua = %d", got)
	}
	if got := queryInt64(t, st, `SELECT COUNT(*) FROM kv WHERE k = '`+kvDuplicateRepair+`' AND v = '1'`); got != 1 {
		t.Fatal("清理后应落标记")
	}
	// 幂等：标记已落，再跑一次不改数据。
	if err := st.RepairDuplicateRecords(); err != nil {
		t.Fatal(err)
	}
	if got := queryInt64(t, st, `SELECT COUNT(*) FROM sessions`); got != 4 {
		t.Fatalf("重复执行不应再改数据, got %d", got)
	}
}

// 同一段放电被结算两次：起始电量与放出电量一致，结束时刻差几十秒、结束电量差 1%。
// 保留较晚结束的一行；起始电量不同或放出电量明显不同的放电记录不能合并。
func TestRepairDuplicateDischarge(t *testing.T) {
	st := openTestStore(t, filepath.Join(t.TempDir(), "battery.db"))
	defer st.Close()

	insert := func(ts, secs, uah, startCap, endCap int64) {
		t.Helper()
		if err := st.InsertDischarge(DischargeRow{
			TS: ts, Secs: secs, UAh: uah, StartCap: startCap, EndCap: endCap,
		}); err != nil {
			t.Fatalf("InsertDischarge: %v", err)
		}
	}
	insert(1000, 13160, 1490000, 31, 2)
	insert(1040, 13180, 1490000, 31, 3) // 重复，保留这行
	insert(2000, 3120, 575000, 78, 66)  // 另一段放电
	insert(5000, 100, 900000, 50, 40)   // 起始电量/电量都与前面不同

	if err := st.KVSet(kvDuplicateRepair, "0"); err != nil {
		t.Fatal(err)
	}
	if err := st.RepairDuplicateRecords(); err != nil {
		t.Fatalf("RepairDuplicateRecords: %v", err)
	}

	if got := queryInt64(t, st, `SELECT COUNT(*) FROM discharge`); got != 3 {
		t.Fatalf("清理后放电行数 = %d, want 3（删掉 1 条重复）", got)
	}
	if got := queryInt64(t, st, `SELECT COUNT(*) FROM discharge WHERE end_ts = 1040`); got != 1 {
		t.Fatal("应保留结束更晚的那条放电记录")
	}
	if got := queryInt64(t, st, `SELECT COUNT(*) FROM discharge WHERE end_ts = 1000`); got != 0 {
		t.Fatal("较早的重复放电记录应被删除")
	}
}

// 判重纯函数的边界：放出电量差 >0.5% 或时长差 >5 分钟都不是同一次放电。
func TestSameDischargeSessionBoundaries(t *testing.T) {
	base := dischargeSig{endTs: 100000, secs: 3600, uah: 1000000, startCap: 40}
	if !sameDischargeSession(base, dischargeSig{endTs: 100050, secs: 3610, uah: 1001000, startCap: 40}) {
		t.Fatal("容差内的两行应判为同一次放电")
	}
	if sameDischargeSession(base, dischargeSig{endTs: 100050, secs: 3610, uah: 1020000, startCap: 40}) {
		t.Fatal("放出电量差 2% 不应判为同一次放电")
	}
	if sameDischargeSession(base, dischargeSig{endTs: 100050, secs: 4000, uah: 1000000, startCap: 40}) {
		t.Fatal("时长差 >5 分钟不应判为同一次放电")
	}
	if sameDischargeSession(base, dischargeSig{endTs: 100050, secs: 3610, uah: 1000000, startCap: 41}) {
		t.Fatal("起始电量不同不应判为同一次放电")
	}
}
