package main

type KVStore interface {
	KVGet(key string) (string, bool)
	KVSet(key, val string) error
}

type SettledSession struct {
	Session
	AccUA    int64
	DesignUA int64
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

func NewLearning(kv KVStore) *Learning {
	return &Learning{kv: kv}
}
