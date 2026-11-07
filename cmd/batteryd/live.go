// 实时节点与守护进程心跳（`batteryd json` 的 live/daemon 顶层字段）。
//
// live 是概览页的「当前状态」出口：四个实时 sysfs 节点（status/capacity/
// current_now/voltage_now）任一可读即输出，全读不到则整体省略；单个字段读不到
// 只省略该字段（沿用「缺失即省略」纪律）。电流必须走 normalizeCurrentUA +
// kv cur_scale + charge_full 的既有归一路径，禁止把原始读数当 µA 下发。
//
// daemon 是守护进程的存活心跳：daemon 每拍原子写 <moddir>/data/daemon.json，
// json/diag 读取后给出年龄与 stale 提示。心跳缺失/损坏时整体省略（旧版兼容），
// stale 只做提示，不改数据、不写 event（避免每次 json 都落库）。

package main

import (
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"strings"
)

// daemonFile 是守护进程心跳文件名（位于 <moddir>/data/）。
const daemonFile = "daemon.json"

// liveCharging 为进行中充电会话的实时摘要；全部字段来自 kv（sess_*），不另算
// 一套结算口径。电量单位 µAh、电流 µA，字段后缀即量纲。
type liveCharging struct {
	StartTs  int64 `json:"start_ts"`
	StartCap int64 `json:"start_cap"` // 会话起始显示电量 %
	// Cap 当前显示电量 %（capacity 节点读不到时省略）
	Cap *int64 `json:"cap,omitempty"`
	// AccUAh 本次会话电量计增量 µAh（kv sess_cc_acc，<=0 省略）
	AccUAh *int64 `json:"acc_uah,omitempty"`
	// Secs now-start_ts，小于 0 取 0
	Secs int64 `json:"secs"`
	// AvgCurrentUA 本次会话平均电流 µA = AccUAh*3600/Secs
	// （AccUAh<=0 或 Secs<=0 时省略）
	AvgCurrentUA *int64 `json:"avg_current_ua,omitempty"`
	// EtaMin 预计充满分钟数，不可推导时省略
	EtaMin *int64 `json:"eta_min,omitempty"`
}

// liveInfo 为实时节点快照；所有字段缺失即省略，四个节点全读不到时整体省略。
type liveInfo struct {
	Status string `json:"status,omitempty"` // sysfs status 原文
	Cap    *int64 `json:"cap,omitempty"`    // capacity %
	// CurrentUA 带符号、经已学刻度归一后的 µA；符号按 liveDirection 统一为
	// 充电为正、放电为负（不信任 current_now 原始符号，部分内核方向相反）
	CurrentUA *int64 `json:"current_ua,omitempty"`
	VoltageUV *int64 `json:"voltage_uv,omitempty"`
	// PowerMW round(CurrentUA*VoltageUV/1e9)，符号与 CurrentUA 一致（充电正/放电负）
	PowerMW *int64 `json:"power_mw,omitempty"`
	// Charging 仅 kv sess_active=="1" 时存在
	Charging *liveCharging `json:"charging,omitempty"`
}

// daemonInfo 为守护进程心跳的读取结果；文件缺失/解析失败时整体省略。
type daemonInfo struct {
	Version string `json:"version,omitempty"`
	TS      *int64 `json:"ts,omitempty"`
	AgeSec  *int64 `json:"age_sec,omitempty"` // max(0, now-ts)
	// Stale 心跳版本 != 当前二进制从 module.prop 读到的版本（两侧都非空才判）
	Stale bool `json:"stale,omitempty"`
}

// daemonHeartbeat 是 <moddir>/data/daemon.json 的落盘结构；版本取当前二进制
// 从 module.prop 读到的 "version(versionCode)"，便于识别「升级后旧 daemon
// 仍在内存运行」。
type daemonHeartbeat struct {
	Version string `json:"version"`
	Pid     int    `json:"pid"`
	TS      int64  `json:"ts"`
}

// writeDaemonHeartbeat 原子写心跳文件（临时文件 + rename，0644）：先写同目录临时
// 文件再 rename，读侧永远看到完整 JSON，不会读到半截内容。写失败由调用方静默
// 忽略——心跳永远不影响主链路。
func writeDaemonHeartbeat(path, version string, ts int64) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.Marshal(daemonHeartbeat{Version: version, Pid: os.Getpid(), TS: ts})
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), daemonFile+".tmp*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	cleanup := func() {
		tmp.Close()
		os.Remove(tmpName)
	}
	if _, err := tmp.Write(data); err != nil {
		cleanup()
		return err
	}
	if err := tmp.Chmod(0o644); err != nil {
		cleanup()
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return err
	}
	if err := os.Rename(tmpName, path); err != nil {
		os.Remove(tmpName)
		return err
	}
	return nil
}

// readDaemonInfo 读取并解析心跳文件：缺失/损坏/空文件返回 nil（旧版兼容）。
// 心跳版本与 curVersion 都非空且不等时标记 stale，仅作提示。
func readDaemonInfo(path string, now int64, curVersion string) *daemonInfo {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var hb daemonHeartbeat
	if err := json.Unmarshal(data, &hb); err != nil {
		return nil
	}
	if strings.TrimSpace(hb.Version) == "" && hb.TS <= 0 {
		return nil // 空心跳（旧版本/写一半）没有可用信息，按缺失处理
	}
	info := &daemonInfo{Version: hb.Version}
	if hb.TS > 0 {
		ts := hb.TS
		age := now - ts
		if age < 0 {
			age = 0
		}
		info.TS = &ts
		info.AgeSec = &age
	}
	if info.Version != "" && curVersion != "" && info.Version != curVersion {
		info.Stale = true
	}
	return info
}

// persistHeartbeat 为 app 侧的心跳写入口：路径固定为 <moddir>/data/daemon.json，
// 版本取当前二进制的 module.prop 版本。失败静默（心跳不影响采样主链路）。
func (a *app) persistHeartbeat(ts int64) {
	_ = writeDaemonHeartbeat(filepath.Join(a.moddir, "data", daemonFile), a.version, ts)
}

// daemonStatus 为 app 侧的心跳读入口：当前版本用于 stale 判定。
func (a *app) daemonStatus(now int64) *daemonInfo {
	return readDaemonInfo(filepath.Join(a.moddir, "data", daemonFile), now, a.version)
}

// readLiveText 读取 sysfs 文本节点（status），去掉首尾空白；空内容视为缺失。
func (a *app) readLiveText(name string) (string, bool) {
	path, err := a.nodePath(name)
	if err != nil {
		return "", false
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", false
	}
	s := strings.TrimSpace(string(data))
	if s == "" {
		return "", false
	}
	return s, true
}

// readLiveSigned 读取带符号整数节点（current_now 放电为负）。
func (a *app) readLiveSigned(name string) (int64, bool) {
	path, err := a.nodePath(name)
	if err != nil {
		return 0, false
	}
	v, err := a.fs.ReadIntSigned(path)
	if err != nil {
		return 0, false
	}
	return v, true
}

// liveDirection 返回实时显示的符号约定：充电 +1、放电 -1、无法判断 0。
//
// sysfs current_now 的方向不守 ABI：标准内核充电为正、放电为负，但部分机型的
// 方向相反（充电中原始读数为负、放电时为正），直接下发原始符号会让充电中的
// 电流/功率显示成负值。因此展示层统一按 status（+ 进行中会话）判向，
// 固定「充电为正、放电为负」，不信任 current_now 原始符号；无会话且状态不明时
// 返回 0，保留原始值。
func liveDirection(status string, sessActive bool) int {
	switch status {
	case "Charging":
		return 1
	case "Discharging":
		return -1
	case "Full", "Not charging":
		if sessActive {
			return 1 // 满电/优化充电的尾段：会话仍在涨电，按充电展示
		}
		return 0
	default:
		if sessActive {
			return 1
		}
		return 0
	}
}

// buildLiveInfo 组装实时节点与进行中充电会话。四个实时节点全读不到时返回 nil
// （json 里 live 整体省略）；单个节点读不到只省略对应字段。
func (a *app) buildLiveInfo(now int64) *liveInfo {
	info := &liveInfo{}
	any := false

	if s, ok := a.readLiveText("status"); ok {
		info.Status = s
		any = true
	}
	// 进行中充电会话：既决定 charging 字段，也参与电流方向判定（尾段 status 可能
	// 已不是 Charging，但会话仍在涨电）。
	active := false
	if v, ok := a.st.KVGet(kvSessActive); ok && v == "1" {
		active = true
	}
	dir := liveDirection(info.Status, active)
	capVal := int64(0)
	if v, err := a.readIntNode("capacity"); err == nil {
		capVal = v
		info.Cap = &capVal
		any = true
	}
	// charge_full 只在需要归一电流/推算 ETA 时读一次（避免 live 全缺失时多余 io）
	fullUAh, fullKnown := int64(0), false
	readFull := func() int64 {
		if !fullKnown {
			fullKnown = true
			if v, err := a.readIntNode("charge_full"); err == nil {
				fullUAh = v
			}
		}
		return fullUAh
	}

	if raw, ok := a.readLiveSigned("current_now"); ok {
		// 必须复用既有归一链路（已学刻度优先，未校准退回 charge_full 锚点/
		// 幅值启发式），禁止把原始读数当 µA 下发
		scale := 0.0
		if txt, has := a.st.KVGet(kvCurrentScale); has {
			scale = parseCurrentScale(txt)
		}
		ua := normalizeCurrentUA(raw, readFull(), scale)
		// 展示符号统一按 status 判向（见 liveDirection），不信任内核原始符号
		if dir > 0 {
			ua = absI64(ua)
		} else if dir < 0 {
			ua = -absI64(ua)
		}
		info.CurrentUA = &ua
		any = true
	}
	if v, err := a.readIntNode("voltage_now"); err == nil {
		info.VoltageUV = &v
		any = true
	}
	if info.CurrentUA != nil && info.VoltageUV != nil {
		// µA × µV = pW，÷1e9 得 mW；符号沿用已定方向的电流（充电正/放电负）
		mw := int64(math.Round(float64(*info.CurrentUA) * float64(*info.VoltageUV) / 1e9))
		info.PowerMW = &mw
	}

	if active {
		startTs := kvInt(a.st, kvSessStartTs)
		ch := &liveCharging{StartTs: startTs, StartCap: kvInt(a.st, kvSessStartCap)}
		if info.Cap != nil {
			ch.Cap = info.Cap
		}
		acc := kvInt(a.st, kvSessCounterAccUAh) // kv sess_cc_acc，µAh
		if acc > 0 {
			ch.AccUAh = &acc
		}
		ch.Secs = now - startTs
		if ch.Secs < 0 {
			ch.Secs = 0
		}
		if acc > 0 && ch.Secs > 0 {
			avg := acc * 3600 / ch.Secs
			ch.AvgCurrentUA = &avg
		}
		if info.Cap != nil && dir > 0 {
			refUAh := readFull()
			if refUAh <= 0 {
				refUAh = a.designUAh
			}
			var avg, cur int64
			if ch.AvgCurrentUA != nil {
				avg = *ch.AvgCurrentUA
			}
			if info.CurrentUA != nil {
				cur = *info.CurrentUA
			}
			ch.EtaMin = chargingEta(refUAh, *info.Cap, avg, cur, info.Status, ch.Secs)
		}
		info.Charging = ch
	}

	if !any {
		return nil
	}
	return info
}

// chargingEta 由当前显示电量与充电速率估算充满剩余分钟数，纯函数便于单测。
//
// 口径：
//   - cap 缺失（调用方保证非 nil）或 ≥100 ⇒ 无法推算，nil；
//   - refUAh（参考满容量 µAh）≤0 ⇒ 无标尺，nil；
//   - 剩余 mAh = refUAh*(100-cap)/100/1000；
//   - 速率优先用会话平均电流 |avgCurrentUA|/1000；平均电流缺失（avg<=0）时
//     退回瞬时 |currentUA|/1000，且仅当 status=="Charging"（其余状态电流不可信）；
//   - 速率 ≤0 ⇒ nil；eta_min=round(剩余mAh/速率mA*60)，夹到 [1,1440]。
//
// secs 为会话已持续秒数（<0 视为无效输入）；平均电流是否成立由调用方按
// Secs>0 提前保证，函数内只做合法性防御。
func chargingEta(refUAh, cap, avgCurrentUA, currentUA int64, status string, secs int64) *int64 {
	if cap < 0 || cap >= 100 || refUAh <= 0 || secs < 0 {
		return nil
	}
	rateMA := float64(absI64(avgCurrentUA)) / 1000
	if rateMA <= 0 {
		if status != "Charging" {
			return nil
		}
		rateMA = float64(absI64(currentUA)) / 1000
	}
	if rateMA <= 0 {
		return nil
	}
	remainingMah := float64(refUAh) * float64(100-cap) / 100 / 1000
	eta := int64(math.Round(remainingMah / rateMA * 60))
	if eta < 1 {
		eta = 1
	}
	if eta > 1440 {
		eta = 1440
	}
	return &eta
}
