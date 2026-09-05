package main

import "strconv"

const (
	stableTempMinC = 15
	// 上界 45°C：锂电池标准充电温度窗上限，充电控制器在此之上才强制降流；
	// 实测夏季机身 41~44°C 的会话被 40°C 门限整批拒收（b 设备 24 会话拒 7），
	// 白丢近三成样本，且未满充降权路径本就抑制其显示拉动，45°C 放行无碍。
	stableTempMaxC = 45
	minDeltaCap    = 20
	// fullSealCap 满充封账下沿：cap=100 封账或 status=Full 去抖结算（实测可落
	// 99）视为满充。仅作 EMA 权重分界而非门控——未满充会话的隐含容量随结束点
	// 系统性偏高（实测设备：内核报数中段滞后于真实 SOC），全部拒收会饿死日常
	// 补电场景，降权采信折中显示漂移与采信量。
	fullSealCap = 99

	kvKeySamples = "samples"
	kvKeyEmaUA   = "ema_ua"
)

// emaBlend 单会话向基线融合：满充会话权重 3/10，未满充会话降权至 1/10——
// 未满充会话的隐含容量偏高（见 fullSealCap 注），降权抑制显示值随拔充习惯
// 漂移，同时保留其采信资格。
func emaBlend(old, est, endCap int64) int64 {
	if endCap >= fullSealCap {
		return (old*7 + est*3) / 10
	}
	return (old*9 + est) / 10
}

type Stable struct{ kv KVStore }

func (s *Stable) OnSession(sr SettledSession) (EstUpdate, error) {
	res := evaluateStable(sr)
	if !res.Accepted {
		return EstUpdate{}, &RejectError{Result: res}
	}

	samples := s.kvInt(kvKeySamples)
	old := s.kvInt(kvKeyEmaUA)
	ema := res.EstUA
	if samples > 0 {
		ema = emaBlend(old, res.EstUA, sr.EndCap)
	}
	samples++

	if err := s.kv.KVSet(kvKeyEmaUA, strconv.FormatInt(ema, 10)); err != nil {
		return EstUpdate{}, err
	}
	if err := s.kv.KVSet(kvKeySamples, strconv.FormatInt(samples, 10)); err != nil {
		return EstUpdate{}, err
	}
	return EstUpdate{EstUA: ema, Samples: samples, Changed: true}, nil
}

// tempOutOfRange 判断会话温度是否越界；temp 节点缺失时 TempMin=TempMax=0
// （未知温度），不做门控。
func tempOutOfRange(sr SettledSession) bool {
	if sr.TempMin == 0 && sr.TempMax == 0 {
		return false
	}
	return sr.TempMin < stableTempMinC || sr.TempMax > stableTempMaxC
}

func evaluateStable(sr SettledSession) SessionResult {
	if tempOutOfRange(sr) {
		return SessionResult{Reason: "temp_out_of_range"}
	}
	delta := sr.EndCap - sr.StartCap
	if delta < minDeltaCap {
		return SessionResult{Reason: "delta_lt_20"}
	}
	uaFull := sr.AccUA * 100 / (delta * 3600)
	if uaFull*2 < sr.DesignUA || uaFull*2 > sr.DesignUA*3 {
		return SessionResult{Reason: "out_of_window"}
	}
	return SessionResult{Accepted: true, EstUA: uaFull}
}

func (s *Stable) kvInt(key string) int64 {
	v, ok := s.kv.KVGet(key)
	if !ok {
		return 0
	}
	n, _ := strconv.ParseInt(v, 10, 64)
	return n
}
