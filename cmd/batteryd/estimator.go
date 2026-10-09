package main

type KVStore interface {
	KVGet(key string) (string, bool)
	KVSet(key, val string) error
}

type SettledSession struct {
	Session
	AccUA int64
	// CounterUAh 会话期电量计差分增量（µAh）：>0 时估算优先采用（与放电侧、
	// 内核 charge_full 同源，免疫 current_now 刻度偏差与满电假电流）；0 表示
	// 节点不可读或刻度异常，回退 AccUA 电流积分口径。
	CounterUAh int64
	DesignUA   int64
}

// fullCapacityUA 由会话推算满容量（µAh）：电量计差分优先，缺失时回退电流积分
// （µA·s ÷ 3600 折算 µAh）。两条口径同以显示涨幅为除数。
func fullCapacityUA(sr SettledSession, delta int64) int64 {
	if sr.CounterUAh > 0 {
		return sr.CounterUAh * 100 / delta
	}
	return sr.AccUA * 100 / (delta * 3600)
}

type SessionResult struct {
	Accepted bool
	EstUA    int64
	Reason   string
}

type EstUpdate struct {
	EstUA   int64
	Samples int64
	Changed bool
	// SigmaMah 置信区间 σ（mAh 域，RLS φᵀPφ 推出；stable 无 P 矩阵恒为 0）
	SigmaMah float64
}

type Estimator interface {
	OnSession(sr SettledSession) (EstUpdate, error)
}

type RejectError struct{ Result SessionResult }

func (e *RejectError) Error() string { return e.Result.Reason }

var _ Estimator = (*Stable)(nil)
var _ Estimator = (*Learning)(nil)
var _ KVStore = (*Store)(nil)

func NewStable(kv KVStore) *Stable {
	return &Stable{kv: kv}
}

func NewLearning(kv KVStore, cellCount int) *Learning {
	return &Learning{kv: kv, cellCount: cellCount}
}
