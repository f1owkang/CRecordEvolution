package main

import (
	"encoding/json"
	"math"
	"sort"
	"strconv"
)

const (
	kvKeyRlsTheta  = "rls_theta"
	kvKeyRlsPSym   = "rls_p_sym"
	kvKeyRatioHist = "ratio_hist"
	kvKeySFullHist = "sfull_hist"

	rlsLambda     = 0.99
	rlsP0         = 100
	ratioHistCap  = 40
	iqrMinSamples = 8
	clipLo        = 0.7
	clipHi        = 1.3

	kvKeyEmaSigma = "ema_sigma_mah"
	sigmaRelFloor = 0.005

	// learningSamples 学习期长度：θ 训练未满时不参与输出混合（与 mlWeight
	// 的起坡点一致），但基线照常随会话移动（见 OnSession）。
	learningSamples = 30
)

type Learning struct{ kv KVStore }

func (l *Learning) OnSession(sr SettledSession) (EstUpdate, error) {
	if tempOutOfRange(sr) {
		return EstUpdate{}, &RejectError{Result: SessionResult{Reason: "temp_out_of_range"}}
	}
	delta := sr.EndCap - sr.StartCap
	if delta < minDeltaCap {
		return EstUpdate{}, &RejectError{Result: SessionResult{Reason: "delta_lt_20"}}
	}
	sFull := sr.AccUA * 100 / (delta * 3600)
	if sFull*2 < sr.DesignUA || sFull*2 > sr.DesignUA*3 {
		return EstUpdate{}, &RejectError{Result: SessionResult{Reason: "out_of_window"}}
	}

	samples := kvInt(l.kv, kvKeySamples)
	ema := kvInt(l.kv, kvKeyEmaUA)
	if samples <= 0 || ema <= 0 {
		return l.seed(sFull)
	}

	hist := l.loadFloats(kvKeyRatioHist, 0)
	r := float64(sFull) / float64(ema)
	if len(hist) >= iqrMinSamples {
		lo, hi := iqrBounds(hist)
		if r < lo || r > hi {
			return EstUpdate{}, &RejectError{Result: SessionResult{Reason: "outlier"}}
		}
	}

	phi := mlPhi(sr.Session)
	theta := l.loadTheta()
	p := l.loadP()
	theta, p = rlsUpdate(theta, p, phi, r)

	// 学习期（samples ≤ 30）θ 不参与输出，但基线经 emaBlend 随会话移动
	// （满充 3/10、未满充降权 1/10）：旧实现整段冻结在种子值，实测设备 5 个
	// 采信会话输出一字不变，学习期长达 1~2 个月，显示信息量反而少于 stable。
	// 正式期维持渐进混合。
	var est int64
	corrected := float64(ema) * clipRatio(dot4(phi, theta))
	if samples <= learningSamples {
		est = emaBlend(ema, sFull, sr.EndCap)
	} else {
		w := mlWeight(samples)
		est = int64((1-w)*float64(ema) + w*corrected)
	}

	hist = append(hist, r)
	if len(hist) > ratioHistCap {
		hist = hist[len(hist)-ratioHistCap:]
	}
	sfullHist := l.loadFloats(kvKeySFullHist, 0)
	sfullHist = append(sfullHist, float64(sFull))
	if len(sfullHist) > ratioHistCap {
		sfullHist = sfullHist[len(sfullHist)-ratioHistCap:]
	}

	// σ（mAh 域）：散布取 sfull_hist（原始隐含容量）的相对标准差，而非
	// ratio_hist——基线随会话移动后 ratio-to-ema 的散布虚假收窄，原始容量的
	// 散布才是估算值不确定度的诚实度量。散布不足 8 个样本时 σ=0，由 main.go
	// 现有 ≤0 → nil 降级隐藏 ±。
	// - 学习期（samples ≤ 30）：φᵀPφ 无预测意义，直接用实测散布；
	// - 正式期：σ_rel = √(φᵀPφ)，保持 0.005 下限，再用实测散布封顶，
	//   避免 P0 与激励不足导致 σ 长期虚高。
	spread, hasSpread := histRelStd(sfullHist)
	var sigmaMah float64
	if samples <= learningSamples {
		if hasSpread {
			sigmaMah = spread * float64(est) / 1000
		}
	} else {
		sigmaRel := math.Sqrt(quadForm(p, phi))
		if sigmaRel < sigmaRelFloor {
			sigmaRel = sigmaRelFloor
		}
		if hasSpread && spread < sigmaRel {
			sigmaRel = spread
		}
		sigmaMah = sigmaRel * corrected / 1000
	}

	samples++

	if err := l.persist(theta, p, hist, sfullHist, est, samples, sigmaMah); err != nil {
		return EstUpdate{}, err
	}
	return EstUpdate{EstUA: est, Samples: samples, Changed: true, SigmaMah: sigmaMah}, nil
}

func (l *Learning) seed(uaFull int64) (EstUpdate, error) {
	if err := l.kv.KVSet(kvKeyEmaUA, strconv.FormatInt(uaFull, 10)); err != nil {
		return EstUpdate{}, err
	}
	if err := l.kv.KVSet(kvKeySamples, "1"); err != nil {
		return EstUpdate{}, err
	}
	if err := l.kv.KVSet(kvKeyEmaSigma, "0"); err != nil {
		return EstUpdate{}, err
	}
	return EstUpdate{EstUA: uaFull, Samples: 1, Changed: true}, nil
}

func (l *Learning) persist(theta [4]float64, p [4][4]float64, hist, sfullHist []float64, ema, samples int64, sigmaMah float64) error {
	thetaJSON, err := json.Marshal(theta[:])
	if err != nil {
		return err
	}
	if err := l.kv.KVSet(kvKeyRlsTheta, string(thetaJSON)); err != nil {
		return err
	}
	pJSON, err := json.Marshal(pToSym(p))
	if err != nil {
		return err
	}
	if err := l.kv.KVSet(kvKeyRlsPSym, string(pJSON)); err != nil {
		return err
	}
	histJSON, err := json.Marshal(hist)
	if err != nil {
		return err
	}
	if err := l.kv.KVSet(kvKeyRatioHist, string(histJSON)); err != nil {
		return err
	}
	sfullJSON, err := json.Marshal(sfullHist)
	if err != nil {
		return err
	}
	if err := l.kv.KVSet(kvKeySFullHist, string(sfullJSON)); err != nil {
		return err
	}
	if err := l.kv.KVSet(kvKeyEmaUA, strconv.FormatInt(ema, 10)); err != nil {
		return err
	}
	if err := l.kv.KVSet(kvKeySamples, strconv.FormatInt(samples, 10)); err != nil {
		return err
	}
	return l.kv.KVSet(kvKeyEmaSigma, strconv.FormatFloat(sigmaMah, 'f', -1, 64))
}

func (l *Learning) loadFloats(key string, wantLen int) []float64 {
	v, ok := l.kv.KVGet(key)
	if !ok {
		return nil
	}
	var out []float64
	if json.Unmarshal([]byte(v), &out) != nil {
		return nil
	}
	if wantLen > 0 && len(out) != wantLen {
		return nil
	}
	return out
}

func (l *Learning) loadTheta() [4]float64 {
	var theta [4]float64
	copy(theta[:], l.loadFloats(kvKeyRlsTheta, 4))
	return theta
}

func (l *Learning) loadP() [4][4]float64 {
	if sym := l.loadFloats(kvKeyRlsPSym, 10); sym != nil {
		return symToP(sym)
	}
	var p [4][4]float64
	for i := range p {
		p[i][i] = rlsP0
	}
	return p
}

func mlPhi(s Session) [4]float64 {
	return [4]float64{1, float64(s.TempAvg) / 40, s.CRate, float64(s.VStart) / 4.4e6}
}

func dot4(a, b [4]float64) float64 {
	var sum float64
	for i := range a {
		sum += a[i] * b[i]
	}
	return sum
}

func matVec(p [4][4]float64, v [4]float64) [4]float64 {
	var out [4]float64
	for i := 0; i < 4; i++ {
		for j := 0; j < 4; j++ {
			out[i] += p[i][j] * v[j]
		}
	}
	return out
}

// quadForm 计算二次型 vᵀPv
func quadForm(p [4][4]float64, v [4]float64) float64 {
	return dot4(v, matVec(p, v))
}

func clipRatio(x float64) float64 {
	if x < clipLo {
		return clipLo
	}
	if x > clipHi {
		return clipHi
	}
	return x
}

func mlWeight(n int64) float64 {
	switch {
	case n <= 30:
		return 0
	case n >= 130:
		return 0.5
	default:
		return float64(n-30) / 100 * 0.5
	}
}

func rlsUpdate(theta [4]float64, p [4][4]float64, phi [4]float64, y float64) ([4]float64, [4][4]float64) {
	pphi := matVec(p, phi)
	denom := rlsLambda + dot4(phi, pphi)
	var k [4]float64
	for i := range k {
		k[i] = pphi[i] / denom
	}
	e := y - dot4(phi, theta)
	for i := range theta {
		theta[i] += k[i] * e
	}
	for i := 0; i < 4; i++ {
		for j := 0; j < 4; j++ {
			p[i][j] = (p[i][j] - k[i]*pphi[j]) / rlsLambda
		}
	}
	return theta, p
}

func pToSym(p [4][4]float64) []float64 {
	out := make([]float64, 0, 10)
	for i := 0; i < 4; i++ {
		for j := i; j < 4; j++ {
			out = append(out, p[i][j])
		}
	}
	return out
}

func symToP(sym []float64) [4][4]float64 {
	var p [4][4]float64
	idx := 0
	for i := 0; i < 4; i++ {
		for j := i; j < 4; j++ {
			p[i][j], p[j][i] = sym[idx], sym[idx]
			idx++
		}
	}
	return p
}

// histRelStd 计算比值历史的总体相对标准差（std/均值，除以 N），
// 作为 σ 的实测散布封顶。样本不足 iqrMinSamples 个或均值非正时 ok=false 表示不可用
func histRelStd(hist []float64) (rel float64, ok bool) {
	if len(hist) < iqrMinSamples {
		return 0, false
	}
	var sum float64
	for _, r := range hist {
		sum += r
	}
	mean := sum / float64(len(hist))
	if mean <= 0 {
		return 0, false
	}
	var ss float64
	for _, r := range hist {
		d := r - mean
		ss += d * d
	}
	return math.Sqrt(ss/float64(len(hist))) / mean, true
}

func iqrBounds(hist []float64) (float64, float64) {
	s := append([]float64(nil), hist...)
	sort.Float64s(s)
	q1 := quantileSorted(s, 0.25)
	q3 := quantileSorted(s, 0.75)
	iqr := q3 - q1
	return q1 - 1.5*iqr, q3 + 1.5*iqr
}

func quantileSorted(xs []float64, q float64) float64 {
	pos := q * float64(len(xs)-1)
	lo := int(pos)
	if lo >= len(xs)-1 {
		return xs[len(xs)-1]
	}
	frac := pos - float64(lo)
	return xs[lo] + frac*(xs[lo+1]-xs[lo])
}
