// ICA（Incremental Capacity Analysis，增量容量分析）主峰追踪 MVP：把会话样本
// 按 ΔV=10 mV 网格分桶累计 ΔQ=I·Δt，对桶密度序列做 5 点滑动平均后在搜索域内
// 取最大者为绝对峰高。文献综述（MDPI Energies 2024，B 级）明言 ICA 无标准化
// 计算方法：分辨率、滤波与显著性参数均系自主选定，实现取舍在此备注。
//
// 纯函数约束：FindPeak 只做数值计算；峰高 rel 化依赖 kv 基准（ica_peak_base），
// 由调用方 recordICA 结合存储层完成，保持本文件无 IO。

package main

const (
	icaBinUV         = 10_000    // ΔV 分桶网格宽（µV），自主选定
	icaSmoothHalf    = 2         // 滑动平均半窗宽，窗口 = 2×2+1 = 5 点（自主选定）
	icaSearchLoUV    = 3_500_000 // 主峰搜索域下沿（µV）
	icaSearchHiUV    = 4_250_000 // 主峰搜索域上沿（µV）
	icaMinProminence = 1.20      // 显著性门槛：平滑峰值须超域内均值×此系数（自主选定）
	kvICAPeakBase    = "ica_peak_base"
)

// FindPeak 在样本行上累计 ΔQ 密度并定位主峰。返回峰所在桶左沿电压 peakUV 与
// 绝对峰高（平滑后最大 dQ/dV 桶值）；无显著主峰或输入不足时 ok=false。
//
// 细节取舍：
//   - ΔQ 归入目标样本所在电压桶，跨桶大步进整段计入到达桶（60s 采样下粗化可接受）；
//   - 仅累计并统计搜索域 [icaSearchLoUV, icaSearchHiUV) 内的桶，域内空洞计 0；
//   - 峰值须显著高于域均值（>×icaMinProminence），平滑缓变线不产出伪峰。
func FindPeak(rows []SampleRow) (peakUV int64, peakH float64, ok bool) {
	loBin := int64(icaSearchLoUV / icaBinUV)
	nBins := int(icaSearchHiUV/icaBinUV - icaSearchLoUV/icaBinUV)
	dens := make([]float64, nBins)
	hit := false

	for i := 1; i < len(rows); i++ {
		prev, cur := rows[i-1], rows[i]
		dt := cur.TS - prev.TS
		if dt <= 0 || cur.UV < prev.UV || cur.UA < 0 {
			continue
		}
		b := cur.UV / icaBinUV
		idx := int(b - loBin)
		if idx < 0 || idx >= nBins {
			continue
		}
		dens[idx] += float64(cur.UA) * float64(dt)
		hit = true
	}
	if !hit {
		return 0, 0, false
	}

	smoothed := movingAvg(dens, icaSmoothHalf)
	maxIdx := -1
	var sum float64
	nHit := 0
	for i, v := range smoothed {
		sum += v
		if dens[i] > 0 {
			nHit++
		}
		if maxIdx < 0 || v > smoothed[maxIdx] {
			maxIdx = i
		}
	}
	if maxIdx < 0 || nHit == 0 {
		return 0, 0, false
	}
	// 均值只统计有数据（轨迹覆盖）的桶：域内未经过的空洞不计入分母，
	// 否则缓变线的首尾空窗会人为压低均值、造成伪显著。
	mean := sum / float64(nHit)
	if h := smoothed[maxIdx]; h > 0 && h > icaMinProminence*mean {
		return (loBin + int64(maxIdx)) * icaBinUV, h, true
	}
	return 0, 0, false
}

// movingAvg 中心滑动平均：两端窗口不足时按实际点数归一。
func movingAvg(xs []float64, half int) []float64 {
	out := make([]float64, len(xs))
	for i := range xs {
		lo, hi := i-half, i+half+1
		if lo < 0 {
			lo = 0
		}
		if hi > len(xs) {
			hi = len(xs)
		}
		var s float64
		for _, v := range xs[lo:hi] {
			s += v
		}
		out[i] = s / float64(hi-lo)
	}
	return out
}
