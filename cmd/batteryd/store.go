package main

import (
	"database/sql"
	"fmt"
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
  valid INTEGER NOT NULL,
  invalid_reason TEXT);
CREATE TABLE IF NOT EXISTS estimates(ts INTEGER PRIMARY KEY, mah INTEGER NOT NULL);
CREATE TABLE IF NOT EXISTS resistance(ts INTEGER PRIMARY KEY, mo REAL NOT NULL);
CREATE TABLE IF NOT EXISTS rest_points(ts INTEGER PRIMARY KEY, uv INTEGER NOT NULL, cap INTEGER NOT NULL);
CREATE TABLE IF NOT EXISTS events(ts INTEGER NOT NULL, kind TEXT NOT NULL, detail TEXT);
CREATE TABLE IF NOT EXISTS samples(ts INTEGER PRIMARY KEY, ua INTEGER NOT NULL, uv INTEGER NOT NULL, cap INTEGER NOT NULL, cc INTEGER NOT NULL DEFAULT 0);
CREATE TABLE IF NOT EXISTS ccct(ts INTEGER PRIMARY KEY, vw_lo INTEGER NOT NULL, vw_hi INTEGER NOT NULL, secs INTEGER NOT NULL);
CREATE TABLE IF NOT EXISTS ica_peaks(session_end_ts INTEGER PRIMARY KEY, peak_uv INTEGER NOT NULL, peak_h_rel REAL NOT NULL);
CREATE TABLE IF NOT EXISTS discharge(
  end_ts INTEGER PRIMARY KEY,
  secs INTEGER NOT NULL,
  uah INTEGER NOT NULL,
  start_cap INTEGER NOT NULL,
  end_cap INTEGER NOT NULL,
  implied INTEGER,
  invalid_reason TEXT);
CREATE INDEX IF NOT EXISTS idx_sessions_end ON sessions(end_ts);
`

// DischargeRow 放电会话一行：uah 为 charge_counter 差分累计的放出电量（µAh），
// implied 为按显示电量掉幅反推的隐含满容量（µAh，掉幅 < disMinEstCap 时为 NULL）。
type DischargeRow struct {
	TS, Secs, Uah    int64
	StartCap, EndCap int64
	Implied          *int64
	InvalidReason    string
}

func (s *Store) InsertDischarge(r DischargeRow) error {
	var implied any
	if r.Implied != nil {
		implied = *r.Implied
	}
	_, err := s.db.Exec(`INSERT OR REPLACE INTO discharge
		(end_ts, secs, uah, start_cap, end_cap, implied, invalid_reason)
		VALUES(?, ?, ?, ?, ?, ?, ?)`,
		r.TS, r.Secs, r.Uah, r.StartCap, r.EndCap, implied, reasonOrNull(r.InvalidReason))
	return err
}

// RecentDischarge 取最近 limit 条放电会话（按 end_ts 倒序）。
func (s *Store) RecentDischarge(limit int) ([]DischargeRow, error) {
	rows, err := s.db.Query(`SELECT end_ts, secs, uah, start_cap, end_cap, implied, invalid_reason
		FROM discharge ORDER BY end_ts DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []DischargeRow{}
	for rows.Next() {
		var r DischargeRow
		var implied sql.NullInt64
		var reason sql.NullString
		if err := rows.Scan(&r.TS, &r.Secs, &r.Uah, &r.StartCap, &r.EndCap, &implied, &reason); err != nil {
			return nil, err
		}
		if implied.Valid {
			v := implied.Int64
			r.Implied = &v
		}
		if reason.Valid {
			r.InvalidReason = reason.String
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func reasonOrNull(s string) any {
	if s == "" {
		return nil
	}
	return s
}

type Store struct{ db *sql.DB }

type Session struct {
	StartTs, EndTs            int64
	StartCap, EndCap          int64
	Ua, AvgI                  int64
	CRate                     float64
	TempMin, TempMax, TempAvg int64
	VStart, Duration          int64
	Valid                     bool
	InvalidReason             string
}

type TsVal struct {
	TS int64
	V  int64
}

func OpenStore(path string) (*Store, error) {
	// wal_autocheckpoint(256)：默认 1000 页（≈4MB）才做检查点，低写入量下
	// 最近数周数据几乎全躺在 -wal 里，任何「只复制 .db 主文件」的场景（用户
	// 提取数据库报障、备份工具）都会拿到缺 WAL 的旧数据。降到 256 页（≈1MB），
	// 连接池每个连接都生效（Exec 单发只作用于当时那条连接）。
	db, err := sql.Open("sqlite", "file:"+filepath.ToSlash(path)+"?_busy_timeout=5000&_pragma=wal_autocheckpoint(256)")
	if err != nil {
		return nil, err
	}
	if _, err := db.Exec(schemaDDL); err != nil {
		_ = db.Close()
		return nil, err
	}
	// 旧库在线迁移：sessions 表补 invalid_reason 列（新库 DDL 已含，重复执行幂等）
	if err := migrateSessionsReason(db); err != nil {
		_ = db.Close()
		return nil, err
	}
	// samples 表补 cc 列（电量计读数，旧行默认 0）
	if err := migrateSamplesCC(db); err != nil {
		_ = db.Close()
		return nil, err
	}
	// WAL：daemon 与 WebUI 的 batteryd json 并发读写同一库，WAL 显著降低锁竞争。
	// 失败仅告警不阻断（回退默认 journal）。
	_, _ = db.Exec("PRAGMA journal_mode=WAL;")
	st := &Store{db: db}
	// 旧库在线迁移：历史样本的电流量纲误判遗留一次性修正（幂等）
	if err := st.repairSampleUnits(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return st, nil
}

// kvSamplesUnitRepair 一次性数据修复标记：历史样本的电流量纲误判遗留已修正。
const kvSamplesUnitRepair = "samples_unit_repair"

// repairSampleUnits 修正历史样本里被量纲误判放大的电流行。旧版
// NormCurrentUAWithFull 拿 charge_full/10 当幅值阈值，把 0.6A 以下的真实 µA 读数
// ×1000 落库（实机自换版当天起每天 200+ 条 |UA| 数百安的行）；这些行会一路污染
// 中段校准与 CCCT/ICA 倍率门控——真机回放里中段隐含值被推到 15075mAh，基准应为
// 5289mAh。手机电池瞬时电流不可能超过 maxCurrentUA，越界行必是 1000 倍误乘，
// 除以 1000 即原值；以 charge_counter 差分为真值校验，修正后积分误差 < 6%。
// 幂等：修正后的行不再越界，重复执行不产生二次改写。
func (s *Store) repairSampleUnits() error {
	if v, ok := s.KVGet(kvSamplesUnitRepair); ok && v == "1" {
		return nil
	}
	res, err := s.db.Exec(`UPDATE samples SET ua = ua / 1000 WHERE abs(ua) > ?`, maxCurrentUA)
	if err != nil {
		return err
	}
	if n, err2 := res.RowsAffected(); err2 == nil && n > 0 {
		_ = s.InsertEvent("unit_repair", fmt.Sprintf("修正 %d 条电流量纲误判样本（÷1000）", n))
	}
	return s.KVSet(kvSamplesUnitRepair, "1")
}

// Checkpoint 主动做 TRUNCATE 检查点：把 WAL 落进主库并把 -wal 清零。有并发
// 读者（WebUI json）占用时可能不完整，失败静默——自动检查点兜底，次日再试。
func (s *Store) Checkpoint() {
	_, _ = s.db.Exec("PRAGMA wal_checkpoint(TRUNCATE);")
}

// migrateSessionsReason 检查 sessions 表是否缺 invalid_reason 列（升级前旧库），
// 缺则 ALTER TABLE 补列；已存在时不动，幂等。
func migrateSessionsReason(db *sql.DB) error {
	has, err := hasColumn(db, "sessions", "invalid_reason")
	if err != nil || has {
		return err
	}
	_, err = db.Exec(`ALTER TABLE sessions ADD COLUMN invalid_reason TEXT`)
	return err
}

// migrateSamplesCC 检查 samples 表是否缺 cc 列（电量计 charge_counter 读数），
// 缺则补列；已存在时不动，幂等。补列后历史行 cc=0，消费方按区间回退电流积分。
func migrateSamplesCC(db *sql.DB) error {
	has, err := hasColumn(db, "samples", "cc")
	if err != nil || has {
		return err
	}
	_, err = db.Exec(`ALTER TABLE samples ADD COLUMN cc INTEGER NOT NULL DEFAULT 0`)
	return err
}

// hasColumn 查 PRAGMA table_info 判断表是否已有某列，供各幂等迁移复用。
func hasColumn(db *sql.DB, table, col string) (bool, error) {
	rows, err := db.Query(`PRAGMA table_info(` + table + `)`)
	if err != nil {
		return false, err
	}
	defer rows.Close()
	for rows.Next() {
		var cid int64
		var name, typ string
		var notNull, pk int64
		var dflt sql.NullString
		if err := rows.Scan(&cid, &name, &typ, &notNull, &dflt, &pk); err != nil {
			return false, err
		}
		if name == col {
			return true, nil
		}
	}
	return false, rows.Err()
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
		temp_min,temp_max,temp_avg,v_start,duration,valid,invalid_reason
	) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		sess.StartTs, sess.EndTs, sess.StartCap, sess.EndCap, sess.Ua, sess.AvgI, sess.CRate,
		sess.TempMin, sess.TempMax, sess.TempAvg, sess.VStart, sess.Duration, boolToInt(sess.Valid),
		sql.NullString{String: sess.InvalidReason, Valid: sess.InvalidReason != ""})
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

// RecentResistance 返回最近 limit 条内阻样本值（按 ts 降序，即最新在前），
// 供显示层做稳健统计（中位数），单条离群不影响口径。
func (s *Store) RecentResistance(limit int) ([]float64, error) {
	rows, err := s.db.Query(`SELECT mo FROM resistance ORDER BY ts DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []float64{}
	for rows.Next() {
		var mo float64
		if err := rows.Scan(&mo); err != nil {
			return nil, err
		}
		out = append(out, mo)
	}
	return out, rows.Err()
}

func (s *Store) InsertRestPoint(ts, uv, cap int64) error {
	_, err := s.db.Exec(`INSERT OR REPLACE INTO rest_points(ts, uv, cap) VALUES(?, ?, ?)`, ts, uv, cap)
	return err
}

type RestPoint struct {
	TS, UV, Cap int64
}

func (s *Store) RecentSessions(limit int) ([]Session, error) {
	rows, err := s.db.Query(`SELECT start_ts,end_ts,start_cap,end_cap,ua,c_rate,
		temp_avg,valid,invalid_reason FROM sessions ORDER BY end_ts DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Session{}
	for rows.Next() {
		var se Session
		var v int64
		var reason sql.NullString
		if err := rows.Scan(&se.StartTs, &se.EndTs, &se.StartCap, &se.EndCap,
			&se.Ua, &se.CRate, &se.TempAvg, &v, &reason); err != nil {
			return nil, err
		}
		se.Valid = v == 1
		se.InvalidReason = reason.String
		out = append(out, se)
	}
	return out, rows.Err()
}

func (s *Store) RecentRestPoints(limit int) ([]RestPoint, error) {
	rows, err := s.db.Query(`SELECT ts, uv, cap FROM rest_points ORDER BY ts DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []RestPoint{}
	for rows.Next() {
		var rp RestPoint
		if err := rows.Scan(&rp.TS, &rp.UV, &rp.Cap); err != nil {
			return nil, err
		}
		out = append(out, rp)
	}
	return out, rows.Err()
}

func (s *Store) InsertEvent(kind, detail string) error {
	_, err := s.db.Exec(`INSERT INTO events(ts, kind, detail) VALUES(?, ?, ?)`,
		time.Now().Unix(), kind, detail)
	return err
}

// InsertSample 写入一条充电样本。ts 为秒级主键：60s tick 下同一秒只写一次、
// 天然唯一；若未来缩短采样周期或出现多写路径，INSERT OR REPLACE 会静默覆盖
// 同 ts 旧行（幂等而非追加），属已知约束。cc 为电量计读数（µAh，0=未采到）。
func (s *Store) InsertSample(ts, ua, uv, cap, cc int64) error {
	_, err := s.db.Exec(`INSERT OR REPLACE INTO samples(ts, ua, uv, cap, cc) VALUES(?, ?, ?, ?, ?)`, ts, ua, uv, cap, cc)
	return err
}

func (s *Store) CountSamples() (int64, error) {
	var n int64
	err := s.db.QueryRow(`SELECT COUNT(*) FROM samples`).Scan(&n)
	return n, err
}

// SamplesRange 返回 [from, to] 闭区间内的样本行，按 ts 升序。
func (s *Store) SamplesRange(from, to int64) ([]SampleRow, error) {
	rows, err := s.db.Query(`SELECT ts, ua, uv, cap, cc FROM samples
		WHERE ts >= ? AND ts <= ? ORDER BY ts ASC`, from, to)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []SampleRow{}
	for rows.Next() {
		var r SampleRow
		if err := rows.Scan(&r.TS, &r.UA, &r.UV, &r.Cap, &r.CC); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// CcctRow 为 ccct 表一行：穿窗上沿时刻、窗下/上沿电压与穿窗耗时。
type CcctRow struct {
	TS, VwLo, VwHi, Secs int64
}

func (s *Store) InsertCCCT(ts, vwLo, vwHi, secs int64) error {
	_, err := s.db.Exec(`INSERT OR REPLACE INTO ccct(ts, vw_lo, vw_hi, secs) VALUES(?, ?, ?, ?)`,
		ts, vwLo, vwHi, secs)
	return err
}

func (s *Store) RecentCCCT(limit int) ([]CcctRow, error) {
	rows, err := s.db.Query(`SELECT ts, vw_lo, vw_hi, secs FROM ccct ORDER BY ts DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []CcctRow{}
	for rows.Next() {
		var c CcctRow
		if err := rows.Scan(&c.TS, &c.VwLo, &c.VwHi, &c.Secs); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// ICAPeakRow 为 ica_peaks 表一行：会话结束时刻、主峰位置（µV）与相对峰高。
type ICAPeakRow struct {
	TS, PeakUV int64
	PeakHRel   float64
}

func (s *Store) InsertICAPeak(ts, peakUV int64, peakHRel float64) error {
	_, err := s.db.Exec(`INSERT OR REPLACE INTO ica_peaks(session_end_ts, peak_uv, peak_h_rel) VALUES(?, ?, ?)`,
		ts, peakUV, peakHRel)
	return err
}

func (s *Store) RecentICAPeaks(limit int) ([]ICAPeakRow, error) {
	rows, err := s.db.Query(`SELECT session_end_ts, peak_uv, peak_h_rel FROM ica_peaks ORDER BY session_end_ts DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ICAPeakRow{}
	for rows.Next() {
		var r ICAPeakRow
		if err := rows.Scan(&r.TS, &r.PeakUV, &r.PeakHRel); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s *Store) PruneBefore(cutoffTs int64) error {
	for _, table := range []struct {
		name  string
		tsCol string
	}{
		{"sessions", "end_ts"},
		{"estimates", "ts"},
		{"resistance", "ts"},
		{"rest_points", "ts"},
		{"events", "ts"},
		{"samples", "ts"},
		{"ccct", "ts"},
		{"ica_peaks", "session_end_ts"},
		{"discharge", "end_ts"},
	} {
		if _, err := s.db.Exec("DELETE FROM "+table.name+" WHERE "+table.tsCol+" < ?", cutoffTs); err != nil {
			return err
		}
	}
	return nil
}
