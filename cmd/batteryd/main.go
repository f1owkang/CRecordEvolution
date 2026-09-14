package main

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	// 内嵌 IANA 时区库：Android 上无系统 tzdata 可读（实测 /etc/localtime、
	// /data/misc/zoneinfo 均不存在），不内嵌则 LoadLocation 必然失败。
	_ "time/tzdata"
)

// 本地时区解析：守护进程以 root 运行，Go 运行时读不到系统时区（TZ 未导出、
// 无 tzdata 路径），time.Now() 回退 UTC，导致日志与 JSON 的时间戳慢 8 小时。
// 解析顺序：① getprop persist.sys.timezone（Android 权威源，任何 ROM 均可，
// 属性服务数据源即 /dev/__properties__ 的 timezone_prop 上下文，真机已验证）
// ② /data/property/persist.sys.timezone 文件直读（AOSP 系兜底，MIUI 等无此
// 文件）③ 保持 time.Local（UTC）——宁可诚实显示 UTC，也不硬编码某个时区。
var (
	localZoneOnce sync.Once
	localZone     *time.Location
)

// tzPropSource / tzFileSource 可注入，供单元测试替换；默认走真机路径。
var (
	tzPropSource = func() (string, bool) {
		out, err := exec.Command("/system/bin/getprop", "persist.sys.timezone").Output()
		if err != nil {
			return "", false
		}
		if s := strings.TrimSpace(string(out)); s != "" {
			return s, true
		}
		return "", false
	}
	tzFileSource = func() (string, bool) {
		b, err := os.ReadFile("/data/property/persist.sys.timezone")
		if err != nil {
			return "", false
		}
		if s := strings.TrimSpace(string(b)); s != "" {
			return s, true
		}
		return "", false
	}
)

func resolveLocalZone() *time.Location {
	for _, src := range []func() (string, bool){tzPropSource, tzFileSource} {
		name, ok := src()
		if !ok {
			continue
		}
		// 非法/非 IANA 名由 LoadLocation 拒绝，顺延下一来源
		if loc, err := time.LoadLocation(name); err == nil {
			return loc
		}
	}
	return time.Local
}

func localNow() time.Time {
	localZoneOnce.Do(func() { localZone = resolveLocalZone() })
	return time.Now().In(localZone)
}

// channel 由构建注入：CI 对 ML 变体使用 -ldflags "-X main.channel=ml"
var channel = "stable"

const (
	usageText = "用法: batteryd <daemon|once|json>\n"

	refreshRetryMax  = 10
	refreshRetryWait = 30 * time.Second
	tickInterval     = 60 * time.Second
	// chargeSampleStep 充电期采样步长：快充电压每 60s 可爬 ~350mV（实测
	// 3835→4189mV/拍），60s 步长会整段跳过 4.20→4.30V 观察窗，CCCT 永远
	// 采不到跨窗点；15s 步长下窗内必有 1~2 个样本。
	chargeSampleStep  = 15 * time.Second
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
	moddir    string
	propPath  string
	fs        SysFS
	st        *Store
	est       Estimator
	designUA  int64
	cellCount int

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
	// 双电芯检测：读 voltage_now，超过 5V 判定为串联双电芯（实际电压 ≈ 单电芯 × 2）。
	// 阈值定 5V：高压单电芯截止 4.45~4.53V（OPPO/一加系常见），静息+满电不得超过
	// ~4.6V；双电芯串联最低 ≈ 2×3.4V = 6.8V。5V 两侧各留充足余量，杜绝把高压
	// 单电芯误判成双电芯（误判会导致所有电压窗口翻倍、CCCT/ICA 静默全哑）。
	// 双电芯设备的 voltage_now 报告的是串联总电压（如 8.4V），所有基于单电芯
	// 电压的阈值（CCCT 窗口、ICA 搜索域、ML 归一化）需相应缩放。
	cellCount := 1
	if vNode, err := fs.FindNode("voltage_now"); err == nil {
		if v, verr := fs.ReadInt(vNode); verr == nil && v > 5_000_000 {
			cellCount = 2
			_ = st.InsertEvent("dual_cell", fmt.Sprintf("检测到双电芯，voltage_now=%dµV", v))
		}
	}
	// 按电芯数缩放所有电压阈值（CCCT 窗口、ICA 搜索域）
	initCCCTVoltage(cellCount)
	initICAVoltage(cellCount)
	var est Estimator = NewStable(st)
	if channel == "ml" {
		est = NewLearning(st, cellCount)
	}
	return &app{
		moddir:    moddir,
		propPath:  filepath.Join(moddir, "module.prop"),
		fs:        fs,
		st:        st,
		est:       est,
		designUA:  designUA,
		cellCount: cellCount,
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

// cycleEquiv 循环当量：累计充入电量 ÷ 设计容量。charged_ua_total 单位 µA·s，
// 须先 ÷3600 折算 µAh 再与 designUA（µAh）同纲相除，否则差 3600 倍；
// designUA 缺失时返回 +Inf，由出口按非有限值降级省略。
func cycleEquiv(totalUAs, designUA int64) float64 {
	if designUA <= 0 {
		return math.Inf(1)
	}
	return float64(totalUAs) / (float64(designUA) * 3600)
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
	// 只用 epoch，时区无关；避免在此路径触发 getprop 时区解析
	now := time.Now()
	day := now.Unix() / 86400
	if day != a.lastPruneDay {
		if err := a.st.PruneBefore(now.Unix() - retainDays*86400); err != nil {
			return err
		}
		// 每日顺手做一次 TRUNCATE 检查点：把 WAL 落进主库清零，主库文件
		// 始终接近自包含（复制单个 .db 不再缺最近数周数据）
		a.st.Checkpoint()
		a.lastPruneDay = day
	}
	return a.refresh()
}

func (a *app) stats() (Design, Snapshot, error) {
	d := a.basics()
	snap := Snapshot{
		CycleEquiv: cycleEquiv(kvInt(a.st, kvChargedTotal), a.designUA),
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
	// 趋势三态：insufficient（数据不足）/stable（无显著变化）/significant（显著变化），判定见 trend.go；
	// 无任何估算点时整体省略，前端显示占位符
	if recent, err := a.st.RecentEstimates(200); err == nil && len(recent) > 0 {
		for i, j := 0, len(recent)-1; i < j; i, j = i+1, j-1 {
			recent[i], recent[j] = recent[j], recent[i]
		}
		tr, ok := FitTrend(recent)
		snap.TrendState = string(tr.State)
		if tr.State == TrendInsufficient {
			span := tr.SpanDay
			snap.TrendSpanDay = &span
		}
		if ok {
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

// resDisplayN 内阻显示取最近 N 条样本的中位数：单窗回归值受电流阶跃污染可达
// 真值的 5~10 倍，在线 EMA（α=0.2、无稳健机制）实测被持续踢高 7 倍，中位数口径
// 无状态且自愈；样本不足（含 90 天清理后无近期充电）按缺失省略。
const resDisplayN = 16

func (a *app) rMoh() *float64 {
	mos, err := a.st.RecentResistance(resDisplayN)
	if err != nil || len(mos) == 0 {
		return nil
	}
	return medianMoh(mos)
}

// medianMoh 取中位内阻（偶数个取中间两值均值）；无正值返回 nil。
func medianMoh(mos []float64) *float64 {
	if len(mos) == 0 {
		return nil
	}
	sorted := append([]float64(nil), mos...)
	sort.Float64s(sorted)
	n := len(sorted)
	var median float64
	if n%2 == 1 {
		median = sorted[n/2]
	} else {
		median = (sorted[n/2-1] + sorted[n/2]) / 2
	}
	if median <= 0 {
		return nil
	}
	return &median
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

	p := NewPipeline(a.fs, a.st, a.est, a.designUA, a.cellCount, time.Now)
	p.Logf(a.appendLog)
	lastStatus := ""
	count := 0
	failStreak := 0
	ticker := time.NewTicker(tickInterval)
	defer ticker.Stop()
	chargingStep := false // 当前 ticker 是否处于充电期 15s 快步长
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

		// 变步长：充电期 15s 快采样（CCCT 观察窗在快充下每 60s 爬 ~350mV，
		// 60s 步长会整段跳过 4.20→4.30V 窗）；其余维持 60s。电量按真实
		// 时间差累积（pipeline 内），步长切换不产生计量偏差。
		wantStep := status == "Charging"
		if wantStep != chargingStep {
			step := tickInterval
			if wantStep {
				step = chargeSampleStep
			}
			ticker.Reset(step)
			chargingStep = wantStep
			if wantStep {
				a.appendLog("[采样] 进入充电期，步长 60s→15s")
			} else {
				a.appendLog("[采样] 退出充电期，步长 15s→60s")
			}
		}

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
	fmt.Fprintf(f, "[%s] %s\n", localNow().Format("01-02 15:04:05"), fmt.Sprintf(format, args...))
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
	// 放电记录摘要（初步观测通道）：最近一次会话与累计段数
	if disRows, derr := a.st.RecentDischarge(1); derr == nil && len(disRows) > 0 {
		r := disRows[0]
		line := fmt.Sprintf("放电记录：%d→%d%% 放出 %d mAh", r.StartCap, r.EndCap, r.Uah/1000)
		if r.Implied != nil {
			line += fmt.Sprintf("（隐含 %d mAh）", *r.Implied/1000)
		}
		fmt.Printf("%s\n", line)
	}
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
		ccct = append(ccct, ccctEntry{TS: c.TS, Secs: c.Secs, VwLo: c.VwLo, VwHi: c.VwHi})
	}
	icaRows, err := a.st.RecentICAPeaks(jsonRecentLimit)
	if err != nil {
		return err
	}
	icaPeaks := make([]icaEntry, 0, len(icaRows))
	for _, ip := range icaRows {
		icaPeaks = append(icaPeaks, icaEntry{TS: ip.TS, PeakUV: ip.PeakUV, PeakHRel: finitePtr(&ip.PeakHRel)})
	}
	disRows, err := a.st.RecentDischarge(jsonRecentLimit)
	if err != nil {
		return err
	}
	disch := make([]dischargeEntry, 0, len(disRows))
	for _, dr := range disRows {
		e := dischargeEntry{TS: dr.TS, Secs: dr.Secs, UahMah: dr.Uah / 1000,
			StartCap: dr.StartCap, EndCap: dr.EndCap, Implied: dr.Implied}
		if e.Implied != nil {
			v := *e.Implied / 1000
			e.Implied = &v
		}
		disch = append(disch, e)
	}
	n, err := a.st.CountSamples()
	if err != nil {
		return err
	}
	b, err := RenderJSON(channel, d, snap, recent, sess, rests, ccct, icaPeaks, disch, n, localNow())
	if err != nil {
		return err
	}
	fmt.Println(string(b))
	return nil
}
