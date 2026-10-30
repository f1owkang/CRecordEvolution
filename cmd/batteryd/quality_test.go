package main

import (
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// 实机口径（2026-09 设备 b，单位 µAh）：ema 5108 / mid 5324 / dis 6004 / full 6050 mAh，
// 四路互差 18% 属正常系统性偏差；量纲误判期中段口径被推到 15592mAh（互差 160%）。
func TestCapacityQualityNormalSpread(t *testing.T) {
	q := CapacityQuality(5_108_000, 5_324_000, 6_004_000, 6_050_000)
	if q == nil {
		t.Fatal("四路齐全应返回互检结果")
	}
	if !q.OK {
		t.Fatalf("正常互差 18%% 应判一致, got spread=%d%%", q.SpreadPct)
	}
	if q.SpreadPct != 18 {
		t.Fatalf("spread = %d, want 18", q.SpreadPct)
	}
	if q.High != "full" || q.Low != "ema" {
		t.Fatalf("偏高/偏低口径 = %s/%s, want full/ema", q.High, q.Low)
	}
	if q.EmaMah == nil || *q.EmaMah != 5108 || q.FullMah == nil || *q.FullMah != 6050 {
		t.Fatalf("各路口径换算 mAh 有误: %+v", q)
	}
}

func TestCapacityQualityFlagsCorruptedChannel(t *testing.T) {
	// 量纲误判实测值：中段 15592mAh 对内核 6050mAh
	q := CapacityQuality(7_189_122, 15_592_000, 6_004_000, 6_050_000)
	if q == nil || q.OK {
		t.Fatalf("量纲误判应判不一致, got %+v", q)
	}
	if q.SpreadPct < 150 || q.High != "mid" {
		t.Fatalf("spread/high = %d%%/%s, want ≥150%%/mid", q.SpreadPct, q.High)
	}
}

func TestCapacityQualityNeedsTwoChannels(t *testing.T) {
	if q := CapacityQuality(0, 0, 6_004_000, 0); q != nil {
		t.Fatalf("单路无从互检应返回 nil, got %+v", q)
	}
	if q := CapacityQuality(5_108_000, 0, 0, 6_050_000); q == nil {
		t.Fatal("两路即可互检")
	}
	// 缺哪路省哪路
	q := CapacityQuality(5_108_000, 0, 6_004_000, 0)
	if q.MidMah != nil || q.FullMah != nil || q.EmaMah == nil || q.DisMah == nil {
		t.Fatalf("缺失路应省略: %+v", q)
	}
}

// 时效判定纯函数：超保留窗口按缺失处理；旧库缺 ts 不判死但要求回填宽限；
// 窗口边界与时钟回拨都不判过期。
func TestJudgeImpliedFreshness(t *testing.T) {
	const (
		now   = int64(1_700_000_000)
		day   = int64(86400)
		value = int64(5_300_000)
	)
	cases := []struct {
		name      string
		value     int64
		ts        int64
		wantValue int64
		wantStale bool
		wantNoTs  bool
		wantAge   int64
	}{
		{"通道缺失", 0, now, 0, false, false, 0},
		{"刚更新", value, now - 10*day, value, false, false, 10 * day},
		{"窗口边界不算过期", value, now - int64(retainDays)*day, value, false, false, int64(retainDays) * day},
		{"超窗口过期", value, now - int64(retainDays+1)*day, 0, true, false, int64(retainDays+1) * day},
		{"旧库缺时间戳", value, 0, value, false, true, 0},
		{"时钟回拨不判过期", value, now + 3600, value, false, false, 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := judgeImpliedFreshness(c.value, c.ts, now, retainDays)
			if got.ValueUAh != c.wantValue || got.Stale != c.wantStale ||
				got.MissingTS != c.wantNoTs || got.AgeSecs != c.wantAge {
				t.Fatalf("judgeImpliedFreshness = %+v, want {value:%d stale:%v missingTS:%v age:%d}",
					got, c.wantValue, c.wantStale, c.wantNoTs, c.wantAge)
			}
		})
	}
}

// stats() 出口共用的只读过滤：过期视为缺失（0），缺 ts 的旧值保留（宽限），
// 未过期原样参与互检。
func TestFreshImpliedUAh(t *testing.T) {
	st := openTestStore(t, filepath.Join(t.TempDir(), "quality.db"))
	defer func() { _ = st.Close() }()
	const now = int64(1_700_000_000)
	day := int64(86400)

	mustKV := func(k, v string) {
		t.Helper()
		if err := st.KVSet(k, v); err != nil {
			t.Fatal(err)
		}
	}
	// 过期：按缺失处理
	mustKV(kvMidImpliedUAh, "15592000")
	mustKV(kvMidImpliedTs, strconv.FormatInt(now-int64(retainDays+1)*day, 10))
	if got := freshImpliedUAh(st, kvMidImpliedUAh, kvMidImpliedTs, now); got != 0 {
		t.Fatalf("过期 mid 应返回 0, got %d", got)
	}
	// 未过期：原值参与互检
	mustKV(kvDisImpliedUAh, "6004000")
	mustKV(kvDisImpliedTs, strconv.FormatInt(now-10*day, 10))
	if got := freshImpliedUAh(st, kvDisImpliedUAh, kvDisImpliedTs, now); got != 6_004_000 {
		t.Fatalf("未过期 dis 应返回原值, got %d", got)
	}
	// 旧库缺 ts：保留原值给宽限，不判死
	mustKV(kvMidImpliedUAh, "5300000")
	if err := st.KVSet(kvMidImpliedTs, "0"); err != nil {
		t.Fatal(err)
	}
	if got := freshImpliedUAh(st, kvMidImpliedUAh, kvMidImpliedTs, now); got != 5_300_000 {
		t.Fatalf("缺 ts 的旧值应保留（宽限）, got %d", got)
	}
}

// 装配层：过期的 mid 不参与互检，避免概览长期拿半年前的旧值误报/掩盖问题，
// 同时落一次 quality_stale 事件说明剔除原因。
func TestCheckQualityExcludesStaleChannel(t *testing.T) {
	r := newPipeRig(t)
	now := r.cur.Unix()
	mustSet := func(k, v string) {
		if err := r.st.KVSet(k, v); err != nil {
			t.Fatal(err)
		}
	}
	mustSet(kvKeyEmaUAh, "5108000")
	mustSet(kvMidImpliedUAh, "15592000") // 量纲误判期的旧值
	mustSet(kvMidImpliedTs, strconv.FormatInt(now-int64(retainDays+1)*86400, 10))
	mustSet(kvDisImpliedUAh, "6004000")
	mustSet(kvDisImpliedTs, strconv.FormatInt(now-10*86400, 10))
	r.p.fullUAh = 6_050_000

	r.p.checkQuality()
	// 剔除过期 mid 后剩 ema/dis/full 三路，互差 18% ⇒ 结论一致
	if v, _ := r.st.KVGet(kvQualityOK); v != "1" {
		t.Fatalf("过期 mid 剔除后应判一致, quality_ok=%q", v)
	}
	var detail string
	if err := r.st.db.QueryRow(
		`SELECT detail FROM events WHERE kind='quality_stale' ORDER BY rowid DESC LIMIT 1`).Scan(&detail); err != nil {
		t.Fatalf("应落 quality_stale 事件: %v", err)
	}
	if !strings.Contains(detail, "mid") || !strings.Contains(detail, strconv.Itoa(retainDays+1)) {
		t.Fatalf("quality_stale detail 应说明 mid 过期天数, got %q", detail)
	}
	var warn int
	if err := r.st.db.QueryRow(`SELECT count(*) FROM events WHERE kind='quality_warn'`).Scan(&warn); err != nil {
		t.Fatal(err)
	}
	if warn != 0 {
		t.Fatalf("剔除过期值后不应误报 quality_warn, got %d", warn)
	}
	// 时间戳仍保留（下次写入会覆盖），不做二次改写
	if got, _ := kvString(t, r.st, kvMidImpliedTs); got != strconv.FormatInt(now-int64(retainDays+1)*86400, 10) {
		t.Fatalf("过期渠道的 ts 不应被改写, got %q", got)
	}
}

// 装配层：旧库只有隐含值没有时间戳时不判死，首次读取回填当前时间给一次宽限，
// 并落事件说明；该次互检仍让值参与，结论翻转逻辑不变。
func TestCheckQualityBackfillsLegacyTimestamps(t *testing.T) {
	r := newPipeRig(t)
	now := r.cur.Unix()
	mustSet := func(k, v string) {
		if err := r.st.KVSet(k, v); err != nil {
			t.Fatal(err)
		}
	}
	mustSet(kvKeyEmaUAh, "5108000")
	mustSet(kvMidImpliedUAh, "15592000") // 旧库值，无 ts
	mustSet(kvDisImpliedUAh, "6004000")  // 旧库值，无 ts
	r.p.fullUAh = 6_050_000

	r.p.checkQuality()
	for _, k := range []string{kvMidImpliedTs, kvDisImpliedTs} {
		if got, _ := kvString(t, r.st, k); got != strconv.FormatInt(now, 10) {
			t.Fatalf("旧库首次读取应把 %s 回填为当前时间 %d, got %q", k, now, got)
		}
	}
	var graceN int
	if err := r.st.db.QueryRow(`SELECT count(*) FROM events WHERE kind='quality_stale' AND detail LIKE '%宽限%'`).Scan(&graceN); err != nil {
		t.Fatal(err)
	}
	if graceN != 2 {
		t.Fatalf("mid/dis 各应落 1 条宽限说明, got %d", graceN)
	}
	// 值仍参与本次互检：mid 15592mAh 与内核 6050mAh 互差超限 ⇒ 结论不变
	var warn int
	if err := r.st.db.QueryRow(`SELECT count(*) FROM events WHERE kind='quality_warn'`).Scan(&warn); err != nil {
		t.Fatal(err)
	}
	if warn != 1 {
		t.Fatalf("宽限值应照常参与互检并给出 1 条 quality_warn, got %d", warn)
	}

	// 再次互检：ts 已回填、值未变 ⇒ 不再重复留痕
	r.p.checkQuality()
	if err := r.st.db.QueryRow(`SELECT count(*) FROM events WHERE kind='quality_stale'`).Scan(&graceN); err != nil {
		t.Fatal(err)
	}
	if graceN != 2 {
		t.Fatalf("ts 回填后不应重复宽限事件, got %d", graceN)
	}
	if err := r.st.db.QueryRow(`SELECT count(*) FROM events WHERE kind='quality_warn'`).Scan(&warn); err != nil {
		t.Fatal(err)
	}
	if warn != 1 {
		t.Fatalf("结论未翻转不应重复 quality_warn, got %d", warn)
	}
}

// 未过期渠道行为不变：正常参与互检，不落任何时效事件。
func TestCheckQualityFreshChannelsUnchanged(t *testing.T) {
	r := newPipeRig(t)
	now := r.cur.Unix()
	mustSet := func(k, v string) {
		if err := r.st.KVSet(k, v); err != nil {
			t.Fatal(err)
		}
	}
	mustSet(kvKeyEmaUAh, "5108000")
	mustSet(kvMidImpliedUAh, "5324000")
	mustSet(kvMidImpliedTs, strconv.FormatInt(now-86400, 10))
	mustSet(kvDisImpliedUAh, "6004000")
	mustSet(kvDisImpliedTs, strconv.FormatInt(now-2*86400, 10))
	r.p.fullUAh = 6_050_000

	r.p.checkQuality()
	if v, _ := r.st.KVGet(kvQualityOK); v != "1" {
		t.Fatalf("未过期四路互差 18%% 应判一致, quality_ok=%q", v)
	}
	if n := countRows(t, r.st, "events"); n != 0 {
		t.Fatalf("未过期渠道不应落时效事件, events=%d", n)
	}
}

// 结论翻转才落事件：连续两次同样的结论只留一条 quality_warn
func TestCheckQualityWarnsOncePerFlip(t *testing.T) {
	r := newPipeRig(t)
	warnCount := func() int {
		t.Helper()
		var n int
		if err := r.st.db.QueryRow(`SELECT count(*) FROM events WHERE kind='quality_warn'`).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	r.st.KVSet(kvKeyEmaUAh, "7189122")
	r.st.KVSet(kvMidImpliedUAh, "15592000")
	r.st.KVSet(kvDisImpliedUAh, "6004000")
	r.p.fullUAh = 6_050_000

	r.p.checkQuality()
	if n := warnCount(); n != 1 {
		t.Fatalf("首次不一致应落 1 条 quality_warn, got %d", n)
	}
	r.p.checkQuality()
	if n := warnCount(); n != 1 {
		t.Fatalf("结论未翻转不应重复留痕, got %d", n)
	}
	// 口径回正后结论翻转：恢复一致（只打日志不落事件）
	r.st.KVSet(kvMidImpliedUAh, "6004000")
	r.p.checkQuality()
	if v, _ := r.st.KVGet(kvQualityOK); v != "1" {
		t.Fatalf("回正后应记为一致, quality_ok=%q", v)
	}
	if n := warnCount(); n != 1 {
		t.Fatalf("恢复一致不应回落 warning, got %d", n)
	}
}
