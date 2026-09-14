// 趋势外推采用日级容量稳健线性拟合（Theil-Sen，点对斜率中位数，抗离群），
// 对模块自身的历史估算点外推每周衰减量，属通用稳健统计。
//
// 显著性判定用 Mann-Kendall 点对符号检验替代早先的 R²≥0.5 门控：单次会话
// 估算噪声（健康电池 ±2~3%，百 mAh 级）远大于真实周衰减（1~2 mAh 量级），
// 实测数据上 R² 长期为负值，该门控等于趋势永远不可达。Mann-Kendall 只问
// 「序关系是否显著单调」，与斜率估计（同为点对统计）天然同源。
//
// 结果分三态上报：insufficient（点数/跨度不足，尚无法判定）、stable（数据
// 足够但无显著变化）、significant（显著变化，方向与速率由斜率给出）；前端
// 据此直接回答「容量趋势如何」，避免把判定过程术语暴露给用户。

package main

import (
	"math"
	"sort"
)

const (
	trendMinPts     = 8
	trendMinSpanDay = 21
	trendMKZ        = 1.96 // Mann-Kendall 双侧 5% 显著性临界值
)

type TrendState string

const (
	TrendInsufficient TrendState = "insufficient"
	TrendStable       TrendState = "stable"
	TrendSignificant  TrendState = "significant"
)

type TrendResult struct {
	MahPerWeek float64
	R2         float64
	State      TrendState
	SpanDay    int64
}

// FitTrend 对容量估计点做 Theil-Sen 稳健线性拟合：x=(ts-first)/86400 天，斜率×7 折算为
// 每周衰减量。中位数斜率对偶发离群点不敏感（OLS 会被单个坏点拉偏）。返回的
// MahPerWeek 与入参 V 同单位（estimates 表存 µAh，调用方需自行 ÷1000 得 mAh，
// 见 main.go stats() 装配）。bool=false 表示未达显著趋势：
// 点数 < trendMinPts / 跨度 < trendMinSpanDay 天 ⇒ State=insufficient；
// 数据足够但 Mann-Kendall |S| ≤ 1.96·√Var ⇒ State=stable。
// 入参任意序，函数内部按 ts 升序整理。
func FitTrend(pts []TsVal) (TrendResult, bool) {
	n := len(pts)
	if n == 0 {
		return TrendResult{}, false
	}
	sorted := make([]TsVal, n)
	copy(sorted, pts)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].TS < sorted[j].TS })
	spanDay := (sorted[n-1].TS - sorted[0].TS) / 86400
	if n < trendMinPts || spanDay < trendMinSpanDay {
		return TrendResult{State: TrendInsufficient, SpanDay: spanDay}, false
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
	mkS := 0
	for i := 0; i < n; i++ {
		for j := i + 1; j < n; j++ {
			dx := xs[j] - xs[i]
			if dx == 0 {
				continue
			}
			slopes = append(slopes, (ys[j]-ys[i])/dx)
			switch {
			case ys[j] > ys[i]:
				mkS++
			case ys[j] < ys[i]:
				mkS--
			}
		}
	}
	if len(slopes) == 0 {
		return TrendResult{State: TrendInsufficient, SpanDay: spanDay}, false
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

	// Mann-Kendall 显著性：Var(S) 取无并列近似（实测并列对结果只有保守方向的
	// 影响），|S| 超过双侧 5% 临界值才认定趋势成立。
	state := TrendStable
	mkVar := float64(n*(n-1)*(2*n+5)) / 18
	if float64(absI64(int64(mkS))) > trendMKZ*math.Sqrt(mkVar) {
		state = TrendSignificant
	}
	return TrendResult{MahPerWeek: slopeDay * 7, R2: r2, State: state, SpanDay: spanDay}, state == TrendSignificant
}
