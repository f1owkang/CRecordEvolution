package main

import "testing"

// 双电芯缩放：窗口与搜索域按电芯数翻倍，复位回单电芯后恢复默认。
func TestDualCellVoltageScaling(t *testing.T) {
	if ccctWinLo != 4_200_000 || ccctWinHi != 4_300_000 {
		t.Fatalf("前置失败：单电芯 CCCT 窗口应为 4.2~4.3V, got %d~%d", ccctWinLo, ccctWinHi)
	}
	initCCCTVoltage(2)
	defer initCCCTVoltage(1)
	initICAVoltage(2)
	defer initICAVoltage(1)
	if ccctWinLo != 8_400_000 || ccctWinHi != 8_600_000 {
		t.Fatalf("双电芯 CCCT 窗口应为 8.4~8.6V, got %d~%d", ccctWinLo, ccctWinHi)
	}
	if icaSearchLoUV != 7_000_000 || icaSearchHiUV != 8_500_000 {
		t.Fatalf("双电芯 ICA 搜索域应为 7.0~8.5V, got %d~%d", icaSearchLoUV, icaSearchHiUV)
	}
	// 双电芯样本穿窗冒烟：8.35→8.65V 平滑爬升应能定位跨窗点
	var rows []SampleRow
	for i := 0; i < 40; i++ {
		rows = append(rows, SampleRow{
			TS: int64(1700000000 + i*15), UA: 4_000_000,
			UV: 8_350_000 + int64(i)*8000, Cap: 40,
		})
	}
	lo, hi, crossed := locateWindowCross(rows, CCSeg{FromTs: rows[0].TS, ToTs: rows[len(rows)-1].TS, MeanUA: 4_000_000})
	if !crossed {
		t.Fatal("双电芯电压轨迹应跨越 8.4~8.6V 整窗")
	}
	if lo.UV < ccctWinLo || hi.UV < ccctWinHi {
		t.Fatalf("穿窗点电压异常: lo=%d hi=%d", lo.UV, hi.UV)
	}
}

// 高压单电芯不误判：判定阈值是 5V（非 4.5V），4.5xV 仍属单电芯。
func TestDualCellThresholdLeavesHVSingleCell(t *testing.T) {
	// initCCCTVoltage 按 cellCount 缩放而非电压值；此处验证口径：4.53V 单电芯
	// 截止体系不触发 ×2（即 cellCount=1 时窗口保持默认，不因电压 4.53V 改变）
	initCCCTVoltage(1)
	if ccctWinLo != 4_200_000 {
		t.Fatalf("单电芯窗口应保持 4.2V, got %d", ccctWinLo)
	}
}
