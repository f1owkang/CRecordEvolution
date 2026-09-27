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

const (
	// microUnitChargeMinUA 量纲锚点分界：power_supply 同一驱动的 charge/current
	// 单位一致。手机电池容量以 µAh 计必 ≥ 100mAh，以 mAh 计不可能 ≥ 100000
	// （100Ah），故此界两侧无歧义，可据 charge_full 的量纲定 current_now 量纲。
	microUnitChargeMinUA int64 = 100_000
	// noAnchorUA 无锚点时的幅值分界：mA 读数超过 10000 即意味着 10A（手机电池
	// 快充上限），故大值必为 µA；小值两种量纲都讲得通，按 mA 处理。保留历史
	// 取值以兼容既有设备行为，两侧残留误差都在尾部（µA 设备 <10mA、mA 设备 >10A）。
	noAnchorUA int64 = 10_000
	// maxCurrentUA 电流物理上界 25A：无论量纲怎么判，结果越界即判为误判并回退
	// 原值，堵死 1000 倍误乘污染积分的方向。
	maxCurrentUA int64 = 25_000_000
)

// NormCurrentUA 无锚点时的电流单位判别（幅值启发式）。生产路径一律带 charge_full
// 走 NormCurrentUAWithFull，本入口仅留给拿不到 charge_full 的场景与测试。
func NormCurrentUA(raw int64) int64 {
	return NormCurrentUAWithFull(raw, 0)
}

// NormCurrentUAWithFull 以 charge_full 的量纲为锚点判别 current_now 单位，并把
// 读数折算成 µA。判据是「量纲」而非「幅值」：同一 power_supply 驱动下 charge 与
// current 单位一致，而 charge_full 是缓变的大量程值，拿它当幅值阈值会把小电流
// 整批误判。
//   - charge_full ≥ microUnitChargeMinUA（µAh 量纲）⇒ current_now 已是 µA，直通；
//   - charge_full 以 mAh 计 ⇒ current_now 为 mA，×1000 折算；
//   - charge_full 不可读（≤0）时退化为幅值启发式，见 noAnchorUA 注。
//
// 最后用 maxCurrentUA 兜底：折算结果超过物理上界即回退原值。
//
// 实测教训（回归）：旧实现按 |current_now| < charge_full/10 判成 mA，把所有
// < 0.6A 的真实 µA 读数 ×1000（40mA 变 40A）。实机采样库显示换上该实现当天起
// 每天 200+ 条百安级电流，充电积分、中段校准、CCCT/ICA 倍率门控全线失真，
// 「实测」估算被推到设计容量的 114%。
func NormCurrentUAWithFull(raw, chargeFullUA int64) int64 {
	var ua int64
	switch {
	case chargeFullUA >= microUnitChargeMinUA:
		ua = raw
	case chargeFullUA > 0:
		ua = raw * 1000
	case absI64(raw) > noAnchorUA:
		ua = raw
	default:
		ua = raw * 1000
	}
	if absI64(ua) > maxCurrentUA {
		return raw
	}
	return ua
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
	// find 走绝对路径优先：守护进程由 service.sh exec 启动，PATH 未必含
	// /system/bin；失败则退回 PATH 查找。两条都失败时安全降级（调用方按
	// 设计容量缺失处理），不会阻断启动。
	var out []byte
	var err error
	for _, bin := range []string{"/system/bin/find", "find"} {
		out, err = exec.Command(bin, "/proc/device-tree/", "-name", "*bat-capacity-mah").Output()
		if err == nil {
			break
		}
	}
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
