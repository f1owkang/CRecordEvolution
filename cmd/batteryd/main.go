package main

import (
	"errors"
	"fmt"
	"io"
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
	fail := func(err error) (*app, error) {
		_ = st.InsertEvent("boot_fail", err.Error())
		_ = st.Close()
		return nil, err
	}
	fs := SysFS{}
	node, err := fs.FindNode("charge_full_design")
	if err != nil {
		return fail(fmt.Errorf("读取出厂设计容量失败：%w", err))
	}
	designUA, err := fs.ReadInt(node)
	if err != nil {
		return fail(fmt.Errorf("读取出厂设计容量失败：%w", err))
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
	node, err := a.fs.FindNode(name)
	if err != nil {
		return 0, err
	}
	return a.fs.ReadInt(node)
}

func (a *app) basics() (Design, int64, error) {
	full, err := a.readIntNode("charge_full")
	if err != nil {
		return Design{}, 0, err
	}
	cycles, err := a.readIntNode("cycle_count")
	if err != nil {
		return Design{}, 0, err
	}
	pct, err := a.readIntNode("capacity")
	if err != nil {
		return Design{}, 0, err
	}
	d := Design{DesignMah: a.designUA / 1000, FullMah: full / 1000, Cycles: cycles}
	return d, pct, nil
}

func (a *app) refresh() error {
	d, pct, err := a.basics()
	if err != nil {
		return err
	}
	return WriteModuleProp(a.propPath, BuildDescription(d, Snapshot{Pct: pct}))
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
	d, pct, err := a.basics()
	if err != nil {
		return Design{}, Snapshot{}, err
	}
	snap := Snapshot{
		Pct:        pct,
		CycleEquiv: float64(kvInt(a.st, kvChargedTotal)) / float64(a.designUA),
		RMoh:       a.rMoh(),
	}
	if ema, samples, ok := a.estimate(); ok {
		snap.EstUA = &ema
		snap.Samples = samples
	}
	tRaw, err := a.readIntNode("temp")
	if err != nil {
		return d, snap, err
	}
	tempC := NormTempC(tRaw)
	snap.TempC = &tempC
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
			fatalf(a, "读取 status 失败：%v", err)
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

	fmt.Printf("出厂设计容量：%d mAh\n", d.DesignMah)
	fmt.Printf("当前电池容量：%d mAh\n", d.FullMah)
	fmt.Printf("循环次数：%d\n", d.Cycles)
	fmt.Printf("剩余容量：%d%%\n", snap.Pct)
	fmt.Printf("实测估算：%s\n", estText)
	fmt.Printf("循环当量：%.2f\n", snap.CycleEquiv)
	fmt.Printf("内阻：%s\n", resText)
	fmt.Printf("电池温度：%.1f℃\n", *snap.TempC)
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
