// 趋势外推采用日级容量线性拟合；思想溯源 Severson et al., Nature Energy 2019（A级）
// 「早期数据预测长期衰减」，斜率特征用法属其后续工作 arXiv 2312.05717。

package main

const (
	trendMinPts     = 8
	trendMinSpanDay = 21
	trendMinR2      = 0.5
)

type TrendResult struct {
	MahPerWeek float64
	R2         float64
}

// FitTrend 对容量估计点做最小二乘线性拟合：x=(ts-first)/86400 天，斜率×7 折算为
// 每周衰减量。返回的 MahPerWeek 与入参 V 同单位（estimates 表存 µAh，调用方需自行
// ÷1000 得 mAh，见 main.go stats() 装配）。bool=false 表示任一门控不过：
// 点数 < trendMinPts / 跨度 < trendMinSpanDay 天 / R² < trendMinR2（SST=0 视为平坦通过 R²=1）。
// 入参任意序，函数内部按 ts 升序整理。
func FitTrend(pts []TsVal) (TrendResult, bool) {
	n := len(pts)
	if n < trendMinPts {
		return TrendResult{}, false
	}
	sorted := make([]TsVal, n)
	copy(sorted, pts)
	for i := 1; i < n; i++ {
		for j := i; j > 0 && sorted[j].TS < sorted[j-1].TS; j-- {
			sorted[j], sorted[j-1] = sorted[j-1], sorted[j]
		}
	}
	spanDay := (sorted[n-1].TS - sorted[0].TS) / 86400
	if spanDay < trendMinSpanDay {
		return TrendResult{}, false
	}

	var sx, sy, sxx, sxy float64
	t0 := sorted[0].TS
	for _, p := range sorted {
		x := float64(p.TS-t0) / 86400
		y := float64(p.V)
		sx += x
		sy += y
		sxx += x * x
		sxy += x * y
	}
	fn := float64(n)
	den := fn*sxx - sx*sx
	if den == 0 {
		return TrendResult{}, false
	}
	slopeDay := (fn*sxy - sx*sy) / den
	intercept := (sy - slopeDay*sx) / fn

	var sse, sst float64
	meanY := sy / fn
	for _, p := range sorted {
		x := float64(p.TS-t0) / 86400
		y := float64(p.V)
		pred := intercept + slopeDay*x
		sse += (y - pred) * (y - pred)
		sst += (y - meanY) * (y - meanY)
	}
	r2 := 1.0
	if sst != 0 {
		r2 = 1 - sse/sst
	}
	if r2 < trendMinR2 {
		return TrendResult{}, false
	}
	return TrendResult{MahPerWeek: slopeDay * 7, R2: r2}, true
}
