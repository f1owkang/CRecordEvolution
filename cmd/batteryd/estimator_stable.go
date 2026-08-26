package main

import "strconv"

const (
	stableTempMinC = 15
	stableTempMaxC = 40
	minDeltaCap    = 20
	kvKeySamples   = "samples"
	kvKeyEmaUA     = "ema_ua"
)

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
		ema = (old*7 + res.EstUA*3) / 10
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
//（未知温度），不做门控。
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
