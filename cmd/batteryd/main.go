package main

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// channel 由构建注入：CI 对 ML 变体使用 -ldflags "-X main.channel=ml"
var channel = "stable"

const (
	usageText = "用法: batteryd <daemon|once|json>\n"

	refreshRetryMax   = 10
	refreshRetryWait  = 30 * time.Second
	tickInterval      = 60 * time.Second
	refreshEveryTicks = 1440
	retainDays        = 90
	jsonRecentLimit   = 10
	tickFailMax       = 5

	logFile       = "batteryd.log"
	maxLogBytes   = 256 << 10 // 约 6h 错误量上限，超限保尾 128KB
	keepTailBytes = 128 << 10
)

func printUsage(w io.Writer) {
	fmt.Fprint(w, usageText)
}

func main() {
	cmd := "daemon"
	if len(os.Args) > 1 {
		cmd = os.Args[1]
	}

	var err error
	switch cmd {
	case "daemon":
		err = runDaemon()
	case "once":
		err = runOnce()
	case "json":
		err = runJson()
	default:
		printUsage(os.Stderr)
		os.Exit(2)
	}

	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

type app struct {
	moddir   string
	propPath string
	fs       SysFS
	st       *Store
	est      Estimator
	designUA int64

	nodePaths    map[string]string
	lastPruneDay int64
}

func deriveModdir(exePath string) string {
	return filepath.Dir(filepath.Dir(exePath))
}

func newApp() (*app, error) {
	exe, err := os.Executable()
	if err != nil {
		return nil, fmt.Errorf("定位可执行文件失败：%w", err)
	}
	moddir := deriveModdir(exe)
	dataDir := filepath.Join(moddir, "data")
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return nil, fmt.Errorf("创建数据目录失败：%w", err)
	}
	st, err := OpenStore(filepath.Join(dataDir, "battery.db"))
	if err != nil {
		return nil, fmt.Errorf("打开数据库失败：%w", err)
	}
	fs := SysFS{}
	// 出厂设计容量缺失/为 0 时不阻断启动：描述省略相应段，实测估算会因无法计算
	// 设计容量窗口而保持「学习期」，但内核读数（当前容量/循环次数）仍可正常展示。
	designUA := int64(0)
	if node, err := fs.FindNode("charge_full_design"); err == nil {
		if v, rerr := fs.ReadInt(node); rerr == nil {
			designUA = v
		}
	}
	if designUA <= 0 {
		_ = st.InsertEvent("design_missing", "charge_full_design 缺失或无效，实测估算停用")
	}
	var est Estimator = NewStable(st)
	if channel == "ml" {
		est = NewLearning(st)
	}
	return &app{
		moddir:   moddir,
		propPath: filepath.Join(moddir, "module.prop"),
		fs:       fs,
		st:       st,
		est:      est,
		designUA: designUA,
	}, nil
}

func (a *app) readIntNode(name string) (int64, error) {
	// 节点路径按进程缓存一次（固定路径未命中才全树扫描），避免每次刷新重复遍历 /sys/devices
	node, err := a.nodePath(name)
	if err != nil {
		return 0, err
	}
	return a.fs.ReadInt(node)
}

// nodePath 仅缓存成功解析的结果；失败不落缓存，下次调用会重新探测，
// 避免把开机瞬间未就绪的节点「永久屏蔽」成假重试。
func (a *app) nodePath(name string) (string, error) {
	if a.nodePaths == nil {
		a.nodePaths = make(map[string]string)
	}
	if p, ok := a.nodePaths[name]; ok {
		return p, nil
	}
	p, err := a.fs.FindNode(name)
	if err != nil {
		return "", err
	}
	a.nodePaths[name] = p
	return p, nil
}

func healthPct(fullUA, designUA int64) int64 {
	if designUA <= 0 {
		return 0
	}
	return fullUA * 100 / designUA
}

func (a *app) basics() Design {
	d := Design{DesignMah: a.designUA / 1000, HasDesign: a.designUA > 0}
	if full, err := a.readIntNode("charge_full"); err == nil {
		d.FullMah = full / 1000
		d.HasFull = true
		if d.HasDesign {
			d.Pct = healthPct(full, a.designUA)
			d.HasPct = true
		}
	}
	if cycles, err := a.readIntNode("cycle_count"); err == nil {
		d.Cycles = cycles
		d.HasCycles = true
	}
	return d
}

func (a *app) refresh() error {
	// 用 stats() 组装描述，让实测估算（EstUA）也能写进 module.prop
	d, snap, err := a.stats()
	if err != nil {
		return err
	}
	return a.refreshWith(d, snap)
}

func (a *app) refreshWith(d Design, snap Snapshot) error {
	desc, err := BuildDescription(d, snap)
	if err != nil {
		return err
	}
	return WriteModuleProp(a.propPath, desc)
}

func (a *app) refreshPruned() error {
	now := time.Now()
	day := now.Unix() / 86400
	if day != a.lastPruneDay {
		if err := a.st.PruneBefore(now.Unix() - retainDays*86400); err != nil {
			return err
		}
		a.lastPruneDay = day
	}
	return a.refresh()
}

func (a *app) stats() (Design, Snapshot, error) {
	d := a.basics()
	snap := Snapshot{
		CycleEquiv: float64(kvInt(a.st, kvChargedTotal)) / float64(a.designUA),
		RMoh:       a.rMoh(),
	}
	if ema, samples, ok := a.estimate(); ok {
		snap.EstUA = &ema
		snap.Samples = samples
	}
	// σ 仅 ML 通道有效：stable 读到残留 kv 值会污染 once/json 出口（describe 另有硬 gate）
	if channel == "ml" {
		snap.SigmaMah = a.sigmaMah()
	}
	if tRaw, err := a.readIntNode("temp"); err == nil {
		tempC := NormTempC(tRaw)
		snap.TempC = &tempC
	}
	// 趋势门控较严（点数/跨度/R²），失败即无此字段，属正常降级
	if recent, err := a.st.RecentEstimates(200); err == nil {
		for i, j := 0, len(recent)-1; i < j; i, j = i+1, j-1 {
			recent[i], recent[j] = recent[j], recent[i]
		}
		if tr, ok := FitTrend(recent); ok {
			// FitTrend 斜率与 estimates 表同单位（µAh/周），换算为 mAh/周
			v := tr.MahPerWeek / 1000
			snap.TrendMahPerWeek = &v
		}
	}
	return d, snap, nil
}

func (a *app) estimate() (int64, int64, bool) {
	samples := kvInt(a.st, kvKeySamples)
	ema := kvInt(a.st, kvKeyEmaUA)
	if samples <= 0 || ema <= 0 {
		return 0, 0, false
	}
	return ema, samples, true
}

func (a *app) rMoh() *float64 {
	v, ok := a.st.KVGet(kvREmaMOhm)
	if !ok {
		return nil
	}
	f, err := strconv.ParseFloat(v, 64)
	if err != nil || f <= 0 {
		return nil
	}
	return &f
}

// sigmaMah 从 kv 读置信区间；解析失败、≤0 或非有限值（学习期/种子重置）⇒ nil
func (a *app) sigmaMah() *float64 {
	v, ok := a.st.KVGet(kvKeyEmaSigma)
	if !ok {
		return nil
	}
	f, err := strconv.ParseFloat(v, 64)
	if err != nil || f <= 0 || math.IsNaN(f) || math.IsInf(f, 0) {
		return nil
	}
	return &f
}

func runDaemon() error {
	a, err := newApp()
	if err != nil {
		return err
	}
	defer a.st.Close()

	a.appendLog("启动 channel=%s", channel)

	var lastErr error
	for i := 0; i < refreshRetryMax; i++ {
		if lastErr = a.refreshPruned(); lastErr == nil {
			break
		}
		time.Sleep(refreshRetryWait)
	}
	if lastErr != nil {
		_ = a.st.InsertEvent("boot_fail", "首次刷新连续失败："+lastErr.Error())
		return fmt.Errorf("首次刷新连续 %d 次失败：%w", refreshRetryMax, lastErr)
	}

	statusNode, err := a.fs.FindNode("status")
	if err != nil {
		return fmt.Errorf("找不到 status 节点：%w", err)
	}

	p := NewPipeline(a.fs, a.st, a.est, a.designUA, time.Now)
	lastStatus := ""
	count := 0
	failStreak := 0
	ticker := time.NewTicker(tickInterval)
	defer ticker.Stop()
	for range ticker.C {
		raw, err := os.ReadFile(statusNode)
		if err != nil {
			// status 读取抖动属于瞬态故障，走连败计数而非直接退出
			streak, dead := tickStrike(failStreak, err)
			failStreak = streak
			msg := "读取 status 失败：" + err.Error()
			if dead {
				fatalf(a, "%s，连续 %d 次", msg, failStreak)
			}
			_ = a.st.InsertEvent("tick_error", msg)
			a.appendLog("%s", msg)
			fmt.Fprintln(os.Stderr, msg)
			continue
		}
		status := strings.TrimSpace(string(raw))

		if _, err := p.Tick(status); err != nil {
			streak, dead := tickStrike(failStreak, err)
			failStreak = streak
			msg := fmt.Sprintf("Tick(status=%s) 失败：%v", status, err)
			if dead {
				fatalf(a, "%s，连续 %d 次", msg, failStreak)
			}
			_ = a.st.InsertEvent("tick_error", msg)
			a.appendLog("%s", msg)
			fmt.Fprintln(os.Stderr, msg)
			continue
		}
		failStreak = 0

		changed := status != lastStatus
		lastStatus = status
		count++
		if changed || count >= refreshEveryTicks {
			if err := a.refreshPruned(); err != nil {
				_ = a.st.InsertEvent("refresh_fail", err.Error())
				a.appendLog("刷新描述失败：%s", err.Error())
				fmt.Fprintln(os.Stderr, "刷新描述失败："+err.Error())
			}
			count = 0
		}
	}
	return nil
}

func tickStrike(prev int, err error) (int, bool) {
	var se *SettleError
	if errors.As(err, &se) {
		return prev, true
	}
	n := prev + 1
	return n, n >= tickFailMax
}

// appendLog 向 <moddir>/data/batteryd.log 追加一行带时间戳的排障日志；
// 超过 maxLogBytes 时收敛为保尾 128KB 的环形语义。日志永远不影响主链路，
// 任何内部错误均静默吞掉。
func (a *app) appendLog(format string, args ...any) {
	path := filepath.Join(a.moddir, "data", logFile)
	_ = os.MkdirAll(filepath.Dir(path), 0o755)
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return // 日志永远不影响主链路
	}
	st, _ := f.Stat()
	if st != nil && st.Size() > maxLogBytes {
		f.Close()
		a.trimLog(path)
		f, err = os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o644)
		if err != nil {
			return
		}
	}
	defer f.Close()
	fmt.Fprintf(f, "[%s] %s\n", time.Now().Format("01-02 15:04:05"), fmt.Sprintf(format, args...))
}

func (a *app) trimLog(path string) {
	data, err := os.ReadFile(path)
	if err != nil || len(data) <= keepTailBytes {
		return
	}
	idx := bytes.IndexByte(data[len(data)-keepTailBytes:], '\n')
	if idx < 0 {
		idx = 0
	}
	tail := data[len(data)-keepTailBytes+idx:]
	_ = os.WriteFile(path, append([]byte("[truncated]\n"), tail...), 0o644)
}

func fatalf(a *app, format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	_ = a.st.InsertEvent("tick_fatal", msg)
	a.appendLog("%s", msg)
	fmt.Fprintln(os.Stderr, msg)
	os.Exit(1)
}

func runOnce() error {
	a, err := newApp()
	if err != nil {
		return err
	}
	defer a.st.Close()

	d, snap, err := a.stats()
	if err != nil {
		return err
	}
	if err := a.refreshWith(d, snap); err != nil {
		return err
	}

	estText := "学习期"
	if snap.EstUA != nil {
		estText = fmt.Sprintf("%d mAh（%d次有效会话）", *snap.EstUA/1000, snap.Samples)
	}
	resText := "采集中"
	if snap.RMoh != nil {
		resText = fmt.Sprintf("%.1f mΩ", *snap.RMoh)
	}

	if d.HasDesign {
		fmt.Printf("出厂设计容量：%d mAh\n", d.DesignMah)
	}
	if d.HasFull {
		fmt.Printf("当前电池容量：%d mAh\n", d.FullMah)
	}
	if d.HasCycles {
		fmt.Printf("循环次数：%d\n", d.Cycles)
	}
	if d.HasPct {
		fmt.Printf("剩余容量：%d%%\n", d.Pct)
	}
	fmt.Printf("实测估算：%s\n", estText)
	if snap.SigmaMah != nil {
		fmt.Printf("估计不确定度：±%.0f mAh\n", *snap.SigmaMah)
	}
	if math.IsNaN(snap.CycleEquiv) || math.IsInf(snap.CycleEquiv, 0) {
		fmt.Printf("循环当量：--\n")
	} else {
		fmt.Printf("循环当量：%.2f\n", snap.CycleEquiv)
	}
	fmt.Printf("内阻：%s\n", resText)
	if snap.TempC != nil {
		fmt.Printf("电池温度：%.1f℃\n", *snap.TempC)
	}
	return nil
}

func runJson() error {
	a, err := newApp()
	if err != nil {
		return err
	}
	defer a.st.Close()

	d, snap, err := a.stats()
	if err != nil {
		return err
	}
	recent, err := a.st.RecentEstimates(jsonRecentLimit)
	if err != nil {
		return err
	}
	sessRows, err := a.st.RecentSessions(jsonRecentLimit)
	if err != nil {
		return err
	}
	sess := make([]sessionEntry, 0, len(sessRows))
	for _, se := range sessRows {
		sess = append(sess, convSession(se))
	}
	restRows, err := a.st.RecentRestPoints(jsonRecentLimit)
	if err != nil {
		return err
	}
	rests := make([]restEntry, 0, len(restRows))
	for _, rp := range restRows {
		rests = append(rests, restEntry{TS: rp.TS, UV: rp.UV, Cap: rp.Cap})
	}
	ccctRows, err := a.st.RecentCCCT(jsonRecentLimit)
	if err != nil {
		return err
	}
	ccct := make([]ccctEntry, 0, len(ccctRows))
	for _, c := range ccctRows {
		ccct = append(ccct, ccctEntry{TS: c.TS, Secs: c.Secs})
	}
	icaRows, err := a.st.RecentICAPeaks(jsonRecentLimit)
	if err != nil {
		return err
	}
	icaPeaks := make([]icaEntry, 0, len(icaRows))
	for _, ip := range icaRows {
		icaPeaks = append(icaPeaks, icaEntry{TS: ip.TS, PeakUV: ip.PeakUV, PeakHRel: ip.PeakHRel})
	}
	n, err := a.st.CountSamples()
	if err != nil {
		return err
	}
	b, err := RenderJSON(channel, d, snap, recent, sess, rests, ccct, icaPeaks, n, time.Now())
	if err != nil {
		return err
	}
	fmt.Println(string(b))
	return nil
}
