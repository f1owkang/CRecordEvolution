package main

import (
	"database/sql"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
)

const schemaDDL = `
CREATE TABLE IF NOT EXISTS kv(k TEXT PRIMARY KEY, v TEXT NOT NULL);
CREATE TABLE IF NOT EXISTS sessions(
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  start_ts INTEGER NOT NULL, end_ts INTEGER NOT NULL,
  start_cap INTEGER, end_cap INTEGER,
  ua INTEGER, avg_i INTEGER, c_rate REAL,
  temp_min INTEGER, temp_max INTEGER, temp_avg INTEGER,
  v_start INTEGER, duration INTEGER,
  valid INTEGER NOT NULL);
CREATE TABLE IF NOT EXISTS estimates(ts INTEGER PRIMARY KEY, mah INTEGER NOT NULL);
CREATE TABLE IF NOT EXISTS resistance(ts INTEGER PRIMARY KEY, mo REAL NOT NULL);
CREATE TABLE IF NOT EXISTS rest_points(ts INTEGER PRIMARY KEY, uv INTEGER NOT NULL, cap INTEGER NOT NULL);
CREATE TABLE IF NOT EXISTS events(ts INTEGER NOT NULL, kind TEXT NOT NULL, detail TEXT);
CREATE TABLE IF NOT EXISTS samples(ts INTEGER PRIMARY KEY, ua INTEGER NOT NULL, uv INTEGER NOT NULL, cap INTEGER NOT NULL);
CREATE INDEX IF NOT EXISTS idx_sessions_end ON sessions(end_ts);
`

type Store struct{ db *sql.DB }

type Session struct {
	StartTs, EndTs                            int64
	StartCap, EndCap                          int64
	Ua, AvgI                                  int64
	CRate                                     float64
	TempMin, TempMax, TempAvg                 int64
	VStart, Duration                          int64
	Valid                                     bool
}

type TsVal struct {
	TS int64
	V  int64
}

func OpenStore(path string) (*Store, error) {
	db, err := sql.Open("sqlite", "file:"+filepath.ToSlash(path)+"?_busy_timeout=5000")
	if err != nil {
		return nil, err
	}
	if _, err := db.Exec(schemaDDL); err != nil {
		_ = db.Close()
		return nil, err
	}
	// WAL：daemon 与 WebUI 的 batteryd json 并发读写同一库，WAL 显著降低锁竞争。
	// 失败仅告警不阻断（回退默认 journal）。
	_, _ = db.Exec("PRAGMA journal_mode=WAL;")
	return &Store{db: db}, nil
}

func (s *Store) Close() error {
	return s.db.Close()
}

func (s *Store) KVGet(key string) (string, bool) {
	var v string
	if err := s.db.QueryRow(`SELECT v FROM kv WHERE k = ?`, key).Scan(&v); err != nil {
		return "", false
	}
	return v, true
}

func (s *Store) KVSet(key, val string) error {
	_, err := s.db.Exec(`INSERT INTO kv(k, v) VALUES(?, ?)
		ON CONFLICT(k) DO UPDATE SET v = excluded.v`, key, val)
	return err
}

func boolToInt(b bool) int64 {
	if b {
		return 1
	}
	return 0
}

func (s *Store) InsertSession(sess Session) (int64, error) {
	res, err := s.db.Exec(`INSERT INTO sessions(
		start_ts,end_ts,start_cap,end_cap,ua,avg_i,c_rate,
		temp_min,temp_max,temp_avg,v_start,duration,valid
	) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		sess.StartTs, sess.EndTs, sess.StartCap, sess.EndCap, sess.Ua, sess.AvgI, sess.CRate,
		sess.TempMin, sess.TempMax, sess.TempAvg, sess.VStart, sess.Duration, boolToInt(sess.Valid))
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (s *Store) InsertEstimate(ts, mahUa int64) error {
	_, err := s.db.Exec(`INSERT OR REPLACE INTO estimates(ts, mah) VALUES(?, ?)`, ts, mahUa)
	return err
}

func (s *Store) RecentEstimates(limit int) ([]TsVal, error) {
	rows, err := s.db.Query(`SELECT ts, mah FROM estimates ORDER BY ts DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]TsVal, 0, limit)
	for rows.Next() {
		var tv TsVal
		if err := rows.Scan(&tv.TS, &tv.V); err != nil {
			return nil, err
		}
		out = append(out, tv)
	}
	return out, rows.Err()
}

func (s *Store) InsertResistance(ts int64, mo float64) error {
	_, err := s.db.Exec(`INSERT OR REPLACE INTO resistance(ts, mo) VALUES(?, ?)`, ts, mo)
	return err
}

func (s *Store) InsertRestPoint(ts, uv, cap int64) error {
	_, err := s.db.Exec(`INSERT OR REPLACE INTO rest_points(ts, uv, cap) VALUES(?, ?, ?)`, ts, uv, cap)
	return err
}

func (s *Store) InsertEvent(kind, detail string) error {
	_, err := s.db.Exec(`INSERT INTO events(ts, kind, detail) VALUES(?, ?, ?)`,
		time.Now().Unix(), kind, detail)
	return err
}

func (s *Store) InsertSample(ts, ua, uv, cap int64) error {
	_, err := s.db.Exec(`INSERT OR REPLACE INTO samples(ts, ua, uv, cap) VALUES(?, ?, ?, ?)`, ts, ua, uv, cap)
	return err
}

func (s *Store) CountSamples() (int64, error) {
	var n int64
	err := s.db.QueryRow(`SELECT COUNT(*) FROM samples`).Scan(&n)
	return n, err
}

func (s *Store) PruneBefore(cutoffTs int64) error {
	for _, table := range []struct {
		name   string
		tsCol  string
	}{
		{"sessions", "end_ts"},
		{"estimates", "ts"},
		{"resistance", "ts"},
		{"rest_points", "ts"},
		{"events", "ts"},
		{"samples", "ts"},
	} {
		if _, err := s.db.Exec("DELETE FROM "+table.name+" WHERE "+table.tsCol+" < ?", cutoffTs); err != nil {
			return err
		}
	}
	return nil
}
