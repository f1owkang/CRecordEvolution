package main

import (
	"encoding/json"
	"math"
	"time"
)

type recentEntry struct {
	TS  int64 `json:"ts"`
	Mah int64 `json:"mah"`
}

type sessionEntry struct {
	StartTs  int64    `json:"start_ts"`
	EndTs    int64    `json:"end_ts"`
	DeltaMah int64    `json:"delta_mah"`
	EstMah   *int64   `json:"est_mah"`
	Valid    bool     `json:"valid"`
	TempAvg  *int64   `json:"temp_avg"`
	CRate    *float64 `json:"c_rate"`
}

type restEntry struct {
	TS  int64 `json:"ts"`
	UV  int64 `json:"uv"`
	Cap int64 `json:"cap"`
}

type jsonDoc struct {
	Channel    string         `json:"channel"`
	DesignMah  *int64         `json:"design_mah"`
	FullMah    *int64         `json:"full_mah"`
	Cycles     *int64         `json:"cycles"`
	Pct        *int64         `json:"pct"`
	EstMah     *int64         `json:"est_mah"`
	Samples    int64          `json:"samples"`
	CycleEquiv *float64       `json:"cycle_equiv"`
	RMoh       *float64       `json:"r_moh"`
	TempC      *float64       `json:"temp_c"`
	Updated    string         `json:"updated"`
	Recent     []recentEntry  `json:"recent"`
	Sessions   []sessionEntry `json:"sessions"`
	RestPoints []restEntry    `json:"rest_points"`
	SamplesN   int64          `json:"samples_n"`
}

func finitePtr(f *float64) *float64 {
	if f == nil || math.IsNaN(*f) || math.IsInf(*f, 0) {
		return nil
	}
	return f
}

func intPtr(v int64) *int64 { return &v }

func convSession(se Session) sessionEntry {
	e := sessionEntry{StartTs: se.StartTs, EndTs: se.EndTs,
		DeltaMah: (se.EndCap - se.StartCap), Valid: se.Valid}
	if e.DeltaMah > 0 {
		mah := se.Ua * 100 / (e.DeltaMah * 3_600_000)
		e.EstMah = &mah
	}
	if se.TempAvg > 0 {
		t := se.TempAvg
		e.TempAvg = &t
	}
	if se.CRate > 0 {
		c := se.CRate
		e.CRate = &c
	}
	return e
}

func RenderJSON(ch string, d Design, snap Snapshot, recent []TsVal, sess []sessionEntry, rests []restEntry, samplesN int64, now time.Time) ([]byte, error) {
	doc := jsonDoc{
		Channel:    ch,
		Samples:    snap.Samples,
		RMoh:       finitePtr(snap.RMoh),
		TempC:      finitePtr(snap.TempC),
		Updated:    now.Format("2006-01-02 15:04:05"),
		Recent:     make([]recentEntry, 0, len(recent)),
		SamplesN:   samplesN,
	}
	if d.HasDesign {
		doc.DesignMah = intPtr(d.DesignMah)
	}
	if d.HasFull {
		doc.FullMah = intPtr(d.FullMah)
	}
	if d.HasCycles {
		doc.Cycles = intPtr(d.Cycles)
	}
	if d.HasPct {
		doc.Pct = intPtr(d.Pct)
	}
	if !math.IsNaN(snap.CycleEquiv) && !math.IsInf(snap.CycleEquiv, 0) {
		doc.CycleEquiv = &snap.CycleEquiv
	}
	if snap.EstUA != nil {
		mah := *snap.EstUA / 1000
		doc.EstMah = &mah
	}
	// estimates 表存 µAh，此处换算为 mAh 展示
	for _, tv := range recent {
		doc.Recent = append(doc.Recent, recentEntry{TS: tv.TS, Mah: tv.V / 1000})
	}
	// sessions/rest_points 直接透传调用方切片：nil 输出 null，非 nil 空切片输出 []，
	// 「空数据 ⇒ [] 禁止 null 噪声」由调用方（runJson）以 make(...,0) 保证
	doc.Sessions = sess
	doc.RestPoints = rests
	b, err := json.Marshal(doc)
	if err != nil {
		return nil, err
	}
	return b, nil
}
