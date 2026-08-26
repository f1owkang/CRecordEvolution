package main

import (
	"encoding/json"
	"errors"
	"math"
	"path/filepath"
	"testing"
)

func newTestLearning(t *testing.T) (*Learning, *Store) {
	t.Helper()
	st, err := OpenStore(filepath.Join(t.TempDir(), "battery.db"))
	if err != nil {
		t.Fatalf("OpenStore: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	return NewLearning(st), st
}

func kvFloats(t *testing.T, st *Store, key string, wantLen int) []float64 {
	t.Helper()
	v, ok := st.KVGet(key)
	if !ok {
		t.Fatalf("缺少 kv[%s]", key)
	}
	var arr []float64
	if err := json.Unmarshal([]byte(v), &arr); err != nil {
		t.Fatalf("kv[%s] 不是合法 JSON 数组: %v", key, err)
	}
	if len(arr) != wantLen {
		t.Fatalf("kv[%s] 长度 = %d, want %d", key, len(arr), wantLen)
	}
	return arr
}

func seedML(t *testing.T, est *Learning) {
	t.Helper()
	sr := SettledSession{Session: baseSession(), AccUA: 6480000000, DesignUA: 4000000}
	upd, err := est.OnSession(sr)
	if err != nil {
		t.Fatalf("种子会话应被接受: %v", err)
	}
	if upd.EstUA != 3000000 || upd.Samples != 1 {
		t.Fatalf("种子会话 = (%d,%d), want (3000000,1)", upd.EstUA, upd.Samples)
	}
}

func TestLearningSeedsFirstSessionAndConverges(t *testing.T) {
	est, st := newTestLearning(t)
	seedML(t, est)

	var lastSess Session
	for i := 0; i < 60; i++ {
		e := kvInt(st, kvKeyEmaUA)
		sess := baseSession()
		sess.StartTs += int64(i) * 10000
		sess.EndTs = sess.StartTs + 6000
		sr := SettledSession{Session: sess, AccUA: e * 2160, DesignUA: 4000000}

		upd, err := est.OnSession(sr)
		if err != nil {
			t.Fatalf("第 %d 次喂入应被接受: %v", i+1, err)
		}
		if upd.Samples != int64(i+2) {
			t.Fatalf("第 %d 次喂入 Samples = %d, want %d", i+1, upd.Samples, i+2)
		}
		lastSess = sess
	}

	var theta [4]float64
	copy(theta[:], kvFloats(t, st, kvKeyRlsTheta, 4))
	kvFloats(t, st, kvKeyRlsPSym, 10)
	hist := kvFloats(t, st, kvKeyRatioHist, 40)

	if got := dot4(mlPhi(lastSess), theta); math.Abs(got-1) > 1e-6 {
		t.Fatalf("θᵀφ = %.12f, 与常数 y=1 的偏差超过 1e-6", got)
	}
	for _, r := range hist {
		if math.Abs(r-1) > 1e-9 {
			t.Fatalf("ratio_hist 中出现非 1 比值 %v", r)
		}
	}
}

func TestLearningClipsCorrectionAtUpperBound(t *testing.T) {
	est, st := newTestLearning(t)
	seedML(t, est)

	hi := SettledSession{Session: baseSession(), AccUA: 9288000000, DesignUA: 4000000}
	for i := 0; i < 30; i++ {
		upd, err := est.OnSession(hi)
		if err != nil {
			t.Fatalf("第 %d 次 y≈1.43 训练会话应被接受: %v", i+1, err)
		}
		if upd.Samples != int64(i+2) {
			t.Fatalf("第 %d 次训练会话 Samples = %d, want %d", i+1, upd.Samples, i+2)
		}
	}
	if gotSamples := kvInt(st, kvKeySamples); gotSamples != 31 {
		t.Fatalf("训练后 samples = %d, want 31", gotSamples)
	}
	if gotEma := kvInt(st, kvKeyEmaUA); gotEma != 3000000 {
		t.Fatalf("w=0 阶段基线不应移动, ema_ua = %d, want 3000000", gotEma)
	}

	var pre [4]float64
	copy(pre[:], kvFloats(t, st, kvKeyRlsTheta, 4))
	if dot4(mlPhi(hi.Session), pre) <= clipHi {
		t.Fatalf("前置条件失败：θᵀφ = %v 未超过 clip 上界 %v", dot4(mlPhi(hi.Session), pre), clipHi)
	}
	e31 := kvInt(st, kvKeyEmaUA)

	upd, err := est.OnSession(hi)
	if err != nil {
		t.Fatalf("探测会话应被接受: %v", err)
	}

	w := mlWeight(31)
	want := int64((1-w)*float64(e31) + w*(float64(e31)*clipHi))
	if want != 3004500 {
		t.Fatalf("期望值自检失败：want = %d, want 3004500(w=%v)", want, w)
	}
	if upd.EstUA != want {
		t.Fatalf("clip 生效时 EstUA = %d, want %d((1-%v)*%d+%v*%d*%v)", upd.EstUA, want, w, e31, w, e31, clipHi)
	}
	if upd.Samples != 32 {
		t.Fatalf("探测会话 Samples = %d, want 32", upd.Samples)
	}

	var post [4]float64
	copy(post[:], kvFloats(t, st, kvKeyRlsTheta, 4))
	if dot4(mlPhi(hi.Session), post) <= clipHi {
		t.Fatalf("探测后 θᵀφ = %v 应仍超过 %v", dot4(mlPhi(hi.Session), post), clipHi)
	}
}

func TestClipRatioBounds(t *testing.T) {
	cases := []struct {
		in   float64
		want float64
	}{
		{0.69, clipLo},
		{clipLo, clipLo},
		{1.0, 1.0},
		{clipHi, clipHi},
		{1.31, clipHi},
	}
	for _, tc := range cases {
		if got := clipRatio(tc.in); got != tc.want {
			t.Fatalf("clipRatio(%v) = %v, want %v", tc.in, got, tc.want)
		}
	}
}

func TestMLWeightSegments(t *testing.T) {
	cases := []struct {
		n    int64
		want float64
	}{
		{0, 0}, {10, 0}, {30, 0},
		{31, 0.005}, {80, 0.25}, {129, 0.495},
		{130, 0.5}, {200, 0.5},
	}
	for _, tc := range cases {
		if got := mlWeight(tc.n); math.Abs(got-tc.want) > 1e-12 {
			t.Fatalf("mlWeight(%d) = %v, want %v", tc.n, got, tc.want)
		}
	}
}

func TestLearningIQROnlyFromEighthRatio(t *testing.T) {
	est, st := newTestLearning(t)
	seedML(t, est)

	low := SettledSession{Session: baseSession(), AccUA: 6480000000, DesignUA: 4000000}
	for i := 0; i < 7; i++ {
		upd, err := est.OnSession(low)
		if err != nil {
			t.Fatalf("第 %d 条正常会话应被接受: %v", i+1, err)
		}
		if upd.Samples != int64(i+2) {
			t.Fatalf("第 %d 条正常会话 Samples = %d, want %d", i+1, upd.Samples, i+2)
		}
	}
	if v, ok := st.KVGet(kvKeyRatioHist); !ok || len(jsonToArray(t, v)) != 7 {
		t.Fatalf("7 条正常会话后 ratio_hist 应恰有 7 个元素, got %q", v)
	}

	outlier := SettledSession{Session: baseSession(), AccUA: 9288000000, DesignUA: 4000000}
	upd, err := est.OnSession(outlier)
	if err != nil {
		t.Fatalf("K=7 时不应做 IQR 剔除，离群会话应被接受: %v", err)
	}
	if upd.Samples != 9 {
		t.Fatalf("K=7 接受后 Samples = %d, want 9", upd.Samples)
	}
	if v, _ := st.KVGet(kvKeyRatioHist); len(jsonToArray(t, v)) != 8 {
		t.Fatalf("接受离群值后 ratio_hist 应有 8 个元素")
	}

	upd2, err := est.OnSession(outlier)
	if err == nil {
		t.Fatalf("K=8 起应做 IQR 剔除，得到 %+v", upd2)
	}
	var re *RejectError
	if !errors.As(err, &re) {
		t.Fatalf("错误类型 = %T, want *RejectError", err)
	}
	if re.Result.Reason != "outlier" {
		t.Fatalf("Reason = %q, want outlier", re.Result.Reason)
	}
	if upd2.Changed {
		t.Fatal("剔除时 Changed 应为 false")
	}
	if v, _ := st.KVGet(kvKeyRatioHist); len(jsonToArray(t, v)) != 8 {
		t.Fatal("被剔除的比值不应写入 ratio_hist")
	}
	if v, _ := st.KVGet(kvKeySamples); v != "9" {
		t.Fatalf(`剔除后 kv[samples] = %q, want "9"`, v)
	}
	if v, _ := st.KVGet(kvKeyEmaUA); v != "3000000" {
		t.Fatalf(`剔除后 kv[ema_ua] = %q, want "3000000"`, v)
	}
}

func jsonToArray(t *testing.T, s string) []float64 {
	t.Helper()
	var arr []float64
	if err := json.Unmarshal([]byte(s), &arr); err != nil {
		t.Fatalf("不是合法 JSON 数组: %v", err)
	}
	return arr
}
