// 诊断信息导出（`batteryd diag`）：给 WebUI 设置页的「复制诊断」按钮用，
// 生成一段**短、可脱敏、可直接贴进 issue** 的环境与结算摘要。
//
// 设计约束：
//   - 只读取固定的设备属性与电池 sysfs 节点，不读序列号 / IMEI / MAC / 账号等
//     任何唯一标识；输出里只有设备型号、系统版本、root 管理器、节点路径、
//     校准/估算/会话摘要与日志尾部。
//   - 长度上限 diagMaxChars：日志行按剩余空间追加，超限截断，保证粘贴到 issue
//     不会被折叠或刷屏。
//   - 数值全部复用已有口径函数（stats / convSession / calibInfo），不另算一套。

package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const (
	// diagMaxChars 诊断文本字符上限（rune）：约 30~40 行，贴 issue 足够。
	diagMaxChars = 1800
	// diagLogLines 日志尾部最多带几行（按剩余空间再裁剪）。
	diagLogLines = 5
	// diagLogLineMax 单行日志清洗后的最大字符数。
	diagLogLineMax = 160
	// diagFieldMax 设备属性/状态文本的单字段最大字符数。
	diagFieldMax = 48
	// diagEventDetailMax 最近一次裁决/校准 detail 的最大字符数。
	diagEventDetailMax = 160
)

// diagEvent 是诊断里唯一带 detail 的事件（最近一次校准/裁决/互检结论）。
type diagEvent struct {
	Kind, Detail string
	TS           int64
}

// diagInput 是 renderDiag 的纯数据输入，便于单测；采集在 app.diagInput 完成。
type diagInput struct {
	Version, Channel     string
	Root                 string
	Manufacturer, Model  string
	Device, Android, ABI string
	SDK                  string

	Design Design
	Snap   Snapshot

	Status     string
	Cap        *int64
	VoltageUV  *int64
	CounterUAh *int64

	Sessions     []Session
	Discharges   []DischargeRow
	EventCounts  map[string]int64
	LastEvent    *diagEvent
	LastSampleTS int64
	Now          int64
	LogTail      []string

	Loc *time.Location
}

// runDiag 实现 `batteryd diag` 子命令：打印诊断文本。
func runDiag() error {
	a, err := newApp()
	if err != nil {
		return err
	}
	defer a.st.Close()
	fmt.Print(renderDiag(a.diagInput()))
	return nil
}

// diagInput 从 app 当前状态采集诊断数据；任何可选项失败都降级为缺失，不整体失败。
func (a *app) diagInput() diagInput {
	d, snap, _ := a.stats()

	in := diagInput{
		Version:      a.version,
		Channel:      channel,
		Root:         detectRootManager(),
		Manufacturer: diagGetprop("ro.product.manufacturer"),
		Model:        diagGetprop("ro.product.model"),
		Device:       diagGetprop("ro.product.device"),
		Android:      diagGetprop("ro.build.version.release"),
		SDK:          diagGetprop("ro.build.version.sdk"),
		ABI:          diagGetprop("ro.product.cpu.abi"),
		Design:       d,
		Snap:         snap,
		Now:          time.Now().Unix(),
		Loc:          localNow().Location(),
	}
	// 实时节点：全部走固定 power_supply/battery 路径，避免诊断时做全树扫描。
	in.Status = a.diagReadText("status")
	in.Cap = a.diagReadInt("capacity")
	in.VoltageUV = a.diagReadInt("voltage_now")
	in.CounterUAh = a.diagReadInt("charge_counter")
	if sessions, err := a.st.RecentSessions(3); err == nil {
		in.Sessions = sessions
	}
	if dis, err := a.st.RecentDischarge(1); err == nil {
		in.Discharges = dis
	}
	if counts, err := a.st.EventCountsSince(time.Now().Unix() - 7*86400); err == nil {
		in.EventCounts = counts
	}
	if ts, err := a.st.LatestSampleTS(); err == nil {
		in.LastSampleTS = ts
	}
	if kind, detail, ts, ok, err := a.st.LatestEvent([]string{
		"counter_scale", "cur_cal", "cur_cal_fail", "quality_warn",
	}); err == nil && ok {
		in.LastEvent = &diagEvent{Kind: kind, Detail: diagClean(detail, diagEventDetailMax), TS: ts}
	}
	in.LogTail = readLogTail(filepath.Join(a.moddir, "data", logFile), diagLogLines)
	return in
}

// diagReadInt 只读固定 /sys/class/power_supply/battery/<name>；不做 FindNode 回退，
// 保证诊断一次点击最多几个文件读，不会因节点缺失全树扫 /sys。
func (a *app) diagReadInt(name string) *int64 {
	path := filepath.Join(defaultPowerSupplyBase, "battery", name)
	v, err := a.fs.ReadInt(path)
	if err != nil {
		return nil
	}
	return &v
}

func (a *app) diagReadText(name string) string {
	data, err := os.ReadFile(filepath.Join(defaultPowerSupplyBase, "battery", name))
	if err != nil {
		return ""
	}
	return diagClean(string(data), diagFieldMax)
}

// detectRootManager 只按管理器固定数据目录判定，不执行任何可能涉及个人数据的命令。
func detectRootManager() string {
	for _, c := range []struct{ path, name string }{
		{"/data/adb/ksu", "KernelSU"},
		{"/data/adb/ap", "APatch"},
		{"/data/adb/magisk", "Magisk"},
	} {
		if fi, err := os.Stat(c.path); err == nil && fi.IsDir() {
			return c.name
		}
	}
	return "unknown"
}

// diagAllowedProps 是诊断允许读取的设备属性白名单。只列非唯一标识；序列号 /
// IMEI / MAC / Android ID / 账号等一律不在表内，diagGetprop 对白名单外的 key
// 直接返回空，并且不会执行 getprop。
var diagAllowedProps = map[string]bool{
	"ro.product.manufacturer":  true,
	"ro.product.model":         true,
	"ro.product.device":        true,
	"ro.build.version.release": true,
	"ro.build.version.sdk":     true,
	"ro.product.cpu.abi":       true,
}

func diagPropAllowed(name string) bool { return diagAllowedProps[name] }

func diagGetprop(name string) string {
	if !diagPropAllowed(name) {
		return ""
	}
	out, err := exec.Command("/system/bin/getprop", name).Output()
	if err != nil {
		return ""
	}
	return diagClean(string(out), diagFieldMax)
}

// diagLogHintsHigh 是高价值排障行；diagLogHintsLow 是启动行（重复时只保留
// 最新一条，避免多次重启的“启动 channel=stable”把校准/结算行挤掉）。
var (
	diagLogHintsHigh = []string{"校准", "结算", "拒绝", "错误", "失败", "去抖", "质量", "counter_scale", "cur_cal", "warning", "error"}
	diagLogHintsLow  = []string{"启动"}
)

func diagHasHint(line string, hints []string) bool {
	for _, h := range hints {
		if strings.Contains(line, h) {
			return true
		}
	}
	return false
}

// diagLogKey 去掉行首时间戳后作为去重键；否则多次重启的“启动 channel=stable”
// 只是时间不同，会占满日志尾部。
func diagLogKey(line string) string {
	if strings.HasPrefix(line, "[") {
		if i := strings.Index(line, "] "); i > 0 {
			return line[i+2:]
		}
	}
	return line
}

// readLogTail 取日志尾部最多 n 行：先挑高价值排障行，再补启动行，最后用普通
// 尾部行补齐；重复行只保留最新一条（例如多次重启的“启动 channel=stable”），
// 全部清洗成单行、限长，最终按时间顺序输出。
func readLogTail(path string, n int) []string {
	data, err := os.ReadFile(path)
	if err != nil || n <= 0 {
		return nil
	}
	lines := strings.Split(strings.TrimRight(string(data), "\r\n"), "\n")
	selected := map[int]bool{}
	seen := map[string]bool{}
	count := 0
	take := func(i int) bool {
		line := diagClean(lines[i], diagLogLineMax)
		if line == "" || seen[diagLogKey(line)] {
			return false
		}
		seen[diagLogKey(line)] = true
		selected[i] = true
		count++
		return true
	}
	for i := len(lines) - 1; i >= 0 && count < n; i-- {
		if diagHasHint(lines[i], diagLogHintsHigh) {
			take(i)
		}
	}
	for i := len(lines) - 1; i >= 0 && count < n; i-- {
		if !selected[i] && diagHasHint(lines[i], diagLogHintsLow) {
			take(i)
		}
	}
	for i := len(lines) - 1; i >= 0 && count < n; i-- {
		if !selected[i] {
			take(i)
		}
	}
	out := make([]string, 0, count)
	for i, line := range lines {
		if !selected[i] {
			continue
		}
		line = diagClean(line, diagLogLineMax)
		if line != "" {
			out = append(out, line)
		}
	}
	return out
}

// diagClean 清洗单行文本：控制字符去掉、连续空白折叠为一个空格、超长截断。
// 同时兜住 getprop 返回换行/制表符把诊断格式撑坏的情况。
func diagClean(s string, maxRunes int) string {
	var b strings.Builder
	prevSpace := false
	for _, r := range s {
		switch {
		case r == ' ' || r == '\t' || r == '\r' || r == '\n':
			if !prevSpace && b.Len() > 0 {
				b.WriteRune(' ')
				prevSpace = true
			}
		case r < 0x20 || r == 0x7f:
			// 其它控制字符直接丢弃
		default:
			b.WriteRune(r)
			prevSpace = false
		}
	}
	out := strings.TrimSpace(b.String())
	if maxRunes > 0 {
		rs := []rune(out)
		if len(rs) > maxRunes {
			out = string(rs[:maxRunes-1]) + "…"
		}
	}
	return out
}

// renderDiag 把采集结果渲染成短文本；日志行按总长度剩余空间追加。
func renderDiag(in diagInput) string {
	var b strings.Builder
	line := func(format string, args ...any) { fmt.Fprintf(&b, format+"\n", args...) }

	line("CRecordEvolution diag（已脱敏：无序列号/IMEI/MAC/账号）")
	line("module=%s channel=%s root=%s android=%s(sdk%s) abi=%s",
		orDash(in.Version), orDash(in.Channel), orDash(in.Root), orDash(in.Android), orDash(in.SDK), orDash(in.ABI))
	line("device=%s %s (%s)", orDash(in.Manufacturer), orDash(in.Model), orDash(in.Device))
	line("calib=%s", diagCalibLine(in.Snap.Calib))
	lastTick := "—"
	if in.LastSampleTS > 0 {
		lastTick = diagTime(in.LastSampleTS, in.Loc)
	}
	if in.Snap.EstUAh != nil {
		line("est=%dmAh samples=%d last_tick=%s", *in.Snap.EstUAh/1000, in.Snap.Samples, lastTick)
	} else {
		line("est=learning samples=%d last_tick=%s", in.Snap.Samples, lastTick)
	}
	if in.Snap.Quality != nil {
		q := in.Snap.Quality
		line("quality=%s spread=%d%% ema/mid/dis/full=%s/%s/%s/%s", qualText(q.OK), q.SpreadPct,
			optQualityMah(q.EmaMah), optQualityMah(q.MidMah), optQualityMah(q.DisMah), optQualityMah(q.FullMah))
	}
	line("live=%s", diagLiveLine(in))
	line("kernel=full=%s design=%s cycles=%s",
		designMah(in.Design.HasFull, in.Design.FullMah), designMah(in.Design.HasDesign, in.Design.DesignMah),
		designCycles(in.Design))
	for _, s := range in.Sessions {
		line("%s", diagSessionLine(s, in.Loc))
	}
	for _, d := range in.Discharges {
		line("%s", diagDischargeLine(d, in.Loc))
	}
	line("events7d=%s", diagEventsLine(in.EventCounts))
	if in.LastEvent != nil {
		line("last_event=%s%s %s: %s", diagTime(in.LastEvent.TS, in.Loc),
			diagAge(in.Now, in.LastEvent.TS), in.LastEvent.Kind, orDash(in.LastEvent.Detail))
	}

	for _, tail := range in.LogTail {
		entry := "log=" + tail + "\n"
		if b.Len()+len(entry) > diagMaxChars {
			break
		}
		b.WriteString(entry)
	}
	return limitDiag(b.String())
}

func diagCalibLine(c *CalibInfo) string {
	if c == nil {
		return "none"
	}
	scale := "auto"
	if c.Scale != nil {
		scale = "x" + strconv.FormatFloat(*c.Scale, 'f', -1, 64)
	}
	anchor := "none"
	if c.AnchorScale != nil {
		anchor = "x" + strconv.FormatFloat(*c.AnchorScale, 'f', -1, 64)
	}
	return fmt.Sprintf("%s %s anchor=%s I_node=%s cc_node=%s",
		scale, orDash(c.Unit), anchor, orDash(c.CurrentNode), orDash(c.CounterNode))
}

func diagLiveLine(in diagInput) string {
	var raw, ua *int64
	if in.Snap.Calib != nil {
		raw, ua = in.Snap.Calib.Raw, in.Snap.Calib.UA
	}
	var b strings.Builder
	fmt.Fprintf(&b, "status=%s cap=%s Iraw=%s I=%s V=%s T=%s cc=%s",
		orDash(in.Status), optCap(in.Cap), optRaw(raw), optCurrent(ua),
		optVolt(in.VoltageUV), optTemp(in.Snap.TempC), optMah(in.CounterUAh))
	return b.String()
}

func diagSessionLine(s Session, loc *time.Location) string {
	delta := s.EndCap - s.StartCap
	when := diagTime(s.EndTs, loc)
	cc := "—"
	// 旧行（升级前）的 cc 列默认 0，无法区分“当时用积分”与“当时用了电量计但未
	// 落库”，因此无 cc 数据时只能标 ?，不能冒充 int。
	src := "?"
	if s.CounterUAh > 0 {
		cc = strconv.FormatInt(s.CounterUAh, 10)
		src = "cc"
	}
	if !s.Valid {
		return fmt.Sprintf("session=%s cap=%d→%d%% Δ%d valid=0 reason=%s avgI=%dmA src=%s",
			when, s.StartCap, s.EndCap, delta, orDash(s.InvalidReason), s.AvgI/1000, src)
	}
	return fmt.Sprintf("session=%s cap=%d→%d%% Δ%d valid=1 avgI=%dmA ua=%dµA·s cc=%s src=%s est=%dmAh",
		when, s.StartCap, s.EndCap, delta, s.AvgI/1000, s.AccUAs, cc, src, sessionEstMah(s))
}

func diagDischargeLine(d DischargeRow, loc *time.Location) string {
	implied := "—"
	if d.Implied != nil {
		implied = fmt.Sprintf("%dmAh", *d.Implied/1000)
	}
	return fmt.Sprintf("discharge=%s cap=%d→%d%% released=%dmAh implied=%s",
		diagTime(d.TS, loc), d.StartCap, d.EndCap, d.UAh/1000, implied)
}

func diagEventsLine(counts map[string]int64) string {
	keys := []string{"cur_cal", "counter_scale", "quality_warn", "tick_error", "sample_fail"}
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, fmt.Sprintf("%s=%d", k, counts[k]))
	}
	return strings.Join(parts, " ")
}

func diagTime(ts int64, loc *time.Location) string {
	if ts <= 0 {
		return "—"
	}
	if loc == nil {
		return time.Unix(ts, 0).UTC().Format("01-02 15:04")
	}
	return time.Unix(ts, 0).In(loc).Format("01-02 15:04")
}

// diagAge 把事件时间与当前时间差格式化成 (19d)/(3h)/(<1h)，提醒 last_event
// 可能是旧事件（events7d 只统计 7 天内，两者时间窗不同）。
func diagAge(now, ts int64) string {
	if now <= 0 || ts <= 0 || now < ts {
		return ""
	}
	d := now - ts
	switch {
	case d >= 86400:
		return fmt.Sprintf(" (%dd)", d/86400)
	case d >= 3600:
		return fmt.Sprintf(" (%dh)", d/3600)
	default:
		return " (<1h)"
	}
}

// limitDiag 按 rune 截断到 diagMaxChars；截断标记长度计入上限，不把字段切得
// 无法阅读。
func limitDiag(s string) string {
	rs := []rune(s)
	if len(rs) <= diagMaxChars {
		return s
	}
	suffix := "\n…(truncated)"
	keep := diagMaxChars - len([]rune(suffix))
	if keep < 1 {
		keep = diagMaxChars
	}
	return strings.TrimRight(string(rs[:keep]), "\n") + suffix
}

func orDash(s string) string {
	if strings.TrimSpace(s) == "" {
		return "—"
	}
	return s
}

func optCap(p *int64) string {
	if p == nil {
		return "—"
	}
	return fmt.Sprintf("%d%%", *p)
}

func optRaw(p *int64) string {
	if p == nil {
		return "—"
	}
	return strconv.FormatInt(*p, 10)
}

func optCurrent(ua *int64) string {
	if ua == nil {
		return "—"
	}
	return fmt.Sprintf("%.2fA", float64(*ua)/1e6)
}

func optVolt(uv *int64) string {
	if uv == nil {
		return "—"
	}
	return fmt.Sprintf("%.2fV", float64(*uv)/1e6)
}

func optTemp(c *float64) string {
	if c == nil {
		return "—"
	}
	return fmt.Sprintf("%.1fC", *c)
}

func optMah(uah *int64) string {
	if uah == nil {
		return "—"
	}
	return fmt.Sprintf("%dmAh", *uah/1000)
}

// optQualityMah 通道互检字段本身已是 mAh，不能再 /1000（真机曾显示 5mAh/6mAh）。
func optQualityMah(mah *int64) string {
	if mah == nil {
		return "—"
	}
	return fmt.Sprintf("%dmAh", *mah)
}

func sessionEstMah(s Session) int64 {
	e := convSession(s)
	if e.EstMah == nil {
		return 0
	}
	return *e.EstMah
}

func designMah(has bool, mah int64) string {
	if !has {
		return "—"
	}
	return fmt.Sprintf("%dmAh", mah)
}

func designCycles(d Design) string {
	if !d.HasCycles {
		return "—"
	}
	return strconv.FormatInt(d.Cycles, 10)
}

func qualText(ok bool) string {
	if ok {
		return "ok"
	}
	return "bad"
}
