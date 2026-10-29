// 电流刻度自校准与结算口径裁决（issue #6 根因修复）。
//
// 背景：Linux power_supply ABI 按「属性」规定单位（current_now 用 µA、charge_*
// 用 µAh），并要求驱动把原始值换算成类单位；Android BatteryManager 同一口径。
// 旧实现进一步假设「同一驱动会同时遵守两者」，但厂商可能只部分遵守——欧加系
// （ColorOS / OnePlus）内核据 issue #6 采样（一加 Ace 3）就是 current_now 报 mA、
// 而同一节点的 charge_full / charge_counter 仍是 µAh。旧实现把该读数当 µA 直通
// （charge_full ≥ 100000 ⇒ 同驱动电流已是 µA），于是电流积分整体小 1000 倍；
// 结算时「电量计 vs 积分相差 3 倍」护栏又把正确的电量计口径判成异常丢弃，估算
// 落到 2mAh 并被 out_of_window 拒收。
//
// 本文件不猜节点、也不信「同驱动单位一致」这个前提：电量计差分就是真实充入
// 电量的标尺——一次充电会话里「电量计差分 µAh」与「原始读数绝对值×dt」（原始
// 单位·秒）的比值即 µA/原始单位。必须用原始读数积分而不是归一后的 accUAs：
// 后者已被当前生效刻度（或锚点）缩放，用它反推会让已学刻度在下一个会话被算回
// ×1（回归用例（模拟 mA 节点）里刻度会在 1000↔1 之间震荡）。比值贴近 10 的
// 整数次幂才采信（本仓库实机采样经验：正常设备原始积分 vs 电量计偏低 5~15%，
// 吸附到 ×1；mA 节点吸附到 ×1000），异常比值不校准、退回原锚点逻辑。注意该
// 吸附只修正「单位/数量级」，不是传感器增益校准。resolveCounterSource 只在
// 「电量计隐含容量在窗内、积分在窗外」时翻转旧行为，其余情况沿用旧口径，避免
// 改动既有设备行为。

package main

import (
	"math"
	"strconv"
)

const (
	// kvCurrentScale 学习到的电流刻度（µA/原始单位，如 "1"、"1000"）。
	// 缺失或非法 ⇒ 未校准，退回 NormCurrentUAWithFull 的 charge_full 锚点逻辑。
	kvCurrentScale = "cur_scale"
	// curScaleSnapTol 比值吸附容差（log10）：0.4 decade，对应 ×0.40~×2.51。
	// 容差只用于单位/数量级吸附，不精校传感器增益：正常设备电流积分相对电量计
	// 偏低 5~15%，落进 ×1 的吸附区；mA 节点落进 ×1000。积分与电量计的比值还会
	// 因双电芯等效容量口径、采样缺口等再偏 2 倍左右，容差需覆盖该范围；<0.5
	// 保证相邻 10 的幂的吸附区互不重叠，不会把 ×1 判成 ×10。
	curScaleSnapTol = 0.4
	// curScaleMinExp/curScaleMaxExp 可采信刻度的数量级上下界：只接受 10 的
	// 整数次幂（0.001~1000），更离谱的比值判为异常不采信。
	curScaleMinExp = -3
	curScaleMaxExp = 3
)

// anchorScaleUA 返回 NormCurrentUAWithFull 在给定 charge_full 下采用的固定
// 倍数：1（µAh 锚点，current 直通）、1000（mAh 锚点，current ×1000）、0（无
// 锚点，逐值幅值启发式）。自校准已改用 rawUAs，不再依赖该倍数；它只用于未
// 校准时的归一与诊断单位标签，0 表示「无固定倍数」而非「无法校准」。
func anchorScaleUA(chargeFullUAh int64) float64 {
	switch {
	case chargeFullUAh >= microUnitChargeMinUAh:
		return 1
	case chargeFullUAh > 0:
		return 1000
	default:
		return 0
	}
}

// normalizeCurrentUA 把 current_now 原始读数换算成 µA（保留符号）：优先用从
// 电量计学习到的刻度；未校准时退回 charge_full 量纲锚点/幅值启发式。学习刻度
// 若把读数推出物理上界（节点语义变了/刻度被脏会话带偏），同样退回原逻辑。
func normalizeCurrentUA(raw, chargeFullUAh int64, learnedScale float64) int64 {
	if learnedScale > 0 {
		if ua := int64(math.Round(float64(raw) * learnedScale)); absI64(ua) <= maxCurrentUA {
			return ua
		}
	}
	return NormCurrentUAWithFull(raw, chargeFullUAh)
}

// currentUnitLabel 把刻度和锚点翻成面向用户的单位标签：ua（µA 直通）/ ma
// （原始读数即 mA）/ auto（未校准且无 ×1/×1000 的 ABI 初判，或学习刻度不是 10
// 的整数次幂；仅用于诊断展示，不参与归一计算）。
func currentUnitLabel(chargeFullUAh int64, learnedScale float64) string {
	scale := learnedScale
	if scale <= 0 {
		scale = anchorScaleUA(chargeFullUAh)
	}
	switch scale {
	case 1:
		return "ua"
	case 1000:
		return "ma"
	default:
		return "auto"
	}
}

// learnCurrentScale 由一次会话的电量计差分（µAh）与「原始读数×dt」积分
// （原始单位·秒）反推 µA/原始单位，并同时返回原始比值供调用方记录。
// 必须用原始读数积分而不是归一后的 accUAs：accUAs 已被当前生效刻度（或锚点）
// 缩放，拿它反推会把已学刻度算回 ×1，导致刻度在 1000↔1 之间来回震荡；原始
// 读数积分不依赖当前刻度，单位变化后也能一次纠正。返回 ok=false 时不修改
// 任何状态。
func learnCurrentScale(rawUAs, counterUAh int64) (scale, ratio float64, ok bool) {
	if rawUAs <= 0 || counterUAh <= 0 {
		return 0, 0, false
	}
	// 真实电量（µA·s）÷ 原始读数·秒 = µA/原始单位
	ratio = float64(counterUAh) * 3600 / float64(rawUAs)
	if !(ratio > 0) || math.IsNaN(ratio) || math.IsInf(ratio, 0) {
		return 0, 0, false
	}
	lg := math.Log10(ratio)
	snap := math.Round(lg)
	if snap < curScaleMinExp || snap > curScaleMaxExp {
		return 0, ratio, false
	}
	// 异常比值（例如电量计缺口让比值偏 3 倍、或电量计与积分本就不同量纲）
	// 一律不采信：宁可继续用旧刻度，也不把刻度改坏。
	if math.Abs(lg-snap) > curScaleSnapTol {
		return 0, ratio, false
	}
	return math.Pow(10, snap), ratio, true
}

// scaledAccUAs 把会话期「原始读数绝对值×dt」积分（原始单位·秒）按电流刻度
// 换算成 µA·s，供学到新刻度的会话就地重算自己的积分电量。rawUAs 或刻度非正、
// 结果溢出时返回 0，调用方保留原累计值。
func scaledAccUAs(rawUAs int64, scale float64) int64 {
	if rawUAs <= 0 || !(scale > 0) {
		return 0
	}
	acc := math.Round(float64(rawUAs) * scale)
	if !(acc > 0) || acc >= float64(math.MaxInt64) {
		return 0
	}
	return int64(acc)
}

// counterResolution 是电量计差分与电流积分相差 3 倍以上时的裁决结果。
// CounterImplied/IntegralImplied 为两路各自反推的隐含满容量（µAh，delta≤0
// 时不计算，保持 0）。
type counterResolution struct {
	UseCounter         bool
	CounterImpliedUAh  int64
	IntegralImpliedUAh int64
	// Reason 机器可读裁决原因：no_counter / no_integral / within_3x /
	// counter_plausible / integral_plausible / ambiguous
	Reason string
}

// resolveCounterSource 在电量计差分与电流积分相差 3 倍以上时判定采信哪一路。
// 旧实现无条件丢弃电量计、回退积分；issue #6 里被丢弃的恰是正确的一路（电流
// 积分因单位误判低估 1000 倍）。新裁决以容量参考值（设计容量，缺失时用内核
// charge_full）做 0.5~1.5 倍窗口，只有「电量计在窗内、积分在窗外」才保留电量计；
// 其余情况维持旧行为，不改变既有设备口径。
func resolveCounterSource(counterUAh, accUAs, deltaCap, refUAh int64) counterResolution {
	res := counterResolution{Reason: "no_counter"}
	if counterUAh <= 0 {
		return res
	}
	accUAh := accUAs / 3600
	if deltaCap > 0 {
		res.CounterImpliedUAh = counterUAh * 100 / deltaCap
		res.IntegralImpliedUAh = accUAh * 100 / deltaCap
	}
	if accUAh <= 0 {
		res.UseCounter = true
		res.Reason = "no_integral"
		return res
	}
	if !(counterUAh > accUAh*3 || accUAh > counterUAh*3) {
		res.UseCounter = true
		res.Reason = "within_3x"
		return res
	}
	if deltaCap <= 0 || refUAh <= 0 {
		res.Reason = "ambiguous"
		return res
	}
	counterOK := capacityInWindow(res.CounterImpliedUAh, refUAh)
	integralOK := capacityInWindow(res.IntegralImpliedUAh, refUAh)
	switch {
	case counterOK && !integralOK:
		res.UseCounter = true
		res.Reason = "counter_plausible"
	case !counterOK && integralOK:
		res.Reason = "integral_plausible"
	default:
		res.Reason = "ambiguous"
	}
	return res
}

// parseCurrentScale 解析 kv 里的刻度文本；非法/非正/非有限值一律视为未校准。
func parseCurrentScale(txt string) float64 {
	f, err := strconv.ParseFloat(txt, 64)
	if err != nil || !(f > 0) || math.IsNaN(f) || math.IsInf(f, 0) {
		return 0
	}
	return f
}
