package main

import (
	"math"
	"testing"
	"time"
)

// absI64 对 math.MinInt64 取负会溢出回自身：必须饱和到 MaxInt64，否则
// plausibleDelta/电流幅值比较会把「极端异常读数」当成负值小电流放行。
func TestAbsI64SaturatesOnMinInt64(t *testing.T) {
	if got := absI64(math.MinInt64); got != math.MaxInt64 {
		t.Fatalf("absI64(MinInt64) = %d, want MaxInt64", got)
	}
	if got := absI64(-42); got != 42 {
		t.Fatalf("absI64(-42) = %d, want 42", got)
	}
	if got := absI64(7); got != 7 {
		t.Fatalf("absI64(7) = %d, want 7", got)
	}
}

// 结算的 end_ts 与 duration 必须来自同一次取时：否则两次取时跨秒会让落库的
// end_ts-start_ts 与 duration 差 1s，历史页时长按 end_ts-start_ts 计算，
// 与 sessions.duration 口径不一致。用「每次调用前进 1 秒」的时钟复现该边界。
func TestSettleUsesSingleClockReading(t *testing.T) {
	r := newPipeRig(t)
	p := r.p
	// 构造一个已累计 20 拍、20%→90% 的会话（隐含容量正好等于设计容量，过门控）。
	p.sess = sessionState{
		active:   true,
		startTs:  tickBaseTs,
		startCap: 20,
		lastCap:  90,
		accUAs:   2_800_000 * 3600,
		rawUAs:   2_800_000 * 3600,
		ticks:    20,
	}
	calls := int64(0)
	p.now = func() time.Time {
		calls++
		return time.Unix(tickBaseTs+600+calls, 0)
	}
	if err := p.settle(); err != nil {
		t.Fatalf("settle: %v", err)
	}
	s := onlySession(t, r.st)
	if s.EndTs-s.StartTs != s.Duration {
		t.Fatalf("end_ts-start_ts=%d 与 duration=%d 不一致（settle 取了多次时钟）",
			s.EndTs-s.StartTs, s.Duration)
	}
}
