package main

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

var ErrNodeNotFound = errors.New("sysfs 节点未找到")

const (
	defaultPowerSupplyBase = "/sys/class/power_supply"
	defaultDevicesRoot     = "/sys/devices"
)

type SysFS struct {
	Base    string
	devices string
}

func (s SysFS) base() string {
	if s.Base != "" {
		return s.Base
	}
	return defaultPowerSupplyBase
}

func (s SysFS) devicesRoot() string {
	if s.devices != "" {
		return s.devices
	}
	return defaultDevicesRoot
}

func (s SysFS) FindNode(name string) (string, error) {
	fixed := filepath.Join(s.base(), "battery", name)
	if f, err := os.Open(fixed); err == nil {
		f.Close()
		return fixed, nil
	}

	found := ""
	_ = filepath.WalkDir(s.devicesRoot(), func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if found != "" {
			return fs.SkipAll
		}
		if !d.IsDir() && d.Name() == name {
			found = path
			return fs.SkipAll
		}
		return nil
	})
	if found == "" {
		return "", ErrNodeNotFound
	}
	return found, nil
}

func (s SysFS) ReadInt(path string) (int64, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, err
	}
	text := strings.TrimSpace(string(data))
	if text == "" || strings.IndexFunc(text, func(r rune) bool {
		return r < '0' || r > '9'
	}) >= 0 {
		return 0, fmt.Errorf("ReadInt %q: 内容不是纯数字", path)
	}
	return strconv.ParseInt(text, 10, 64)
}

// ReadIntSigned 读取带可选正负号的整数。power_supply 规范约定放电电流为负值，
// 因此 current_now 必须走此入口；其余非负节点仍用严格校验的 ReadInt。
func (s SysFS) ReadIntSigned(path string) (int64, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, fmt.Errorf("ReadIntSigned %q: %w", path, err)
	}
	text := strings.TrimSpace(string(data))
	if text == "" {
		return 0, fmt.Errorf("ReadIntSigned %q: 空内容", path)
	}
	v, err := strconv.ParseInt(text, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("ReadIntSigned %q: 内容不是整数", path)
	}
	return v, nil
}

func NormCurrentUA(raw int64) int64 {
	if raw > 10000 {
		return raw
	}
	return raw * 1000
}

// NormCurrentUAWithFull 用 charge_full 交叉校验 current_now 单位。
// 启发式：若 |current_now| < max(charge_full/10, 10000)，认为是 mA 并 ×1000；
// 否则当作 µA 直通。chargeFullUA ≤ 0 时退化为原始启发式。
// 保底 10000 防止低电量时 charge_full/10 过小导致 µA 误判为 mA。
func NormCurrentUAWithFull(raw, chargeFullUA int64) int64 {
	abs := raw
	if abs < 0 {
		abs = -abs
	}
	threshold := chargeFullUA / 10
	if threshold < 10000 {
		threshold = 10000
	}
	if chargeFullUA > 0 && abs < threshold {
		return raw * 1000
	}
	if abs > 10000 {
		return raw
	}
	return raw * 1000
}

func NormTempC(raw int64) float64 {
	var c float64
	if raw >= 100 {
		c = float64(raw) / 10
	} else {
		c = float64(raw)
	}
	// 防御性边界：锂电合理温度 -40~80°C，超出范围截断防止脏数据污染 ML
	if c < -40 {
		c = -40
	} else if c > 80 {
		c = 80
	}
	return c
}

// readDTBatteryCapacity 从设备树读取 vivo,bat-capacity-mah（µAh）。
// VIVO/iQOO 等设备不把 charge_full_design 暴露到 sysfs，但设备树中有此值。
// 通过 find 命令搜索 /proc/device-tree 下含 "bat-capacity-mah" 的属性。
func readDTBatteryCapacity() (int64, error) {
	out, err := exec.Command("find", "/proc/device-tree/", "-name", "*bat-capacity-mah").Output()
	if err != nil {
		return 0, err
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	for _, p := range lines {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		data, rerr := os.ReadFile(p)
		if rerr != nil || len(data) < 4 {
			continue
		}
		// 设备树属性是 big-endian 32 位整数，单位 mAh，需转换为 µAh
		v := binary.BigEndian.Uint32(data[:4])
		if v > 0 {
			return int64(v) * 1000, nil
		}
	}
	return 0, fmt.Errorf("设备树中 bat-capacity-mah 无有效值")
}
