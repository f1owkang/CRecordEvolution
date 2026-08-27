package main

import (
	"errors"
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

func (p *Pipeline) Tick(status string) (TickOutcome, error) {
	outcome := TickOutcome{}
	if status == "Charging" {
		return outcome, p.tickCharging(&outcome)
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
	return nil
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
