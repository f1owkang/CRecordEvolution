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
	// kvMidImpliedUAh 最近一次通过门控的中段隐含容量（µAh），供通道互检
	kvMidImpliedUAh = "mid_implied_ua"
)

// midAccum 为中段校准的累计器：按显示百分点窗格累计电量（µAh）与穿越次数。
type midAccum struct {
	q    [100]float64
	pass [100]int
}

// add 并入一段的窗格归因；bad 窗格 Q 与 pass 都不计（该窗格只覆盖了一部分，
// 若仍记一整次穿越会把均值拉低）。
func (a *midAccum) add(seg *midSegQ) {
	for c, q := range seg.q {
		if seg.bad[c] {
			continue
		}
		a.q[c] += q
		a.pass[c]++
	}
}

// implied 校验窗格覆盖并按 pass 归一为隐含满容量：任一窗格穿越次数低于
// midMinBinPass 即判覆盖不足。Q 已是 µAh，(µAh/窗格/次) × 100% 即 µAh 量级。
func (a *midAccum) implied(lo, hi int) (int64, bool) {
	var sumQ, sumP float64
	for c := int64(lo); c < int64(hi); c++ {
		if a.pass[c] < midMinBinPass {
			return 0, false
		}
		sumQ += a.q[c]
		sumP += float64(a.pass[c])
	}
	if sumP <= 0 {
		return 0, false
	}
	return int64(sumQ / sumP * 100), true
}

// midSegQ 为一段连续样本的窗格归因：q 为窗格累计电量（µAh），bad 标记该段内
// 只因缺电量计读数而覆盖不全的窗格。
type midSegQ struct {
	q   map[int64]float64
	bad map[int64]bool
}

// attribute 把区间电量 q 均摊到跨越的 1% 窗格（c1>c0）；百分比停滞的区间整段
// 归入当前窗格（若在窗内）。
func (sg *midSegQ) attribute(c0, c1 int64, q float64, lo, hi int) {
	if c1 > c0 {
		share := q / float64(c1-c0)
		for c := c0; c < c1 && c < int64(hi); c++ {
			if c >= int64(lo) && c >= 0 && c < 100 {
				sg.q[c] += share
			}
		}
		return
	}
	if c0 >= int64(lo) && c0 < int64(hi) {
		sg.q[c0] += q
	}
}

// markBad 标记 [c0,c1) 内覆盖不全的窗格；百分比停滞（c1<=c0）时只标 c0。
func (sg *midSegQ) markBad(c0, c1 int64, lo, hi int) {
	if c1 > c0 {
		for c := c0; c < c1 && c < int64(hi); c++ {
			if c >= int64(lo) && c >= 0 && c < 100 {
				sg.bad[c] = true
			}
		}
		return
	}
	if c0 >= int64(lo) && c0 < int64(hi) {
		sg.bad[c0] = true
	}
}

// midIntervalQ 计算单个区间（from→to，间隔 dt 秒）的电量（µAh）。ok=false 表示
// 该区间无法归因：要么数据本身无效（电量计零/负增量或超 midSegMaxUAh，跳过但
// 不扣该窗格 pass），要么本机有电量计却缺读数（needMark=true，该窗格覆盖不全）。
func midIntervalQ(from, to SampleRow, dt int64, hasCounter bool) (q float64, needMark, ok bool) {
	if from.CounterUAh > 0 && to.CounterUAh > 0 {
		d := to.CounterUAh - from.CounterUAh
		if d <= 0 || d > midSegMaxUAh {
			return 0, false, false
		}
		return float64(d), false, true
	}
	if hasCounter {
		return 0, true, false
	}
	return float64(from.UA) * float64(dt) / 3600, false, true
}

// attributeSegment 把一段连续正电流样本逐区间归因到窗格上。
func attributeSegment(seg []SampleRow, hasCounter bool, lo, hi int) *midSegQ {
	sg := &midSegQ{q: map[int64]float64{}, bad: map[int64]bool{}}
	for k := 1; k < len(seg); k++ {
		from, to := seg[k-1], seg[k]
		c0, c1 := from.Cap, to.Cap
		dt := to.TS - from.TS
		if dt <= 0 || c1 < c0 {
			continue // 数据本身无效：跳过该区间，但不扣该窗格 pass
		}
		q, needMark, ok := midIntervalQ(from, to, dt, hasCounter)
		switch {
		case ok:
			sg.attribute(c0, c1, q, lo, hi)
		case needMark:
			sg.markBad(c0, c1, lo, hi) // 有电量计却缺读数：该窗格 Q 与 pass 都不计
		}
	}
	return sg
}

// midHasCounter 判断本机是否有电量计读数：样本里出现过 CounterUAh>0 即说明有
// charge_counter，此时缺读数的区间不得回退电流积分（两种口径不能混用）。
func midHasCounter(rows []SampleRow) bool {
	for _, r := range rows {
		if r.CounterUAh > 0 {
			return true
		}
	}
	return false
}

// midSegEnd 返回从 i 开始的正电流连续段结束下标（相邻间隔 ≤ midSegMaxDt 且电流
// 为正）；rows[i] 非正电流时返回 i+1，即单点跳过。
func midSegEnd(rows []SampleRow, i int) int {
	if rows[i].UA <= 0 {
		return i + 1
	}
	j := i + 1
	for j < len(rows) && rows[j].TS-rows[j-1].TS <= midSegMaxDt && rows[j].UA > 0 {
		j++
	}
	return j
}

// MidImplied 聚合充电样本并返回 pass 归一的中段隐含满容量（与 EstUAh 同单位，
// 即 µAh 量级数值）。判定口径：
//   - 样本按时间连续（相邻间隔 ≤ midSegMaxDt）且电流为正切成一段；
//   - 段内区间电量优先取电量计差分（charge_counter，µAh 直读，与放电侧/内核
//     charge_full 同源，免疫 current_now 刻度偏差）；整批样本都没有电量计读数
//     （设备无电量计）时才回退电流积分 Q=I·dt÷3600（µA·s→µAh）；
//   - 有电量计读数（样本里出现过 CounterUAh>0）时，缺读数的旧区间直接跳过：电流
//     积分相对电量计系统性偏低 10~13%（实测中段因此从 5747 被拉到 5381mAh），
//     两种口径不能混用；覆盖不足时宁可不校准；
//   - 区间电量均摊到跨越的 1% 窗格（c1>c0）；c1<c0（电量计回修）、dt≤0 与
//     增量超限的区间无法归因，跳过；百分比停滞的区间归入当前窗格（若在窗内）；
//     同一段内若有区间因缺电量计读数被跳过，对应窗格 Q 与 pass 都不计——部分覆盖
//     若仍记一整次穿越，会把该窗格的均值拉低；
//   - 窗口内任一窗格穿越次数 < midMinBinPass ⇒ 覆盖不足，ok=false（防止只用
//     局部区间充电的习惯把窗口算偏）；
//   - 隐含容量 = ΣQ/Σpass × 100% 折算（Q 已是 µAh，无需再除以 3600）。
func MidImplied(rows []SampleRow, lo, hi int) (int64, bool) {
	hasCounter := midHasCounter(rows)
	var acc midAccum
	for i := 0; i < len(rows); {
		j := midSegEnd(rows, i)
		if j-i >= 2 {
			acc.add(attributeSegment(rows[i:j], hasCounter, lo, hi))
		}
		i = j
	}
	return acc.implied(lo, hi)
}

// calibrateMid 结算采信后的中段校准装配：至多每日一次；EMA 未初始化或覆盖
// 不足时静默跳过。混合沿用部分会话权重（1/10），量级保守且与既有语义一致。
// 任何失败只落 events，绝不影响结算主链路。
func (p *Pipeline) calibrateMid(now int64) {
	if now-kvInt(p.st, kvMidCalTs) < midCalGapSecs {
		return
	}
	ema := kvInt(p.st, kvKeyEmaUAh)
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
	// 与会话通道同口径的合理性门控：中段隐含值来自电量计差分（无电量计读数才
	// 回退积分），同样会吃到量纲误判/脏样本（实测被 1000 倍误乘的样本推到 15592mAh，
	// 比设计容量还高 1.5 倍），不设门则这条旁路能把无界垃圾灌进用户可见的实测值。
	if !capacityInWindow(mid, p.designUAh) {
		p.log("[校准] 中段分窗(%d~%d%%) 隐含=%dmAh 超出设计容量窗口，不采信",
			midWinLo, midWinHi, mid/1000)
		_ = p.st.InsertEvent("mid_cal_skip", fmt.Sprintf("mid=%d 超出窗口", mid))
		return
	}
	blended := emaBlend(ema, mid, 0)
	if err := p.st.KVSet(kvKeyEmaUAh, strconv.FormatInt(blended, 10)); err != nil {
		_ = p.st.InsertEvent("mid_cal_fail", err.Error())
		return
	}
	// 留给通道互检（checkQuality）作对照口径：值与最近更新时间成对落盘，
	// 互检据此判时效（超保留窗口的旧值不参与互检）。先写值再写 ts；ts 写失败
	// 只留痕，不影响校准主链路。
	_ = p.st.KVSet(kvMidImpliedUAh, strconv.FormatInt(mid, 10))
	if err := p.st.KVSet(kvMidImpliedTs, strconv.FormatInt(now, 10)); err != nil {
		_ = p.st.InsertEvent("mid_cal_fail", "写 mid_implied_ts 失败："+err.Error())
	}
	_ = p.st.KVSet(kvMidCalTs, strconv.FormatInt(now, 10))
	p.log("[校准] 中段分窗(%d~%d%%) 隐含=%dmAh，按1/10权重并入（%d→%d）",
		midWinLo, midWinHi, mid/1000, ema/1000, blended/1000)
	_ = p.st.InsertEvent("mid_cal", fmt.Sprintf("mid=%d ema=%d→%d", mid, ema, blended))
}
