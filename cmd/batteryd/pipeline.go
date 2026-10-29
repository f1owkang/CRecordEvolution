package main

import (
	"errors"
	"fmt"
	"math"
	"strconv"
	"time"
)

const (
	tickSeconds  int64 = 60
	sealCapacity int64 = 100

	kvChargedTotal = "charged_ua_total"

	restQuietUA int64 = 100000
	// 100mA 门限：20mA 在实机（灭屏底耗 20~80mA 且周期性唤醒）9 天零触发，
	// rest_points 全空；100mA 下的 IR 压降（~30mΩ × 100mA = 3mV）低于去重
	// 漂移门限 5mV，不破坏静置电压指纹语义。
	restMinTicks            = 3
	restDedupCapDelta int64 = 1
	restDedupUVDrift  int64 = 5000
	restRelaxTicks          = 10 // e-Energy '23：静置约 10 分钟电压收敛，可作 SoH 指纹

	// tailQuietUA CV 尾段电流门限：内核电量计报 100% 时真实充电常仍以 ~1A
	// 持续数分钟（实测显示 100% 后电流自 1.4A 缓降再入涓流），首拍封账会
	// 系统性少计电量、估算偏低。非 Discharging 状态下正向电流高于此值视为
	// 充电延续。
	tailQuietUA int64 = 100000

	// tailDropUV 满电后电压回落门控（µV/电芯）：满电后内核仍可能报 Charging
	// 且电流为正，但那是充电器直供系统的电流——端电压会较充电峰值显著回落
	// （实测 -66~-93mV），而真实 CV 尾段紧贴峰值（-1~-3mV）。7A 若真充入电池，
	// 端电压不可能反而下降，故以「满电后电压较峰值低过此阈值」判为假电流，
	// 不计入会话电量与循环吞吐。按电芯数缩放（双电芯串联压降成比例）。
	tailDropUV int64 = 30_000

	resWindowMax  int = 30
	resMinSamples int = 20
	// resMinStdUA 150mA：50mA 门限下电流近乎平稳的窗靠测量噪声凑方差，
	// 回归斜率被 OCV 漂移主导（实测单值可达真值 5~10 倍，246/281 mΩ）；
	// 收紧后两台实测设备 p99 从 181/259 降到 49/232 mΩ。
	resMinStdUA float64 = 150000
	resMinMOhm  float64 = 0.5
	resMaxMOhm  float64 = 200

	kvSessActive         = "sess_active"
	kvSessStartTs        = "sess_start_ts"
	kvSessStartCap       = "sess_start_cap"
	kvSessVStart         = "sess_v_start_uv"
	kvSessAcc            = "sess_acc_uas"
	kvSessTicks          = "sess_ticks"
	kvSessTempMin        = "sess_temp_min"
	kvSessTempMax        = "sess_temp_max"
	kvSessTempSum        = "sess_temp_sum"
	kvSessTempN          = "sess_temp_n"
	kvSessLastCap        = "sess_last_cap"
	kvSessLastTs         = "sess_last_tick_ts"
	kvSessCounterAccUAh  = "sess_cc_acc"
	kvSessCounterLastUAh = "sess_cc_last"
	kvSessCounterTs      = "sess_cc_ts"
	// kvSessRawUAs 会话期「原始电流读数绝对值 × dt」累计（原始单位·秒），只用于
	// 反推 µA/原始单位；不能用被当前刻度缩放过的 accUAs 反推（见 currentcal.go）。
	kvSessRawUAs = "sess_raw_uas"
)

// plausibleDelta 判「真实电量变化」与「电量计跳变/回修」：用 Δ/Δt 反推电流，
// 超出物理上界（maxCurrentUA）判为跳变，跳过不计。长间隙的 Δ 虽大但 Δt 也大，
// 反推电流合理、照常计入——这正是逐拍按阈值筛的旧实现漏掉休眠间隙电量的原因。
func plausibleDelta(d, dt int64) bool {
	if dt <= 0 {
		return false
	}
	return absI64(d)*3600 <= maxCurrentUA*dt
}

// TickOutcome 汇总一次 Tick 的副作用（当前仅有会话结算完成标志）。
type TickOutcome struct{ SessionSettled bool }

// SettleError 标记结算或落库失败；tickStrike 据此判定守护进程需退出。
type SettleError struct{ Err error }

// Error 返回带中文前缀的结算错误信息。
func (e *SettleError) Error() string { return "结算或落库失败：" + e.Err.Error() }

// Unwrap 返回底层错误，支持 errors.Is/As 链。
func (e *SettleError) Unwrap() error { return e.Err }

// SysfsTransient 标记暂时性 sysfs 读取失败（节点暂时不可用、suspend/resume 竞态等）。
// tickStrike 不会将此类错误计入连续失败计数，避免瞬态故障杀掉守护进程。
type SysfsTransient struct{ Err error }

// Error 返回带中文前缀的瞬时读错误信息。
func (e *SysfsTransient) Error() string { return "sysfs 暂时不可读：" + e.Err.Error() }

// Unwrap 返回底层错误，支持 errors.Is/As 链。
func (e *SysfsTransient) Unwrap() error { return e.Err }

// sessionState 为充电会话累计状态；persistSession 整体写入 kv 以支持重启续记。
type sessionState struct {
	active   bool
	startTs  int64
	startCap int64
	vStartUV int64
	accUAs   int64
	ticks    int64
	// lastTickTs 上一拍充电 tick 的墙钟（秒）：变步长采样下电量按真实
	// 时间差累积，不再假设固定 tickSeconds。0 表示本会话尚无上一拍。
	lastTickTs int64
	tempMin    int64
	tempMax    int64
	tempSum    float64
	tempN      int64
	lastCap    int64
	// counterAccUAh/counterLastUAh/counterTs 为会话期电量计口径（charge_counter，
	// µAh）：逐拍把「物理可信」的增量（含负增量）累进 counterAccUAh——零/负增量是
	// 电量计正常抖动（实测零增量 96 次、负增量 26 次，只累加正增量会高估约 21%），
	// 必须正负都记；单拍巨幅跳变（Δ/Δt 反推电流超 maxCurrentUA）判为回修，跳过不计。
	// 结算取 counterAccUAh 作为本会话净充入电量，与放电侧、内核 charge_full 同源，
	// 免疫 current_now 刻度偏差与满电假电流。counterHole 标记电量计在开账后才可用
	// （增量只覆盖部分区间）。
	counterAccUAh  int64
	counterLastUAh int64
	counterTs      int64
	counterHole    bool
	// rawUAs 会话期「原始电流读数绝对值 × dt」累计（原始单位·秒），与 accUAs
	// 同步计入门控（suppressed 拍不计）；自校准只用它反推 µA/原始单位，避免用
	// 被当前刻度缩放过的 accUAs 反推导致刻度震荡。
	rawUAs int64
}

// Pipeline 是采样状态机：每拍读 sysfs、累计会话/放电/吞吐并在状态翻转时结算。
type Pipeline struct {
	fs        SysFS
	st        *Store
	est       Estimator
	designUAh int64
	fullUAh   int64 // charge_full µAh，用于电流单位交叉校验
	cellCount int   // 电芯串联数：1=单电芯，2=双电芯（voltage_now > 5V 判定，依据见 main.go）
	now       func() time.Time

	nodePaths map[string]string

	// curScale 从电量计差分学习到的电流刻度（µA/原始单位），0=未校准，退回
	// charge_full 锚点逻辑（见 currentcal.go）。
	curScale float64

	sess sessionState
	dis  disState

	// disCounterOff charge_counter 首读失败后永久禁用放电记录（负缓存，
	// 避免每拍全树扫 /sys）
	disCounterOff bool

	// peakChargeUV 本次充电插入周期内的电压峰值（µV）：满电后电压回落判据的
	// 基准。拔出充电器（转入 Discharging）时清零，下次插入重新建立。
	peakChargeUV int64
	// supLogged 已就「停计电量」打过日志（状态翻转去重）
	supLogged bool
	// lastDisSampleTs 上次放电样本落库时刻（秒），用于 5 分钟降采样
	lastDisSampleTs int64

	// notChargStreak status 连续非 Charging 的拍数（去抖计数，不持久化：
	// 进程重启后从 0 重新计数，最多多等 3 拍才结算，无害）
	notChargStreak int

	// lastChargeTs 上一拍灌电（Charging/尾段）的墙钟秒，供 charged_ua_total
	// 跨会话计 dt；与 sess.lastTickTs 独立——满电浮充无会话也照计吞吐。
	lastChargeTs int64

	winI []int64
	winV []int64

	restStreak  int
	lastRestUV  int64
	lastRestCap int64

	// logf 决策点日志通道（main 注入 appendLog，nil 安全）：只记会话级
	// 事件——开启/满电跳过/去抖/结算/CCCT 采信，不逐拍刷屏
	logf   func(format string, args ...any)
	logOn  bool // 上拍是否处于充电会话（满电跳过去重）
	logDeb bool // 已打过「进入去抖」标记（同一轮去抖只打一次）
}

// Logf 注入决策点日志通道；在 NewPipeline 之后调用。
func (p *Pipeline) Logf(fn func(format string, args ...any)) {
	p.logf = fn
}

func (p *Pipeline) log(format string, args ...any) {
	if p.logf != nil {
		p.logf(format, args...)
	}
}

// NewPipeline 装配 Pipeline，并恢复未完成的充电/放电会话与 charge_full 缓存。
func NewPipeline(fs SysFS, st *Store, est Estimator, designUAh int64, cellCount int, clock func() time.Time) *Pipeline {
	p := &Pipeline{
		fs:        fs,
		st:        st,
		est:       est,
		designUAh: designUAh,
		cellCount: cellCount,
		now:       clock,
		nodePaths: map[string]string{},
	}
	// 读取 charge_full 用于电流单位交叉校验（读不到不影响功能）
	if path, err := fs.FindNode("charge_full"); err == nil {
		if v, err := fs.ReadInt(path); err == nil {
			p.fullUAh = v
		}
	}
	// 恢复已学习的电流刻度；未校准时 curScale=0，走锚点逻辑
	p.curScale = parseCurrentScale(kvText(st, kvCurrentScale))
	p.restoreSession()
	p.restoreDischarge()
	return p
}

// setFullUAh 刷新 charge_full 缓存（满充后 charge_full 会更新）。
func (p *Pipeline) setFullUAh(v int64) { p.fullUAh = v }

// SetCellCount 更新 Pipeline 的电芯数（由 redetectCellCount 触发）。
func (p *Pipeline) SetCellCount(n int) { p.cellCount = n }

func kvText(st KVStore, key string) string {
	v, _ := st.KVGet(key)
	return v
}

func kvInt(st KVStore, key string) int64 {
	n, _ := strconv.ParseInt(kvText(st, key), 10, 64)
	return n
}

func kvFloat(st KVStore, key string) float64 {
	f, _ := strconv.ParseFloat(kvText(st, key), 64)
	return f
}

func absI64(v int64) int64 {
	if v < 0 {
		return -v
	}
	return v
}

func tempAvgOf(sum float64, n int64) int64 {
	if n <= 0 {
		return 0
	}
	return int64(math.Round(sum / float64(n)))
}

// crRate 计算充电倍率；designUAh 缺失时返回 0（避免 +Inf 污染数据库和下游特征向量）。
func crRate(avgI, designUAh int64) float64 {
	if designUAh <= 0 {
		return 0
	}
	return float64(avgI) / float64(designUAh)
}

func (p *Pipeline) restoreSession() {
	if v, ok := p.st.KVGet(kvSessActive); !ok || v != "1" {
		return
	}
	p.sess = sessionState{
		active:         true,
		startTs:        kvInt(p.st, kvSessStartTs),
		startCap:       kvInt(p.st, kvSessStartCap),
		vStartUV:       kvInt(p.st, kvSessVStart),
		accUAs:         kvInt(p.st, kvSessAcc),
		ticks:          kvInt(p.st, kvSessTicks),
		tempMin:        kvInt(p.st, kvSessTempMin),
		tempMax:        kvInt(p.st, kvSessTempMax),
		tempSum:        kvFloat(p.st, kvSessTempSum),
		tempN:          kvInt(p.st, kvSessTempN),
		lastCap:        kvInt(p.st, kvSessLastCap),
		lastTickTs:     kvInt(p.st, kvSessLastTs),
		counterAccUAh:  kvInt(p.st, kvSessCounterAccUAh),
		counterLastUAh: kvInt(p.st, kvSessCounterLastUAh),
		counterTs:      kvInt(p.st, kvSessCounterTs),
		rawUAs:         kvInt(p.st, kvSessRawUAs),
		// 电量计增量带上读数时刻一起恢复：重启间隙的增量按 Δ/Δt 反推电流判
		// 可信后照常计入（物理上合理的长间隙不丢电量）；电量计在开账后才可用
		// 才算缺口（accumulate 判）。
	}
}

func (p *Pipeline) nodePath(name string) (string, error) {
	if path, ok := p.nodePaths[name]; ok {
		return path, nil
	}
	path, err := p.fs.FindNode(name)
	if err != nil {
		return "", err
	}
	p.nodePaths[name] = path
	return path, nil
}

func (p *Pipeline) readNode(name string) (int64, error) {
	path, err := p.nodePath(name)
	if err != nil {
		return 0, err
	}
	return p.fs.ReadInt(path)
}

// readNodeSigned 带符号读取（放电电流为负，见 sysfs.ReadIntSigned）。
func (p *Pipeline) readNodeSigned(name string) (int64, error) {
	path, err := p.nodePath(name)
	if err != nil {
		return 0, err
	}
	return p.fs.ReadIntSigned(path)
}

// currentUA 把 current_now 原始读数归一为 µA（保留符号）：优先用从电量计
// 学习到的刻度，未校准时退回 charge_full 锚点/幅值启发式。所有电流消费方
// （会话积分、尾段判续、放电样本、静置门控、CCCT/ICA 倍率门控）必须走这里，
// 不要各自对原始读数做单位推断——current_now 的量纲可能随内核/厂商不同。
func (p *Pipeline) currentUA(raw int64) int64 {
	return normalizeCurrentUA(raw, p.fullUAh, p.curScale)
}

// energyRefUAh 返回容量口径裁决的参考满容量：优先设计容量，缺失时用内核
// charge_full（电量计自学习值）。两者都缺失时返回 0，裁决退回电流积分。
func (p *Pipeline) energyRefUAh() int64 {
	if p.designUAh > 0 {
		return p.designUAh
	}
	return p.fullUAh
}

// readChargeCounter 读电量计 charge_counter（µAh）。节点确认不存在时置负缓存
// （disCounterOff），避免充电期每拍全树扫 /sys；瞬时读取失败不置缓存，下一拍重试。
func (p *Pipeline) readChargeCounter() (int64, bool) {
	if p.disCounterOff {
		return 0, false
	}
	counterUAh, err := p.readNode("charge_counter")
	if err != nil {
		if errors.Is(err, ErrNodeNotFound) {
			p.disCounterOff = true
		}
		return 0, false
	}
	return counterUAh, true
}

// notChargDebounce 非充电状态去抖：status 连续非 Charging 达到该拍数才真正
// 结算会话。米系满电保护/优化充电会让 status 在充电过程中间歇抖为
// Not charging/Full，单拍即断会把同一晚充电切碎成一串涨幅不足的废会话
// （实测：4 条 delta=0 垃圾行全部源于连续 2 拍抖动）。3 拍≈3 分钟，与
// ACC「多次采样确认、不信单拍 status」的策略同源。
const notChargDebounce = 3

// Tick 按当前 status 推进一拍采样与会话状态机。
func (p *Pipeline) Tick(status string) (TickOutcome, error) {
	outcome := TickOutcome{}
	// 转入放电即复位充电峰值：下次插入重新建立，避免跨充电周期沿用旧峰值
	if status == "Discharging" {
		p.peakChargeUV = 0
	}
	// 放电记录与充电会话独立：Discharging 差分累计、Charging 触发结算，
	// 任何错误均内部消化，绝不影响充电主链路
	p.trackDischarge(status)
	if status == "Charging" {
		// 回到 Charging：去抖计数清零，原会话（若在去抖等待期）原样继续
		p.notChargStreak = 0
		p.logDeb = false
		return outcome, p.tickCharging(&outcome)
	}
	// 非充电状态但电池电流仍在正向灌入（CV 尾段 / 满电保护抖动的 Full、
	// Not charging）：视为充电延续，继续累计并重置去抖。内核电量计报 100%
	// 时真实充电常还要持续数分钟，等电流停歇再走去抖结算。
	if status != "Discharging" && p.sess.active {
		if iUA, rawAbs, flowing := p.tailCurrent(); flowing {
			p.notChargStreak = 0
			p.logDeb = false
			return outcome, p.tickTailCharge(iUA, rawAbs)
		}
	}
	p.notChargStreak++
	// 去抖期内不算断开：保留会话，等下一拍
	if p.sess.active && p.notChargStreak < notChargDebounce {
		if !p.logDeb {
			p.logDeb = true
			p.log("[会话] status=%s，去抖等待（第 %d/%d 拍）", status, p.notChargStreak, notChargDebounce)
		}
		return outcome, nil
	}
	if err := p.tickResting(status); err != nil {
		return outcome, err
	}
	if p.sess.active {
		if err := p.settle(); err != nil {
			return outcome, err
		}
		outcome.SessionSettled = true
		if err := p.resetSession(); err != nil {
			return outcome, &SettleError{Err: err}
		}
		p.logOn = false
	}
	return outcome, nil
}

// chargeSuppressed 判本拍电流是否为「满电后充电器直供系统」的假充电电流：
// 满电（cap≥fullSealCap）且电压较本次充电峰值回落超过 tailDropUV×电芯数。
// 首次读到的电压即建立峰值；电压读取失败（vUV<=0）时无从判断，一律放行。
func (p *Pipeline) chargeSuppressed(capVal, vUV int64) bool {
	sup := false
	switch {
	case vUV <= 0:
		// 读不到电压：无从判断，放行（宁多计不漏计）
	case vUV > p.peakChargeUV:
		p.peakChargeUV = vUV // 峰值刷新，本拍必不是回落
	case capVal >= fullSealCap && p.peakChargeUV > 0:
		scale := int64(1)
		if p.cellCount > 1 {
			scale = int64(p.cellCount)
		}
		sup = p.peakChargeUV-vUV > tailDropUV*scale
	}
	// 只在状态翻转时落日志，避免逐拍刷屏
	if sup && !p.supLogged {
		p.supLogged = true
		p.log("[充电] 满电后电压回落 %.3fV（较峰值 -%dmV），判为充电器供系统，停计电量",
			float64(vUV)/1e6, (p.peakChargeUV-vUV)/1000)
	} else if !sup && p.supLogged {
		p.supLogged = false
		p.log("[充电] 电压回到峰值附近（%.3fV），恢复计电", float64(vUV)/1e6)
	}
	return sup
}

// tailCurrent 读带符号电流并判别是否仍在充电方向灌入。单位归一统一走
// currentUA（已学习刻度优先），再按原始符号回填；返回归一后电流与原始读数
// 绝对值（后者供会话自校准累计）。放电（负值）与读取失败均返回未灌入。
func (p *Pipeline) tailCurrent() (int64, int64, bool) {
	iRaw, err := p.readNodeSigned("current_now")
	if err != nil {
		return 0, 0, false
	}
	iUA := p.currentUA(iRaw)
	if iUA > tailQuietUA {
		return iUA, absI64(iRaw), true
	}
	return 0, 0, false
}

func (p *Pipeline) tickCharging(outcome *TickOutcome) error {
	capVal, err := p.readNode("capacity")
	if err != nil {
		return &SysfsTransient{Err: err}
	}
	iRaw, err := p.readNodeSigned("current_now")
	if err != nil {
		return &SysfsTransient{Err: err}
	}
	iUA := absI64(p.currentUA(iRaw))

	// 电压先读：既用于内阻/样本，也用于「满电后假充电电流」门控（见
	// chargeSuppressed）。峰值的建立与判定都依赖本拍电压。
	vUV, verr := p.readNode("voltage_now")
	if verr != nil {
		vUV = 0
	}
	suppressed := p.chargeSuppressed(capVal, vUV)

	// 吞吐累计先于会话判定：满电插入不开会话，但浮充电量照计（循环当量口径）；
	// 假充电电流不计入，否则循环数虚增
	if !suppressed {
		if err := p.chargeThroughput(iUA); err != nil {
			return err
		}
	}

	var tempC float64
	haveTemp := false
	if tRaw, terr := p.readNode("temp"); terr == nil {
		tempC = NormTempC(tRaw)
		haveTemp = true
	}
	if vUV > 0 {
		p.pushWindow(iUA, vUV)
		if err := p.evalResistance(); err != nil {
			return err
		}
	}
	counterUAh, _ := p.readChargeCounter()
	if vUV > 0 && capVal > 0 {
		if err := p.st.InsertSample(p.now().Unix(), iUA, vUV, capVal, counterUAh); err != nil {
			_ = p.st.InsertEvent("sample_fail", err.Error())
		}
	}

	s := &p.sess
	if !s.active {
		// 已满（cap≥sealCapacity）时插入充电器：米系满电保护下 delta 恒为 0，
		// 该会话必被 delta_lt_20 拒收，只产生垃圾行——不开启会话，等真实回落。
		if capVal >= sealCapacity {
			if !p.logOn {
				p.logOn = true
				p.log("[会话] 满电插入（cap=%d%%），不开会话", capVal)
			}
			return nil
		}
		s.active = true
		s.startTs = p.now().Unix()
		s.startCap = capVal
		s.vStartUV = vUV
		p.logOn = true
		p.log("[会话] 开始于 %s cap=%d%%", p.now().Format("01-02 15:04:05"), capVal)
	}
	// 显示 100% 不等于充电完成（内核报数早于真实充满）：不在此封账，CV 尾段
	// 由 Tick 的 tailCurrent 路径继续累计，待电流停歇后走去抖结算。
	return p.accumulate(tickReading{
		iUA:        iUA,
		rawAbs:     absI64(iRaw),
		capVal:     capVal,
		tempC:      tempC,
		haveTemp:   haveTemp,
		suppressed: suppressed,
		counterUAh: counterUAh,
	})
}

// tickTailCharge 非充电状态下仍在灌电的一拍（CV 尾段）：仅对已活跃会话累计，
// 不开新会话（满电插入不开会话）、不做内阻回归（恒压段 dV/dI 语义不成立）。
// rawAbs 为原始电流读数绝对值，供自校准累计原始读数·秒。
func (p *Pipeline) tickTailCharge(iUA, rawAbs int64) error {
	capVal, err := p.readNode("capacity")
	if err != nil {
		return &SysfsTransient{Err: err}
	}
	vUV, verr := p.readNode("voltage_now")
	if verr != nil {
		vUV = 0
	}
	// 尾段同样受峰值回落门控：满电后系统高负载下，本路径也会读到充电器
	// 直供系统的假电流（实测该场景持续 30+ 分钟、虚增可达 1.1Ah）
	suppressed := p.chargeSuppressed(capVal, vUV)
	if !suppressed {
		if err := p.chargeThroughput(iUA); err != nil {
			return err
		}
	}
	counterUAh, _ := p.readChargeCounter()
	if vUV > 0 && capVal > 0 {
		if err := p.st.InsertSample(p.now().Unix(), iUA, vUV, capVal, counterUAh); err != nil {
			_ = p.st.InsertEvent("sample_fail", err.Error())
		}
	}
	var tempC float64
	haveTemp := false
	if tRaw, terr := p.readNode("temp"); terr == nil {
		tempC = NormTempC(tRaw)
		haveTemp = true
	}
	return p.accumulate(tickReading{
		iUA:        iUA,
		rawAbs:     rawAbs,
		capVal:     capVal,
		tempC:      tempC,
		haveTemp:   haveTemp,
		suppressed: suppressed,
		counterUAh: counterUAh,
	})
}

// tickReading 归集一拍充电读数。字段在调用点具名传入：iUA 与 rawAbs 都是
// int64 且相邻，位置参数写反了编译器不会报错，具名字段可消除这类隐患。
type tickReading struct {
	iUA        int64   // 归一到 µA 的电流（已按已学刻度换算）
	rawAbs     int64   // current_now 原始读数绝对值，仅用于电流刻度自校准
	capVal     int64   // 显示电量（%）
	tempC      float64 // 电池温度（℃）
	haveTemp   bool    // 本拍是否读到温度
	suppressed bool    // 满电后电压回落判定的假电流：不计电量
	counterUAh int64   // 本拍电量计读数（µAh，0=未采到）
}

// accumulate 向活跃会话累计一拍充电读数（Charging 拍与 CV 尾段拍共用）；
// 调用方保证会话已开启。
func (p *Pipeline) accumulate(rd tickReading) error {
	s := &p.sess
	now := p.now().Unix()
	dt := p.tickDelta(now)
	// firstTick：会话开账拍（含从未累计过的首拍）；重启续记后 lastTickTs 已恢复
	// 非 0，故不误判为开账拍，电量计基线由 restoreSession 带回。
	firstTick := s.lastTickTs == 0
	s.lastTickTs = now
	p.accumulateCounter(rd.counterUAh, now, firstTick)
	// suppressed：满电后电压回落的假充电电流不计入电量（时间基线照常推进，
	// 避免下一拍 dt 跨越被丢弃的区间）
	if !rd.suppressed {
		s.accUAs += rd.iUA * dt
		s.rawUAs += rd.rawAbs * dt
		s.ticks++
	}
	s.lastCap = rd.capVal
	if rd.haveTemp {
		p.accumulateTemp(rd.tempC)
	}
	return p.persistSession()
}

// tickDelta 本拍实际间隔（秒）：daemon 充电期 15s、其余 60s 变步长，固定
// tickSeconds 会把充电期电量高估数倍；上限 90s 覆盖步长切换间隙，同时把去抖期
// 回充拍按整段时长计的高估限制在一拍内。
func (p *Pipeline) tickDelta(now int64) int64 {
	dt := tickSeconds
	if last := p.sess.lastTickTs; last > 0 {
		if d := now - last; d >= 1 && d <= 90 {
			dt = d
		}
	}
	return dt
}

// accumulateCounter 累计本拍电量计增量：只跳过物理不可能的跳变（回修/复位），
// 零与负增量照记，长间隙的大增量也照记（Δ/Δt 反推电流仍合理）。
func (p *Pipeline) accumulateCounter(counterUAh, now int64, firstTick bool) {
	s := &p.sess
	if counterUAh <= 0 {
		return
	}
	if s.counterLastUAh == 0 {
		s.counterLastUAh = counterUAh
		if !firstTick {
			s.counterHole = true // 电量计在开账后才可用：增量只覆盖部分区间
		}
	} else {
		if d := counterUAh - s.counterLastUAh; plausibleDelta(d, now-s.counterTs) {
			s.counterAccUAh += d
		}
		s.counterLastUAh = counterUAh
	}
	s.counterTs = now
}

// accumulateTemp 累计会话期温度的 min/max/均值三统计。
func (p *Pipeline) accumulateTemp(tempC float64) {
	s := &p.sess
	ti := int64(tempC)
	if s.tempN == 0 {
		s.tempMin, s.tempMax = ti, ti
	} else if ti < s.tempMin {
		s.tempMin = ti
	}
	if ti > s.tempMax {
		s.tempMax = ti
	}
	s.tempSum += tempC
	s.tempN++
}

// chargeThroughput 全局充电吞吐累计（循环当量口径）：独立于会话生命周期，
// 结算后的满电浮充、封账后复插的补电脉冲也计入。dt 用跨会话的 lastChargeTs
// 差值，clamp 同会话口径。
func (p *Pipeline) chargeThroughput(iUA int64) error {
	dt := tickSeconds
	now := p.now().Unix()
	if p.lastChargeTs > 0 {
		if d := now - p.lastChargeTs; d >= 1 && d <= 90 {
			dt = d
		}
	}
	p.lastChargeTs = now
	total := kvInt(p.st, kvChargedTotal) + iUA*dt
	if err := p.st.KVSet(kvChargedTotal, strconv.FormatInt(total, 10)); err != nil {
		return &SettleError{Err: err}
	}
	return nil
}

func (p *Pipeline) persistSession() error {
	s := &p.sess
	sets := []struct {
		key string
		val string
	}{
		{kvSessActive, "1"},
		{kvSessStartTs, strconv.FormatInt(s.startTs, 10)},
		{kvSessStartCap, strconv.FormatInt(s.startCap, 10)},
		{kvSessVStart, strconv.FormatInt(s.vStartUV, 10)},
		{kvSessAcc, strconv.FormatInt(s.accUAs, 10)},
		{kvSessTicks, strconv.FormatInt(s.ticks, 10)},
		{kvSessTempMin, strconv.FormatInt(s.tempMin, 10)},
		{kvSessTempMax, strconv.FormatInt(s.tempMax, 10)},
		{kvSessTempSum, strconv.FormatFloat(s.tempSum, 'f', -1, 64)},
		{kvSessTempN, strconv.FormatInt(s.tempN, 10)},
		{kvSessLastCap, strconv.FormatInt(s.lastCap, 10)},
		{kvSessLastTs, strconv.FormatInt(s.lastTickTs, 10)},
		{kvSessCounterAccUAh, strconv.FormatInt(s.counterAccUAh, 10)},
		{kvSessCounterLastUAh, strconv.FormatInt(s.counterLastUAh, 10)},
		{kvSessCounterTs, strconv.FormatInt(s.counterTs, 10)},
		{kvSessRawUAs, strconv.FormatInt(s.rawUAs, 10)},
	}
	for _, it := range sets {
		if err := p.st.KVSet(it.key, it.val); err != nil {
			return err
		}
	}
	return nil
}

func (p *Pipeline) resetSession() error {
	p.sess = sessionState{}
	return p.st.KVSet(kvSessActive, "0")
}

// settle 结算一个已结束的充电会话：先裁决采信哪一路电量口径，再把会话交给
// 估算器落库，最后跑中段校准/特征采集/互检等旁路。
func (p *Pipeline) settle() error {
	s := &p.sess
	// 结算开始即置会话为不活跃：若进程在此之后、写库完成前被杀，重启后不会
	// 重复结算（最多丢失本次会话，绝不产生重复行）。
	if err := p.st.KVSet(kvSessActive, "0"); err != nil {
		return &SettleError{Err: err}
	}
	// duration 用墙钟差：变步长采样下拍数×固定秒数不再成立；墙钟口径同时覆盖
	// 浮充/去抖期，avgI 口径随之更贴近真实平均电流。
	duration := p.now().Unix() - s.startTs
	avgI := int64(0)
	if duration > 0 {
		avgI = s.accUAs / duration
	}
	dur := (time.Duration(duration) * time.Second).Truncate(time.Second)
	row := Session{
		StartTs:  s.startTs,
		EndTs:    p.now().Unix(),
		StartCap: s.startCap,
		EndCap:   s.lastCap,
		AccUAs:   s.accUAs,
		AvgI:     avgI,
		CRate:    crRate(avgI, p.designUAh),
		TempMin:  s.tempMin,
		TempMax:  s.tempMax,
		TempAvg:  tempAvgOf(s.tempSum, s.tempN),
		VStart:   s.vStartUV,
		Duration: duration,
		Valid:    false,
	}
	counterUAh, counterSrc := p.adjudicateCounter(s, &row)
	// 电流刻度自校准（第一次有效会话起生效）：本次会话的估算已由上面的口径裁决
	// 兜住，校准用于之后的采样/积分/倍率门控。必须用「原始读数·秒」而不是
	// accUAs（已被当前刻度/锚点缩放），否则已学刻度会在下一会话被反推回 ×1。
	// 学到新刻度时本会话已累计的积分电量按新刻度重算（avgI/倍率同步），否则
	// 教会刻度的这个会话会永远停在旧量纲（avgI≈0、倍率 0.00C），历史里看起来
	// 像是没校准。
	if counterUAh > 0 {
		if scale, learned := p.learnCurrentScaleFromSession(s.rawUAs, counterUAh); learned {
			if acc := scaledAccUAs(s.rawUAs, scale); acc > 0 {
				row.AccUAs = acc
				if duration > 0 {
					avgI = acc / duration
				}
				row.AvgI = avgI
				row.CRate = crRate(avgI, p.designUAh)
			}
		}
	}
	sr := SettledSession{Session: row, AccUAs: row.AccUAs, CounterUAh: counterUAh, DesignUAh: p.designUAh}
	// sessions.cc 落库本会话实际采信的电量计电量（µAh）：历史页「估算」按同一口径
	// 显示，避免展示口径与结算口径不一致。
	row.CounterUAh = counterUAh
	accepted, err := p.applySettlement(&row, sr, dur, avgI, counterSrc)
	if err != nil || !accepted {
		return err
	}
	// 以下为采信后的旁路：任何失败仅记 events，绝不影响结算主链路。
	// 中段分窗容量校准、CCCT 特征采集（同源 ICA 复用其
	// 过门样本行避免重复扫库）、四路容量口径互检。
	p.calibrateMid(row.EndTs)
	p.recordICA(row.EndTs, p.recordCCCT(s.startTs, row.EndTs))
	p.checkQuality()
	return nil
}

// adjudicateCounter 裁决本会话采信哪一路电量，返回采信的电量计增量（µAh，
// 0 表示回退电流积分）与日志用的口径标签。两条校验：
//  1. 电量计在开账后才可用（counterHole）⇒ 增量只覆盖部分区间，回退电流积分；
//  2. 电量计与电流积分相差 3 倍以上 ⇒ 用参考满容量裁决，只有「电量计在窗内、
//     积分在窗外」才保留电量计（两路都不在窗内时回退电流积分），见 currentcal.go。
func (p *Pipeline) adjudicateCounter(s *sessionState, row *Session) (int64, string) {
	counterUAh := s.counterAccUAh
	if s.counterHole {
		if counterUAh > 0 {
			p.log("[结算] 会话期电量计增量不完整（%dmAh，只覆盖部分区间），回退电流积分口径",
				counterUAh/1000)
		}
		return 0, "积分"
	}
	if counterUAh <= 0 {
		return 0, "积分"
	}
	res := resolveCounterSource(counterUAh, s.accUAs, row.EndCap-row.StartCap, p.energyRefUAh())
	switch {
	case !res.UseCounter:
		_ = p.st.InsertEvent("counter_scale", fmt.Sprintf(
			"counter=%dµAh 积分=%dµAh 隐含(counter/积分)=%d/%dµAh ref=%dµAh 回退积分(%s)",
			counterUAh, s.accUAs/3600, res.CounterImpliedUAh, res.IntegralImpliedUAh,
			p.energyRefUAh(), res.Reason))
		p.log("[结算] 电量计口径异常（%dmAh vs 积分 %dmAh，隐含 %dmAh vs %dmAh），回退电流积分",
			counterUAh/1000, s.accUAs/3600/1000,
			res.CounterImpliedUAh/1000, res.IntegralImpliedUAh/1000)
		return 0, "积分"
	case res.Reason == "counter_plausible":
		_ = p.st.InsertEvent("counter_scale", fmt.Sprintf(
			"counter=%dµAh 积分=%dµAh 隐含(counter/积分)=%d/%dµAh ref=%dµAh 保留电量计",
			counterUAh, s.accUAs/3600, res.CounterImpliedUAh, res.IntegralImpliedUAh,
			p.energyRefUAh()))
		p.log("[结算] 电流积分口径异常（%dmAh vs 电量计 %dmAh，隐含 %dmAh vs %dmAh），保留电量计口径",
			s.accUAs/3600/1000, counterUAh/1000,
			res.IntegralImpliedUAh/1000, res.CounterImpliedUAh/1000)
	}
	return counterUAh, "电量计"
}

// applySettlement 把会话交给估算器并落库：被拒时记录拒绝原因并返回
// accepted=false，采信时写 sessions/estimates 并返回 accepted=true。
// 采信后的旁路（中段校准/特征采集/互检）由调用方负责。
func (p *Pipeline) applySettlement(row *Session, sr SettledSession, dur time.Duration, avgI int64, counterSrc string) (bool, error) {
	upd, err := p.est.OnSession(sr)
	if err != nil {
		var re *RejectError
		if !errors.As(err, &re) {
			return false, &SettleError{Err: err}
		}
		row.Valid = false
		row.InvalidReason = re.Result.Reason
		p.log("[结算] 拒绝 reason=%s cap=%d→%d dur=%s avgI=%dmA",
			re.Result.Reason, row.StartCap, row.EndCap, dur, avgI/1000)
		if _, insErr := p.st.InsertSession(*row); insErr != nil {
			return false, &SettleError{Err: insErr}
		}
		if evErr := p.st.InsertEvent(re.Result.Reason, ""); evErr != nil {
			return false, &SettleError{Err: evErr}
		}
		return false, nil
	}
	row.Valid = true
	// σ 仅 ML 通道产出（stable 恒为 0）：≤0 时省略段，避免打印误导性的 σ=0.0；
	// 采信日志只保留一处模板，σ 段按需拼接。
	sigmaNote := ""
	if upd.SigmaMah > 0 {
		sigmaNote = fmt.Sprintf(" σ=%.1f", upd.SigmaMah)
	}
	p.log("[结算] 采信 cap=%d→%d dur=%s avgI=%dmA 口径=%s 估算=%dmAh%s",
		row.StartCap, row.EndCap, dur, avgI/1000, counterSrc, upd.EstUAh/1000, sigmaNote)
	if _, insErr := p.st.InsertSession(*row); insErr != nil {
		return false, &SettleError{Err: insErr}
	}
	if estErr := p.st.InsertEstimate(row.EndTs, upd.EstUAh); estErr != nil {
		return false, &SettleError{Err: estErr}
	}
	return true, nil
}

// learnCurrentScaleFromSession 用本次会话的「电量计差分 vs 原始读数·秒」反推
// µA/原始单位并落盘（kv cur_scale），供之后所有采样/积分/倍率门控使用。
// 只在结算已采信电量计口径后调用：电量计是否可信、比值是否贴近 10 的整数次幂
// 都已由 resolveCounterSource 与 learnCurrentScale 把关，因此允许覆盖旧刻度，
// current_now 单位变化后由后续会话自动纠正。学不出刻度时落 cur_cal_fail 事件
// 并带上原始比值，供诊断定位。返回 scale 与「本次是否学到新刻度」：同刻度不
// 重复落盘，调用方据此决定要不要重算本会话的积分电量。任何失败只留痕，
// 绝不影响结算主链路。
func (p *Pipeline) learnCurrentScaleFromSession(rawUAs, counterUAh int64) (float64, bool) {
	scale, ratio, ok := learnCurrentScale(rawUAs, counterUAh)
	if !ok {
		// 已采信电量计却学不出刻度：落带原始比值的 cur_cal_fail 事件，
		// 便于区分「电源节点单位反常」与「采样缺口/电量计口径错」。
		if ratio > 0 {
			_ = p.st.InsertEvent("cur_cal_fail", fmt.Sprintf(
				"比值=%.2f×（电量计/原始积分）rawUAs=%d 电量计=%dµAh，偏离 10 的整数次幂超过 %.1f decade",
				ratio, rawUAs, counterUAh, curScaleSnapTol))
		}
		return 0, false
	}
	old := p.curScale
	if old == scale {
		return scale, false
	}
	if err := p.st.KVSet(kvCurrentScale, strconv.FormatFloat(scale, 'f', -1, 64)); err != nil {
		_ = p.st.InsertEvent("cur_cal_fail", err.Error())
		return 0, false
	}
	p.curScale = scale
	_ = p.st.InsertEvent("cur_cal", fmt.Sprintf("scale=%g 旧=%g 比值=%.2f rawUAs=%d 电量计=%dµAh",
		scale, old, ratio, rawUAs, counterUAh))
	// 只在单位口径真正变化时打日志：正常设备 ×1 属预期，无需刷日志
	if old > 0 || scale != anchorScaleUA(p.fullUAh) {
		p.log("[校准] 电流刻度 %g→%g µA/原始单位（电量计差分与原始读数积分比值）", old, scale)
	}
	return scale, true
}

// recordCCCT 从会话样本中识别跨 0.1V 窗的恒流段并落 ccct 表。倍率门控
// ≤1C：段均值超过设计容量不采信；designUAh 缺失时无从判定倍率，
// 同样视为不采信。0/≥2 个跨界段、查询失败等均以事件落痕后静默返回。
// 返回通过倍率门控的恒流段覆盖样本行，供同源特征（ICA）顺延复用；
// 门控不成立（设计容量缺失 / 读样本出错 / 无过门段）时返回 nil。
func (p *Pipeline) recordCCCT(startTs, end int64) []SampleRow {
	ev := func(kind, detail string) { _ = p.st.InsertEvent(kind, detail) }
	if p.designUAh <= 0 {
		ev("ccct_skip", "设计容量缺失，无法判倍率，跳过 CCCT")
		return nil
	}
	rows, err := p.st.SamplesRange(startTs, end)
	if err != nil {
		ev("ccct_skip", err.Error())
		return nil
	}
	segs := DetectCCSegs(rows, ccctSegWin)
	gateRejected := 0
	crossings := 0
	var loS, hiS SampleRow
	var gatedRows []SampleRow
	for _, g := range segs {
		// 倍率门控 ≤1C：按本机实测数据定标（正常快充恒流段均值 0.9~1.25C，
		// 旧 ≤C/2 门限把合法快充一票否决）。Fly & Chen 2020 表明高倍率下
		// ICA 峰显著退化（文献推荐 C/24~C/25），1C 属工程折衷，同源特征
		// 只按趋势方向采信。
		if g.MeanUA > p.designUAh {
			gateRejected++
			continue
		}
		gatedRows = append(gatedRows, rowsInRange(rows, g)...)
		l, h, cross := locateWindowCross(rows, g)
		if !cross {
			continue
		}
		crossings++
		loS, hiS = l, h
	}
	switch {
	case crossings == 1:
		p.log("[CCCT] 采信穿窗 %dmV→%dmV 耗时 %d 分钟", loS.UV/1000, hiS.UV/1000, (hiS.TS-loS.TS)/60)
		if ierr := p.st.InsertCCCT(hiS.TS, loS.UV, hiS.UV, hiS.TS-loS.TS); ierr != nil {
			ev("ccct_skip", ierr.Error())
		}
	case crossings == 0:
		// 归因全量明细：恒流段总数、过门/超限分布、跨窗数。旧文案只报
		// 「倍率超限」，实测掩盖主根因（过门段未跨整窗 / 会话起点高于窗）。
		if gateRejected > 0 {
			ev("ccct_skip", fmt.Sprintf("恒流段 %d 个(过门 %d/超限 %d)，均未跨越整窗，未采信",
				len(segs), len(segs)-gateRejected, gateRejected))
		} else {
			ev("cc_unstable", fmt.Sprintf("恒流段 %d 个，均未跨越整窗", len(segs)))
		}
	default:
		ev("ccct_skip", fmt.Sprintf("%d 个恒流段同时跨越整窗，无法唯一归因", crossings))
	}
	return gatedRows
}

// recordICA 与 CCCT 共用同一批过倍率门控的恒流段样本行，在 CCCT 分析之后顺延
// 执行：FindPeak 定位主峰与绝对峰高，按 kv 基准（ica_peak_base）rel 化后落
// ica_peaks 表。首个合格会话写基准并记 rel=1；基准异常（非正数或非数值，
// 含 0/负/NaN）跳过本会话。与 recordCCCT 同口径：一切失败仅记 events 留痕后
// 静默返回，绝不影响结算主链路。
func (p *Pipeline) recordICA(endTs int64, gatedRows []SampleRow) {
	ev := func(kind, detail string) { _ = p.st.InsertEvent(kind, detail) }
	if len(gatedRows) == 0 {
		return
	}
	uv, hAbs, ok := FindPeak(gatedRows)
	if !ok {
		// 平滑线无显著主峰属常态而非异常：静默跳过，不产生事件噪声。
		return
	}
	rel := 1.0
	if txt, has := p.st.KVGet(kvICAPeakBase); has {
		base, perr := strconv.ParseFloat(txt, 64)
		if perr != nil || !(base > 0) { // 须为正数：同时拦 0/负/NaN（NaN 比较为 false）
			ev("ica_skip", "ica_peak_base 基准异常："+txt)
			return
		}
		rel = hAbs / base
	} else if serr := p.st.KVSet(kvICAPeakBase, strconv.FormatFloat(hAbs, 'f', -1, 64)); serr != nil {
		ev("ica_skip", serr.Error())
		return
	}
	if math.IsInf(rel, 0) || math.IsNaN(rel) {
		// 基准虽为正但小到令 rel 溢出为 Inf 时，写库会连带 json.Marshal 整体
		// 报错失效：按基准异常同路径降级留痕。
		ev("ica_skip", "ica_peak_base 过小致 rel 非有限："+strconv.FormatFloat(rel, 'g', -1, 64))
		return
	}
	if ierr := p.st.InsertICAPeak(endTs, uv, rel); ierr != nil {
		ev("ica_skip", ierr.Error())
	}
}

func (p *Pipeline) pushWindow(iUA, vUV int64) {
	p.winI = append(p.winI, iUA)
	p.winV = append(p.winV, vUV)
	if len(p.winI) > resWindowMax {
		p.winI = p.winI[len(p.winI)-resWindowMax:]
		p.winV = p.winV[len(p.winV)-resWindowMax:]
	}
}

func (p *Pipeline) evalResistance() error {
	n := len(p.winI)
	if n < resMinSamples || n != len(p.winV) {
		return nil
	}
	var sumI, sumV, sumII, sumIV float64
	for k := 0; k < n; k++ {
		fi, fv := float64(p.winI[k]), float64(p.winV[k])
		sumI += fi
		sumV += fv
		sumII += fi * fi
		sumIV += fi * fv
	}
	fn := float64(n)
	denom := fn*sumII - sumI*sumI
	if denom == 0 {
		return nil
	}
	meanI := sumI / fn
	var sqSum float64
	for k := 0; k < n; k++ {
		d := float64(p.winI[k]) - meanI
		sqSum += d * d
	}
	if math.Sqrt(sqSum/fn) < resMinStdUA {
		return nil
	}
	// 内阻样本仅在充电态采集，充电时 V = OCV + I·R，斜率 dV/dI = +R，取正号。
	// 显示口径（rMoh）对表中最近样本取中位数，这里只负责把过门限的原始值落表。
	mo := ((fn*sumIV - sumI*sumV) / denom) * 1000
	if mo < resMinMOhm || mo > resMaxMOhm {
		return nil
	}
	ts := p.now().Unix()
	return p.st.InsertResistance(ts, mo)
}

func (p *Pipeline) tickResting(status string) error {
	if status != "Discharging" {
		p.restStreak = 0
		return nil
	}
	iRaw, err := p.readNodeSigned("current_now")
	if err != nil {
		return &SysfsTransient{Err: err}
	}
	if absI64(p.currentUA(iRaw)) >= restQuietUA {
		p.restStreak = 0
		return nil
	}
	p.restStreak++
	if p.restStreak < restMinTicks {
		return nil
	}
	uv, uerr := p.readNode("voltage_now")
	if uerr != nil {
		return nil
	}
	capVal, cerr := p.readNode("capacity")
	if cerr != nil {
		return nil
	}
	if p.restStreak == restRelaxTicks {
		// 收敛指纹点：绕过去重强制记录（不同 ts 本就不冲突）
		if err := p.st.InsertRestPoint(p.now().Unix(), uv, capVal); err != nil {
			return &SettleError{Err: err}
		}
		p.lastRestCap, p.lastRestUV = capVal, uv
		return nil
	}
	driftLimit := restDedupUVDrift * int64(p.cellCount)
	if absI64(capVal-p.lastRestCap) < restDedupCapDelta &&
		absI64(uv-p.lastRestUV) <= driftLimit {
		return nil
	}
	if err := p.st.InsertRestPoint(p.now().Unix(), uv, capVal); err != nil {
		return &SettleError{Err: err}
	}
	p.lastRestCap, p.lastRestUV = capVal, uv
	return nil
}
