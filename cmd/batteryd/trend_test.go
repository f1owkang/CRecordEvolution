package main

import "testing"

func TestFitTrend(t *testing.T) {
	var rising []TsVal
	base := int64(1735689600) // 2025-01-01
	for d := 0; d < 30; d++ {
		rising = append(rising, TsVal{TS: base + int64(d)*86400, V: 4_500_000 - int64(d)*3000})
	}
	tr, ok := FitTrend(rising)
	if !ok {
		t.Fatal("30天线性降应通过门控")
	}
	if tr.MahPerWeek > -19_000 || tr.MahPerWeek < -22_000 {
		t.Fatalf("斜率每周约 -21 mAh, got %.0f", tr.MahPerWeek)
	}
	if _, ok := FitTrend(rising[:5]); ok {
		t.Fatal("点数不足应拒绝")
	}
	short := []TsVal{{TS: base, V: 4_500_000}, {TS: base + 40*86400, V: 4_400_000}}
	if _, ok := FitTrend(short); ok {
		t.Fatal("跨度不足应拒绝")
	}
	var flat []TsVal
	for d := 0; d < 8; d++ {
		flat = append(flat, TsVal{TS: base + int64(d)*2*86400, V: 4_500_000})
	}
	if _, ok := FitTrend(flat); ok {
		t.Fatal("跨度不足应拒绝")
	}
	var noisy []TsVal
	for d := 0; d < 24; d++ {
		v := int64(4_500_000) - int64(d)*1000
		if d%2 == 0 {
			v += 400_000
		} else {
			v -= 380_000
		}
		noisy = append(noisy, TsVal{TS: base + int64(d)*86400, V: v})
	}
	if _, ok := FitTrend(noisy); ok {
		t.Fatal("R² 过低应拒绝")
	}
}

func TestFitTrendRobustToOutlier(t *testing.T) {
	base := int64(1735689600)
	var pts []TsVal
	// 25 天每天 -20000 µAh（每周 -140000）；真实斜率陡峭，便于离群点与基线区分。
	for d := 0; d < 25; d++ {
		pts = append(pts, TsVal{TS: base + int64(d)*86400, V: 4_500_000 - int64(d)*20_000})
	}
	// 末端高杠杆点注入 +200000 µAh：单点让 OLS 斜率拉到约 -18400/天（周 -128800），
	// 而 Theil-Sen 中位数斜率不受影响（含该点的点对斜率中位数仍被纯基线点对压住）。
	// 此偏移量 R²≈0.83 仍通过门槛，正是「门控放行但 OLS 失真」的区间。
	pts[24].V += 200_000

	tr, ok := FitTrend(pts)
	if !ok {
		t.Fatal("R²≈0.85 应通过门控")
	}
	// 真值 -140000/周，±5% 窗口：OLS 会给出 -120000（出窗），Theil-Sen 保持 -140000。
	if tr.MahPerWeek > -133_000 || tr.MahPerWeek < -147_000 {
		t.Fatalf("离群点不应拉偏稳健斜率，got %.0f", tr.MahPerWeek)
	}
}
