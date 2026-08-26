package main

import (
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

	nodePaths map[string]string
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

// nodePath 缓存 FindNode 结果，未命中时解析一次并记录空结果，避免重复全树扫描。
func (a *app) nodePath(name string) (string, error) {
	if a.nodePaths == nil {
		a.nodePaths = make(map[string]string)
	}
	if p, ok := a.nodePaths[name]; ok {
		if p == "" {
			return "", fmt.Errorf("找不到节点：%s", name)
		}
		return p, nil
	}
	p, err := a.fs.FindNode(name)
	if err != nil {
		a.nodePaths[name] = ""
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
	if tRaw, err := a.readIntNode("temp"); err == nil {
		tempC := NormTempC(tRaw)
		snap.TempC = &tempC
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

func runDaemon() error {
	a, err := newApp()
	if err != nil {
		return err
	}
	defer a.st.Close()

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

func fatalf(a *app, format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	_ = a.st.InsertEvent("tick_fatal", msg)
	fmt.Fprintln(os.Stderr, msg)
	os.Exit(1)
}

func runOnce() error {
	a, err := newApp()
	if err != nil {
		return err
	}
	defer a.st.Close()

	if err := a.refresh(); err != nil {
		return err
	}
	d, snap, err := a.stats()
	if err != nil {
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
	b, err := RenderJSON(channel, d, snap, recent, time.Now())
	if err != nil {
		return err
	}
	fmt.Println(string(b))
	return nil
}
