package main

import (
	"os"
	"path/filepath"
	"testing"
)

// issue #7 回归：荣耀 Magic7（HONOR PTP-AN00）内核上 charge_counter 按 mAh 报数、
// current_now 按 mA 报数，而 charge_full / charge_full_design 仍是 ABI 的 µAh。
// 旧实现把 counter 当 µAh 直通：会话隐含容量只有 5mAh 被 out_of_window 拒收，
// 电流自校准的比值又因两路同时 milli 化而相消（≈1.4）学成 ×1，估算永远 learning。
// 修复后 counter 按 charge_full 锚点归一為 µAh，会话采信，且电流刻度自动学到 ×1000。
func TestIssue7CounterMilliampDevice(t *testing.T) {
	r := newPipeRig(t)
	// charge_full 4Ah（µAh 量纲）；counter 以 mAh 报数（满充约 4000）
	writeFile(t, filepath.Join(r.fs.Base, "battery", "charge_full"), fmtNode(4_000_000))
	r.rebuildPipeline()
	if err := r.st.KVSet(kvKeyEmaUAh, "4800000"); err != nil {
		t.Fatal(err)
	}
	if err := r.st.KVSet(kvKeySamples, "3"); err != nil {
		t.Fatal(err)
	}

	cc := int64(800) // 20% × 4000mAh
	putCounter(t, r, cc)
	// 61 拍 cap 20→80：counter 每拍 +40mAh（=40000µAh），合计 2400mAh ⇒ 隐含
	// 4000mAh 落在设计容量窗口；电流原始 3000（mA）被旧锚点当 µA 直通 ⇒ 积分
	// 隐含只有约 3mAh，口径裁决应判 counter_plausible。
	for i := int64(0); i <= 60; i++ {
		r.put(20+i, 3_000, 4_100_000)
		if i > 0 {
			cc += 40
			putCounter(t, r, cc)
		}
		r.step("Charging")
	}
	r.put(80, 20_000, 4_100_000)
	for k := 0; k < 3; k++ {
		if out := r.step("Full"); out.SessionSettled {
			break
		}
	}

	sess := onlySession(t, r.st)
	if !sess.Valid {
		t.Fatalf("会话应采信，实际被拒 reason=%s", sess.InvalidReason)
	}
	if sess.CounterUAh != 2_400_000 {
		t.Fatalf("sessions.cc = %d, want 2400000（mAh 读数已归一为 µAh）", sess.CounterUAh)
	}
	// 采信 counter 口径：隐含 = 2400000×100/60 = 4000000µAh；涨幅 60 < 满充封账
	// ⇒ 按 1/10 权重并入旧 EMA（4800000），结果必须真的融合过（不能等于任一侧）。
	wantEma := (int64(4_800_000)*9 + int64(4_000_000)) / 10
	if got := kvInt(r.st, kvKeyEmaUAh); got != wantEma {
		t.Fatalf("EMA = %d, want %d（电量计隐含 4000mAh 按 1/10 融合）", got, wantEma)
	}
	// counter 归一后自校准比值 ≈800 ⇒ 吸附到 ×1000，电流随之一并纠正
	if got, ok := kvString(t, r.st, kvCurrentScale); !ok || got != "1000" {
		t.Fatalf("kv[cur_scale] = (%q,%v), want (\"1000\",true)", got, ok)
	}
	// 落库样本的 cc 必须是归一后的 µAh（中段校准/历史页都吃这个字段），电流
	// 则按新刻度回填成 3A。
	rows, err := r.st.SamplesRange(sess.StartTs, sess.EndTs)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) < 30 {
		t.Fatalf("会话样本数 = %d，过少", len(rows))
	}
	if rows[0].CounterUAh != 800_000 {
		t.Fatalf("首条样本 cc = %dµAh, want 800000（mAh 读数 ×1000）", rows[0].CounterUAh)
	}
	for _, row := range rows {
		if row.UA != 3_000_000 {
			t.Fatalf("样本 ua = %dµA, want 3000000（已按新刻度回填）", row.UA)
		}
	}
}

// issue #7 放电侧同源回归：放电差分读的是同一个 mAh 报数的 charge_counter，
// 不归一的话放出电量、samples.cc、放电差分校准都会小 1000 倍。
func TestIssue7DischargeCounterMilliampDevice(t *testing.T) {
	r := newPipeRig(t)
	writeFile(t, filepath.Join(r.fs.Base, "battery", "charge_full"), fmtNode(4_000_000))
	r.rebuildPipeline()

	cc := int64(3200) // 80% × 4000mAh
	putCounter(t, r, cc)
	r.put(80, -800, 3_900_000)
	r.step("Discharging")
	// 80%→60% 逐拍掉 1%，counter 每拍 -40mAh（差分通过 plausibleDelta）
	for i := int64(0); i < 20; i++ {
		cc -= 40
		putCounter(t, r, cc)
		r.put(79-i, -800, 3_900_000)
		r.step("Discharging")
	}
	// 转充电触发结算；放电样本应在第一拍落一条 cc=3200000 的记录
	r.put(60, 0, 3_900_000)
	r.step("Charging")

	rows, err := r.st.RecentDischarge(1)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("放电行数 = %d, want 1", len(rows))
	}
	if rows[0].UAh != 800_000 {
		t.Fatalf("放电放出电量 = %dµAh, want 800000（mAh 差分 ×1000）", rows[0].UAh)
	}
	if rows[0].Implied == nil || *rows[0].Implied != 4_000_000 {
		t.Fatalf("放电隐含 = %v, want 4000000µAh（800000×100/20）", rows[0].Implied)
	}
	var firstCC int64
	if err := r.st.db.QueryRow(`SELECT cc FROM samples ORDER BY ts ASC LIMIT 1`).Scan(&firstCC); err != nil {
		t.Fatal(err)
	}
	if firstCC != 3_200_000 {
		t.Fatalf("放电样本 cc = %dµAh, want 3200000（mAh 读数 ×1000）", firstCC)
	}
}

// 两个 Store 句柄指向同一库：迁移标记对第二个句柄可见，重复调用只乘一次。
// （守护进程与 WebUI 的 json/diag 子进程就是各自开 Store 并发触碰同一文件。）
func TestRepairCounterUnitsTwoStoreHandles(t *testing.T) {
	path := filepath.Join(t.TempDir(), "battery.db")
	st1 := openTestStore(t, path)
	t.Cleanup(func() { _ = st1.Close() })
	st2 := openTestStore(t, path)
	t.Cleanup(func() { _ = st2.Close() })

	mustExec(t, st1, `INSERT INTO samples(ts, ua, uv, cap, cc) VALUES(1, 0, 3900000, 30, 1200)`)
	if err := st1.RepairCounterUnits(1000, 1000); err != nil {
		t.Fatal(err)
	}
	if err := st2.RepairCounterUnits(1000, 1000); err != nil {
		t.Fatalf("第二个 Store 迁移应幂等返回: %v", err)
	}
	if got := queryInt64(t, st2, `SELECT cc FROM samples`); got != 1_200_000 {
		t.Fatalf("两个句柄重复迁移后 samples.cc = %d, want 1200000（只乘一次）", got)
	}
}

// 升级入口：epoch 截断 + 不确定就下次启动重试。迁移只处理 epoch 之前的旧行，
// 归一后的新行永远不被二次 ×1000（可安全重试）。
func TestRepairCounterUnitsOnUpgrade(t *testing.T) {
	writeBattery := func(root string, counter, full, cap int64) SysFS {
		battery := filepath.Join(root, "battery")
		writeFile(t, filepath.Join(battery, "charge_counter"), fmtNode(counter))
		writeFile(t, filepath.Join(battery, "charge_full"), fmtNode(full))
		writeFile(t, filepath.Join(battery, "capacity"), fmtNode(cap))
		return SysFS{Base: root, devices: filepath.Join(root, "devices")}
	}

	// mAh 口径机型：迁移历史行并落标记
	fs := writeBattery(t.TempDir(), 1874, 5_226_000, 37)
	st := openTestStore(t, filepath.Join(t.TempDir(), "battery.db"))
	t.Cleanup(func() { _ = st.Close() })
	mustExec(t, st, `INSERT INTO samples(ts, ua, uv, cap, cc) VALUES(1, 0, 3900000, 37, 1874)`)
	repairCounterUnitsOnUpgrade(st, fs)
	if got := queryInt64(t, st, `SELECT cc FROM samples`); got != 1_874_000 {
		t.Fatalf("升级迁移后 samples.cc = %d, want 1874000", got)
	}
	wantKV(t, st, kvCounterUnitRepair, "1")
	// 标记已落：后续启动即使库里出现归一后的新行也不会再乘
	mustExec(t, st, `INSERT INTO samples(ts, ua, uv, cap, cc) VALUES(2, 0, 3900000, 38, 1874000)`)
	repairCounterUnitsOnUpgrade(st, fs)
	if got := queryInt64(t, st, `SELECT cc FROM samples WHERE ts=2`); got != 1_874_000 {
		t.Fatalf("归一后的新行被二次放大成 %d", got)
	}

	// 重试（首启未完成，flag 缺失、epoch 已写）：只迁移 epoch 之前的行，
	// epoch 之后写入的归一值保持不变 —— 这是「可安全重试」的核心保证。
	st5 := openTestStore(t, filepath.Join(t.TempDir(), "battery.db"))
	t.Cleanup(func() { _ = st5.Close() })
	mustExec(t, st5, `INSERT INTO samples(ts, ua, uv, cap, cc) VALUES(1, 0, 3900000, 37, 1874)`)
	mustExec(t, st5, `INSERT INTO samples(ts, ua, uv, cap, cc) VALUES(2000, 0, 3900000, 38, 1874000)`)
	if err := st5.KVSet(kvCounterUnitEpoch, "1000"); err != nil {
		t.Fatal(err)
	}
	repairCounterUnitsOnUpgrade(st5, fs)
	if got := queryInt64(t, st5, `SELECT cc FROM samples WHERE ts=1`); got != 1_874_000 {
		t.Fatalf("旧行未迁移: %d", got)
	}
	if got := queryInt64(t, st5, `SELECT cc FROM samples WHERE ts=2000`); got != 1_874_000 {
		t.Fatalf("epoch 之后的新行被二次放大成 %d", got)
	}
	wantKV(t, st5, kvCounterUnitRepair, "1")

	// 读数缺失（charge_counter 不可读）：本次不判定，不落终态标记，下次启动重试
	partial := writeBattery(t.TempDir(), 1874, 5_226_000, 37)
	if err := os.Remove(filepath.Join(partial.Base, "battery", "charge_counter")); err != nil {
		t.Fatal(err)
	}
	st2 := openTestStore(t, filepath.Join(t.TempDir(), "battery.db"))
	t.Cleanup(func() { _ = st2.Close() })
	mustExec(t, st2, `INSERT INTO samples(ts, ua, uv, cap, cc) VALUES(1, 0, 3900000, 37, 1874)`)
	repairCounterUnitsOnUpgrade(st2, partial)
	if got := queryInt64(t, st2, `SELECT cc FROM samples`); got != 1874 {
		t.Fatalf("读数缺失时不应迁移，samples.cc = %d", got)
	}
	if _, ok := kvString(t, st2, kvCounterUnitRepair); ok {
		t.Fatal("读数缺失时应保留重试机会（不落终态标记）")
	}

	// 幅值像 mAh 但显示电量为 0（刚开机/瞬态）：本次不判定，下次启动重试
	pending := writeBattery(t.TempDir(), 20, 5_226_000, 0)
	st4 := openTestStore(t, filepath.Join(t.TempDir(), "battery.db"))
	t.Cleanup(func() { _ = st4.Close() })
	mustExec(t, st4, `INSERT INTO samples(ts, ua, uv, cap, cc) VALUES(1, 0, 3900000, 0, 20)`)
	repairCounterUnitsOnUpgrade(st4, pending)
	if got := queryInt64(t, st4, `SELECT cc FROM samples`); got != 20 {
		t.Fatalf("显示电量缺失时不应迁移，samples.cc = %d", got)
	}
	if _, ok := kvString(t, st4, kvCounterUnitRepair); ok {
		t.Fatal("量纲不确定时应保留重试机会（不落终态标记）")
	}

	// 正常 µAh 机型：只落标记，不动数据
	normalFS := writeBattery(t.TempDir(), 1_874_000, 5_226_000, 37)
	st3 := openTestStore(t, filepath.Join(t.TempDir(), "battery.db"))
	t.Cleanup(func() { _ = st3.Close() })
	mustExec(t, st3, `INSERT INTO samples(ts, ua, uv, cap, cc) VALUES(1, 0, 3900000, 37, 1874000)`)
	repairCounterUnitsOnUpgrade(st3, normalFS)
	if got := queryInt64(t, st3, `SELECT cc FROM samples`); got != 1_874_000 {
		t.Fatalf("µAh 机型样本被改写成 %d", got)
	}
	wantKV(t, st3, kvCounterUnitRepair, "1")
}

// 迁移实现本身：mAh 口径的历史 cc 数据一次性 ×1000，只动电量计专用列；重复
// 执行幂等；正常 µAh 机型（scale=1）只落标记不改数据。
func TestRepairCounterUnits(t *testing.T) {
	st := openTestStore(t, filepath.Join(t.TempDir(), "battery.db"))
	t.Cleanup(func() { _ = st.Close() })

	mustExec(t, st, `INSERT INTO samples(ts, ua, uv, cap, cc) VALUES(1, 1000000, 3900000, 30, 1200)`)
	mustExec(t, st, `INSERT INTO samples(ts, ua, uv, cap, cc) VALUES(2, 1000000, 3900000, 31, 0)`)
	// epoch 之后写入的行：已是归一值，迁移不得再乘
	mustExec(t, st, `INSERT INTO samples(ts, ua, uv, cap, cc) VALUES(2000, 1000000, 3900000, 32, 1200000)`)
	mustExec(t, st, `INSERT INTO sessions(start_ts, end_ts, start_cap, end_cap, ua, avg_i, c_rate, valid, cc)
		VALUES(1, 2, 30, 80, 1000, 0, 0, 0, 1500)`)
	mustExec(t, st, `INSERT INTO discharge(end_ts, secs, uah, start_cap, end_cap, implied)
		VALUES(3, 600, 1200, 80, 40, 5000)`)
	for _, kv := range []struct{ k, v string }{
		{kvSessStartTs, "1"}, {kvSessCounterAccUAh, "700"}, {kvSessCounterLastUAh, "1200"},
		{kvDisStartTs, "3"}, {kvDisAcc, "300"}, {kvDisLastCounterUAh, "800"},
	} {
		if err := st.KVSet(kv.k, kv.v); err != nil {
			t.Fatal(err)
		}
	}

	if err := st.RepairCounterUnits(1000, 1000); err != nil {
		t.Fatalf("RepairCounterUnits: %v", err)
	}
	if got := queryInt64(t, st, `SELECT cc FROM samples WHERE ts=1`); got != 1_200_000 {
		t.Fatalf("samples.cc = %d, want 1200000", got)
	}
	if got := queryInt64(t, st, `SELECT cc FROM samples WHERE ts=2`); got != 0 {
		t.Fatalf("cc=0 的行不应被改写，got %d", got)
	}
	if got := queryInt64(t, st, `SELECT cc FROM samples WHERE ts=2000`); got != 1_200_000 {
		t.Fatalf("epoch 之后的新行被二次放大成 %d", got)
	}
	if got := queryInt64(t, st, `SELECT cc FROM sessions`); got != 1_500_000 {
		t.Fatalf("sessions.cc = %d, want 1500000", got)
	}
	if got := queryInt64(t, st, `SELECT uah FROM discharge`); got != 1_200_000 {
		t.Fatalf("discharge.uah = %d, want 1200000", got)
	}
	if got := queryInt64(t, st, `SELECT implied FROM discharge`); got != 5_000_000 {
		t.Fatalf("discharge.implied = %d, want 5000000（与 uah 同源同步迁移）", got)
	}
	for _, kv := range []struct {
		k    string
		want int64
	}{
		{kvSessCounterAccUAh, 700_000}, {kvSessCounterLastUAh, 1_200_000},
		{kvDisAcc, 300_000}, {kvDisLastCounterUAh, 800_000},
	} {
		if got := kvInt(st, kv.k); got != kv.want {
			t.Fatalf("kv[%s] = %d, want %d", kv.k, got, kv.want)
		}
	}
	if got := queryInt64(t, st, `SELECT count(*) FROM events WHERE kind='cc_unit_repair'`); got != 1 {
		t.Fatalf("cc_unit_repair 事件数 = %d, want 1", got)
	}
	wantKV(t, st, kvCounterUnitRepair, "1")

	// 幂等：再次执行不再乘 1000
	if err := st.RepairCounterUnits(1000, 1000); err != nil {
		t.Fatal(err)
	}
	if got := queryInt64(t, st, `SELECT cc FROM samples WHERE ts=1`); got != 1_200_000 {
		t.Fatalf("重复迁移后 samples.cc = %d, want 1200000（幂等）", got)
	}

	// 正常口径机型：scale=1 只落标记，不改任何数据
	normal := openTestStore(t, filepath.Join(t.TempDir(), "battery.db"))
	t.Cleanup(func() { _ = normal.Close() })
	mustExec(t, normal, `INSERT INTO samples(ts, ua, uv, cap, cc) VALUES(1, 0, 3900000, 30, 1200000)`)
	if err := normal.RepairCounterUnits(1, 1000); err != nil {
		t.Fatal(err)
	}
	if got := queryInt64(t, normal, `SELECT cc FROM samples WHERE ts=1`); got != 1_200_000 {
		t.Fatalf("µAh 机型样本被改写成 %d", got)
	}
	wantKV(t, normal, kvCounterUnitRepair, "1")
}
