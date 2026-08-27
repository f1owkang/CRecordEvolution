// 准恒流段识别器：滑动窗电流变异系数 |σ/μ| ≤ 0.10 判平稳，相邻平稳窗同向衔接
// 合并成段；电压位于观察序列最高 3% 区间且电流较段均值下滑逾 30% 的后续窗判为
// CV 尾段剔除。恒流前提溯源 Lin 2022, Energy（A 级）；倍率门控（≤C/2）不在识
// 别器内做，由调用方持 designUA 过滤（Fly & Chen，B 级速率约束），保持纯函数。

package main

import (
	"math"
	"sort"
)

const (
	ccMaxCV       = 0.10 // 平稳判别：窗口内 |σ/μ| 上限
	ccTailDrop    = 0.30 // CV 尾段：较段均值下滑逾此比例
	ccTopVoltFrac = 0.03 // CV 尾段：电压须处观察序列最高 3% 区间
)

type SampleRow struct{ TS, UA, UV, Cap int64 }

type CCSeg struct {
	FromTs int64
	ToTs   int64
	MeanUA int64
}

// DetectCCSegs 把采样行按步长 win 切成非重叠窗，窗内电流变异系数 |σ/μ| ≤ ccMaxCV
// 判平稳；最大连续平稳窗串合并成候选段。段内自前往后首个「平均电压达全序列最高
// ccTopVoltFrac 区间、且窗平均电流低于段均值 ×(1-ccTailDrop)」的窗及其后续窗整体
// 剔除（CC-CV 转折尾段）。样本数 < win×2 的残段丢弃。结果按时间升序返回；输入不足
// 一个窗或 win 非正时返回 nil。
func DetectCCSegs(samps []SampleRow, win int) []CCSeg {
	if win <= 0 || len(samps) < win {
		return nil
	}
	nBlk := len(samps) / win
	fn := float64(win)
	stat := make([]bool, nBlk)
	meanUA := make([]float64, nBlk)
	meanUV := make([]float64, nBlk)
	for b := 0; b < nBlk; b++ {
		w := samps[b*win : (b+1)*win]
		var sa, sv float64
		for _, r := range w {
			sa += float64(r.UA)
			sv += float64(r.UV)
		}
		meanUA[b], meanUV[b] = sa/fn, sv/fn
		if mu := math.Abs(meanUA[b]); mu > 0 {
			var ss float64
			for _, r := range w {
				d := float64(r.UA) - meanUA[b]
				ss += d * d
			}
			stat[b] = math.Sqrt(ss/fn)/mu <= ccMaxCV
		}
	}

	thrv := voltTopThreshold(samps)
	segs := make([]CCSeg, 0, 4)
	b := 0
	for b < nBlk {
		if !stat[b] {
			b++
			continue
		}
		e := b
		for e+1 < nBlk && stat[e+1] {
			e++
		}
		lo, hi := b*win, (e+1)*win
		var sum float64
		for _, r := range samps[lo:hi] {
			sum += float64(r.UA)
		}
		// 基数口径：segMean 含待剔候选窗自身（brief 字面「较段均值下滑逾 30%」），
		// 长尾时会抬高基数导致偏保守多剔，属有意取舍。
		segMean := sum / float64(hi-lo)
		for i := e; i >= b; i-- { // 后续窗均为尾段：首个命中处截断
			if meanUV[i] >= thrv && meanUA[i] < segMean*(1-ccTailDrop) {
				hi = i * win
			}
		}
		if hi-lo >= win*2 {
			into := samps[lo:hi]
			var sumSeg int64
			for _, r := range into {
				sumSeg += r.UA
			}
			segs = append(segs, CCSeg{
				FromTs: into[0].TS,
				ToTs:   into[len(into)-1].TS,
				MeanUA: int64(math.Round(float64(sumSeg) / float64(len(into)))),
			})
		}
		b = e + 1
	}
	return segs
}

// voltTopThreshold 返回观察序列 UV 的最高 ccTopVoltFrac 区间下界：升序数组中
// 自顶端倒数 ceil(n×frac) 个样本，取其中的最小值。
func voltTopThreshold(samps []SampleRow) float64 {
	uvs := make([]float64, len(samps))
	for i, r := range samps {
		uvs[i] = float64(r.UV)
	}
	sort.Float64s(uvs)
	idx := len(uvs) - int(math.Ceil(float64(len(uvs))*ccTopVoltFrac))
	return uvs[idx]
}
