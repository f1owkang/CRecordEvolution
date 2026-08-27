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
