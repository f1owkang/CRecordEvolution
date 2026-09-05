// CCCT（固定电压窗恒流充电时长）特征：取唯一「自窗下沿以下起步、越过窗上沿」
// 的恒流段，段内首个 uv≥WinLo 与首个 uv≥WinHi 两点的时长即穿窗耗时，机制上等价
// 于该窗内充入的电量 Q=I·t。窗口取 0.1 V 格点（Lin 2022 原文口径），位置经两台
// 实测设备全会话扫描定标在 4.2~4.3V：3.9~4.0V 落在低电量区，日常补电会话起点
// 普遍高于窗上沿（belowLo 永不成立），而 4.2~4.3V 被任何充至 ~80% 以上的会话
// 跨越且跨窗段平稳。倍率门控（≤1C）由调用方持 designUA 过滤，本文件保持纯函数。

package main

const (
	ccctWinLo  = 4_200_000 // 观察窗下沿（µV）
	ccctWinHi  = 4_300_000 // 观察窗上沿（µV）
	ccctSegWin = 5         // DetectCCSegs 切窗步长（样本数）
)

// locateWindowCross 在 seg 时间范围内扫描 rows：判定是否「跨越整窗」——先有
// uv<WinLo 的起步样本，随后升到首个 uv≥WinLo（出下沿）与首个 uv≥WinHi（过
// 上沿）。命中时返回这两个穿越点；否则 crossed=false。
func locateWindowCross(rows []SampleRow, seg CCSeg) (lo, hi SampleRow, crossed bool) {
	var belowLo bool
	var haveLo bool
	for _, r := range rows {
		if r.TS < seg.FromTs || r.TS > seg.ToTs {
			continue
		}
		if !haveLo {
			if r.UV < ccctWinLo {
				belowLo = true
			} else if r.UV >= ccctWinLo {
				lo = r
				haveLo = true
			}
			continue
		}
		if r.UV >= ccctWinHi && r.TS > lo.TS {
			return lo, r, belowLo
		}
	}
	return SampleRow{}, SampleRow{}, false
}

// rowsInRange 返回 rows 中落在 seg [FromTs, ToTs] 闭区间内的子集（保持升序），
// 供同源特征（ICA）复用恒流段覆盖样本。
func rowsInRange(rows []SampleRow, seg CCSeg) []SampleRow {
	out := []SampleRow{}
	for _, r := range rows {
		if r.TS >= seg.FromTs && r.TS <= seg.ToTs {
			out = append(out, r)
		}
	}
	return out
}

// AnalyzeCCCT 求唯一跨越整窗 [ccctWinLo, ccctWinHi] 的恒流段穿窗耗时。
// 0 或 ≥2 个跨界段均返回 ok=false（无可采信样本 / 无法唯一归因）。
// 注意：belowLo 只在 lo 锚定前累计，出下沿后的回落不参与判定。
func AnalyzeCCCT(rows []SampleRow, segs []CCSeg) (secs int64, ok bool) {
	crossings := 0
	var lo, hi SampleRow
	for _, g := range segs {
		l, h, cross := locateWindowCross(rows, g)
		if !cross {
			continue
		}
		crossings++
		lo, hi = l, h
	}
	if crossings != 1 || hi.TS <= lo.TS {
		return 0, false
	}
	return hi.TS - lo.TS, true
}
