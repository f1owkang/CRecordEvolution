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
}

// BuildDescription 按可用数据逐段组装描述：缺哪个字段就省略哪个段。
func BuildDescription(d Design, snap Snapshot) (string, error) {
	var parts []string
	if snap.EstUA != nil {
		if channel == "ml" && snap.SigmaMah != nil && *snap.SigmaMah > 0 {
			parts = append(parts, fmt.Sprintf("实测 %d±%d mAh", *snap.EstUA/1000, int64(math.Round(*snap.SigmaMah))))
		} else {
			parts = append(parts, fmt.Sprintf("实测 %d mAh", *snap.EstUA/1000))
		}
	}
	if d.HasPct {
		parts = append(parts, fmt.Sprintf("健康 %d%%", d.Pct))
	}
	if d.HasFull {
		parts = append(parts, fmt.Sprintf("当前 %d mAh", d.FullMah))
	}
	if d.HasDesign {
		parts = append(parts, fmt.Sprintf("设计 %d mAh", d.DesignMah))
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
