package main

import (
	"math"
	"path/filepath"
	"strconv"
	"testing"
	"time"
)

const testDesignUA = 4000000

const tickBaseTs = int64(1700000000)

type pipeRig struct {
	t   *testing.T
	fs  SysFS
	st  *Store
	est *Stable
	cur time.Time
	p   *Pipeline
}

func newPipeRig(t *testing.T) *pipeRig {
	t.Helper()
	base := t.TempDir()
	r := &pipeRig{
		t:   t,
		fs:  SysFS{Base: base, devices: filepath.Join(base, "devices")},
		cur: time.Unix(tickBaseTs, 0),
	}
	r.st = openTestStore(t, filepath.Join(t.TempDir(), "battery.db"))
	t.Cleanup(func() { _ = r.st.Close() })
	r.est = NewStable(r.st)
	r.rebuildPipeline()
	return r
}

func (r *pipeRig) rebuildPipeline() {
	r.t.Helper()
	clock := func() time.Time { return r.cur }
	r.p = NewPipeline(r.fs, r.st, r.est, testDesignUA, clock)
}

func fmtNode(v int64) string { return strconv.FormatInt(v, 10) + "\n" }

func (r *pipeRig) put(capV, iUA, vUV int64) {
	r.t.Helper()
	battery := filepath.Join(r.fs.Base, "battery")
	writeFile(r.t, filepath.Join(battery, "capacity"), fmtNode(capV))
	writeFile(r.t, filepath.Join(battery, "current_now"), fmtNode(iUA))
	writeFile(r.t, filepath.Join(battery, "temp"), "250\n")
	writeFile(r.t, filepath.Join(battery, "voltage_now"), fmtNode(vUV))
}

func (r *pipeRig) step(status string) TickOutcome {
	r.t.Helper()
	r.cur = r.cur.Add(time.Minute)
	out, err := r.p.Tick(status)
	if err != nil {
		r.t.Fatalf("Tick(%q): %v", status, err)
	}
	return out
}

func onlySession(t *testing.T, st *Store) Session {
	t.Helper()
	var s Session
	err := st.db.QueryRow(`SELECT start_ts,end_ts,start_cap,end_cap,ua,avg_i,c_rate,
		temp_min,temp_max,temp_avg,v_start,duration,valid FROM sessions ORDER BY id`).Scan(
		&s.StartTs, &s.EndTs, &s.StartCap, &s.EndCap, &s.Ua, &s.AvgI, &s.CRate,
		&s.TempMin, &s.TempMax, &s.TempAvg, &s.VStart, &s.Duration, &s.Valid)
	if err != nil {
		t.Fatalf("读取会话行: %v", err)
	}
	return s
}

func queryInt64(t *testing.T, st *Store, query string) int64 {
	t.Helper()
	var v int64
	if err := st.db.QueryRow(query).Scan(&v); err != nil {
		t.Fatalf("查询 %q: %v", query, err)
	}
	return v
}

func queryFloat64(t *testing.T, st *Store, query string) float64 {
	t.Helper()
	var v float64
	if err := st.db.QueryRow(query).Scan(&v); err != nil {
		t.Fatalf("查询 %q: %v", query, err)
	}
	return v
}

func wantKV(t *testing.T, st *Store, key, val string) {
	t.Helper()
	got, ok := kvString(t, st, key)
	if !ok || got != val {
		t.Fatalf("kv[%q] = (%q,%v), want %q", key, got, ok, val)
	}
}

func TestPipelineSealsOnceAtFullCharge(t *testing.T) {
	r := newPipeRig(t)

	settledCount := 0
	for k := int64(1); k <= 17; k++ {
		capV := int64(15) + 5*k
		r.put(capV, 6000000, 4200000)
		out := r.step("Charging")
		if out.SessionSettled {
			settledCount++
			if k != 17 {
				t.Fatalf("第 %d tick 提前封账结算", k)
			}
		}
	}
	if settledCount != 1 {
		t.Fatalf("封账结算次数 = %d, want 1", settledCount)
	}

	sess := onlySession(t, r.st)
	wantStart := tickBaseTs + 60
	if sess.StartTs != wantStart || sess.EndTs != tickBaseTs+1020 {
		t.Fatalf("时间戳 = (%d,%d), want (%d,%d)", sess.StartTs, sess.EndTs, wantStart, tickBaseTs+1020)
	}
	if sess.StartCap != 20 || sess.EndCap != 100 {
		t.Fatalf("cap 区间 = [%d,%d], want [20,100]", sess.StartCap, sess.EndCap)
	}
	if sess.Ua != 6120000000 || sess.AvgI != 6000000 || sess.Duration != 1020 {
		t.Fatalf("ua/avg_i/duration = (%d,%d,%d), want (6120000000,6000000,1020)",
			sess.Ua, sess.AvgI, sess.Duration)
	}
	if math.Abs(sess.CRate-1.5) > 1e-9 {
		t.Fatalf("c_rate = %v, want 1.5", sess.CRate)
	}
	if sess.TempMin != 25 || sess.TempMax != 25 || sess.TempAvg != 25 {
		t.Fatalf("temp 三元 = (%d,%d,%d), want 全为 25", sess.TempMin, sess.TempMax, sess.TempAvg)
	}
	if sess.VStart != 4200000 || !sess.Valid {
		t.Fatalf("v_start=%d valid=%v, want 4200000/true", sess.VStart, sess.Valid)
	}
	if n := countRows(t, r.st, "sessions"); n != 1 {
		t.Fatalf("sessions 行数 = %d, want 1", n)
	}
	if n := countRows(t, r.st, "estimates"); n != 1 {
		t.Fatalf("estimates 行数 = %d, want 1", n)
	}
	if mah := queryInt64(t, r.st, `SELECT mah FROM estimates`); mah != 2125000 {
		t.Fatalf("估算值 = %d, want 2125000", mah)
	}
	if estTs := queryInt64(t, r.st, `SELECT ts FROM estimates`); estTs != tickBaseTs+1020 {
		t.Fatalf("estimates ts = %d, want %d", estTs, tickBaseTs+1020)
	}
	wantKV(t, r.st, "charged_ua_total", "6120000000")

	for i := 0; i < 3; i++ {
		r.put(100, 6000000, 4200000)
		if out := r.step("Charging"); out.SessionSettled {
			t.Fatal("封账后浮充不应再次结算")
		}
	}
	if n := countRows(t, r.st, "sessions"); n != 1 {
		t.Fatalf("浮充阶段 sessions 行数 = %d, want 1", n)
	}
	wantKV(t, r.st, "charged_ua_total", "7200000000")

	r.put(100, 15000, 4300000)
	if out := r.step("Discharging"); out.SessionSettled {
		t.Fatal("封账会话拔出时不应二次结算")
	}
	if n := countRows(t, r.st, "sessions"); n != 1 {
		t.Fatalf("拔出后 sessions 行数 = %d, want 1", n)
	}
	if n := countRows(t, r.st, "rest_points"); n != 0 {
		t.Fatalf("单次低电流 tick 不应记录静息点, rows = %d", n)
	}
	wantKV(t, r.st, "sess_active", "0")

	r.put(99, 6000000, 4200000)
	r.step("Charging")
	r.put(100, 6000000, 4200000)
	if out := r.step("Charging"); !out.SessionSettled {
		t.Fatal("重新插电后满电应再次封账结算")
	}
	if n := countRows(t, r.st, "sessions"); n != 2 {
		t.Fatalf("复插满电后 sessions 行数 = %d, want 2", n)
	}
}

func TestPipelineSettlesOnUnplugOnce(t *testing.T) {
	r := newPipeRig(t)

	for _, capV := range []int64{10, 22, 34, 46, 58, 70} {
		r.put(capV, 12000000, 4200000)
		if out := r.step("Charging"); out.SessionSettled {
			t.Fatal("未拔出且未满电不应结算")
		}
	}

	r.put(70, 500000, 4300000)
	out := r.step("Discharging")
	if !out.SessionSettled {
		t.Fatal("拔出应触发结算")
	}

	sess := onlySession(t, r.st)
	if sess.StartTs != tickBaseTs+60 || sess.EndTs != tickBaseTs+420 {
		t.Fatalf("时间戳 = (%d,%d), want (%d,%d)", sess.StartTs, sess.EndTs,
			tickBaseTs+60, tickBaseTs+420)
	}
	if sess.StartCap != 10 || sess.EndCap != 70 {
		t.Fatalf("cap 区间 = [%d,%d], want [10,70]", sess.StartCap, sess.EndCap)
	}
	if sess.Ua != 4320000000 || sess.AvgI != 12000000 || sess.Duration != 360 {
		t.Fatalf("ua/avg_i/duration = (%d,%d,%d), want (4320000000,12000000,360)",
			sess.Ua, sess.AvgI, sess.Duration)
	}
	if math.Abs(sess.CRate-3.0) > 1e-9 {
		t.Fatalf("c_rate = %v, want 3.0", sess.CRate)
	}
	if sess.TempMin != 25 || sess.TempMax != 25 || sess.TempAvg != 25 {
		t.Fatalf("temp 三元 = (%d,%d,%d), want 全为 25", sess.TempMin, sess.TempMax, sess.TempAvg)
	}
	if sess.VStart != 4200000 || !sess.Valid {
		t.Fatalf("v_start=%d valid=%v, want 4200000/true", sess.VStart, sess.Valid)
	}
	if n := countRows(t, r.st, "estimates"); n != 1 {
		t.Fatalf("estimates 行数 = %d, want 1", n)
	}
	if mah := queryInt64(t, r.st, `SELECT mah FROM estimates`); mah != 2000000 {
		t.Fatalf("估算值 = %d, want 2000000(窗口下界恰通过)", mah)
	}
	wantKV(t, r.st, "charged_ua_total", "4320000000")
	if n := countRows(t, r.st, "rest_points"); n != 0 {
		t.Fatalf("放电大电流不应记录静息点, rows = %d", n)
	}

	r.put(69, 500000, 4310000)
	if out := r.step("Discharging"); out.SessionSettled {
		t.Fatal("无活动会话的第二次拔出 tick 不应结算")
	}
	if n := countRows(t, r.st, "sessions"); n != 1 {
		t.Fatalf("sessions 行数 = %d, want 1", n)
	}
}

func TestPipelineRestoresSessionAcrossRestart(t *testing.T) {
	r := newPipeRig(t)

	for _, capV := range []int64{10, 22, 34} {
		r.put(capV, 12000000, 4200000)
		r.step("Charging")
	}
	wantKV(t, r.st, "sess_active", "1")
	wantKV(t, r.st, "sess_acc_uas", "2160000000")
	wantKV(t, r.st, "sess_start_cap", "10")
	wantKV(t, r.st, "sess_ticks", "3")
	wantKV(t, r.st, "sess_last_cap", "34")

	r.rebuildPipeline()

	for _, capV := range []int64{46, 58, 70} {
		r.put(capV, 12000000, 4200000)
		if out := r.step("Charging"); out.SessionSettled {
			t.Fatal("恢复后的会话不应提前结算")
		}
	}
	r.put(70, 500000, 4300000)
	if out := r.step("Discharging"); !out.SessionSettled {
		t.Fatal("跨重启续算后拔出应结算")
	}

	if n := countRows(t, r.st, "sessions"); n != 1 {
		t.Fatalf("两段采样应合并为一行会话, rows = %d", n)
	}
	sess := onlySession(t, r.st)
	if sess.StartTs != tickBaseTs+60 || sess.EndTs != tickBaseTs+420 {
		t.Fatalf("时间戳 = (%d,%d), want (%d,%d)", sess.StartTs, sess.EndTs,
			tickBaseTs+60, tickBaseTs+420)
	}
	if sess.StartCap != 10 || sess.EndCap != 70 {
		t.Fatalf("cap 区间 = [%d,%d], want [10,70]", sess.StartCap, sess.EndCap)
	}
	if sess.Ua != 4320000000 || sess.AvgI != 12000000 || sess.Duration != 360 {
		t.Fatalf("ua/avg_i/duration = (%d,%d,%d), want 两段合计 (4320000000,12000000,360)",
			sess.Ua, sess.AvgI, sess.Duration)
	}
	if !sess.Valid {
		t.Fatal("恢复续算的会话应为有效")
	}
	if mah := queryInt64(t, r.st, `SELECT mah FROM estimates`); mah != 2000000 {
		t.Fatalf("估算值 = %d, want 2000000", mah)
	}
	wantKV(t, r.st, "charged_ua_total", "4320000000")
	wantKV(t, r.st, "sess_active", "0")
}

func TestPipelineRestOCVThreeTicksAndDedup(t *testing.T) {
	r := newPipeRig(t)

	type restRow struct {
		ts, uv, cap int64
	}
	loadRestPoints := func() []restRow {
		t.Helper()
		rows, err := r.st.db.Query(`SELECT ts, uv, cap FROM rest_points ORDER BY ts`)
		if err != nil {
			t.Fatalf("查询 rest_points: %v", err)
		}
		defer rows.Close()
		var out []restRow
		for rows.Next() {
			var row restRow
			if err := rows.Scan(&row.ts, &row.uv, &row.cap); err != nil {
				t.Fatalf("扫描 rest_points: %v", err)
			}
			out = append(out, row)
		}
		if err := rows.Err(); err != nil {
			t.Fatalf("遍历 rest_points: %v", err)
		}
		return out
	}

	lowTick := func(capV, vUV int64) {
		r.put(capV, 15000, vUV)
		r.step("Discharging")
	}

	tsOfNext := func() int64 { return r.cur.Unix() + 60 }

	lowTick(80, 4100000)
	lowTick(80, 4100000)
	firstTS := tsOfNext()
	lowTick(80, 4100000)
	rows := loadRestPoints()
	if len(rows) != 1 {
		t.Fatalf("连续三 tick 静息应记 1 条, rows = %d", len(rows))
	}
	if rows[0].ts != firstTS || rows[0].uv != 4100000 || rows[0].cap != 80 {
		t.Fatalf("首条静息点 = %+v, want {%d 4100000 80}", rows[0], firstTS)
	}

	lowTick(80, 4100000)
	lowTick(80, 4100000)
	if rows := loadRestPoints(); len(rows) != 1 {
		t.Fatalf("数值不变应去重, rows = %d", len(rows))
	}

	secondTS := tsOfNext()
	lowTick(79, 4100000)
	if rows := loadRestPoints(); len(rows) != 2 {
		t.Fatalf("cap 变化 ≥1 应再记一条, rows = %d", len(rows))
	}
	lowTick(79, 4100000)
	if rows := loadRestPoints(); len(rows) != 2 {
		t.Fatalf("cap 与电压均未变应去重, rows = %d", len(rows))
	}

	thirdTS := tsOfNext()
	lowTick(79, 4094000)
	rows = loadRestPoints()
	if len(rows) != 3 {
		t.Fatalf("电压漂移 >5000µV 应再记一条, rows = %d", len(rows))
	}
	if rows[1].ts != secondTS || rows[1].uv != 4100000 || rows[1].cap != 79 {
		t.Fatalf("第二条静息点 = %+v, want {%d 4100000 79}", rows[1], secondTS)
	}
	if rows[2].ts != thirdTS || rows[2].uv != 4094000 || rows[2].cap != 79 {
		t.Fatalf("第三条静息点 = %+v, want {%d 4094000 79}", rows[2], thirdTS)
	}

	lowTickBigCurrent := func(capV, iRaw, vUV int64) {
		r.put(capV, iRaw, vUV)
		r.step("Discharging")
	}
	lowTickBigCurrent(78, 25000, 4094000)
	lowTick(77, 4090000)
	lowTick(77, 4090000)
	if rows := loadRestPoints(); len(rows) != 3 {
		t.Fatalf("大电流打断三连计数后不足三 tick 不应记录, rows = %d", len(rows))
	}
}

func TestPipelineResistanceSyntheticSlopeWithinFivePercent(t *testing.T) {
	currents := []int64{1000000, 2000000, 3000000}
	// 物理正确的充电关系：V = OCV + I·R，斜率 dV/dI 为正
	voltOf := func(iUA int64) int64 { return 4200000 + iUA/20 }
	runTicks := func(r *pipeRig, n int) {
		for k := 0; k < n; k++ {
			i := currents[k%len(currents)]
			r.put(50, i, voltOf(i))
			r.step("Charging")
		}
	}

	t.Run("不足20样本不回归", func(t *testing.T) {
		r := newPipeRig(t)
		runTicks(r, 19)
		if n := countRows(t, r.st, "resistance"); n != 0 {
			t.Fatalf("不足 20 样本不应回归, rows = %d", n)
		}
		if _, ok := kvString(t, r.st, "r_ema_mo"); ok {
			t.Fatal("未回归不应写 r_ema_mo")
		}
	})

	t.Run("恒流零方差跳过回归", func(t *testing.T) {
		r := newPipeRig(t)
		for k := 0; k < 24; k++ {
			r.put(50, 2000000, 4100000)
			r.step("Charging")
		}
		if n := countRows(t, r.st, "resistance"); n != 0 {
			t.Fatalf("std(I)=0 应跳过回归, rows = %d", n)
		}
		if _, ok := kvString(t, r.st, "r_ema_mo"); ok {
			t.Fatal("未产生有效内阻不应写 r_ema_mo")
		}
	})

	t.Run("合成斜率还原±5%", func(t *testing.T) {
		r := newPipeRig(t)
		runTicks(r, 24)
		n := countRows(t, r.st, "resistance")
		if n < 5 {
			t.Fatalf("样本达标后每 tick 都应尝试回归, rows = %d, want ≥5", n)
		}
		mo := queryFloat64(t, r.st, `SELECT mo FROM resistance ORDER BY ts DESC LIMIT 1`)
		if math.Abs(mo-50) > 50*0.05 {
			t.Fatalf("还原内阻 = %.4f mΩ, 超出 50mΩ ±5%% 容差", mo)
		}
		kvVal, ok := kvString(t, r.st, "r_ema_mo")
		if !ok {
			t.Fatal("回归成功应写 kv.r_ema_mo")
		}
		ema, err := strconv.ParseFloat(kvVal, 64)
		if err != nil {
			t.Fatalf("kv.r_ema_mo = %q 不是数字: %v", kvVal, err)
		}
		if ema <= 0 || math.Abs(ema-mo) > 1e-9 {
			t.Fatalf("EMA 值 = %.4f, 与最新内阻 %.4f 不一致(稳态序列应收敛到新值)", ema, mo)
		}
	})
}

func TestPipelineChargingTickWritesSampleRow(t *testing.T) {
	r := newPipeRig(t)

	r.put(50, 1200000, 4200000)
	r.step("Charging")

	n := queryInt64(t, r.st, `SELECT COUNT(*) FROM samples`)
	if n != 1 {
		t.Fatalf("samples 行数 = %d, want 1", n)
	}
	var ts, ua, uv, capVal int64
	if err := r.st.db.QueryRow(`SELECT ts, ua, uv, cap FROM samples`).Scan(&ts, &ua, &uv, &capVal); err != nil {
		t.Fatalf("读取样本行: %v", err)
	}
	wantTs := tickBaseTs + 60
	if ts != wantTs || ua != 1200000 || uv != 4200000 || capVal != 50 {
		t.Fatalf("样本行 = (%d,%d,%d,%d), want (%d,1200000,4200000,50)", ts, ua, uv, capVal, wantTs)
	}
}

func TestPipelineChargingSampleGuards(t *testing.T) {
	r := newPipeRig(t)
	battery := filepath.Join(r.fs.Base, "battery")

	writeFile(r.t, filepath.Join(battery, "capacity"), fmtNode(50))
	writeFile(r.t, filepath.Join(battery, "current_now"), fmtNode(1200000))
	writeFile(r.t, filepath.Join(battery, "temp"), "250\n")
	r.step("Charging")
	if n, _ := r.st.CountSamples(); n != 0 {
		t.Fatalf("voltage 缺失(vUV=0)时 samples 行数 = %d, want 0", n)
	}

	r.put(0, 1200000, 4200000)
	r.step("Charging")
	if n, _ := r.st.CountSamples(); n != 0 {
		t.Fatalf("capacity=0 时 samples 行数 = %d, want 0", n)
	}

	r.put(50, 1200000, 4200000)
	r.step("Charging")
	if n, _ := r.st.CountSamples(); n != 1 {
		t.Fatalf("uv/cap 恢复正常应落 1 行, rows = %d", n)
	}
}

func TestPipelineChargingSamplesContinueAfterSeal(t *testing.T) {
	r := newPipeRig(t)

	r.put(100, 1200000, 4200000)
	if out := r.step("Charging"); !out.SessionSettled {
		t.Fatal("cap=100 应触发封账结算")
	}
	wantTS := tickBaseTs + 60
	var ts int64
	if err := r.st.db.QueryRow(`SELECT ts FROM samples`).Scan(&ts); err != nil {
		t.Fatalf("封账 tick 应落样本行: %v", err)
	}
	if ts != wantTS {
		t.Fatalf("样本 ts = %d, want %d", ts, wantTS)
	}

	r.put(100, 1500000, 4210000)
	r.step("Charging")
	if out := r.step("Charging"); out.SessionSettled {
		t.Fatal("封账后浮充不应再次结算")
	}
	rows := queryInt64(t, r.st, `SELECT COUNT(*) FROM samples`)
	if rows != 3 {
		t.Fatalf("samples 行数 = %d, want 3(密封后两个浮充 tick 也应各落一行)", rows)
	}
}

func TestPipelineRejectedSessionRecorded(t *testing.T) {
	r := newPipeRig(t)

	r.put(30, 6000000, 4200000)
	r.step("Charging")
	r.put(35, 6000000, 4200000)
	r.step("Charging")
	r.put(35, 500000, 4300000)
	if out := r.step("Discharging"); !out.SessionSettled {
		t.Fatal("拒绝结算仍是结算事件, SessionSettled 应为 true")
	}

	if n := countRows(t, r.st, "sessions"); n != 1 {
		t.Fatalf("被拒会话也应落库, rows = %d", n)
	}
	sess := onlySession(t, r.st)
	if sess.Valid {
		t.Fatal("delta<20 会话应为 valid=0")
	}
	if n := countRows(t, r.st, "estimates"); n != 0 {
		t.Fatalf("被拒会话不应写 estimates, rows = %d", n)
	}
	var evKind string
	if err := r.st.db.QueryRow(`SELECT kind FROM events`).Scan(&evKind); err != nil {
		t.Fatalf("读取事件: %v", err)
	}
	if evKind != "delta_lt_20" {
		t.Fatalf("event.kind = %q, want delta_lt_20", evKind)
	}
	wantKV(t, r.st, "sess_active", "0")
}
