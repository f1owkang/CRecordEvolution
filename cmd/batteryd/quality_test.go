package main

import "testing"

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
	r.st.KVSet(kvKeyEmaUA, "7189122")
	r.st.KVSet(kvMidImpliedUA, "15592000")
	r.st.KVSet(kvDisImpliedUA, "6004000")
	r.p.fullUA = 6_050_000

	r.p.checkQuality()
	if n := warnCount(); n != 1 {
		t.Fatalf("首次不一致应落 1 条 quality_warn, got %d", n)
	}
	r.p.checkQuality()
	if n := warnCount(); n != 1 {
		t.Fatalf("结论未翻转不应重复留痕, got %d", n)
	}
	// 口径回正后结论翻转：恢复一致（只打日志不落事件）
	r.st.KVSet(kvMidImpliedUA, "6004000")
	r.p.checkQuality()
	if v, _ := r.st.KVGet(kvQualityOK); v != "1" {
		t.Fatalf("回正后应记为一致, quality_ok=%q", v)
	}
	if n := warnCount(); n != 1 {
		t.Fatalf("恢复一致不应回落 warning, got %d", n)
	}
}
