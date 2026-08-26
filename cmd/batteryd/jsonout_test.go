package main

import (
	"bytes"
	"encoding/json"
	"math"
	"strings"
	"testing"
	"time"
)

func TestRenderJSONGoldenWithNulls(t *testing.T) {
	d := Design{DesignMah: 4000, FullMah: 3850, Cycles: 210}
	snap := Snapshot{Pct: 96}
	now := time.Date(2026, 8, 26, 14, 30, 5, 0, time.UTC)
	want := `{"channel":"stable","design_mah":4000,"full_mah":3850,"cycles":210,"pct":96,"est_mah":null,"samples":0,"cycle_equiv":0,"r_moh":null,"temp_c":null,"updated":"2026-08-26 14:30:05","recent":[]}`
	got, err := RenderJSON("stable", d, snap, []TsVal{}, now)
	if err != nil {
		t.Fatalf("RenderJSON: %v", err)
	}
	if string(got) != want {
		t.Fatalf("got  %s\nwant %s", got, want)
	}
}

func TestRenderJSONGoldenFull(t *testing.T) {
	estUA := int64(4501234)
	rMoh := 0.85
	tempC := 28.5
	d := Design{DesignMah: 5000, FullMah: 4600, Cycles: 322}
	snap := Snapshot{Pct: 92, EstUA: &estUA, Samples: 42, CycleEquiv: 12.5, RMoh: &rMoh, TempC: &tempC}
	recent := []TsVal{{TS: 1778000000, V: 4501}, {TS: 1777999940, V: 4498}}
	now := time.Date(2026, 8, 26, 9, 5, 1, 0, time.UTC)
	want := `{"channel":"ml","design_mah":5000,"full_mah":4600,"cycles":322,"pct":92,"est_mah":4501,"samples":42,"cycle_equiv":12.5,"r_moh":0.85,"temp_c":28.5,"updated":"2026-08-26 09:05:01","recent":[{"ts":1778000000,"mah":4501},{"ts":1777999940,"mah":4498}]}`
	got, err := RenderJSON("ml", d, snap, recent, now)
	if err != nil {
		t.Fatalf("RenderJSON: %v", err)
	}
	if string(got) != want {
		t.Fatalf("got  %s\nwant %s", got, want)
	}
}

func TestRenderJSONSanitizesNonFiniteFloats(t *testing.T) {
	d := Design{DesignMah: 4000, FullMah: 3850, Cycles: 210}
	now := time.Date(2026, 8, 26, 14, 30, 5, 0, time.UTC)

	nanR := math.NaN()
	infTemp := math.Inf(1)
	snapNaN := Snapshot{Pct: 96, CycleEquiv: 3.5, RMoh: &nanR}
	out, err := RenderJSON("stable", d, snapNaN, nil, now)
	if err != nil {
		t.Fatalf("RenderJSON: %v", err)
	}
	if len(out) == 0 {
		t.Fatal("输出为空：Marshal 错误被静默吞掉")
	}
	if !json.Valid(out) {
		t.Fatalf("输出非法 JSON: %s", out)
	}
	if want := `"r_moh":null`; !strings.Contains(string(out), want) {
		t.Fatalf("NaN 的 r_moh 应渲染为 %s，got %s", want, out)
	}

	infCycle := math.Inf(-1)
	snapInf := Snapshot{Pct: 96, CycleEquiv: infCycle, TempC: &infTemp}
	out, err = RenderJSON("stable", d, snapInf, nil, now)
	if err != nil {
		t.Fatalf("RenderJSON: %v", err)
	}
	if len(out) == 0 {
		t.Fatal("输出为空：Marshal 错误被静默吞掉")
	}
	if !json.Valid(out) {
		t.Fatalf("输出非法 JSON: %s", out)
	}
	for _, want := range []string{`"cycle_equiv":null`, `"temp_c":null`} {
		if !strings.Contains(string(out), want) {
			t.Fatalf("%s 缺失，got %s", want, out)
		}
	}
	if bytes.Contains(out, []byte("Infinity")) || bytes.Contains(out, []byte("NaN")) {
		t.Fatalf("非有限值泄漏进输出: %s", out)
	}
}
