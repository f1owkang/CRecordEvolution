package main

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
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

func NormCurrentUA(raw int64) int64 {
	if raw > 10000 {
		return raw
	}
	return raw * 1000
}

func NormTempC(raw int64) float64 {
	if raw >= 100 {
		return float64(raw) / 10
	}
	return float64(raw)
}
