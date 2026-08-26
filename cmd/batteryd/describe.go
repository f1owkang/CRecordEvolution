package main

import (
	"fmt"
	"os"
	"strings"
)

type Design struct {
	DesignMah int64
	FullMah   int64
	Cycles    int64
}

type Snapshot struct {
	Pct        int64
	EstUA      *int64
	Samples    int64
	CycleEquiv float64
	RMoh       *float64
	TempC      *float64
}

func BuildDescription(d Design, snap Snapshot) string {
	base := fmt.Sprintf("出厂设计容量为：%d mAh，当前电池容量为：%d mAh，电池循环次数为：%d次，估算剩余容量百分比为：%d%%",
		d.DesignMah, d.FullMah, d.Cycles, snap.Pct)
	if snap.EstUA != nil {
		base += fmt.Sprintf("，实测估算容量为：%d mAh", *snap.EstUA/1000)
	}
	if channel == "ml" {
		base = "[ML实验版]" + base
	}
	return base
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
