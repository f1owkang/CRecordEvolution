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

func TestLearningRejectsColdSeed(t *testing.T) {
	// ML 通道同样启用温度门控：种子阶段冷/热会话不得初始化 EMA
	est, _ := newTestLearning(t)

	sess := baseSession()
	sess.TempMin = 8
	sess.TempMax = 10
	sr := SettledSession{Session: sess, AccUA: 6480000000, DesignUA: 4000000}

	_, err := est.OnSession(sr)
	if err == nil {
		t.Fatal("越温种子会话应被拒绝")
	}
	var re *RejectError
	if !errors.As(err, &re) {
		t.Fatalf("错误类型 = %T, want *RejectError", err)
	}
	if re.Result.Reason != "temp_out_of_range" {
		t.Fatalf("Reason = %q, want temp_out_of_range", re.Result.Reason)
	}
	if _, ok := est.kv.KVGet(kvKeyEmaUA); ok {
		t.Fatal("拒绝后 EMA 不应被写入")
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

	// P0 从 1e4 收紧到 100 后 RLS 每步增益变小，60 会话对常数 y=1 的
	// 收敛精度由 1e-6 量级放宽到 1e-4 量级（实测偏差 ≈ 4.7e-5）
	if got := dot4(mlPhi(lastSess), theta); math.Abs(got-1) > 1e-4 {
		t.Fatalf("θᵀφ = %.12f, 与常数 y=1 的偏差超过 1e-4", got)
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

func TestLearningSigma(t *testing.T) {
	est, st := newTestLearning(t)
	seedML(t, est)

	// 种子分支应把 σ 重置为 0，避免沿用上一台设备/旧数据的残留区间
	if v, _ := st.KVGet(kvKeyEmaSigma); v != "0" {
		t.Fatalf("种子后 kv[%s] = %q, want \"0\"", kvKeyEmaSigma, v)
	}

	// 种子后首批会话仍处学习期（samples≤30）且 ratio_hist 不足 8 个：
	// φᵀPφ 尚无意义、实测散布也不可用，σ 应为 0（由上层 ≤0→nil 降级隐藏 ±）
	upd, err := est.OnSession(SettledSession{Session: baseSession(), AccUA: 6480000000, DesignUA: 4000000})
	if err != nil {
		t.Fatalf("会话应被接受: %v", err)
	}
	if upd.SigmaMah != 0 {
		t.Fatalf("散布不足 8 个样本时 SigmaMah = %v, want 0", upd.SigmaMah)
	}
	if v, _ := st.KVGet(kvKeyEmaSigma); v != "0" {
		t.Fatalf("散布不足时 kv[%s] = %q, want \"0\"", kvKeyEmaSigma, v)
	}
}

// popRelStd 测试内独立实现：总体口径相对标准差（std/均值，除以 N），
// 与生产实现分开，避免用被测代码自证期望值
func popRelStd(xs []float64) float64 {
	var sum float64
	for _, x := range xs {
		sum += x
	}
	mean := sum / float64(len(xs))
	var ss float64
	for _, x := range xs {
		d := x - mean
		ss += d * d
	}
	return math.Sqrt(ss/float64(len(xs))) / mean
}

// correctedFromKV 从 kv 读回更新后的 θ，与 σ 计算时刻的 EMA（喂入前的 ema_ua）
// 重算校正容量（mAh 域 σ 的换算基数；persist 后 kv[ema_ua] 已是混合输出 est，不可直接用）
func correctedFromKV(t *testing.T, st *Store, sess Session, emaUA int64) float64 {
	t.Helper()
	var theta [4]float64
	copy(theta[:], kvFloats(t, st, kvKeyRlsTheta, 4))
	return float64(emaUA) * clipRatio(dot4(mlPhi(sess), theta))
}

func setKVJSON(t *testing.T, st *Store, key string, v any) {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal kv[%s]: %v", key, err)
	}
	if err := st.KVSet(key, string(b)); err != nil {
		t.Fatalf("写 kv[%s]: %v", key, err)
	}
}

func TestLearningSigmaZeroUntilSpreadReady(t *testing.T) {
	est, st := newTestLearning(t)
	seedML(t, est)

	// 学习期（samples≤30）内、hist 长度 1..7 不足 8：实测散布不可用，σ 恒为 0
	for i := 0; i < 7; i++ {
		upd, err := est.OnSession(SettledSession{Session: baseSession(), AccUA: 6480000000, DesignUA: 4000000})
		if err != nil {
			t.Fatalf("第 %d 条正常会话应被接受: %v", i+1, err)
		}
		if upd.SigmaMah != 0 {
			t.Fatalf("第 %d 条会话 hist 长度 %d < 8, SigmaMah = %v, want 0", i+1, i+1, upd.SigmaMah)
		}
	}

	// 第 8 条会话构造微散布（r=1.02）：hist 恰满 8 个，学习期出口应给出
	// σ = 实测相对散布 × corrected/1000（而非 φᵀPφ 的巨大值）
	sess := baseSession()
	emaBefore := kvInt(st, kvKeyEmaUA)
	sr := SettledSession{Session: sess, AccUA: 6609600000, DesignUA: 4000000}
	upd, err := est.OnSession(sr)
	if err != nil {
		t.Fatalf("第 8 条会话应被接受: %v", err)
	}
	if upd.SigmaMah <= 0 {
		t.Fatalf("hist 满 8 后 SigmaMah = %v, want > 0", upd.SigmaMah)
	}
	hist := kvFloats(t, st, kvKeyRatioHist, 8)
	want := popRelStd(hist) * correctedFromKV(t, st, sess, emaBefore) / 1000
	if math.Abs(upd.SigmaMah-want) > 1e-6 {
		t.Fatalf("学习期出口 SigmaMah = %v, want 实测散布换算 %v", upd.SigmaMah, want)
	}
}

func TestLearningSigmaCappedByEmpiricalSpread(t *testing.T) {
	est, st := newTestLearning(t)

	// 预置一个「P 矩阵远未收敛」的正式期状态：samples=50（>30），
	// P 对角仍为旧量级 1e4，ratio_hist 为 12 个有散布的固定比值
	if err := st.KVSet(kvKeySamples, "50"); err != nil {
		t.Fatalf("写 kv[samples]: %v", err)
	}
	if err := st.KVSet(kvKeyEmaUA, "3000000"); err != nil {
		t.Fatalf("写 kv[ema_ua]: %v", err)
	}
	setKVJSON(t, st, kvKeyRlsPSym, []float64{1e4, 0, 0, 0, 1e4, 0, 0, 1e4, 0, 1e4})
	setKVJSON(t, st, kvKeyRatioHist, []float64{0.96, 0.97, 0.98, 0.99, 1.00, 1.01, 1.02, 1.03, 1.04, 1.05, 1.06, 1.07})

	sess := baseSession()
	emaBefore := kvInt(st, kvKeyEmaUA)
	upd, err := est.OnSession(SettledSession{Session: sess, AccUA: 6480000000, DesignUA: 4000000})
	if err != nil {
		t.Fatalf("会话应被接受: %v", err)
	}

	// σ 不得超过实测相对散布对应的 mAh 上界
	hist := kvFloats(t, st, kvKeyRatioHist, 13)
	corrected := correctedFromKV(t, st, sess, emaBefore)
	upper := popRelStd(hist) * corrected / 1000
	if upd.SigmaMah <= 0 || upd.SigmaMah > upper+1e-9 {
		t.Fatalf("SigmaMah = %v, want ∈ (0, 实测散布上界 %v]", upd.SigmaMah, upper)
	}

	// 且确实压低了 P 通道：封顶后的 σ_rel 应小于 √(φᵀPφ)
	var p [4][4]float64
	p = symToP(kvFloats(t, st, kvKeyRlsPSym, 10))
	sigmaRelP := math.Sqrt(quadForm(p, mlPhi(sess)))
	if got := upd.SigmaMah * 1000 / corrected; got >= sigmaRelP {
		t.Fatalf("封顶后 σ_rel = %v, 应小于 P 通道 √(φᵀPφ) = %v", got, sigmaRelP)
	}
}

func TestLearningSigmaConvergesWithTighterP0(t *testing.T) {
	est, st := newTestLearning(t)
	seedML(t, est)

	// 正常收敛场景：r 围绕 1.0 以 ±2% 真实感噪声波动（固定 5 点循环，
	// 实测相对散布 ≈ 0.014）。旧实现（P0=1e4 且无封顶）60 次后 σ_rel ≈ 0.15；
	// P0 收紧 + 实测散布封顶后应明显小于 0.05
	pattern := []float64{1.000, 1.020, 0.980, 1.010, 0.990}
	var sess Session
	var upd EstUpdate
	var emaBefore int64
	for i := 0; i < 60; i++ {
		sess = baseSession()
		emaBefore = kvInt(st, kvKeyEmaUA)
		var err error
		upd, err = est.OnSession(SettledSession{
			Session:  sess,
			AccUA:    int64(math.Round(pattern[i%len(pattern)] * float64(emaBefore) * 2160)),
			DesignUA: 4000000,
		})
		if err != nil {
			t.Fatalf("第 %d 次喂入应被接受: %v", i+1, err)
		}
	}

	corrected := correctedFromKV(t, st, sess, emaBefore)
	sigmaRel := upd.SigmaMah * 1000 / corrected
	if sigmaRel >= 0.05 {
		t.Fatalf("60 次会话后 σ_rel = %v, want < 0.05（P0 收紧后应明显收敛）", sigmaRel)
	}
	if sigmaRel <= 0 {
		t.Fatalf("60 次会话后 σ_rel = %v, want > 0（不应走 σ=0 降级路径）", sigmaRel)
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
