package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"testing"
	"time"
)

func TestChargingEta(t *testing.T) {
	// 参考满容量 5000mAh、当前 50% ⇒ 剩余 2500mAh
	cases := []struct {
		name    string
		refUAh  int64
		cap     int64
		avgUA   int64
		curUA   int64
		status  string
		secs    int64
		want    *int64
		wantNil bool
	}{
		{name: "平均电流优先", refUAh: 5_000_000, cap: 50, avgUA: 5_000_000, curUA: 1_000_000,
			status: "Charging", secs: 600, want: intPtr(30)}, // 2500/5000*60
		{name: "平均电流缺失时退回瞬时电流", refUAh: 5_000_000, cap: 50, avgUA: 0, curUA: 2_000_000,
			status: "Charging", secs: 0, want: intPtr(75)}, // 2500/2000*60
		{name: "非充电态不采信瞬时电流", refUAh: 5_000_000, cap: 50, avgUA: 0, curUA: 2_000_000,
			status: "Discharging", wantNil: true},
		{name: "cap=100 视为已充满", refUAh: 5_000_000, cap: 100,
			avgUA: 5_000_000, status: "Charging", wantNil: true},
		{name: "cap 超界", refUAh: 5_000_000, cap: 101,
			avgUA: 5_000_000, status: "Charging", wantNil: true},
		{name: "无容量标尺", refUAh: 0, cap: 50,
			avgUA: 5_000_000, status: "Charging", wantNil: true},
		{name: "速率为零", refUAh: 5_000_000, cap: 50, avgUA: 0, curUA: 0,
			status: "Charging", wantNil: true},
		{name: "负平均电流取幅值", refUAh: 5_000_000, cap: 50, avgUA: -5_000_000,
			status: "Charging", secs: 600, want: intPtr(30)},
		{name: "下限夹到 1 分钟", refUAh: 5_000_000, cap: 99, avgUA: 25_000_000,
			status: "Charging", want: intPtr(1)},
		{name: "上限夹到 1440 分钟", refUAh: 50_000_000, cap: 0, avgUA: 1_000_000,
			status: "Charging", want: intPtr(1440)},
		{name: "负持续秒数视为无效", refUAh: 5_000_000, cap: 50, avgUA: 5_000_000,
			status: "Charging", secs: -1, wantNil: true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := chargingEta(c.refUAh, c.cap, c.avgUA, c.curUA, c.status, c.secs)
			if c.wantNil {
				if got != nil {
					t.Fatalf("chargingEta 应为 nil, got %d", *got)
				}
				return
			}
			if got == nil || *got != *c.want {
				t.Fatalf("chargingEta = %v, want %d", got, *c.want)
			}
		})
	}
}

// putLiveNode 写一个假 sysfs 实时节点。
func putLiveNode(t *testing.T, fs SysFS, name, content string) {
	t.Helper()
	writeFile(t, filepath.Join(fs.Base, "battery", name), content)
}

func TestBuildLiveInfoChargingSession(t *testing.T) {
	fs := newTestSysFS(t)
	st := openTestStore(t, filepath.Join(t.TempDir(), "battery.db"))
	defer func() { _ = st.Close() }()

	now := int64(1_700_000_600)
	putLiveNode(t, fs, "status", "Charging\n")
	putLiveNode(t, fs, "capacity", "42\n")
	putLiveNode(t, fs, "current_now", "3000\n") // 原始 mA，学习刻度应为 ×1000
	putLiveNode(t, fs, "voltage_now", "4200000\n")
	putLiveNode(t, fs, "charge_full", "5000000\n")
	if err := st.KVSet(kvCurrentScale, "1000"); err != nil {
		t.Fatal(err)
	}
	for k, v := range map[string]string{
		kvSessActive:        "1",
		kvSessStartTs:       strconv.FormatInt(now-600, 10),
		kvSessStartCap:      "20",
		kvSessCounterAccUAh: "1000000",
	} {
		if err := st.KVSet(k, v); err != nil {
			t.Fatalf("预置 kv[%s]: %v", k, err)
		}
	}

	a := &app{fs: fs, st: st, nodePaths: map[string]string{}}
	live := a.buildLiveInfo(now)
	if live == nil {
		t.Fatal("实时节点可读时不应省略 live")
	}
	if live.Status != "Charging" || live.Cap == nil || *live.Cap != 42 {
		t.Fatalf("status/cap 不符: %+v", live)
	}
	// 电流必须经已学刻度 ×1000 归一，不能把原始 3000 当 µA
	if live.CurrentUA == nil || *live.CurrentUA != 3_000_000 {
		t.Fatalf("current_ua 归一错误: %v", live.CurrentUA)
	}
	if live.VoltageUV == nil || *live.VoltageUV != 4_200_000 {
		t.Fatalf("voltage_uv 不符: %v", live.VoltageUV)
	}
	if live.PowerMW == nil || *live.PowerMW != 12_600 {
		t.Fatalf("power_mw = %v, want 12600", live.PowerMW)
	}
	ch := live.Charging
	if ch == nil {
		t.Fatal("sess_active=1 时应输出 charging")
	}
	if ch.StartTs != now-600 || ch.StartCap != 20 || ch.Cap == nil || *ch.Cap != 42 {
		t.Fatalf("charging 基础字段不符: %+v", ch)
	}
	if ch.AccUAh == nil || *ch.AccUAh != 1_000_000 {
		t.Fatalf("acc_uah = %v, want 1000000", ch.AccUAh)
	}
	if ch.Secs != 600 {
		t.Fatalf("secs = %d, want 600", ch.Secs)
	}
	if ch.AvgCurrentUA == nil || *ch.AvgCurrentUA != 6_000_000 {
		t.Fatalf("avg_current_ua = %v, want 6000000", ch.AvgCurrentUA)
	}
	// refUAh=5000000、cap=42 ⇒ 剩余 2900mAh；6A ⇒ 29 分钟
	if ch.EtaMin == nil || *ch.EtaMin != 29 {
		t.Fatalf("eta_min = %v, want 29", ch.EtaMin)
	}
}

func TestLiveDirection(t *testing.T) {
	cases := []struct {
		name   string
		status string
		active bool
		want   int
	}{
		{"充电", "Charging", false, 1},
		{"充电且有会话", "Charging", true, 1},
		{"放电", "Discharging", false, -1},
		{"放电（会话尾段未结算）", "Discharging", true, -1},
		{"满电尾段会话仍在涨电", "Full", true, 1},
		{"满电无会话", "Full", false, 0},
		{"优化充电暂停无会话", "Not charging", false, 0},
		{"未知状态有会话按充电", "Unknown", true, 1},
		{"未知状态无会话保留原始", "", false, 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := liveDirection(c.status, c.active); got != c.want {
				t.Fatalf("liveDirection(%q,%v) = %d, want %d", c.status, c.active, got, c.want)
			}
		})
	}
}

// TestBuildLiveInfoSignConvention 锁死展示符号约定：电流/功率统一「充电为正、
// 放电为负」，由 status 判向，不能跟着 current_now 的原始符号走（部分内核
// 充电报负、放电报正，直接透传会让充电中的电流/功率显示成负值）。
func TestBuildLiveInfoSignConvention(t *testing.T) {
	cases := []struct {
		name       string
		status     string
		rawCurrent string
		sessActive bool
		wantUA     int64
		wantMW     int64
	}{
		{"内核方向相反的充电", "Charging", "-3000\n", false, 3_000_000, 12_600},
		{"内核方向相反的放电", "Discharging", "1000\n", false, -1_000_000, -4_200},
		{"标准内核充电", "Charging", "3000\n", false, 3_000_000, 12_600},
		{"标准内核放电", "Discharging", "-1000\n", false, -1_000_000, -4_200},
		{"满电尾段会话仍在充电", "Full", "-3000\n", true, 3_000_000, 12_600},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			fs := newTestSysFS(t)
			st := openTestStore(t, filepath.Join(t.TempDir(), "battery.db"))
			defer func() { _ = st.Close() }()

			putLiveNode(t, fs, "status", c.status+"\n")
			putLiveNode(t, fs, "capacity", "50\n")
			putLiveNode(t, fs, "current_now", c.rawCurrent)
			putLiveNode(t, fs, "voltage_now", "4200000\n")
			putLiveNode(t, fs, "charge_full", "5000000\n")
			if err := st.KVSet(kvCurrentScale, "1000"); err != nil {
				t.Fatal(err)
			}
			if c.sessActive {
				for k, v := range map[string]string{
					kvSessActive:   "1",
					kvSessStartTs:  "1000",
					kvSessStartCap: "40",
				} {
					if err := st.KVSet(k, v); err != nil {
						t.Fatalf("预置 kv[%s]: %v", k, err)
					}
				}
			}

			a := &app{fs: fs, st: st, nodePaths: map[string]string{}}
			live := a.buildLiveInfo(1600)
			if live == nil || live.CurrentUA == nil {
				t.Fatalf("current_ua 缺失: %+v", live)
			}
			if *live.CurrentUA != c.wantUA {
				t.Fatalf("current_ua = %d, want %d", *live.CurrentUA, c.wantUA)
			}
			if live.PowerMW == nil || *live.PowerMW != c.wantMW {
				t.Fatalf("power_mw = %v, want %d", live.PowerMW, c.wantMW)
			}
			// 放电时即使会话尚未结算，也不给出「充满预估」
			if c.status == "Discharging" && live.Charging != nil && live.Charging.EtaMin != nil {
				t.Fatalf("放电中不应输出 eta_min: %v", *live.Charging.EtaMin)
			}
		})
	}
}

func TestBuildLiveInfoPartialAndMissing(t *testing.T) {
	st := openTestStore(t, filepath.Join(t.TempDir(), "battery.db"))
	defer func() { _ = st.Close() }()

	// 四个实时节点全缺失 ⇒ live 整体省略（即使 kv 里有进行中会话）
	if err := st.KVSet(kvSessActive, "1"); err != nil {
		t.Fatal(err)
	}
	a := &app{fs: newTestSysFS(t), st: st, nodePaths: map[string]string{}}
	if live := a.buildLiveInfo(1000); live != nil {
		t.Fatalf("实时节点全缺失应省略 live, got %+v", live)
	}

	// 只读到 capacity：live 保留，其余字段省略；无 charge_full/design 且
	// status/电流缺失 ⇒ eta 不可推导
	fs := newTestSysFS(t)
	putLiveNode(t, fs, "capacity", "55\n")
	a = &app{fs: fs, st: st, nodePaths: map[string]string{}}
	live := a.buildLiveInfo(1000)
	if live == nil || live.Cap == nil || *live.Cap != 55 {
		t.Fatalf("仅 capacity 时 live 应只含 cap: %+v", live)
	}
	if live.Status != "" || live.CurrentUA != nil || live.VoltageUV != nil || live.PowerMW != nil {
		t.Fatalf("缺失字段应省略: %+v", live)
	}
	if live.Charging == nil || live.Charging.EtaMin != nil {
		t.Fatalf("无参考容量时 eta 应省略: %+v", live.Charging)
	}

	// sess_active!=1 时不输出 charging
	if err := st.KVSet(kvSessActive, "0"); err != nil {
		t.Fatal(err)
	}
	if live := a.buildLiveInfo(1000); live == nil || live.Charging != nil {
		t.Fatalf("sess_active=0 不应输出 charging: %+v", live)
	}
}

func TestDaemonHeartbeatRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "data", daemonFile)
	now := int64(1_700_000_100)
	if err := writeDaemonHeartbeat(path, "1.3.8(37)", now-12); err != nil {
		t.Fatalf("写心跳: %v", err)
	}
	// 权限 0644（Windows 无 POSIX 权限位，只有 Android/Linux 上可断言），且不留临时文件
	fi, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" {
		if perm := fi.Mode().Perm(); perm != 0o644 {
			t.Fatalf("心跳权限 = %o, want 644", perm)
		}
	}
	tmps, _ := filepath.Glob(filepath.Join(filepath.Dir(path), daemonFile+".tmp*"))
	if len(tmps) != 0 {
		t.Fatalf("不应残留临时文件: %v", tmps)
	}
	// 落盘内容含 version/pid/ts
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var hb daemonHeartbeat
	if err := json.Unmarshal(data, &hb); err != nil {
		t.Fatalf("心跳不是合法 JSON: %v (%s)", err, data)
	}
	if hb.Version != "1.3.8(37)" || hb.Pid != os.Getpid() || hb.TS != now-12 {
		t.Fatalf("心跳内容不符: %+v", hb)
	}

	info := readDaemonInfo(path, now, "1.3.8(37)")
	if info == nil {
		t.Fatal("心跳文件存在时应解析成功")
	}
	if info.Version != "1.3.8(37)" || info.TS == nil || *info.TS != now-12 {
		t.Fatalf("daemonInfo 基础字段不符: %+v", info)
	}
	if info.AgeSec == nil || *info.AgeSec != 12 {
		t.Fatalf("age_sec = %v, want 12", info.AgeSec)
	}
	if info.Stale {
		t.Fatal("版本一致不应标记 stale")
	}

	// 心跳版本 != 当前二进制版本 ⇒ stale（仅提示）
	if stale := readDaemonInfo(path, now, "1.3.9(38)"); stale == nil || !stale.Stale {
		t.Fatalf("版本不一致应标记 stale: %+v", stale)
	}
	// 当前版本未知（module.prop 读不到）⇒ 不判 stale
	if unknown := readDaemonInfo(path, now, ""); unknown == nil || unknown.Stale {
		t.Fatalf("当前版本未知时不应判 stale: %+v", unknown)
	}
	// now 早于 ts（时钟回拨）⇒ age 取 0，不出现负数
	if back := readDaemonInfo(path, now-100, "1.3.8(37)"); back == nil || back.AgeSec == nil || *back.AgeSec != 0 {
		t.Fatalf("时钟回拨时 age_sec 应取 0: %+v", back)
	}
}

func TestReadDaemonInfoMissingOrBroken(t *testing.T) {
	dir := t.TempDir()
	if got := readDaemonInfo(filepath.Join(dir, "missing.json"), 1, "1.3.8(37)"); got != nil {
		t.Fatalf("心跳缺失应返回 nil: %+v", got)
	}
	broken := filepath.Join(dir, "broken.json")
	writeFile(t, broken, "{not json")
	if got := readDaemonInfo(broken, 1, "1.3.8(37)"); got != nil {
		t.Fatalf("心跳损坏应返回 nil: %+v", got)
	}
	empty := filepath.Join(dir, "empty.json")
	writeFile(t, empty, "{}")
	if got := readDaemonInfo(empty, 1, "1.3.8(37)"); got != nil {
		t.Fatalf("空心跳应返回 nil: %+v", got)
	}
}

// 写入失败的目录（父路径是普通文件）不得 panic，且 app.persistHeartbeat 静默
// 忽略错误，不影响主链路。
func TestPersistHeartbeatSilentOnFailure(t *testing.T) {
	blocked := filepath.Join(t.TempDir(), "blocked")
	writeFile(t, blocked, "x")
	a := &app{moddir: blocked, version: "1.3.8(37)"}
	a.persistHeartbeat(time.Now().Unix()) // 不 panic、不返回错误即可
}

// app 侧心跳读入口：persistHeartbeat 写入后 daemonStatus 能读到当前版本。
func TestAppDaemonStatus(t *testing.T) {
	dir := t.TempDir()
	a := &app{moddir: dir, version: "1.3.8(37)"}
	now := int64(1_700_000_000)
	a.persistHeartbeat(now - 5)
	info := a.daemonStatus(now)
	if info == nil || info.Version != "1.3.8(37)" || info.AgeSec == nil || *info.AgeSec != 5 || info.Stale {
		t.Fatalf("app 心跳读取不符: %+v", info)
	}
}
