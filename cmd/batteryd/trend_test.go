package main

import "testing"

func TestFitTrend(t *testing.T) {
	base := int64(1735689600) // 2025-01-01
	var rising []TsVal
	for d := 0; d < 30; d++ {
		rising = append(rising, TsVal{TS: base + int64(d)*86400, V: 4_500_000 - int64(d)*3000})
	}
	tr, ok := FitTrend(rising)
	if !ok || tr.State != TrendSignificant {
		t.Fatal("30天单调线性降应为显著趋势")
	}
	if tr.MahPerWeek > -19_000 || tr.MahPerWeek < -22_000 {
		t.Fatalf("斜率每周约 -21 mAh, got %.0f", tr.MahPerWeek)
	}
	tr5, ok := FitTrend(rising[:5])
	if ok || tr5.State != TrendInsufficient {
		t.Fatal("点数不足应为 insufficient")
	}
	if tr5.SpanDay != 4 {
		t.Fatalf("insufficient 应携带跨度天数, got %d", tr5.SpanDay)
	}
	short := []TsVal{{TS: base, V: 4_500_000}, {TS: base + 40*86400, V: 4_400_000}}
	if _, ok := FitTrend(short); ok {
		t.Fatal("跨度不足应拒绝")
	}
	var flat []TsVal
	for d := 0; d < 8; d++ {
		flat = append(flat, TsVal{TS: base + int64(d)*2*86400, V: 4_500_000})
	}
	if tr, ok := FitTrend(flat); ok || tr.State != TrendInsufficient {
		t.Fatal("跨度不足应为 insufficient")
	}
}

// 交替噪声（无净衰减）数据足够但序关系不显著 ⇒ flat，区别于「还在积累」。
func TestFitTrendStableNoise(t *testing.T) {
	base := int64(1735689600)
	var noisy []TsVal
	for d := 0; d < 24; d++ {
		v := int64(4_500_000)
		if d%2 == 0 {
			v += 400_000
		} else {
			v -= 400_000
		}
		noisy = append(noisy, TsVal{TS: base + int64(d)*86400, V: v})
	}
	tr, ok := FitTrend(noisy)
	if ok || tr.State != TrendStable {
		t.Fatalf("无净衰减的交替噪声应为 stable, got state=%s ok=%v", tr.State, ok)
	}
	// 全等值序列：斜率为 0、S=0，同样是平稳而非趋势
	var flat []TsVal
	for d := 0; d < 25; d++ {
		flat = append(flat, TsVal{TS: base + int64(d)*86400, V: 5_200_000})
	}
	if tr, ok := FitTrend(flat); ok || tr.State != TrendStable {
		t.Fatalf("全等值应为 stable, got state=%s ok=%v", tr.State, ok)
	}
}

// 真实信噪比回归：以实测设备量级（µAh 值 + 百 mAh 级噪声）验证健康电池
// 不应被判为显著趋势——R² 门控时代这类数据是永久拒绝，现应落为 flat。
func TestFitTrendRealisticNoiseIsFlat(t *testing.T) {
	vals := []int64{5031321, 5248992, 5390526, 5187214, 5194461, 5128611,
		5025493, 4873371, 5018747, 5077712, 5118972, 5105287, 5280233,
		5276404, 5200105, 5301036, 5263403, 5234970, 5180247, 5217032, 5307552}
	base := int64(1787838387)
	pts := make([]TsVal, len(vals))
	for i, v := range vals {
		pts[i] = TsVal{TS: base + int64(i)*19*3600, V: v}
	}
	// 跨度 19×21h ≈ 16.6 天不足 21 天 ⇒ insufficient；拉宽到 21 天整（20 间隔 × 90720s）再验 stable
	for i := range pts {
		pts[i].TS = base + int64(i)*90_720
	}
	tr, ok := FitTrend(pts)
	if ok || tr.State != TrendStable {
		t.Fatalf("实测噪声量级应为 stable, got state=%s ok=%v", tr.State, ok)
	}
	if tr.MahPerWeek > 0 {
		t.Logf("注意：拟合斜率为正（噪声伪趋势 %.1f µAh/周），未达显著不展示", tr.MahPerWeek)
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
	// Theil-Sen 中位数斜率不受影响（含该点的点对斜率中位数仍被纯基线点对压住），
	// 序关系显著性（Mann-Kendall）也不被单点翻转。
	pts[24].V += 200_000

	tr, ok := FitTrend(pts)
	if !ok || tr.State != TrendSignificant {
		t.Fatal("陡峭单调降 + 单离群点应仍为显著趋势")
	}
	// 真值 -140000/周，±5% 窗口：OLS 会给出 -120000（出窗），Theil-Sen 保持 -140000。
	if tr.MahPerWeek > -133_000 || tr.MahPerWeek < -147_000 {
		t.Fatalf("离群点不应拉偏稳健斜率, got %.0f", tr.MahPerWeek)
	}
}
