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
	// kvKeyRatioOutlierStreak 连续被 IQR 判离群的会话数：连续离群说明基线已漂移
	// 到新工况（单位纠正/换电池/校准把 EMA 拖走），只拒收不学习会死锁——hist 与
	// EMA 都不变，之后每次会话都会同样离群。连续达 outlierEscape 次后放行一次，
	// 把该比值并入历史（不整段重置，避免在正常波动上反复「重置-离群」振荡）。
	kvKeyRatioOutlierStreak = "ratio_outlier_streak"

	rlsLambda     = 0.99
	rlsP0         = 100
	ratioHistCap  = 40
	iqrMinSamples = 8
	outlierEscape = 3
	// ratioBandMin 是 IQR 门的最小带宽（相对中位比值的全宽）。历史比值高度集中
	// （整数化 deltaCap / 固定充电习惯）时 IQR 可能收缩到接近 0，任何正常的
	// 采样抖动都会被判离群并触发离群逃逸-重置的振荡；给门一个物理下限。
	ratioBandMin = 0.06
	clipLo       = 0.7
	clipHi       = 1.3

	kvKeyEmaSigma = "ema_sigma_mah"
	sigmaRelFloor = 0.005

	// learningSamples 学习期长度：θ 训练未满时不参与输出混合（与 mlWeight
	// 的起坡点一致），但基线照常随会话移动（见 OnSession）。
	learningSamples = 30
)

// Learning 是 ML 实验版估算器：RLS 拟合会话特征到容量比，σ 由 P 矩阵与样本散布给出。
type Learning struct {
	kv        KVStore
	cellCount int
}

// ResetModel 清除 RLS 模型状态（theta、P 矩阵、历史直方图）。
// 在 cellCount 变化后调用，避免旧模型在新归一化下产生错误预测。
func (l *Learning) ResetModel() {
	_ = l.kv.KVSet(kvKeyRlsTheta, "")
	_ = l.kv.KVSet(kvKeyRlsPSym, "")
	_ = l.kv.KVSet(kvKeyRatioHist, "")
	_ = l.kv.KVSet(kvKeySFullHist, "")
	_ = l.kv.KVSet(kvKeyRatioOutlierStreak, "0")
	_ = l.kv.KVSet(kvKeySamples, "0")
}

// OnSession 结算一个充电会话：门控、离群剔除后更新 RLS 模型与 EMA 基线。
func (l *Learning) OnSession(sr SettledSession) (EstUpdate, error) {
	if tempOutOfRange(sr) {
		return EstUpdate{}, &RejectError{Result: SessionResult{Reason: "temp_out_of_range"}}
	}
	delta := sr.EndCap - sr.StartCap
	if delta < minDeltaCap {
		return EstUpdate{}, &RejectError{Result: SessionResult{Reason: "delta_lt_20"}}
	}
	sFull := fullCapacityUAh(sr, delta)
	if !capacityInWindow(sFull, sr.DesignUAh) {
		return EstUpdate{}, &RejectError{Result: SessionResult{Reason: "out_of_window"}}
	}

	samples := kvInt(l.kv, kvKeySamples)
	ema := kvInt(l.kv, kvKeyEmaUAh)
	if samples <= 0 || ema <= 0 {
		return l.seed(sFull)
	}

	hist := l.loadFloats(kvKeyRatioHist, 0)
	r := float64(sFull) / float64(ema)
	if len(hist) >= iqrMinSamples {
		lo, hi := iqrBounds(hist)
		if hi-lo < ratioBandMin {
			mid := (lo + hi) / 2
			lo, hi = mid-ratioBandMin/2, mid+ratioBandMin/2
		}
		if r < lo || r > hi {
			streak := kvInt(l.kv, kvKeyRatioOutlierStreak) + 1
			if streak < outlierEscape {
				if err := l.kv.KVSet(kvKeyRatioOutlierStreak, strconv.FormatInt(streak, 10)); err != nil {
					return EstUpdate{}, err
				}
				return EstUpdate{}, &RejectError{Result: SessionResult{Reason: "outlier"}}
			}
			// 连续离群达阈值：本次比值作为新工况样本并入历史（保留旧样本，避免
			// 整段重置后在正常波动上再次触发「重置-离群」振荡），放行本次让模型
			// 开始吸收新基线。
			if err := l.kv.KVSet(kvKeyRatioOutlierStreak, "0"); err != nil {
				return EstUpdate{}, err
			}
		}
	}
	// 本次通过门控：清零连续离群计数（单次偶发离群不跨会话累计）。
	if kvInt(l.kv, kvKeyRatioOutlierStreak) != 0 {
		_ = l.kv.KVSet(kvKeyRatioOutlierStreak, "0")
	}

	phi := mlPhi(sr.Session, l.cellCount)
	theta := l.loadTheta()
	p := l.loadP()
	theta, p = rlsUpdate(theta, p, phi, r)

	// 学习期（samples ≤ 30）θ 不参与输出，但基线经 emaBlend 随会话移动
	// （满充 3/10、未满充降权 1/10）：若整段冻结在种子值，输出在 5 个采信会话
	// 内一字不变，学习期可长达 1~2 个月，显示信息量反而少于 stable。
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
	// - 学习期（samples ≤ 30）：φᵀPφ 无预测意义，直接用样本散布；
	// - 正式期：σ_rel = √(φᵀPφ)，保持 0.005 下限，再用样本散布封顶，
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
	return EstUpdate{EstUAh: est, Samples: samples, Changed: true, SigmaMah: sigmaMah}, nil
}

func (l *Learning) seed(fullUAh int64) (EstUpdate, error) {
	if err := l.kv.KVSet(kvKeyEmaUAh, strconv.FormatInt(fullUAh, 10)); err != nil {
		return EstUpdate{}, err
	}
	if err := l.kv.KVSet(kvKeySamples, "1"); err != nil {
		return EstUpdate{}, err
	}
	if err := l.kv.KVSet(kvKeyEmaSigma, "0"); err != nil {
		return EstUpdate{}, err
	}
	return EstUpdate{EstUAh: fullUAh, Samples: 1, Changed: true}, nil
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
	if err := l.kv.KVSet(kvKeyEmaUAh, strconv.FormatInt(ema, 10)); err != nil {
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

func mlPhi(s Session, cellCount int) [4]float64 {
	vNorm := 4.4e6 * float64(cellCount)
	vFeat := float64(s.VStart) / vNorm
	// VStart=0 表示电压读取失败，用中位值 0.9 作为 fallback 避免污染特征向量
	if s.VStart == 0 {
		vFeat = 0.9
	}
	return [4]float64{1, float64(s.TempAvg) / 40, s.CRate, vFeat}
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
// 作为 σ 的样本散布封顶。样本不足 iqrMinSamples 个或均值非正时 ok=false 表示不可用
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
