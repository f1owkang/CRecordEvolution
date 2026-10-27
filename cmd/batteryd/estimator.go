package main

// KVStore 是 kv 层的读写接口，估算器与 Pipeline 共用，便于测试注入。
type KVStore interface {
	KVGet(key string) (string, bool)
	KVSet(key, val string) error
}

// SettledSession 是结算结果与估算器之间的输入契约：AccUAs 为电流积分电量
// （µA·s），CounterUAh 为电量计差分电量（µAh）。
type SettledSession struct {
	Session
	AccUAs int64
	// CounterUAh 会话期电量计差分增量（µAh）：>0 时估算优先采用（与放电侧、
	// 内核 charge_full 同源，免疫 current_now 刻度偏差与满电假电流）；0 表示
	// 节点不可读或刻度异常，回退 AccUAs 电流积分口径。
	CounterUAh int64
	DesignUAh  int64
}

// fullCapacityUAh 由会话推算满容量（µAh）：电量计差分优先，缺失时回退电流积分
// （µA·s ÷ 3600 折算 µAh）。两条口径同以显示涨幅为除数。
func fullCapacityUAh(sr SettledSession, delta int64) int64 {
	if sr.CounterUAh > 0 {
		return sr.CounterUAh * 100 / delta
	}
	return sr.AccUAs * 100 / (delta * 3600)
}

// SessionResult 为一次会话的估算判定结果；Accepted=false 时 Reason 给出门控原因。
type SessionResult struct {
	Accepted bool
	EstUAh   int64
	Reason   string
}

// EstUpdate 为估算器输出：EstUAh 为融合后满容量（µAh），SigmaMah 为 mAh 域不确定度。
type EstUpdate struct {
	EstUAh  int64
	Samples int64
	Changed bool
	// SigmaMah 置信区间 σ（mAh 域，RLS φᵀPφ 推出；stable 无 P 矩阵恒为 0）
	SigmaMah float64
}

// Estimator 是容量估计器接口：每个有效会话调用一次 OnSession。
type Estimator interface {
	OnSession(sr SettledSession) (EstUpdate, error)
}

// RejectError 表示会话被门控拒绝（预期结果，不是系统错误）。
type RejectError struct{ Result SessionResult }

// Error 返回门控拒绝原因，实现 error 接口。
func (e *RejectError) Error() string { return e.Result.Reason }

var _ Estimator = (*Stable)(nil)
var _ Estimator = (*Learning)(nil)
var _ KVStore = (*Store)(nil)

// NewStable 创建默认（stable）估算器。
func NewStable(kv KVStore) *Stable {
	return &Stable{kv: kv}
}

// NewLearning 创建 ML 实验版估算器；cellCount 用于 VStart 归一化。
func NewLearning(kv KVStore, cellCount int) *Learning {
	return &Learning{kv: kv, cellCount: cellCount}
}
