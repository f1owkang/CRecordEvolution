package main

import (
	"database/sql"
	"math"
	"path/filepath"
	"testing"
)

func openTestStore(t *testing.T, path string) *Store {
	t.Helper()
	s, err := OpenStore(path)
	if err != nil {
		t.Fatalf("OpenStore(%q): %v", path, err)
	}
	return s
}

func mustExec(t *testing.T, s *Store, query string) {
	t.Helper()
	if _, err := s.db.Exec(query); err != nil {
		t.Fatalf("执行 %q: %v", query, err)
	}
}

func countRows(t *testing.T, s *Store, table string) int {
	t.Helper()
	var n int
	if err := s.db.QueryRow("SELECT COUNT(*) FROM " + table).Scan(&n); err != nil {
		t.Fatalf("COUNT %s: %v", table, err)
	}
	return n
}

func TestOpenStoreMigrateTwiceNoError(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "battery.db")
	for i := 0; i < 2; i++ {
		s := openTestStore(t, dbPath)
		if err := s.Close(); err != nil {
			t.Fatalf("第 %d 次 Close: %v", i+1, err)
		}
	}
}

func TestKVRoundTrip(t *testing.T) {
	s := openTestStore(t, filepath.Join(t.TempDir(), "battery.db"))
	defer func() { _ = s.Close() }()

	if _, ok := s.KVGet("missing"); ok {
		t.Fatal("不存在的键不应命中")
	}

	if err := s.KVSet("ema_ua", "1234567"); err != nil {
		t.Fatalf("KVSet: %v", err)
	}
	if v, ok := s.KVGet("ema_ua"); !ok || v != "1234567" {
		t.Fatalf("KVGet = (%q,%v), want (1234567,true)", v, ok)
	}

	if err := s.KVSet("ema_ua", "42"); err != nil {
		t.Fatalf("KVSet 覆盖写: %v", err)
	}
	if v, ok := s.KVGet("ema_ua"); !ok || v != "42" {
		t.Fatalf("覆盖后 KVGet = (%q,%v), want (42,true)", v, ok)
	}
}

func TestInsertSessionPersists(t *testing.T) {
	s := openTestStore(t, filepath.Join(t.TempDir(), "battery.db"))
	defer func() { _ = s.Close() }()

	in := Session{
		StartTs: 1000, EndTs: 2000,
		StartCap: 20, EndCap: 100,
		AccUAs: 3000000, AvgI: 1500000,
		CRate:   0.75,
		TempMin: 25, TempMax: 33, TempAvg: 29,
		VStart: 4200000, Duration: 1000,
		Valid: true,
	}
	id, err := s.InsertSession(in)
	if err != nil {
		t.Fatalf("InsertSession: %v", err)
	}
	if id <= 0 {
		t.Fatalf("InsertSession 返回 id = %d, 应为正数", id)
	}

	var got Session
	var valid int64
	err = s.db.QueryRow(
		`SELECT start_ts,end_ts,start_cap,end_cap,ua,avg_i,c_rate,temp_min,temp_max,temp_avg,v_start,duration,valid
		 FROM sessions WHERE id = ?`, id,
	).Scan(&got.StartTs, &got.EndTs, &got.StartCap, &got.EndCap, &got.AccUAs, &got.AvgI,
		&got.CRate, &got.TempMin, &got.TempMax, &got.TempAvg, &got.VStart, &got.Duration, &valid)
	if err != nil {
		t.Fatalf("读回会话: %v", err)
	}
	got.Valid = valid == 1
	if got != in {
		t.Fatalf("会话读回不一致:\n want %+v\n got  %+v", in, got)
	}
}

func TestRecentEstimatesOrderAndLimit(t *testing.T) {
	s := openTestStore(t, filepath.Join(t.TempDir(), "battery.db"))
	defer func() { _ = s.Close() }()

	inserts := []struct {
		ts  int64
		mah int64
	}{{300, 30}, {100, 10}, {200, 20}}
	for _, it := range inserts {
		if err := s.InsertEstimate(it.ts, it.mah); err != nil {
			t.Fatalf("InsertEstimate(ts=%d): %v", it.ts, err)
		}
	}

	got, err := s.RecentEstimates(2)
	if err != nil {
		t.Fatalf("RecentEstimates: %v", err)
	}
	want := []TsVal{{TS: 300, V: 30}, {TS: 200, V: 20}}
	if len(got) != len(want) {
		t.Fatalf("RecentEstimates(2) 长度 = %d, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("RecentEstimates(2)[%d] = %+v, want %+v", i, got[i], want[i])
		}
	}
}

func TestSampleRoundtripAndPrune(t *testing.T) {
	dir := t.TempDir()
	st, err := OpenStore(filepath.Join(dir, "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	if err := st.InsertSample(1000, 500000, 3800000, 50, 0); err != nil {
		t.Fatal(err)
	}
	if err := st.InsertSample(2000, 600000, 3850000, 55, 0); err != nil {
		t.Fatal(err)
	}
	n, err := st.CountSamples()
	if err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Fatalf("CountSamples=%d, want 2", n)
	}
	if err := st.PruneBefore(1500); err != nil {
		t.Fatal(err)
	}
	if n, _ = st.CountSamples(); n != 1 {
		t.Fatalf("after prune n=%d, want 1", n)
	}
}

// samples.cc 列往返 + 旧库补列迁移：v1.3.3 前的库没有该列，OpenStore 应幂等
// 补列，历史行 cc=0（有电量计时消费方跳过，仅整批无 cc 才回退积分）。
func TestSamplesCounterColumnRoundtripAndMigration(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "t.db")
	st, err := OpenStore(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.InsertSample(1000, 500000, 3800000, 50, 1234567); err != nil {
		t.Fatal(err)
	}
	rows, err := st.SamplesRange(0, 2000)
	if err != nil || len(rows) != 1 {
		t.Fatalf("SamplesRange 行数 = %d err=%v", len(rows), err)
	}
	if rows[0].CounterUAh != 1234567 {
		t.Fatalf("cc 往返 = %d, want 1234567", rows[0].CounterUAh)
	}
	// 模拟旧库：删列 → 重新 OpenStore 触发迁移
	if _, err := st.db.Exec(`ALTER TABLE samples DROP COLUMN cc`); err != nil {
		t.Skipf("当前 SQLite 不支持 DROP COLUMN，跳过迁移用例: %v", err)
	}
	_ = st.Close()
	st2, err := OpenStore(path)
	if err != nil {
		t.Fatalf("旧库重开应自动补列: %v", err)
	}
	defer st2.Close()
	rows2, err := st2.SamplesRange(0, 2000)
	if err != nil || len(rows2) != 1 {
		t.Fatalf("迁移后 SamplesRange 行数 = %d err=%v", len(rows2), err)
	}
	if rows2[0].CounterUAh != 0 {
		t.Fatalf("补列后历史行 cc 应为 0，got %d", rows2[0].CounterUAh)
	}
}

func TestPruneBeforeStrictLessThanBoundary(t *testing.T) {
	s := openTestStore(t, filepath.Join(t.TempDir(), "battery.db"))
	defer func() { _ = s.Close() }()

	if _, err := s.InsertSession(Session{StartTs: 1, EndTs: 899, Valid: true}); err != nil {
		t.Fatalf("InsertSession(end=899): %v", err)
	}
	if _, err := s.InsertSession(Session{StartTs: 2, EndTs: 900, Valid: true}); err != nil {
		t.Fatalf("InsertSession(end=900): %v", err)
	}
	mustExec(t, s, `INSERT INTO estimates(ts,mah) VALUES(899,11),(900,22)`)
	mustExec(t, s, `INSERT INTO resistance(ts,mo) VALUES(899,1.5),(900,2.5)`)
	mustExec(t, s, `INSERT INTO rest_points(ts,uv,cap) VALUES(899,4000000,50),(900,4100000,60)`)
	mustExec(t, s, `INSERT INTO events(ts,kind,detail) VALUES(899,'k','a'),(900,'k','b')`)

	if err := s.PruneBefore(900); err != nil {
		t.Fatalf("PruneBefore(900): %v", err)
	}

	for _, table := range []string{"sessions", "estimates", "resistance", "rest_points", "events"} {
		if n := countRows(t, s, table); n != 1 {
			t.Fatalf("%s 剩余行数 = %d, want 1(cutoff 恰等的行必须保留)", table, n)
		}
	}

	var endTs int64
	if err := s.db.QueryRow(`SELECT end_ts FROM sessions`).Scan(&endTs); err != nil || endTs != 900 {
		t.Fatalf("sessions 留下的 end_ts = %d (err=%v), want 900", endTs, err)
	}
	var ts int64
	if err := s.db.QueryRow(`SELECT ts FROM estimates`).Scan(&ts); err != nil || ts != 900 {
		t.Fatalf("estimates 留下的 ts = %d (err=%v), want 900", ts, err)
	}
}

func TestRecentSessionsAndRestPoints(t *testing.T) {
	dir := t.TempDir()
	st, err := OpenStore(filepath.Join(dir, "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	_, err = st.InsertSession(Session{StartTs: 100, EndTs: 200, StartCap: 20, EndCap: 80,
		AccUAs: 2_160_000_000, Valid: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = st.InsertSession(Session{StartTs: 300, EndTs: 400, Valid: false}); err != nil {
		t.Fatal(err)
	}
	ss, err := st.RecentSessions(10)
	if err != nil || len(ss) != 2 {
		t.Fatalf("sessions=%d err=%v", len(ss), err)
	}
	if ss[0].EndTs != 400 || ss[0].Valid {
		t.Fatalf("倒序/valid 字段错误: %+v", ss[0])
	}
	if err = st.InsertRestPoint(500, 3_900_000, 60); err != nil {
		t.Fatal(err)
	}
	rps, err := st.RecentRestPoints(10)
	if err != nil || len(rps) != 1 || rps[0].UV != 3_900_000 || rps[0].Cap != 60 {
		t.Fatalf("rest=%+v err=%v", rps, err)
	}
}

func TestSamplesRangeInclusiveAscending(t *testing.T) {
	s := openTestStore(t, filepath.Join(t.TempDir(), "battery.db"))
	defer func() { _ = s.Close() }()

	for ts := int64(100); ts <= 500; ts += 100 {
		if err := s.InsertSample(ts, 100_000+ts, 4_000_000+ts, int64(ts)/100, 0); err != nil {
			t.Fatalf("InsertSample(%d): %v", ts, err)
		}
	}
	got, err := s.SamplesRange(200, 400)
	if err != nil {
		t.Fatalf("SamplesRange: %v", err)
	}
	want := []SampleRow{
		{TS: 200, UA: 100_000 + 200, UV: 4_000_000 + 200, Cap: 2},
		{TS: 300, UA: 100_000 + 300, UV: 4_000_000 + 300, Cap: 3},
		{TS: 400, UA: 100_000 + 400, UV: 4_000_000 + 400, Cap: 4},
	}
	if len(got) != len(want) {
		t.Fatalf("SamplesRange(200,400) 长度 = %d, want %d(端点含)", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("SamplesRange[%d] = %+v, want %+v", i, got[i], want[i])
		}
	}
	if empty, err := s.SamplesRange(501, 600); err != nil || len(empty) != 0 {
		t.Fatalf("空区间应返回空切片且无错: (%v, %v)", empty, err)
	}
}

// legacySessionsDDL 为不含 invalid_reason 列的旧版 sessions 表结构，
// 用于构造升级前旧库、验证 OpenStore 的在线迁移。
const legacySessionsDDL = `
CREATE TABLE IF NOT EXISTS sessions(
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  start_ts INTEGER NOT NULL, end_ts INTEGER NOT NULL,
  start_cap INTEGER, end_cap INTEGER,
  ua INTEGER, avg_i INTEGER, c_rate REAL,
  temp_min INTEGER, temp_max INTEGER, temp_avg INTEGER,
  v_start INTEGER, duration INTEGER,
  valid INTEGER NOT NULL);`

func TestStoreMigratesLegacySessionsInvalidReason(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "battery.db")

	legacy, err := sql.Open("sqlite", "file:"+filepath.ToSlash(dbPath)+"?_busy_timeout=5000")
	if err != nil {
		t.Fatalf("打开旧库: %v", err)
	}
	if _, err := legacy.Exec(legacySessionsDDL); err != nil {
		t.Fatalf("建旧版表: %v", err)
	}
	if err := legacy.Close(); err != nil {
		t.Fatalf("关闭旧库: %v", err)
	}

	s := openTestStore(t, dbPath)
	defer func() { _ = s.Close() }()

	if _, err := s.InsertSession(Session{StartTs: 100, EndTs: 200, Valid: false, InvalidReason: "delta_lt_20"}); err != nil {
		t.Fatalf("InsertSession: %v", err)
	}
	if _, err := s.InsertSession(Session{StartTs: 300, EndTs: 400, Valid: true}); err != nil {
		t.Fatalf("InsertSession: %v", err)
	}

	got, err := s.RecentSessions(10)
	if err != nil {
		t.Fatalf("RecentSessions: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("RecentSessions 行数 = %d, want 2", len(got))
	}
	// RecentSessions 按 end_ts 倒序：最新（valid）在前，被拒在后
	if !got[0].Valid || got[0].InvalidReason != "" {
		t.Fatalf("有效会话原因应为空, got valid=%v reason=%q", got[0].Valid, got[0].InvalidReason)
	}
	if got[1].Valid || got[1].InvalidReason != "delta_lt_20" {
		t.Fatalf("被拒会话应为 valid=0 且带原因, got valid=%v reason=%q", got[1].Valid, got[1].InvalidReason)
	}
}

func TestCCCTRoundtripAndPrune(t *testing.T) {
	s := openTestStore(t, filepath.Join(t.TempDir(), "battery.db"))
	defer func() { _ = s.Close() }()

	if err := s.InsertCCCT(100, 3_900_000, 4_000_000, 960); err != nil {
		t.Fatalf("InsertCCCT: %v", err)
	}
	if err := s.InsertCCCT(200, 3_910_000, 4_010_000, 480); err != nil {
		t.Fatalf("InsertCCCT: %v", err)
	}
	got, err := s.RecentCCCT(1)
	if err != nil {
		t.Fatalf("RecentCCCT: %v", err)
	}
	want := (CcctRow{TS: 200, VwLo: 3_910_000, VwHi: 4_010_000, Secs: 480})
	if len(got) != 1 || got[0] != want {
		t.Fatalf("RecentCCCT(1) = %+v, want %v", got, want)
	}

	if err := s.PruneBefore(150); err != nil {
		t.Fatalf("PruneBefore: %v", err)
	}
	if n := countRows(t, s, "ccct"); n != 1 {
		t.Fatalf("清理后 ccct 行数 = %d, want 1", n)
	}
}

// sessions.cc 为会话期电量计差分 µAh：InsertSession 写入后必须能由
// RecentSessions / SessionsSince 原样回读，供历史页按电量计口径显示估算。
func TestSessionCounterRoundtrip(t *testing.T) {
	s := openTestStore(t, filepath.Join(t.TempDir(), "battery.db"))
	defer func() { _ = s.Close() }()

	in := Session{StartTs: 1000, EndTs: 2000, StartCap: 20, EndCap: 80,
		AccUAs: 36_000_000, CounterUAh: 2_500_000, AvgI: 2_000_000, Valid: true}
	if _, err := s.InsertSession(in); err != nil {
		t.Fatalf("InsertSession: %v", err)
	}
	for name, rows := range map[string]func() ([]Session, error){
		"RecentSessions": func() ([]Session, error) { return s.RecentSessions(1) },
		"SessionsSince":  func() ([]Session, error) { return s.SessionsSince(1) },
	} {
		got, err := rows()
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if len(got) != 1 || got[0].CounterUAh != 2_500_000 || got[0].AvgI != 2_000_000 {
			t.Fatalf("%s 回读 cc/avg_i = %+v, want CounterUAh=2500000 AvgI=2000000", name, got)
		}
	}
	if n, err := s.CountSessions(); err != nil || n != 1 {
		t.Fatalf("CountSessions = (%d,%v), want (1,nil)", n, err)
	}
}

// 缺 cc 列的库：OpenStore 需幂等补列，历史行默认 0（cc=0 时展示回退电流积分口径）。
func TestSessionCounterMigration(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "battery.db")
	db, err := sql.Open("sqlite", "file:"+filepath.ToSlash(dbPath)+"?_busy_timeout=5000")
	if err != nil {
		t.Fatalf("打开旧库: %v", err)
	}
	if _, err := db.Exec(`CREATE TABLE sessions(
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		start_ts INTEGER NOT NULL, end_ts INTEGER NOT NULL,
		start_cap INTEGER, end_cap INTEGER,
		ua INTEGER, avg_i INTEGER, c_rate REAL,
		temp_min INTEGER, temp_max INTEGER, temp_avg INTEGER,
		v_start INTEGER, duration INTEGER,
		valid INTEGER NOT NULL, invalid_reason TEXT)`); err != nil {
		t.Fatalf("建旧表: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO sessions(start_ts,end_ts,start_cap,end_cap,ua,avg_i,c_rate,
		temp_min,temp_max,temp_avg,v_start,duration,valid)
		VALUES(100,200,20,80,1000000,800000,0.2,25,30,27,4000000,1000,1)`); err != nil {
		t.Fatalf("插旧行: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("关闭旧库: %v", err)
	}

	s := openTestStore(t, dbPath)
	defer func() { _ = s.Close() }()
	got, err := s.RecentSessions(1)
	if err != nil {
		t.Fatalf("迁移后 RecentSessions: %v", err)
	}
	if len(got) != 1 || !got[0].Valid || got[0].CounterUAh != 0 || got[0].AccUAs != 1_000_000 {
		t.Fatalf("迁移后旧行 = %+v, want valid 且 cc=0、ua=1000000", got)
	}
	// 补列后新写入正常
	if _, err := s.InsertSession(Session{StartTs: 300, EndTs: 400, StartCap: 20,
		EndCap: 80, CounterUAh: 2_000_000, Valid: true}); err != nil {
		t.Fatalf("迁移后 InsertSession: %v", err)
	}
	got, err = s.RecentSessions(2)
	if err != nil {
		t.Fatalf("RecentSessions(2): %v", err)
	}
	if len(got) != 2 || got[0].CounterUAh != 2_000_000 {
		t.Fatalf("新行 cc 未落库: %+v", got)
	}
}

// 诊断依赖的两条查询：最新样本时间（判断数据新鲜度）与最近一条指定 kind 的
// 事件（带 detail，用于说明最近一次校准/裁决）。
func TestLatestSampleAndEvent(t *testing.T) {
	s := openTestStore(t, filepath.Join(t.TempDir(), "battery.db"))
	defer func() { _ = s.Close() }()

	if ts, err := s.LatestSampleTS(); err != nil || ts != 0 {
		t.Fatalf("空库 LatestSampleTS = (%d,%v), want (0,nil)", ts, err)
	}
	if err := s.InsertSample(100, 1_000_000, 4_000_000, 50, 0); err != nil {
		t.Fatal(err)
	}
	if err := s.InsertSample(200, 2_000_000, 4_100_000, 60, 0); err != nil {
		t.Fatal(err)
	}
	if ts, err := s.LatestSampleTS(); err != nil || ts != 200 {
		t.Fatalf("LatestSampleTS = (%d,%v), want (200,nil)", ts, err)
	}

	mustExec(t, s, `INSERT INTO events(ts,kind,detail) VALUES
		(100,'cur_cal','old'),(200,'counter_scale','detail-200'),(300,'tick_error','other')`)
	kind, detail, ts, ok, err := s.LatestEvent([]string{"cur_cal", "counter_scale"})
	if err != nil || !ok || kind != "counter_scale" || detail != "detail-200" || ts != 200 {
		t.Fatalf("LatestEvent = (%q,%q,%d,%v,%v), want (counter_scale,detail-200,200,true,nil)",
			kind, detail, ts, ok, err)
	}
	if _, _, _, ok, err := s.LatestEvent([]string{"missing"}); err != nil || ok {
		t.Fatalf("无匹配事件应返回 ok=false: (%v,%v)", ok, err)
	}
}

// 电流刻度回填的存储层：只缩放 [from,to] 区间内的 ua，cap/uv/cc 不动；
// 比例非法/区间非法/可能溢出时返回错误且不改数据。
func TestScaleSamplesUAInRange(t *testing.T) {
	s := openTestStore(t, filepath.Join(t.TempDir(), "battery.db"))
	defer func() { _ = s.Close() }()

	if err := s.InsertSample(100, 3_000, 4_100_000, 20, 0); err != nil {
		t.Fatal(err)
	}
	if err := s.InsertSample(200, -3_000, 4_100_000, 30, 6_000_000); err != nil {
		t.Fatal(err)
	}
	if err := s.InsertSample(400, 3_000, 4_100_000, 40, 0); err != nil {
		t.Fatal(err)
	}
	n, err := s.ScaleSamplesUAInRange(100, 200, 1000)
	if err != nil {
		t.Fatalf("ScaleSamplesUAInRange: %v", err)
	}
	if n != 2 {
		t.Fatalf("更新行数 = %d, want 2", n)
	}
	rows, err := s.SamplesRange(100, 200)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 {
		t.Fatalf("区间样本数 = %d, want 2", len(rows))
	}
	if rows[0].UA != 3_000_000 || rows[1].UA != -3_000_000 {
		t.Fatalf("缩放后 ua = %d/%d, want 3000000/-3000000", rows[0].UA, rows[1].UA)
	}
	// cap/uv/cc 不动
	if rows[0].Cap != 20 || rows[0].UV != 4_100_000 || rows[1].CounterUAh != 6_000_000 {
		t.Fatalf("cap/uv/cc 不应被缩放: %+v / %+v", rows[0], rows[1])
	}
	// 区间外样本保持不变
	out, err := s.SamplesRange(400, 400)
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 1 || out[0].UA != 3_000 {
		t.Fatalf("区间外样本被误改: %+v", out)
	}
	// 反向缩放可还原
	if _, err := s.ScaleSamplesUAInRange(100, 200, 0.001); err != nil {
		t.Fatalf("反向缩放: %v", err)
	}
	rows, _ = s.SamplesRange(100, 200)
	if rows[0].UA != 3_000 || rows[1].UA != -3_000 {
		t.Fatalf("反向缩放后 ua = %d/%d, want 3000/-3000", rows[0].UA, rows[1].UA)
	}

	for _, f := range []float64{0, -1, math.NaN(), math.Inf(1)} {
		if _, err := s.ScaleSamplesUAInRange(100, 200, f); err == nil {
			t.Fatalf("非法比例 %v 应返回错误", f)
		}
	}
	if _, err := s.ScaleSamplesUAInRange(200, 100, 2); err == nil {
		t.Fatal("反向区间应返回错误")
	}
	// 溢出保护：缩放后超 int64 时报错且不改写
	if err := s.InsertSample(500, math.MaxInt64/2+1, 4_100_000, 50, 0); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ScaleSamplesUAInRange(500, 500, 1000); err == nil {
		t.Fatal("溢出缩放应返回错误")
	}
	got, err := s.SamplesRange(500, 500)
	if err != nil || len(got) != 1 || got[0].UA != math.MaxInt64/2+1 {
		t.Fatalf("溢出失败后样本不应被修改: %+v, %v", got, err)
	}
}

// 内阻回填的存储层：按 1/刻度 缩放区间内 mo，区间外不动。
func TestScaleResistanceInRange(t *testing.T) {
	s := openTestStore(t, filepath.Join(t.TempDir(), "battery.db"))
	defer func() { _ = s.Close() }()

	if err := s.InsertResistance(100, 50); err != nil {
		t.Fatal(err)
	}
	if err := s.InsertResistance(200, 0.5); err != nil {
		t.Fatal(err)
	}
	if err := s.InsertResistance(300, 80); err != nil {
		t.Fatal(err)
	}
	n, err := s.ScaleResistanceInRange(100, 200, 0.001)
	if err != nil {
		t.Fatalf("ScaleResistanceInRange: %v", err)
	}
	if n != 2 {
		t.Fatalf("更新行数 = %d, want 2", n)
	}
	mos, err := s.RecentResistance(3)
	if err != nil {
		t.Fatal(err)
	}
	want := []float64{80, 0.0005, 0.05} // 新→旧
	if len(mos) != len(want) {
		t.Fatalf("内阻样本数 = %d, want %d", len(mos), len(want))
	}
	for i := range want {
		if math.Abs(mos[i]-want[i]) > 1e-12 {
			t.Fatalf("mo[%d] = %v, want %v", i, mos[i], want[i])
		}
	}
	for _, f := range []float64{0, -2, math.NaN(), math.Inf(-1)} {
		if _, err := s.ScaleResistanceInRange(100, 200, f); err == nil {
			t.Fatalf("非法比例 %v 应返回错误", f)
		}
	}
	if _, err := s.ScaleResistanceInRange(200, 100, 2); err == nil {
		t.Fatal("反向区间应返回错误")
	}
}
