package main

import (
	"path/filepath"
	"testing"
)

// ICA 基准迁移：旧口径（跨段间隙计入主峰）的 base 必须先清掉，否则新口径下
// 所有 rel 会掉到 ~0.1；迁移动一次后不再重复清理新基准。
func TestMigrateICAPeakBaseOnce(t *testing.T) {
	st := openTestStore(t, filepath.Join(t.TempDir(), "battery.db"))
	t.Cleanup(func() { _ = st.Close() })

	if err := st.KVSet(kvICAPeakBase, "529149000"); err != nil {
		t.Fatalf("预置旧基准: %v", err)
	}
	migrateICAPeakBase(st)
	if _, ok := st.KVGet(kvICAPeakBase); ok {
		t.Fatal("迁移后旧 ica_peak_base 应被删除")
	}
	if v, ok := st.KVGet(kvICAPeakBaseSchema); !ok || v != icaPeakBaseSchema {
		t.Fatalf("迁移标记 = (%q,%v), want (%q,true)", v, ok, icaPeakBaseSchema)
	}
	var kind string
	if err := st.db.QueryRow(`SELECT kind FROM events ORDER BY ts DESC, rowid DESC LIMIT 1`).Scan(&kind); err != nil {
		t.Fatalf("读迁移事件: %v", err)
	}
	if kind != "ica_rebase" {
		t.Fatalf("迁移事件 kind = %q, want ica_rebase", kind)
	}

	// 新口径基准写入后，再次启动（newApp 每次都会调）不得被清掉。
	if err := st.KVSet(kvICAPeakBase, "154338000"); err != nil {
		t.Fatalf("写新基准: %v", err)
	}
	migrateICAPeakBase(st)
	if v, ok := st.KVGet(kvICAPeakBase); !ok || v != "154338000" {
		t.Fatalf("二次迁移不应清理新基准, got (%q,%v)", v, ok)
	}
}

// 旧版本会在「首次回填失败但 daily 已有一行」时误落完成标记；daemon 启动修复
// 必须忽略该标记、按 estimates 全量重算，且只跑一次。
func TestRepairDailyBackfillHealsStaleMarker(t *testing.T) {
	st := openTestStore(t, filepath.Join(t.TempDir(), "battery.db"))
	t.Cleanup(func() { _ = st.Close() })

	base := int64(1_760_000_000)
	points := []struct {
		ts  int64
		uah int64
	}{
		{base, 4_000_000},
		{base + 3_600, 4_050_000},
		{base + 86_400, 4_100_000},
	}
	for _, p := range points {
		if _, err := st.db.Exec(`INSERT INTO estimates(ts, mah) VALUES(?, ?)`, p.ts, p.uah); err != nil {
			t.Fatalf("写 estimates: %v", err)
		}
	}
	expected := aggregateDaily([]TsVal{
		{TS: points[0].ts, V: points[0].uah},
		{TS: points[1].ts, V: points[1].uah},
		{TS: points[2].ts, V: points[2].uah},
	})

	// 旧 bug 现场：daily 只有当天一条、完成标记已落。
	day1 := localDayKey(points[0].ts)
	if _, err := st.db.Exec(`INSERT INTO daily(day, ts, uah, n) VALUES(?, ?, ?, 1)`,
		day1, points[0].ts, points[0].uah); err != nil {
		t.Fatalf("写 daily: %v", err)
	}
	if err := st.KVSet(kvDailyBackfill, "1"); err != nil {
		t.Fatalf("置完成标记: %v", err)
	}

	if err := st.RepairDailyBackfill(); err != nil {
		t.Fatalf("RepairDailyBackfill: %v", err)
	}
	got, err := st.DailyEstimates(archiveQueryLimit)
	if err != nil {
		t.Fatalf("DailyEstimates: %v", err)
	}
	if len(got) != len(expected) {
		t.Fatalf("daily 行数 = %d, want %d", len(got), len(expected))
	}
	for i := range expected {
		if got[i].Day != expected[i].Day || got[i].TS != expected[i].TS ||
			got[i].UAh != expected[i].UAh || got[i].N != expected[i].N {
			t.Fatalf("day %d = (%d,%d,%d,%d), want (%d,%d,%d,%d)", i,
				got[i].Day, got[i].TS, got[i].UAh, got[i].N,
				expected[i].Day, expected[i].TS, expected[i].UAh, expected[i].N)
		}
	}
	if v, ok := st.KVGet(kvDailyBackfillRepair); !ok || v != dailyBackfillRepairSchema {
		t.Fatalf("修复标记 = (%q,%v), want (%q,true)", v, ok, dailyBackfillRepairSchema)
	}

	// 幂等：第二次调用不再写事件。
	var events int
	if err := st.db.QueryRow(`SELECT COUNT(*) FROM events WHERE kind='daily_repair'`).Scan(&events); err != nil {
		t.Fatalf("统计事件: %v", err)
	}
	if err := st.RepairDailyBackfill(); err != nil {
		t.Fatalf("第二次 RepairDailyBackfill: %v", err)
	}
	var events2 int
	if err := st.db.QueryRow(`SELECT COUNT(*) FROM events WHERE kind='daily_repair'`).Scan(&events2); err != nil {
		t.Fatalf("统计事件: %v", err)
	}
	if events2 != events {
		t.Fatalf("修复应只跑一次: events %d → %d", events, events2)
	}
}

// 从未回填过的库（完成标记缺失）不归修复逻辑管，交给 BackfillDaily。
func TestRepairDailyBackfillNoopBeforeFirstBackfill(t *testing.T) {
	st := openTestStore(t, filepath.Join(t.TempDir(), "battery.db"))
	t.Cleanup(func() { _ = st.Close() })
	if err := st.RepairDailyBackfill(); err != nil {
		t.Fatalf("RepairDailyBackfill: %v", err)
	}
	if _, ok := st.KVGet(kvDailyBackfillRepair); ok {
		t.Fatal("未回填过的库不应落修复标记")
	}
}
