package main

import "testing"

func TestDetectCCSegs(t *testing.T) {
	var rows []SampleRow
	base := int64(1000)
	for i := 0; i < 30; i++ { // 平稳段 500mA
		rows = append(rows, SampleRow{TS: base + int64(i), UA: 500_000, UV: 3_800_000 + int64(i)*2_000, Cap: int64(i)})
	}
	for i := 30; i < 45; i++ { // CV 尾段电流阶梯下滑
		rows = append(rows, SampleRow{TS: base + int64(i), UA: 500_000 - int64(i-29)*30_000, UV: 4_350_000 + int64(i-30)*1_000, Cap: 30})
	}
	segs := DetectCCSegs(rows, 5)
	if len(segs) != 1 {
		t.Fatalf("want 1 seg, got %d", len(segs))
	}
	if segs[0].MeanUA != 500_000 {
		t.Fatalf("均值=%d", segs[0].MeanUA)
	}
	noise := []SampleRow{{TS: 1, UA: 100_000}, {TS: 2, UA: 900_000}} // 碎点不成段
	if got := DetectCCSegs(noise, 5); len(got) != 0 {
		t.Fatalf("碎点应为 0 段, got %d", got)
	}
}
