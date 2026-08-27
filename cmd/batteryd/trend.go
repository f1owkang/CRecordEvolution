// 趋势外推采用日级容量稳健线性拟合（Theil-Sen，点对斜率中位数，抗离群）；
// 思想溯源 Severson et al., Nature Energy 2019（A级）「早期数据预测长期衰减」，
// 斜率特征用法属其后续工作 arXiv 2312.05717。

package main

import "sort"

const (
	trendMinPts     = 8
	trendMinSpanDay = 21
	trendMinR2      = 0.5
)

type TrendResult struct {
	MahPerWeek float64
	R2         float64
}

// FitTrend 对容量估计点做 Theil-Sen 稳健线性拟合：x=(ts-first)/86400 天，斜率×7 折算为
// 每周衰减量。中位数斜率对偶发离群点不敏感（OLS 会被单个坏点拉偏）。返回的
// MahPerWeek 与入参 V 同单位（estimates 表存 µAh，调用方需自行 ÷1000 得 mAh，
// 见 main.go stats() 装配）。bool=false 表示任一门控不过：
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

	t0 := sorted[0].TS
	xs := make([]float64, n)
	ys := make([]float64, n)
	for i, p := range sorted {
		xs[i] = float64(p.TS-t0) / 86400
		ys[i] = float64(p.V)
	}

	// Theil-Sen：所有点对斜率取中位数。
	var slopes []float64
	for i := 0; i < n; i++ {
		for j := i + 1; j < n; j++ {
			dx := xs[j] - xs[i]
			if dx == 0 {
				continue
			}
			slopes = append(slopes, (ys[j]-ys[i])/dx)
		}
	}
	if len(slopes) == 0 {
		return TrendResult{}, false
	}
	sort.Float64s(slopes)
	slopeDay := slopes[len(slopes)/2]

	// 截距取 y−slope·x 的中位数。
	ints := make([]float64, n)
	for i := range xs {
		ints[i] = ys[i] - slopeDay*xs[i]
	}
	sort.Float64s(ints)
	intercept := ints[len(ints)/2]

	var sse, sst float64
	meanY := 0.0
	for _, y := range ys {
		meanY += y
	}
	meanY /= float64(n)
	for i := range xs {
		y := ys[i]
		pred := intercept + slopeDay*xs[i]
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
