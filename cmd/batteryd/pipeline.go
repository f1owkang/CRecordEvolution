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
	kvREmaMOhm     = "r_ema_mo"

	restQuietUA       int64 = 20000
	restMinTicks            = 3
	restDedupCapDelta int64 = 1
	restDedupUVDrift  int64 = 5000
	restRelaxTicks          = 10 // e-Energy '23：静置约 10 分钟电压收敛，可作 SoH 指纹

	resWindowMax  int     = 30
	resMinSamples int     = 20
	resMinStdUA   float64 = 50000
	resMinMOhm    float64 = 0.5
	resMaxMOhm    float64 = 500

	kvSessActive   = "sess_active"
	kvSessSealed   = "sess_sealed"
	kvSessStartTs  = "sess_start_ts"
	kvSessStartCap = "sess_start_cap"
	kvSessVStart   = "sess_v_start_uv"
	kvSessAcc      = "sess_acc_uas"
	kvSessTicks    = "sess_ticks"
	kvSessTempMin  = "sess_temp_min"
	kvSessTempMax  = "sess_temp_max"
	kvSessTempSum  = "sess_temp_sum"
	kvSessTempN    = "sess_temp_n"
	kvSessLastCap  = "sess_last_cap"
)

type TickOutcome struct{ SessionSettled bool }

type SettleError struct{ Err error }

func (e *SettleError) Error() string { return "结算或落库失败：" + e.Err.Error() }

func (e *SettleError) Unwrap() error { return e.Err }

type sessionState struct {
	active   bool
	sealed   bool
	startTs  int64
	startCap int64
	vStartUV int64
	accUAs   int64
	ticks    int64
	tempMin  int64
	tempMax  int64
	tempSum  float64
	tempN    int64
	lastCap  int64
}

type Pipeline struct {
	fs       SysFS
	st       *Store
	est      Estimator
	designUA int64
	now      func() time.Time

	nodePaths map[string]string

	sess sessionState

	// notChargStreak status 连续非 Charging 的拍数（去抖计数，不持久化：
	// 进程重启后从 0 重新计数，最多多等 3 拍才结算，无害）
	notChargStreak int

	winI []int64
	winV []int64

	restStreak  int
	lastRestUV  int64
	lastRestCap int64
}

func NewPipeline(fs SysFS, st *Store, est Estimator, designUA int64, clock func() time.Time) *Pipeline {
	p := &Pipeline{
		fs:        fs,
		st:        st,
		est:       est,
		designUA:  designUA,
		now:       clock,
		nodePaths: map[string]string{},
	}
	p.restoreSession()
	return p
}

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

func btoa(b bool) string {
	if b {
		return "1"
	}
	return "0"
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

func (p *Pipeline) restoreSession() {
	if v, ok := p.st.KVGet(kvSessActive); !ok || v != "1" {
		return
	}
	p.sess = sessionState{
		active:   true,
		startTs:  kvInt(p.st, kvSessStartTs),
		startCap: kvInt(p.st, kvSessStartCap),
		vStartUV: kvInt(p.st, kvSessVStart),
		accUAs:   kvInt(p.st, kvSessAcc),
		ticks:    kvInt(p.st, kvSessTicks),
		tempMin:  kvInt(p.st, kvSessTempMin),
		tempMax:  kvInt(p.st, kvSessTempMax),
		tempSum:  kvFloat(p.st, kvSessTempSum),
		tempN:    kvInt(p.st, kvSessTempN),
		lastCap:  kvInt(p.st, kvSessLastCap),
	}
	p.sess.sealed = kvText(p.st, kvSessSealed) == "1"
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

// notChargDebounce 非充电状态去抖：status 连续非 Charging 达到该拍数才真正
// 结算会话。米系满电保护/优化充电会让 status 在充电过程中间歇抖为
// Not charging/Full，单拍即断会把同一晚充电切碎成一串涨幅不足的废会话
// （实测：4 条 delta=0 垃圾行全部源于连续 2 拍抖动）。3 拍≈3 分钟，与
// ACC「多次采样确认、不信单拍 status」的策略同源。
const notChargDebounce = 3

func (p *Pipeline) Tick(status string) (TickOutcome, error) {
	outcome := TickOutcome{}
	if status == "Charging" {
		// 回到 Charging：去抖计数清零，原会话（若在去抖等待期）原样继续
		p.notChargStreak = 0
		return outcome, p.tickCharging(&outcome)
	}
	p.notChargStreak++
	// 去抖期内不算断开：保留会话，等下一拍
	if p.sess.active && p.notChargStreak < notChargDebounce {
		return outcome, nil
	}
	if err := p.tickResting(status); err != nil {
		return outcome, err
	}
	if p.sess.active && !p.sess.sealed {
		if err := p.settle(); err != nil {
			return outcome, err
		}
		outcome.SessionSettled = true
	}
	if p.sess.active {
		if err := p.resetSession(); err != nil {
			return outcome, &SettleError{Err: err}
		}
	}
	return outcome, nil
}

func (p *Pipeline) tickCharging(outcome *TickOutcome) error {
	capVal, err := p.readNode("capacity")
	if err != nil {
		return err
	}
	iRaw, err := p.readNodeSigned("current_now")
	if err != nil {
		return err
	}
	iAbs := absI64(iRaw)
	iUA := absI64(NormCurrentUA(iAbs))

	var tempC float64
	haveTemp := false
	if tRaw, terr := p.readNode("temp"); terr == nil {
		tempC = NormTempC(tRaw)
		haveTemp = true
	}
	vUV, verr := p.readNode("voltage_now")
	if verr != nil {
		vUV = 0
	}
	if vUV > 0 {
		p.pushWindow(iUA, vUV)
		if err := p.evalResistance(); err != nil {
			return err
		}
	}
	if vUV > 0 && capVal > 0 {
		if err := p.st.InsertSample(p.now().Unix(), iUA, vUV, capVal); err != nil {
			_ = p.st.InsertEvent("sample_fail", err.Error())
		}
	}

	s := &p.sess
	if !s.active {
		// 已满（cap≥sealCapacity）时插入充电器：米系满电保护下 delta 恒为 0，
		// 该会话必被 delta_lt_20 拒收，只产生垃圾行——不开启会话，等真实回落。
		if capVal >= sealCapacity {
			return nil
		}
		s.active = true
		s.startTs = p.now().Unix()
		s.startCap = capVal
		s.vStartUV = vUV
	}
	if !s.sealed {
		s.accUAs += iUA * tickSeconds
		s.ticks++
		s.lastCap = capVal
		if haveTemp {
			ti := int64(tempC)
			if s.tempN == 0 || ti < s.tempMin {
				if s.tempN == 0 {
					s.tempMin, s.tempMax = ti, ti
				} else {
					s.tempMin = ti
				}
			}
			if ti > s.tempMax {
				s.tempMax = ti
			}
			s.tempSum += tempC
			s.tempN++
		}
	}

	total := kvInt(p.st, kvChargedTotal) + iUA*tickSeconds
	if err := p.st.KVSet(kvChargedTotal, strconv.FormatInt(total, 10)); err != nil {
		return &SettleError{Err: err}
	}

	if !s.sealed {
		if err := p.persistSession(); err != nil {
			return &SettleError{Err: err}
		}
		if capVal >= sealCapacity {
			if err := p.settle(); err != nil {
				return err
			}
			outcome.SessionSettled = true
			s.sealed = true
			if err := p.persistSession(); err != nil {
				return &SettleError{Err: err}
			}
		}
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
		{kvSessSealed, btoa(s.sealed)},
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

func (p *Pipeline) settle() error {
	s := &p.sess
	// 结算开始即置会话为不活跃：若进程在此之后、写库完成前被杀，
	// 重启后不会重复结算（最多丢失本次会话，绝不产生重复行）。
	if err := p.st.KVSet(kvSessActive, "0"); err != nil {
		return &SettleError{Err: err}
	}
	duration := s.ticks * tickSeconds
	var avgI int64
	if duration > 0 {
		avgI = s.accUAs / duration
	}
	row := Session{
		StartTs:  s.startTs,
		EndTs:    p.now().Unix(),
		StartCap: s.startCap,
		EndCap:   s.lastCap,
		Ua:       s.accUAs,
		AvgI:     avgI,
		CRate:    float64(avgI) / float64(p.designUA),
		TempMin:  s.tempMin,
		TempMax:  s.tempMax,
		TempAvg:  tempAvgOf(s.tempSum, s.tempN),
		VStart:   s.vStartUV,
		Duration: duration,
		Valid:    false,
	}
	sr := SettledSession{Session: row, AccUA: s.accUAs, DesignUA: p.designUA}

	upd, err := p.est.OnSession(sr)
	if err != nil {
		var re *RejectError
		if !errors.As(err, &re) {
			return &SettleError{Err: err}
		}
		row.Valid = false
		row.InvalidReason = re.Result.Reason
		if _, insErr := p.st.InsertSession(row); insErr != nil {
			return &SettleError{Err: insErr}
		}
		if evErr := p.st.InsertEvent(re.Result.Reason, ""); evErr != nil {
			return &SettleError{Err: evErr}
		}
		return nil
	}
	row.Valid = true
	if _, insErr := p.st.InsertSession(row); insErr != nil {
		return &SettleError{Err: insErr}
	}
	if estErr := p.st.InsertEstimate(row.EndTs, upd.EstUA); estErr != nil {
		return &SettleError{Err: estErr}
	}
	// CCCT 特征采集：只在有效结算后做一次，60~240 行扫描毫秒级；一切
	// 失败仅记 events，静默跳过，绝不影响结算主链路。同源特征 ICA 复用
	// 其返回的过门段样本行，避免重复扫库。
	p.recordICA(row.EndTs, p.recordCCCT(s.startTs, row.EndTs))
	return nil
}

// recordCCCT 从会话样本中识别跨 0.1V 窗的恒流段并落 ccct 表。倍率门控
// ≤1C：段均值超过设计容量不采信；designUA 缺失时无从判定倍率，
// 同样视为不采信。0/≥2 个跨界段、查询失败等均以事件落痕后静默返回。
// 返回通过倍率门控的恒流段覆盖样本行，供同源特征（ICA）顺延复用；
// 门控不成立（设计容量缺失 / 读样本出错 / 无过门段）时返回 nil。
func (p *Pipeline) recordCCCT(startTs, end int64) []SampleRow {
	ev := func(kind, detail string) { _ = p.st.InsertEvent(kind, detail) }
	if p.designUA <= 0 {
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
		// 倍率门控 ≤1C：实测本机正常快充恒流段均值 0.9~1.25C，旧 ≤C/2
		// 门限把合法快充一票否决（ccct_skip「段均值×2>设计容量」），
		// 依 Fly & Chen 速率约束口径放宽到 1C。
		if g.MeanUA > p.designUA {
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
		if ierr := p.st.InsertCCCT(hiS.TS, loS.UV, hiS.UV, hiS.TS-loS.TS); ierr != nil {
			ev("ccct_skip", ierr.Error())
		}
	case crossings == 0 && gateRejected > 0:
		ev("ccct_skip", fmt.Sprintf("%d 个恒流段倍率超限(段均值>%d µA)，未采信", gateRejected, p.designUA))
	case crossings == 0:
		ev("cc_unstable", "无可跨越整窗的恒流段")
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
	mo := ((fn*sumIV - sumI*sumV) / denom) * 1000
	if mo < resMinMOhm || mo > resMaxMOhm {
		return nil
	}
	ts := p.now().Unix()
	if err := p.st.InsertResistance(ts, mo); err != nil {
		return &SettleError{Err: err}
	}
	ema := mo
	if old, ok := p.st.KVGet(kvREmaMOhm); ok {
		if f, perr := strconv.ParseFloat(old, 64); perr == nil && f > 0 {
			ema = f*0.8 + mo*0.2
		}
	}
	if err := p.st.KVSet(kvREmaMOhm, strconv.FormatFloat(ema, 'f', -1, 64)); err != nil {
		return &SettleError{Err: err}
	}
	return nil
}

func (p *Pipeline) tickResting(status string) error {
	if status != "Discharging" {
		p.restStreak = 0
		return nil
	}
	iRaw, err := p.readNodeSigned("current_now")
	if err != nil {
		return err
	}
	if absI64(NormCurrentUA(absI64(iRaw))) >= restQuietUA {
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
	if absI64(capVal-p.lastRestCap) < restDedupCapDelta &&
		absI64(uv-p.lastRestUV) <= restDedupUVDrift {
		return nil
	}
	if err := p.st.InsertRestPoint(p.now().Unix(), uv, capVal); err != nil {
		return &SettleError{Err: err}
	}
	p.lastRestCap, p.lastRestUV = capVal, uv
	return nil
}
