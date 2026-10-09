// 中段分窗容量校准：对历史充电样本按「1% 显示百分点」分桶、按穿越次数
// （pass）归一，聚合出 [30,90)% 窗口的隐含满容量，作为低权校准量并入 EMA。
//
// 依据：电量计的百分比→mAh 映射在中段诚实（本机实测 30~90/40~90/50~90/
// 60~95 四窗独立算出 5289~5311 mAh 高度一致），顶部 95~98% 每百分点仅交付
// 中段一半（满充会话反推天然保守 ~3%）；跨多次充电聚合的方差远低于单会话。
// 文献中部分充电数据分析（ICA/SVR）亦有先例（MDPI Energies 2024 综述及其
// 引文）。收益：无需满充也能产出高质量容量估计，且不受顶部压缩影响。
//
// 纪律与 ica.go 一致：MidImplied 为无 IO 纯函数；装配（查库、kv 门控、EMA
// 混合、日志/events 落痕）在 Pipeline.calibrateMid 完成。

package main

import (
	"fmt"
	"strconv"
)

const (
	midWinLo    = 30  // 窗口下沿（显示百分点），避开低端保留电量映射失真
	midWinHi    = 90  // 窗口上沿（显示百分点），排除顶部压缩段（95~98% 减半）
	midSegMaxDt = 300 // 段内相邻样本最大间隔（秒），超出视为段断裂（重启/停充）
	// midSegMaxUAh 区间电量计增量上限：≤300s 段内即便 12A 快充也只有 1Ah，
	// 超过判为电量计回修/跳变，该区间无法归因（跳过）。
	midSegMaxUAh  int64 = 2_000_000
	midMinBinPass       = 3     // 每个窗格的最少穿越次数，低于此判覆盖不足、不校准
	midCalGapSecs       = 86400 // 校准最小间隔：至多每日一次，避免重复计入
	kvMidCalTs          = "mid_cal_ts"
	// kvMidImpliedUA 最近一次通过门控的中段隐含容量（µAh），供通道互检
	kvMidImpliedUA = "mid_implied_ua"
)

// MidImplied 聚合充电样本并返回 pass 归一的中段隐含满容量（与 EstUA 同单位，
// 即 µAh 量级数值）。判定口径：
//   - 样本按时间连续（相邻间隔 ≤ midSegMaxDt）且电流为正切成一段；
//   - 段内区间电量优先取电量计差分（charge_counter，µAh 直读，与放电侧/内核
//     charge_full 同源，免疫 current_now 刻度偏差）；旧行无 cc 时回退电流积分
//     Q=I·dt÷3600（µA·s→µAh），两者统一到 µAh 可混用；
//   - 区间电量均摊到跨越的 1% 窗格（c1>c0）；c1<c0（电量计回修）、dt≤0 与
//     增量超限的区间无法归因，跳过；百分比停滞的区间归入当前窗格（若在窗内）；
//   - 窗口内任一窗格穿越次数 < midMinBinPass ⇒ 覆盖不足，ok=false（防止只用
//     局部区间充电的习惯把窗口算偏）；
//   - 隐含容量 = ΣQ/Σpass × 100% 折算（Q 已是 µAh，无需再除以 3600）。
func MidImplied(rows []SampleRow, lo, hi int) (int64, bool) {
	var binQ [100]float64
	var binP [100]int

	n := len(rows)
	for i := 0; i < n; {
		if rows[i].UA <= 0 {
			i++
			continue
		}
		j := i + 1
		for j < n && rows[j].TS-rows[j-1].TS <= midSegMaxDt && rows[j].UA > 0 {
			j++
		}
		if seg := rows[i:j]; len(seg) >= 2 {
			segQ := map[int64]float64{}
			for k := 1; k < len(seg); k++ {
				dt := seg[k].TS - seg[k-1].TS
				c0, c1 := seg[k-1].Cap, seg[k].Cap
				if dt <= 0 || c1 < c0 {
					continue
				}
				var q float64 // µAh
				if cc0, cc1 := seg[k-1].CC, seg[k].CC; cc0 > 0 && cc1 > 0 {
					d := cc1 - cc0
					if d <= 0 || d > midSegMaxUAh {
						continue // 电量计回修/跳变：区间无法归因
					}
					q = float64(d)
				} else {
					q = float64(seg[k-1].UA) * float64(dt) / 3600
				}
				if c1 > c0 {
					for c := c0; c < c1 && c < int64(hi); c++ {
						if c >= int64(lo) && c >= 0 && c < 100 {
							segQ[c] += q / float64(c1-c0)
						}
					}
				} else if c0 >= int64(lo) && c0 < int64(hi) {
					segQ[c0] += q
				}
			}
			for c, q := range segQ {
				binQ[c] += q
				binP[c]++
			}
		}
		i = j
	}

	var sumQ, sumP float64
	for c := int64(lo); c < int64(hi); c++ {
		if binP[c] < midMinBinPass {
			return 0, false
		}
		sumQ += binQ[c]
		sumP += float64(binP[c])
	}
	if sumP <= 0 {
		return 0, false
	}
	// (µAh/窗格/次) × 100% = µAh 量级的隐含满容量
	implied := sumQ / sumP * 100
	return int64(implied), true
}

// calibrateMid 结算采信后的中段校准装配：至多每日一次；EMA 未初始化或覆盖
// 不足时静默跳过。混合沿用部分会话权重（1/10），量级保守且与既有语义一致。
// 任何失败只落 events，绝不影响结算主链路。
func (p *Pipeline) calibrateMid(now int64) {
	if now-kvInt(p.st, kvMidCalTs) < midCalGapSecs {
		return
	}
	ema := kvInt(p.st, kvKeyEmaUA)
	if ema <= 0 {
		return
	}
	rows, err := p.st.SamplesRange(0, now)
	if err != nil {
		_ = p.st.InsertEvent("mid_cal_fail", err.Error())
		return
	}
	mid, ok := MidImplied(rows, midWinLo, midWinHi)
	if !ok || mid <= 0 {
		return
	}
	// 与会话通道同口径的合理性门控：中段隐含值同样由电流积分折算而来，会
	// 同步吃到量纲误判/脏样本（实测被 1000 倍误乘的样本推到 15592mAh，比设计
	// 容量还高 1.5 倍），不设门则这条旁路能把无界垃圾灌进用户可见的实测值。
	if !capacityInWindow(mid, p.designUA) {
		p.log("[校准] 中段分窗(%d~%d%%) 隐含=%dmAh 超出设计容量窗口，不采信",
			midWinLo, midWinHi, mid/1000)
		_ = p.st.InsertEvent("mid_cal_skip", fmt.Sprintf("mid=%d 超出窗口", mid))
		return
	}
	blended := emaBlend(ema, mid, 0)
	if err := p.st.KVSet(kvKeyEmaUA, strconv.FormatInt(blended, 10)); err != nil {
		_ = p.st.InsertEvent("mid_cal_fail", err.Error())
		return
	}
	// 留给通道互检（checkQuality）作对照口径
	_ = p.st.KVSet(kvMidImpliedUA, strconv.FormatInt(mid, 10))
	_ = p.st.KVSet(kvMidCalTs, strconv.FormatInt(now, 10))
	p.log("[校准] 中段分窗(%d~%d%%) 隐含=%dmAh，按1/10权重并入（%d→%d）",
		midWinLo, midWinHi, mid/1000, ema/1000, blended/1000)
	_ = p.st.InsertEvent("mid_cal", fmt.Sprintf("mid=%d ema=%d→%d", mid, ema, blended))
}
