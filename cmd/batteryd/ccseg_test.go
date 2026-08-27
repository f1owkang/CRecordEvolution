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

// TestDetectCCSegsCVRamp 缓坡 CV 尾段：电流缓降（窗内变异系数远低于门限，CV 判据
// 拦不住）且电压升至观察序列最高 3% 区间，须由尾剔分支截断，只保留前段。
func TestDetectCCSegsCVRamp(t *testing.T) {
	var rows []SampleRow
	base := int64(2000)
	for i := 0; i < 20; i++ { // 前段平稳 500 mA 低电压
		rows = append(rows, SampleRow{TS: base + int64(i), UA: 500_000, UV: 4_000_000})
	}
	for i := 20; i < 25; i++ { // CV 尾段：缓降 + 高电压（恒值占满最高区间）
		rows = append(rows, SampleRow{TS: base + int64(i), UA: 330_000 - int64(i-20)*10_000, UV: 4_350_000})
	}
	segs := DetectCCSegs(rows, 5)
	if len(segs) != 1 {
		t.Fatalf("缓坡尾剔后应剩 1 段, got %d", len(segs))
	}
	if segs[0].MeanUA != 500_000 || segs[0].ToTs != base+19 || segs[0].FromTs != base {
		t.Fatalf("应截断至前段[0..19]: %+v", segs[0])
	}
}

// TestDetectCCSegsNoOvertrim 反向样例：电压虽升到最高 3% 区间，但电流降幅 ≤30%，
// 不满足尾剔阈值另一侧条件，不得误杀——整段完整保留。
func TestDetectCCSegsNoOvertrim(t *testing.T) {
	var rows []SampleRow
	base := int64(3000)
	for i := 0; i < 20; i++ {
		rows = append(rows, SampleRow{TS: base + int64(i), UA: 500_000, UV: 4_000_000})
	}
	for i := 20; i < 25; i++ { // 仅降约 12%（均值 440k 对 488k 基数），高电压
		rows = append(rows, SampleRow{TS: base + int64(i), UA: 460_000 - int64(i-20)*10_000, UV: 4_350_000})
	}
	segs := DetectCCSegs(rows, 5)
	if len(segs) != 1 {
		t.Fatalf("浅降不应剔除, got %d 段", len(segs))
	}
	if segs[0].MeanUA != 488_000 || segs[0].ToTs != base+24 {
		t.Fatalf("应保留全段[0..24] 均值 488000: %+v", segs[0])
	}
}

// TestDetectCCSegsLowVoltDipKept 变异锁定用例：低电压段的深凹谷（电流降逾 30%、
// 电压不达最高 3% 区间）不得触发尾剔。若「最高 3%」分位方向取反成最低分位，
// 本用例的凹谷会被误剔（ToTs 提前），据此钉住分位方向。
func TestDetectCCSegsLowVoltDipKept(t *testing.T) {
	var rows []SampleRow
	base := int64(4000)
	for i := 0; i < 20; i++ {
		rows = append(rows, SampleRow{TS: base + int64(i), UA: 500_000, UV: 4_000_000})
	}
	for i := 20; i < 25; i++ { // 凹谷：深降至 300k 但电压塌到序列最低区
		rows = append(rows, SampleRow{TS: base + int64(i), UA: 300_000, UV: 3_900_000})
	}
	for i := 25; i < 30; i++ { // 回升 500 mA
		rows = append(rows, SampleRow{TS: base + int64(i), UA: 500_000, UV: 4_000_000})
	}
	segs := DetectCCSegs(rows, 5)
	if len(segs) != 1 {
		t.Fatalf("低电压凹谷不应剔除, got %d 段", len(segs))
	}
	if segs[0].ToTs != base+29 {
		t.Fatalf("凹谷被误剔? ToTs=%d want %d", segs[0].ToTs, base+29)
	}
}
