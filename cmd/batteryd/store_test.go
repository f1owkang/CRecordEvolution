package main

import (
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
		Ua: 3000000, AvgI: 1500000,
		CRate:    0.75,
		TempMin:  25, TempMax: 33, TempAvg: 29,
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
	).Scan(&got.StartTs, &got.EndTs, &got.StartCap, &got.EndCap, &got.Ua, &got.AvgI,
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
	if err := st.InsertSample(1000, 500000, 3800000, 50); err != nil {
		t.Fatal(err)
	}
	if err := st.InsertSample(2000, 600000, 3850000, 55); err != nil {
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
		Ua: 2_160_000_000, Valid: true})
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
		if err := s.InsertSample(ts, 100_000+ts, 4_000_000+ts, int64(ts)/100); err != nil {
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
