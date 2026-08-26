package main

import (
	"errors"
	"path/filepath"
	"testing"
)

func newTestStable(t *testing.T) (*Stable, *Store) {
	t.Helper()
	st, err := OpenStore(filepath.Join(t.TempDir(), "battery.db"))
	if err != nil {
		t.Fatalf("OpenStore: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	return NewStable(st), st
}

func baseSession() Session {
	return Session{
		StartTs: 1000, EndTs: 2000,
		StartCap: 20, EndCap: 80,
		Ua:      8208000000,
		AvgI:    1500000,
		CRate:   0.375,
		TempMin: 25, TempMax: 33, TempAvg: 29,
		VStart: 4200000, Duration: 1000,
		Valid: true,
	}
}

func kvString(t *testing.T, st *Store, key string) (string, bool) {
	t.Helper()
	return st.KVGet(key)
}

func TestOnSessionRejectsTempOutOfRange(t *testing.T) {
	est, st := newTestStable(t)

	cases := []struct {
		name    string
		tempMin int64
		tempMax int64
	}{
		{"temp_min低于15", 12, 35},
		{"temp_max高于40", 20, 41},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			sess := baseSession()
			sess.TempMin = tc.tempMin
			sess.TempMax = tc.tempMax
			sr := SettledSession{Session: sess, AccUA: 8208000000, DesignUA: 4000000}

			upd, err := est.OnSession(sr)
			if err == nil {
				t.Fatalf("越温会话应被拒绝，得到 %+v", upd)
			}
			var re *RejectError
			if !errors.As(err, &re) {
				t.Fatalf("错误类型 = %T, want *RejectError", err)
			}
			if re.Result.Accepted {
				t.Fatal("拒绝结果 Accepted 不应为 true")
			}
			if re.Result.Reason != "temp_out_of_range" {
				t.Fatalf("Reason = %q, want temp_out_of_range", re.Result.Reason)
			}
			if upd.Changed {
				t.Fatal("拒绝时 Changed 应为 false")
			}
			if _, ok := kvString(t, st, "samples"); ok {
				t.Fatal("拒绝时 samples 不应写入")
			}
		})
	}
}

func TestOnSessionRejectsDeltaBelow20(t *testing.T) {
	est, st := newTestStable(t)

	sess := baseSession()
	sess.EndCap = 39
	sr := SettledSession{Session: sess, AccUA: 8208000000, DesignUA: 4000000}

	upd, err := est.OnSession(sr)
	if err == nil {
		t.Fatalf("delta<20 会话应被拒绝，得到 %+v", upd)
	}
	var re *RejectError
	if !errors.As(err, &re) {
		t.Fatalf("错误类型 = %T, want *RejectError", err)
	}
	if re.Result.Reason != "delta_lt_20" {
		t.Fatalf("Reason = %q, want delta_lt_20", re.Result.Reason)
	}
	if upd.Changed {
		t.Fatal("拒绝时 Changed 应为 false")
	}
	if _, ok := kvString(t, st, "samples"); ok {
		t.Fatal("拒绝时 samples 不应写入")
	}
}

func TestOnSessionRejectsOutsideWindow(t *testing.T) {
	est, _ := newTestStable(t)

	cases := []struct {
		name  string
		accUA int64
	}{
		{"低于0.5倍设计容量", 3420000000},
		{"高于1.5倍设计容量", 13176000000},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			sr := SettledSession{Session: baseSession(), AccUA: tc.accUA, DesignUA: 4000000}

			upd, err := est.OnSession(sr)
			if err == nil {
				t.Fatalf("窗口外会话应被拒绝，得到 %+v", upd)
			}
			var re *RejectError
			if !errors.As(err, &re) {
				t.Fatalf("错误类型 = %T, want *RejectError", err)
			}
			if re.Result.Reason != "out_of_window" {
				t.Fatalf("Reason = %q, want out_of_window", re.Result.Reason)
			}
			if upd.Changed {
				t.Fatal("拒绝时 Changed 应为 false")
			}
		})
	}
}

func TestOnSessionEMAAcrossSessions(t *testing.T) {
	est, st := newTestStable(t)

	first := SettledSession{Session: baseSession(), AccUA: 1440000000, DesignUA: 4000000}
	first.EndCap = 40
	first.TempMin = 15
	first.TempMax = 40

	upd, err := est.OnSession(first)
	if err != nil {
		t.Fatalf("首会话(delta恰为20、窗口下界恰为0.5倍、温度恰在15/40边界)应被接受: %v", err)
	}
	if !upd.Changed {
		t.Fatal("接受时 Changed 应为 true")
	}
	if upd.EstUA != 2000000 {
		t.Fatalf("首会话 EstUA = %d, want 2000000(种子即 ua_full)", upd.EstUA)
	}
	if upd.Samples != 1 {
		t.Fatalf("首会话 Samples = %d, want 1", upd.Samples)
	}
	if v, ok := kvString(t, st, "samples"); !ok || v != "1" {
		t.Fatalf(`kv[samples] = (%q,%v), want ("1",true)`, v, ok)
	}
	if v, ok := kvString(t, st, "ema_ua"); !ok || v != "2000000" {
		t.Fatalf(`kv[ema_ua] = (%q,%v), want ("2000000",true)`, v, ok)
	}

	second := SettledSession{Session: baseSession(), AccUA: 4500000360, DesignUA: 4000000}
	second.StartCap = 10
	second.EndCap = 40
	second.StartTs = 5000
	second.EndTs = 6000

	upd2, err := est.OnSession(second)
	if err != nil {
		t.Fatalf("第二会话应被接受: %v", err)
	}
	if !upd2.Changed {
		t.Fatal("接受时 Changed 应为 true")
	}
	if upd2.EstUA != 2650000 {
		t.Fatalf("第二会话 EstUA = %d, want 2650000((2000000*7+4166667*3)/10 向下取整)", upd2.EstUA)
	}
	if upd2.Samples != 2 {
		t.Fatalf("第二会话 Samples = %d, want 2", upd2.Samples)
	}
	if v, ok := kvString(t, st, "samples"); !ok || v != "2" {
		t.Fatalf(`kv[samples] = (%q,%v), want ("2",true)`, v, ok)
	}
	if v, ok := kvString(t, st, "ema_ua"); !ok || v != "2650000" {
		t.Fatalf(`kv[ema_ua] = (%q,%v), want ("2650000",true)`, v, ok)
	}
}

type brokenKV struct{}

func (brokenKV) KVGet(string) (string, bool) { return "", false }
func (brokenKV) KVSet(string, string) error  { return errors.New("disk full") }

func TestOnSessionPropagatesKVPersistError(t *testing.T) {
	est := NewStable(brokenKV{})

	sr := SettledSession{Session: baseSession(), AccUA: 8208000000, DesignUA: 4000000}

	upd, err := est.OnSession(sr)
	if err == nil || err.Error() != "disk full" {
		t.Fatalf("err = %v, want disk full", err)
	}
	if upd.Changed {
		t.Fatal("持久化失败时 Changed 应为 false")
	}
}
