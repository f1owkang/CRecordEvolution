package main

import (
	"path/filepath"
	"strconv"
	"testing"
)

// 同一次充电会话被两个 daemon 各结算一次（issue #12）：起止电量、有效性一致，
// 只在起止时刻差几十秒、avgI/ua 不同。清理后应只留最后写入的一行，正常会话不动；
// 结束电量差 1 个百分点（相邻拍读到不同格）也算同一次，差 >1 则宁可少删。
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
	// 结束电量差 1 个百分点：相邻拍读到不同格，仍是同一次（保留后写入的 41）。
	insert(5000, 5600, 30, 40, "", 111)
	insert(5000, 5601, 30, 41, "", 222)
	// 结束电量差 3 个百分点：不是同一段，保留两行。
	insert(7000, 7600, 50, 60, "", 333)
	insert(7000, 7600, 50, 63, "", 444)

	res, err := st.RepairDuplicateRecords(1_000_000)
	if err != nil {
		t.Fatalf("RepairDuplicateRecords: %v", err)
	}
	if !res.Removed() || res.Sessions != 2 || res.Discharges != 0 {
		t.Fatalf("清理结果 = %+v, want 2 会话", res)
	}
	if got := queryInt64(t, st, `SELECT COUNT(*) FROM sessions`); got != 5 {
		t.Fatalf("清理后会话数 = %d, want 5（删掉 2 条重复）", got)
	}
	if got := queryInt64(t, st, `SELECT COUNT(*) FROM sessions WHERE start_ts = 3000`); got != 1 {
		t.Fatalf("同一会话应只留 1 行, got %d", got)
	}
	// 保留的是后写入（id 更大）的那行：ua 取第二次结算的值。
	if got := queryInt64(t, st, `SELECT ua FROM sessions WHERE start_ts = 3000`); got != 613380000 {
		t.Fatalf("应保留最后写入的会话行, ua = %d", got)
	}
	if got := queryInt64(t, st, `SELECT end_cap FROM sessions WHERE start_ts = 5000`); got != 41 {
		t.Fatalf("结束电量差 1 的重复行应保留后写入的 41, got %d", got)
	}
	if got := queryInt64(t, st, `SELECT COUNT(*) FROM sessions WHERE start_ts = 7000`); got != 2 {
		t.Fatalf("结束电量差 3 不应合并, got %d 行", got)
	}
	// 累计结果与事件：供 JSON/diag/设置页展示。
	info := st.DupRepairInfo()
	if info == nil || info.Sessions != 2 || info.TS != 1_000_000 {
		t.Fatalf("dup_repair 信息 = %+v", info)
	}
	if got := queryInt64(t, st, `SELECT COUNT(*) FROM events WHERE kind = 'dup_repair'`); got != 1 {
		t.Fatalf("应落一条 dup_repair 事件, got %d", got)
	}

	// 幂等：没有新的重复行时再扫不漏删、不重复计数。
	res2, err := st.RepairDuplicateRecords(1_000_300)
	if err != nil {
		t.Fatal(err)
	}
	if res2.Removed() {
		t.Fatalf("二次扫描不应再删, got %+v", res2)
	}
	if got := queryInt64(t, st, `SELECT COUNT(*) FROM sessions`); got != 5 {
		t.Fatalf("重复执行不应再改数据, got %d", got)
	}
}

// 清理必须可重跑：第一次清理之后旧 daemon 仍可能写进新的重复行（issue #12 的
// 升级场景），下一次触发必须能再收干净，并累加计数。
func TestRepairDuplicateRecordsReruns(t *testing.T) {
	st := openTestStore(t, filepath.Join(t.TempDir(), "battery.db"))
	defer st.Close()

	insert := func(startTs, endTs, startCap, endCap int64) {
		t.Helper()
		if _, err := st.InsertSession(Session{
			StartTs: startTs, EndTs: endTs, StartCap: startCap, EndCap: endCap,
			AccUAs: 1000, AvgI: 10, Valid: false, InvalidReason: "delta_lt_20",
		}); err != nil {
			t.Fatalf("InsertSession: %v", err)
		}
	}
	insert(1000, 2000, 30, 40)
	insert(1000, 2002, 30, 40)

	if res, err := st.RepairDuplicateRecords(1_000_000); err != nil || res.Sessions != 1 {
		t.Fatalf("首次清理 = %+v err=%v", res, err)
	}
	// 首次清理之后又出现的重复对（旧 daemon 仍在写库）。
	insert(5000, 6000, 20, 30)
	insert(5000, 6001, 20, 30)

	res, err := st.RepairDuplicateRecords(1_000_300)
	if err != nil {
		t.Fatal(err)
	}
	if res.Sessions != 1 {
		t.Fatalf("二次清理应再删 1 条, got %+v", res)
	}
	if got := queryInt64(t, st, `SELECT COUNT(*) FROM sessions`); got != 2 {
		t.Fatalf("两次清理后应剩 2 行, got %d", got)
	}
	info := st.DupRepairInfo()
	if info == nil || info.Sessions != 2 {
		t.Fatalf("累计删除数应累加到 2, got %+v", info)
	}
}

// 删除重复会话时同步扣回 charged_ua_total：多 daemon 各自把同一会话的积分
// 加进吞吐，保留最后写入那行后，被删行的 ua 就是多算的部分（issue #12 遗留的
// 循环当量偏高）。扣减必须与删除同事务、且重复执行不二次扣减。
func TestRepairDuplicateDeductsChargedTotal(t *testing.T) {
	st := openTestStore(t, filepath.Join(t.TempDir(), "battery.db"))
	defer st.Close()

	const total = int64(10_000_000_000)
	if err := st.KVSet(kvChargedTotal, strconv.FormatInt(total, 10)); err != nil {
		t.Fatal(err)
	}
	insert := func(endTs, ua int64) {
		t.Helper()
		if _, err := st.InsertSession(Session{
			StartTs: 1000, EndTs: endTs, StartCap: 30, EndCap: 40,
			AccUAs: ua, AvgI: 10, Valid: false, InvalidReason: "delta_lt_20",
		}); err != nil {
			t.Fatalf("InsertSession: %v", err)
		}
	}
	// 同一会话的三行（多 daemon 各结算一次）：3.6e6 µA·s = 1mAh。
	insert(2000, 3_600_000)
	insert(2002, 7_200_000)
	insert(2004, 10_800_000)

	res, err := st.RepairDuplicateRecords(1_000_000)
	if err != nil {
		t.Fatal(err)
	}
	if res.Sessions != 2 || res.ChargeUAs != 3_600_000+7_200_000 {
		t.Fatalf("清理结果 = %+v, want 2 会话 + 10.8e6 µA·s", res)
	}
	wantTotal := total - res.ChargeUAs
	if got := kvInt(st, kvChargedTotal); got != wantTotal {
		t.Fatalf("charged_ua_total = %d, want %d（扣回被删会话的充入）", got, wantTotal)
	}
	info := st.DupRepairInfo()
	if info == nil || info.ChargeMah != 3 {
		t.Fatalf("dup_repair 扣减展示 = %+v, want charge_mah=3", info)
	}
	// 二次扫描没有可删行 → 不得再扣。
	if res2, err := st.RepairDuplicateRecords(1_000_300); err != nil || res2.Removed() {
		t.Fatalf("二次扫描 = %+v err=%v", res2, err)
	}
	if got := kvInt(st, kvChargedTotal); got != wantTotal {
		t.Fatalf("二次扫描不应再扣 charged_ua_total: %d", got)
	}
}

// 扫描节流：daemon 启动（force）总是扫，其余调用点 5 分钟内不重复开写事务。
func TestDuplicatesScanDue(t *testing.T) {
	st := openTestStore(t, filepath.Join(t.TempDir(), "battery.db"))
	defer st.Close()

	if !st.DuplicatesScanDue(1_000_000, false) {
		t.Fatal("从未扫描过应允许首次扫描")
	}
	if _, err := st.RepairDuplicateRecords(1_000_000); err != nil {
		t.Fatal(err)
	}
	if st.DuplicatesScanDue(1_000_000+dupScanMinInterval-1, false) {
		t.Fatal("间隔内不应重扫")
	}
	if !st.DuplicatesScanDue(1_000_000+dupScanMinInterval, false) {
		t.Fatal("超过间隔应重扫")
	}
	if !st.DuplicatesScanDue(1_000_100, true) {
		t.Fatal("force 应跳过节流")
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

	res, err := st.RepairDuplicateRecords(1_000_000)
	if err != nil {
		t.Fatalf("RepairDuplicateRecords: %v", err)
	}
	if res.Sessions != 0 || res.Discharges != 1 {
		t.Fatalf("清理结果 = %+v, want 1 放电", res)
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
	if got := queryInt64(t, st, `SELECT v FROM kv WHERE k = '`+kvDupRemovedDischarges+`'`); got != 1 {
		t.Fatalf("dup 放电计数 = %d, want 1", got)
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

// 会话判重边界：结束电量差 1 个百分点仍是同一次，差 2 就不再合并；时刻/原因不同
// 都不合并。
func TestSameChargeSessionBoundaries(t *testing.T) {
	base := sessionSig{startTs: 1000, endTs: 2000, startCap: 30, endCap: 40, valid: false, reason: "delta_lt_20"}
	if !sameChargeSession(base, sessionSig{startTs: 1000, endTs: 2000, startCap: 30, endCap: 41, valid: false, reason: "delta_lt_20"}) {
		t.Fatal("结束电量差 1 应判为同一次会话")
	}
	if sameChargeSession(base, sessionSig{startTs: 1000, endTs: 2000, startCap: 30, endCap: 42, valid: false, reason: "delta_lt_20"}) {
		t.Fatal("结束电量差 2 不应合并")
	}
	if sameChargeSession(base, sessionSig{startTs: 1000, endTs: 2000, startCap: 30, endCap: 40, valid: true}) {
		t.Fatal("有效性不同不应合并")
	}
	if sameChargeSession(base, sessionSig{startTs: 1000, endTs: 2200, startCap: 30, endCap: 40, valid: false, reason: "delta_lt_20"}) {
		t.Fatal("结束时刻超出容差不应合并")
	}
}
