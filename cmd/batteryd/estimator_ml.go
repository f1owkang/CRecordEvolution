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

	rlsLambda     = 0.99
	rlsP0         = 1e4
	ratioHistCap  = 40
	iqrMinSamples = 8
	clipLo        = 0.7
	clipHi        = 1.3

	kvKeyEmaSigma = "ema_sigma_mah"
	sigmaRelFloor = 0.005
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

	corrected := float64(ema) * clipRatio(dot4(phi, theta))
	w := mlWeight(samples)
	est := int64((1-w)*float64(ema) + w*corrected)

	// RLS 预测方差 σ_rel = √(φᵀPφ)（rlsUpdate 之后的新 P），下限保护 0.005（0.5%）；
	// σ_mAh = σ_rel × 校正容量/1000，mL→mAh 域换算
	sigmaRel := math.Sqrt(quadForm(p, phi))
	if sigmaRel < sigmaRelFloor {
		sigmaRel = sigmaRelFloor
	}
	sigmaMah := sigmaRel * corrected / 1000

	samples++

	hist = append(hist, r)
	if len(hist) > ratioHistCap {
		hist = hist[len(hist)-ratioHistCap:]
	}

	if err := l.persist(theta, p, hist, est, samples, sigmaMah); err != nil {
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

func (l *Learning) persist(theta [4]float64, p [4][4]float64, hist []float64, ema, samples int64, sigmaMah float64) error {
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
