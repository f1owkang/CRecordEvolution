package main

import (
	"errors"
	"fmt"
	"math"
	"os"
	"strings"
)

// ErrNoData 表示没有任何可用电池数据，无法组装描述。
var ErrNoData = errors.New("无可用电池数据")

// Design 为内核口径的电池静态信息（容量单位 mAh，Pct 为内核健康百分比）。
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

// Snapshot 为估算出口快照：EstUAh 为实测满容量（µAh），其余为显示层派生量。
type Snapshot struct {
	// Version 为模块版本（"1.3.6(35)"），供 WebUI 关于页展示；读不到时为空。
	Version    string
	EstUAh     *int64
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
	// Calib 电流刻度校准诊断（设置页展示），节点全缺失时为 nil
	Calib *CalibInfo
}

// BuildDescription 按可用数据逐段组装描述：缺哪个字段就省略哪个段。
// 两条独立路径的健康度都前置，并标明来源，避免把「实测 5814mAh」和
// 「健康 95%」当成同一个口径：
//
//	内核健康 = charge_full / charge_full_design（系统电量计读数）
//	实测健康 = 本模块估算容量 / 设计容量
//
// 有实测值时，满充/设计等中间量留给 WebUI，描述行保持一行可读。
// 文案按界面语言输出：中文分支保持原字节不变，英文分支见 i18n.go；
// 数值与单位之间保留空格，中文分隔符「｜」对应英文 " | "。
func BuildDescription(d Design, snap Snapshot) (string, error) {
	var parts []string
	if d.HasPct {
		if d.HasFull && d.FullMah > 0 {
			parts = append(parts, uiText(
				fmt.Sprintf("内核健康 %d%%（%d mAh）", d.Pct, d.FullMah),
				fmt.Sprintf("Kernel health %d%% (%d mAh)", d.Pct, d.FullMah),
			))
		} else {
			parts = append(parts, fmt.Sprintf("%s %d%%", uiText("内核健康", "Kernel health"), d.Pct))
		}
	}
	if snap.EstUAh != nil {
		estMah := *snap.EstUAh / 1000
		if d.HasDesign && d.DesignMah > 0 {
			pct := int64(math.Round(float64(estMah) / float64(d.DesignMah) * 100))
			body := fmt.Sprintf("%d mAh", estMah)
			if channel == "ml" && snap.SigmaMah != nil && *snap.SigmaMah > 0 {
				body = fmt.Sprintf("%d±%d mAh", estMah, int64(math.Round(*snap.SigmaMah)))
			}
			// 中文全角括号紧贴百分号；英文半角括号前留空格，避免 "90%(3600 mAh)"
			parts = append(parts, uiText(
				fmt.Sprintf("实测健康 %d%%（%s）", pct, body),
				fmt.Sprintf("Measured health %d%% (%s)", pct, body),
			))
		} else if channel == "ml" && snap.SigmaMah != nil && *snap.SigmaMah > 0 {
			// 缺设计容量时无法折算百分比，只报实测容量
			parts = append(parts, fmt.Sprintf("%s %d±%d mAh", uiText("实测", "Measured"), estMah, int64(math.Round(*snap.SigmaMah))))
		} else {
			parts = append(parts, fmt.Sprintf("%s %d mAh", uiText("实测", "Measured"), estMah))
		}
	} else {
		// 没有实测值时保留内核口径的上下文；内核健康已带满充 mAh 时不再重复“满充”。
		if d.HasFull && !d.HasPct {
			parts = append(parts, fmt.Sprintf("%s %d mAh", uiText("满充", "Full charge"), d.FullMah))
		}
		if d.HasDesign {
			parts = append(parts, fmt.Sprintf("%s %d mAh", uiText("设计", "Design"), d.DesignMah))
		}
	}
	if d.HasCycles {
		parts = append(parts, fmt.Sprintf("%s %d%s", uiText("循环", "Cycles"), d.Cycles, uiText("次", "")))
	}
	if snap.TempC != nil {
		t := *snap.TempC
		if !math.IsNaN(t) && !math.IsInf(t, 0) {
			parts = append(parts, uiText(
				fmt.Sprintf("温度 %.1f℃", t),
				fmt.Sprintf("Temp %.1f°C", t),
			))
		}
	}
	if len(parts) == 0 {
		return "", ErrNoData
	}
	desc := strings.Join(parts, uiText("｜", " | "))
	if channel == "ml" {
		// 英文前缀后补一个空格更自然；中文保持原样紧贴描述
		desc = uiText("[ML实验版]", "[ML experimental] ") + desc
	}
	return desc, nil
}

// WriteModuleProp 用临时文件 + rename 原子替换 module.prop 的 description 行。
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
