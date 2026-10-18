package main

import (
	"errors"
	"fmt"
	"math"
	"os"
	"strings"
)

var ErrNoData = errors.New("无可用电池数据")

type Design struct {
	DesignMah int64
	HasDesign bool
	FullMah   int64
	HasFull   bool
	Cycles    int64
	HasCycles bool
	Pct       int64
	HasPct    bool
}

type Snapshot struct {
	EstUA      *int64
	Samples    int64
	CycleEquiv float64
	RMoh       *float64
	TempC      *float64
	SigmaMah   *float64

	TrendMahPerWeek *float64
	TrendState      string
	TrendSpanDay    *int64

	// Quality 通道互检结果（各路容量口径互相印证），无从互检时为 nil
	Quality *Quality
}

// BuildDescription 按可用数据逐段组装描述：缺哪个字段就省略哪个段。
// 两条独立路径的健康度都前置，并标明来源，避免把「实测 5814mAh」和
// 「健康 95%」当成同一个口径：
//
//	内核健康 = charge_full / charge_full_design（系统电量计读数）
//	实测健康 = 本模块估算容量 / 设计容量
//
// 有实测值时，满充/设计等中间量留给 WebUI，描述行保持一行可读。
func BuildDescription(d Design, snap Snapshot) (string, error) {
	var parts []string
	if d.HasPct {
		parts = append(parts, fmt.Sprintf("内核健康 %d%%", d.Pct))
	}
	if snap.EstUA != nil {
		estMah := *snap.EstUA / 1000
		if d.HasDesign && d.DesignMah > 0 {
			pct := int64(math.Round(float64(estMah) / float64(d.DesignMah) * 100))
			if channel == "ml" && snap.SigmaMah != nil && *snap.SigmaMah > 0 {
				parts = append(parts, fmt.Sprintf("实测健康 %d%%（%d±%d mAh）", pct, estMah, int64(math.Round(*snap.SigmaMah))))
			} else {
				parts = append(parts, fmt.Sprintf("实测健康 %d%%（%d mAh）", pct, estMah))
			}
		} else if channel == "ml" && snap.SigmaMah != nil && *snap.SigmaMah > 0 {
			// 缺设计容量时无法折算百分比，只报实测容量
			parts = append(parts, fmt.Sprintf("实测 %d±%d mAh", estMah, int64(math.Round(*snap.SigmaMah))))
		} else {
			parts = append(parts, fmt.Sprintf("实测 %d mAh", estMah))
		}
	} else {
		// 没有实测值时保留内核口径的上下文
		if d.HasFull {
			parts = append(parts, fmt.Sprintf("满充 %d mAh", d.FullMah))
		}
		if d.HasDesign {
			parts = append(parts, fmt.Sprintf("设计 %d mAh", d.DesignMah))
		}
	}
	if d.HasCycles {
		parts = append(parts, fmt.Sprintf("循环 %d次", d.Cycles))
	}
	if len(parts) == 0 {
		return "", ErrNoData
	}
	desc := strings.Join(parts, "｜")
	if channel == "ml" {
		desc = "[ML实验版]" + desc
	}
	return desc, nil
}

func WriteModuleProp(propPath, newDescription string) error {
	data, err := os.ReadFile(propPath)
	if err != nil {
		return err
	}
	lines := strings.Split(string(data), "\n")
	found := false
	for i, line := range lines {
		if strings.HasPrefix(line, "description=") {
			lines[i] = "description=" + newDescription
			found = true
		}
	}
	if !found {
		if n := len(lines); n > 0 && lines[n-1] == "" {
			lines = append(lines[:n-1], "description="+newDescription, "")
		} else {
			lines = append(lines, "description="+newDescription)
		}
	}
	tmp := propPath + ".tmp"
	if err := os.WriteFile(tmp, []byte(strings.Join(lines, "\n")), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, propPath)
}
