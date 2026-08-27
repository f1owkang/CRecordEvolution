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
	d := Design{DesignMah: 4000, HasDesign: true, FullMah: 3850, HasFull: true, Cycles: 210, HasCycles: true, Pct: 96, HasPct: true}
	snap := Snapshot{}
	now := time.Date(2026, 8, 26, 14, 30, 5, 0, time.UTC)
	want := `{"channel":"stable","design_mah":4000,"full_mah":3850,"cycles":210,"pct":96,"est_mah":null,"samples":0,"cycle_equiv":0,"r_moh":null,"temp_c":null,"updated":"2026-08-26 14:30:05","recent":[],"sessions":null,"rest_points":null,"samples_n":0}`
	got, err := RenderJSON("stable", d, snap, []TsVal{}, nil, nil, 0, now)
	if err != nil {
		t.Fatalf("RenderJSON: %v", err)
	}
	if string(got) != want {
		t.Fatalf("got  %s\nwant %s", got, want)
	}
}

func TestRenderJSONGoldenPartialNulls(t *testing.T) {
	// 缺设计容量与循环次数 → 对应字段渲染为 null
	d := Design{FullMah: 3850, HasFull: true}
	now := time.Date(2026, 8, 26, 14, 30, 5, 0, time.UTC)
	want := `{"channel":"stable","design_mah":null,"full_mah":3850,"cycles":null,"pct":null,"est_mah":null,"samples":0,"cycle_equiv":0,"r_moh":null,"temp_c":null,"updated":"2026-08-26 14:30:05","recent":[],"sessions":null,"rest_points":null,"samples_n":0}`
	got, err := RenderJSON("stable", d, Snapshot{}, []TsVal{}, nil, nil, 0, now)
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
	d := Design{DesignMah: 5000, HasDesign: true, FullMah: 4600, HasFull: true, Cycles: 322, HasCycles: true, Pct: 92, HasPct: true}
	snap := Snapshot{EstUA: &estUA, Samples: 42, CycleEquiv: 12.5, RMoh: &rMoh, TempC: &tempC}
	// estimates 表存 µAh，RenderJSON 负责 /1000 转 mAh
	recent := []TsVal{{TS: 1778000000, V: 4501000}, {TS: 1777999940, V: 4498000}}
	now := time.Date(2026, 8, 26, 9, 5, 1, 0, time.UTC)
	want := `{"channel":"ml","design_mah":5000,"full_mah":4600,"cycles":322,"pct":92,"est_mah":4501,"samples":42,"cycle_equiv":12.5,"r_moh":0.85,"temp_c":28.5,"updated":"2026-08-26 09:05:01","recent":[{"ts":1778000000,"mah":4501},{"ts":1777999940,"mah":4498}],"sessions":null,"rest_points":null,"samples_n":0}`
	got, err := RenderJSON("ml", d, snap, recent, nil, nil, 0, now)
	if err != nil {
		t.Fatalf("RenderJSON: %v", err)
	}
	if string(got) != want {
		t.Fatalf("got  %s\nwant %s", got, want)
	}
}

func TestRenderJSONSanitizesNonFiniteFloats(t *testing.T) {
	d := Design{DesignMah: 4000, HasDesign: true, FullMah: 3850, HasFull: true, Cycles: 210, HasCycles: true, Pct: 96, HasPct: true}
	now := time.Date(2026, 8, 26, 14, 30, 5, 0, time.UTC)

	nanR := math.NaN()
	infTemp := math.Inf(1)
	snapNaN := Snapshot{CycleEquiv: 3.5, RMoh: &nanR}
	out, err := RenderJSON("stable", d, snapNaN, nil, nil, nil, 0, now)
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
	snapInf := Snapshot{CycleEquiv: infCycle, TempC: &infTemp}
	out, err = RenderJSON("stable", d, snapInf, nil, nil, nil, 0, now)
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

func TestRenderJSONNewFields(t *testing.T) {
	d := Design{HasDesign: true, DesignMah: 4500, HasFull: true, FullMah: 4450, HasCycles: true, Cycles: 123}
	snap := Snapshot{}
	recent := []TsVal{{TS: 1, V: 4_400_000}}
	sess := []sessionEntry{{StartTs: 100, EndTs: 200, DeltaMah: 60, EstMah: intPtr(4500), Valid: true}}
	rests := []restEntry{{TS: 300, UV: 3_900_000, Cap: 60}}
	b, err := RenderJSON("stable", d, snap, recent, sess, rests, 42, time.Unix(0, 0))
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	for _, want := range []string{`"samples_n":42`, `"valid":true`, `"est_mah":4500`, `"uv":3900000`} {
		if !strings.Contains(s, want) {
			t.Fatalf("缺字段 %s: %s", want, s)
		}
	}
	b2, _ := RenderJSON("stable", Design{}, Snapshot{}, nil, nil, nil, 0, time.Unix(0, 0))
	if strings.Contains(string(b2), `"sessions":[{`) {
		t.Fatalf("空数据不应出会话数组元素: %s", b2)
	}
}
