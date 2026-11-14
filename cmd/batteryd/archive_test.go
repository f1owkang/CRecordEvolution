package main

import (
	"path/filepath"
	"strconv"
	"testing"
	"time"
)

// localTestBase 返回本机时区下某天中午的 Unix 秒：加 1 小时必然仍属同一天，
// 加 2 天必然换日，用于构造与运行机器时区无关的档案测试数据。
func localTestBase(t *testing.T) int64 {
	t.Helper()
	localZoneOnce.Do(func() { localZone = resolveLocalZone() })
	return time.Date(2026, 11, 10, 12, 0, 0, 0, localZone).Unix()
}

func TestDayKeyAtUsesLocalDate(t *testing.T) {
	loc, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		t.Fatalf("LoadLocation: %v", err)
	}
	cases := []struct {
		ts   int64
		want int64
	}{
		// 上海 23:30 仍是当天
		{time.Date(2026, 11, 14, 23, 30, 0, 0, loc).Unix(), 20261114},
		// 上海 00:30 = UTC 前一天 16:30：必须按本地日算成 1115
		{time.Date(2026, 11, 15, 0, 30, 0, 0, loc).Unix(), 20261115},
		{time.Date(2026, 11, 14, 16, 30, 0, 0, time.UTC).Unix(), 20261115},
	}
	for _, tc := range cases {
		if got := dayKeyAt(tc.ts, loc); got != tc.want {
			t.Fatalf("dayKeyAt(%d)=%d want %d", tc.ts, got, tc.want)
		}
	}
}

func TestDownsampleDaily(t *testing.T) {
	rows := make([]DailyRow, 10)
	for i := range rows {
		rows[i] = DailyRow{Day: int64(20260101 + i), TS: 1000 + int64(i), UAh: int64(1000 * (i + 1)), N: 1}
	}
	got := downsampleDaily(rows, 3)
	if len(got) != 3 {
		t.Fatalf("len=%d want 3", len(got))
	}
	// 桶 [0,3) / [3,6) / [6,10)：容量取桶内均值，时间戳取桶内最后一天。
	if got[0].UAh != 2000 || got[0].TS != 1002 || got[0].Day != 20260103 {
		t.Fatalf("桶0=%+v，期望均值 2000、末点 ts=1002", got[0])
	}
	if got[1].UAh != 5000 || got[1].TS != 1005 {
		t.Fatalf("桶1=%+v，期望均值 5000、末点 ts=1005", got[1])
	}
	if got[2].UAh != 8500 || got[2].TS != 1009 {
		t.Fatalf("桶2=%+v，期望均值 8500、末点 ts=1009", got[2])
	}
	var n int64
	for _, d := range got {
		n += d.N
	}
	if n != 10 {
		t.Fatalf("聚合后点数 %d，期望覆盖全部 10 天", n)
	}
	// 不超过上限时原样返回；空输入不 panic。
	if again := downsampleDaily(rows, 10); len(again) != 10 {
		t.Fatalf("未超上限不应压缩，len=%d", len(again))
	}
	if len(downsampleDaily(nil, 3)) != 0 {
		t.Fatal("空输入应返回空")
	}
}

func TestInsertEstimateBuildsDailyArchive(t *testing.T) {
	s := openTestStore(t, filepath.Join(t.TempDir(), "battery.db"))
	defer func() { _ = s.Close() }()

	base := localTestBase(t)
	ts1 := base
	ts2 := base + 3600 // 同日较晚的一笔
	ts3 := base + 2*86400

	for _, it := range []struct {
		ts  int64
		mah int64
	}{{ts1, 4_100_000}, {ts2, 4_200_000}, {ts3, 4_050_000}} {
		if err := s.InsertEstimate(it.ts, it.mah); err != nil {
			t.Fatalf("InsertEstimate(%d): %v", it.ts, err)
		}
	}
	rows, err := s.DailyEstimates(10)
	if err != nil {
		t.Fatalf("DailyEstimates: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("档案行数=%d want 2（同日多笔并成一行）", len(rows))
	}
	if rows[0].UAh != 4_200_000 || rows[0].TS != ts2 || rows[0].N != 2 {
		t.Fatalf("首行=%+v，期望取当日最后一笔 4200000µAh、n=2", rows[0])
	}
	if rows[1].UAh != 4_050_000 || rows[1].TS != ts3 || rows[1].N != 1 {
		t.Fatalf("次行=%+v", rows[1])
	}
	// limit 只取最近若干天，仍按日期升序。
	last, err := s.DailyEstimates(1)
	if err != nil {
		t.Fatalf("DailyEstimates(1): %v", err)
	}
	if len(last) != 1 || last[0].TS != ts3 {
		t.Fatalf("limit=1 应只返回最新一天，got %+v", last)
	}
}

func TestBackfillDailyOnceFromEstimates(t *testing.T) {
	s := openTestStore(t, filepath.Join(t.TempDir(), "battery.db"))
	defer func() { _ = s.Close() }()

	base := localTestBase(t)
	// 直接写 estimates 表模拟升级前的旧库（绕过 InsertEstimate，档案仍为空）。
	mustExec(t, s, "INSERT INTO estimates(ts,mah) VALUES"+
		"("+strconv.FormatInt(base, 10)+",4100000),"+
		"("+strconv.FormatInt(base+3600, 10)+",4200000),"+
		"("+strconv.FormatInt(base+2*86400, 10)+",4050000)")

	if err := s.BackfillDaily(); err != nil {
		t.Fatalf("BackfillDaily: %v", err)
	}
	if n := countRows(t, s, "daily"); n != 2 {
		t.Fatalf("回填后档案行数=%d want 2", n)
	}
	if v, ok := s.KVGet(kvDailyBackfill); !ok || v != "1" {
		t.Fatalf("回填标记=%q ok=%v", v, ok)
	}
	rows, err := s.DailyEstimates(10)
	if err != nil {
		t.Fatalf("DailyEstimates: %v", err)
	}
	if len(rows) != 2 || rows[0].UAh != 4_200_000 || rows[0].N != 2 || rows[1].UAh != 4_050_000 {
		t.Fatalf("回填内容=%+v", rows)
	}
	// 幂等：重复执行不累加点数、不重复落行。
	if err := s.BackfillDaily(); err != nil {
		t.Fatalf("BackfillDaily(2): %v", err)
	}
	if n := countRows(t, s, "daily"); n != 2 {
		t.Fatalf("二次回填后档案行数=%d want 2", n)
	}
	rows2, err := s.DailyEstimates(10)
	if err != nil {
		t.Fatalf("DailyEstimates(2): %v", err)
	}
	if rows2[0].N != 2 {
		t.Fatalf("二次回填不应累加点数，got n=%d", rows2[0].N)
	}
}

func TestPruneBeforeKeepsDailyArchive(t *testing.T) {
	s := openTestStore(t, filepath.Join(t.TempDir(), "battery.db"))
	defer func() { _ = s.Close() }()

	base := localTestBase(t)
	if err := s.InsertEstimate(base, 4_000_000); err != nil {
		t.Fatalf("InsertEstimate: %v", err)
	}
	// 清理窗口切到写入时刻之后：明细应被删掉，长期档案必须保留。
	if err := s.PruneBefore(base + 86400); err != nil {
		t.Fatalf("PruneBefore: %v", err)
	}
	if n := countRows(t, s, "estimates"); n != 0 {
		t.Fatalf("estimates 应被清理，剩 %d 行", n)
	}
	if n := countRows(t, s, "daily"); n != 1 {
		t.Fatalf("daily 不应被清理，剩 %d 行", n)
	}
}
