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

type jsonDoc struct {
	Channel    string        `json:"channel"`
	DesignMah  int64         `json:"design_mah"`
	FullMah    int64         `json:"full_mah"`
	Cycles     int64         `json:"cycles"`
	Pct        int64         `json:"pct"`
	EstMah     *int64        `json:"est_mah"`
	Samples    int64         `json:"samples"`
	CycleEquiv *float64      `json:"cycle_equiv"`
	RMoh       *float64      `json:"r_moh"`
	TempC      *float64      `json:"temp_c"`
	Updated    string        `json:"updated"`
	Recent     []recentEntry `json:"recent"`
}

func finitePtr(f *float64) *float64 {
	if f == nil || math.IsNaN(*f) || math.IsInf(*f, 0) {
		return nil
	}
	return f
}

func RenderJSON(ch string, d Design, snap Snapshot, recent []TsVal, now time.Time) ([]byte, error) {
	doc := jsonDoc{
		Channel:    ch,
		DesignMah:  d.DesignMah,
		FullMah:    d.FullMah,
		Cycles:     d.Cycles,
		Pct:        snap.Pct,
		Samples:    snap.Samples,
		RMoh:       finitePtr(snap.RMoh),
		TempC:      finitePtr(snap.TempC),
		Updated:    now.Format("2006-01-02 15:04:05"),
		Recent:     make([]recentEntry, 0, len(recent)),
	}
	if !math.IsNaN(snap.CycleEquiv) && !math.IsInf(snap.CycleEquiv, 0) {
		doc.CycleEquiv = &snap.CycleEquiv
	}
	if snap.EstUA != nil {
		mah := *snap.EstUA / 1000
		doc.EstMah = &mah
	}
	for _, tv := range recent {
		doc.Recent = append(doc.Recent, recentEntry{TS: tv.TS, Mah: tv.V})
	}
	b, err := json.Marshal(doc)
	if err != nil {
		return nil, err
	}
	return b, nil
}
