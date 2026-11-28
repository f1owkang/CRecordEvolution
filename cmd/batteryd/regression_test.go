package main

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

// 静置计数必须随「离开放电态」清零：旧实现只在 tickResting 内清，而充电/尾段/
// 去抖分支都在调用 tickResting 之前 return，计数会跨充电周期冻结。冻结后拔充
// 第 1 拍就满足 ≥3 拍门槛（早期静置点提前落库），且 ==10 拍的收敛指纹点再也
// 命中不了。
func TestRestStreakResetsAcrossChargeCycles(t *testing.T) {
	r := newPipeRig(t)

	// 阶段一：静置放电 4 拍（cap/uv 不变，去重基线稳定），第 3 拍落早期点。
	for i := 0; i < 4; i++ {
		r.put(50, 20_000, 3_800_000)
		r.step("Discharging")
	}
	if n := countRows(t, r.st, "rest_points"); n != 1 {
		t.Fatalf("连续静置 4 拍应只落 1 个早期点, rows=%d", n)
	}

	// 阶段二：满电浮充 2 拍（不开会话，不结算，旧实现的 streak 从此冻结在 4）。
	for i := 0; i < 2; i++ {
		r.put(100, 6_000_000, 4_350_000)
		r.step("Charging")
	}

	// 阶段三：拔充后第 1 拍静置。cap/uv 与上次静置点不同，去重不会掩盖问题。
	r.put(95, 20_000, 3_900_000)
	r.step("Discharging")
	if n := countRows(t, r.st, "rest_points"); n != 1 {
		t.Fatalf("跨充电周期后第 1 拍静置不应落点（旧实现会立刻写）, rows=%d", n)
	}
	// 第 2 拍仍不落点，第 3 拍满足 3 拍门槛才落早期点。
	for i := 0; i < 2; i++ {
		r.put(95, 20_000, 3_900_000)
		r.step("Discharging")
		want := 2
		if i == 0 {
			want = 1
		}
		if n := countRows(t, r.st, "rest_points"); n != want {
			t.Fatalf("第 %d 拍静置后 rows=%d, want %d", i+1, n, want)
		}
	}
	// 继续静置到第 10 拍：restRelaxTicks 的收敛指纹点应强制落库（不受去重抑制）。
	for i := 3; i < 10; i++ {
		r.put(95, 20_000, 3_900_000)
		r.step("Discharging")
	}
	if n := countRows(t, r.st, "rest_points"); n != 3 {
		t.Fatalf("第 10 拍应强制落收敛指纹点, rows=%d want 3", n)
	}
}

// rest_points 只是展示用观测：写失败必须只落事件、不能升级成 SettleError 让
// 守护进程立即退出（静息放电每 60s 写一次，一次 SQLITE_BUSY 就会永久停采）。
func TestRestPointWriteFailureIsNotFatal(t *testing.T) {
	r := newPipeRig(t)
	if err := r.st.Close(); err != nil {
		t.Fatalf("关闭 store: %v", err)
	}
	var lastErr error
	for i := 0; i < 3; i++ {
		r.put(50, 20_000, 3_800_000)
		_, lastErr = r.p.Tick("Discharging")
	}
	if lastErr != nil {
		t.Fatalf("静置点写失败不应让 Tick 返回错误, got %v", lastErr)
	}
	var se *SettleError
	if errors.As(lastErr, &se) {
		t.Fatal("静置点写失败不得包装成 SettleError")
	}
}

// counterHole（电量计开账后才可用）必须随会话持久化：旧实现只在内存里置位，
// daemon 重启恢复后会把「只覆盖后半段」的增量当全程采信，隐含容量偏低。
// 节点晚可用本身在真机上很少见（readChargeCounterRaw 对「节点确认不存在」有
// 进程级负缓存），但确认场景是「首次读瞬时失败、随后恢复」，所以直接测持久化
// 契约与恢复后的结算口径。
func TestCounterHolePersistsAcrossRestart(t *testing.T) {
	r := newPipeRig(t)
	p := r.p
	p.sess = sessionState{
		active:   true,
		startTs:  tickBaseTs,
		startCap: 20,
		lastCap:  60,
		accUAs:   2_000_000 * 3600,
		ticks:    4,
		// 电量计在开账后才可用：增量只覆盖部分区间。
		counterLastUAh: 4_000_000,
		counterAccUAh:  1_400_000,
		counterHole:    true,
	}
	if err := p.persistSession(); err != nil {
		t.Fatalf("persistSession: %v", err)
	}
	wantKV(t, r.st, kvSessCounterHole, "1")

	// 模拟 daemon 重启：恢复的会话必须带回 counterHole。
	r.rebuildPipeline()
	if !r.p.sess.counterHole {
		t.Fatal("重启恢复后 counterHole 应保持为 true")
	}
	if r.p.sess.counterAccUAh != 1_400_000 {
		t.Fatalf("恢复 counterAccUAh = %d, want 1400000", r.p.sess.counterAccUAh)
	}
	// 结算走裁决：hole ⇒ 回退电流积分，sessions.cc 必须为 0。
	counterUAh, _ := r.p.adjudicateCounter(&r.p.sess, &Session{StartCap: 20, EndCap: 60})
	if counterUAh != 0 {
		t.Fatalf("counterHole 会话应回退积分（cc=0），got %d", counterUAh)
	}
}

// history 趋势里没有 ICA 峰时 ica_peaks 必须是 [] 而不是 null（字段无 omitempty，
// nil 会破坏「空数据输出 []」的既有契约）。
func TestHistoryEmptyIcaPeaksIsArray(t *testing.T) {
	st := openTestStore(t, filepath.Join(t.TempDir(), "battery.db"))
	t.Cleanup(func() { _ = st.Close() })
	doc, err := buildHistoryDoc(st, historyOptions{Days: 30, Limit: 20, Type: "all"}, time.Unix(1_760_000_000, 0))
	if err != nil {
		t.Fatalf("buildHistoryDoc: %v", err)
	}
	b, err := json.Marshal(doc)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	s := string(b)
	if strings.Contains(s, `"ica_peaks":null`) {
		t.Fatalf("ica_peaks 不应为 null: %s", s)
	}
	if !strings.Contains(s, `"ica_peaks":[]`) {
		t.Fatalf("ica_peaks 应为 []: %s", s)
	}
}

// 首次回填失败后 daemon 会先写出一条 daily 行；旧实现看到 daily 非空就直接落
// 「已完成」标记，升级前仍在 estimates 里的历史点永远进不了档案。回填必须按
// estimates 重新聚合覆盖写，而不是以「已有行」为由跳过。
func TestBackfillDailyRecomputesWhenDailyAlreadyHasRows(t *testing.T) {
	st := openTestStore(t, filepath.Join(t.TempDir(), "battery.db"))
	t.Cleanup(func() { _ = st.Close() })
	base := int64(1_760_000_000)
	for _, row := range []struct {
		ts int64
		v  int64
	}{
		{base, 4_000_000},
		{base + 3_600, 4_050_000},
		{base + 86_400, 4_100_000},
	} {
		if _, err := st.db.Exec(`INSERT INTO estimates(ts, mah) VALUES(?, ?)`, row.ts, row.v); err != nil {
			t.Fatalf("写 estimates: %v", err)
		}
	}
	// 模拟「回填失败后 daemon 正常结算先写了一条 daily」：值/点数都是旧的。
	day1 := localDayKey(base)
	if _, err := st.db.Exec(`INSERT INTO daily(day, ts, uah, n) VALUES(?, ?, ?, ?)`,
		day1, base, 4_000_000, 1); err != nil {
		t.Fatalf("写 daily: %v", err)
	}

	if err := st.BackfillDaily(); err != nil {
		t.Fatalf("BackfillDaily: %v", err)
	}
	var n, uah int64
	if err := st.db.QueryRow(`SELECT n, uah FROM daily WHERE day = ?`, day1).Scan(&n, &uah); err != nil {
		t.Fatalf("读 day1: %v", err)
	}
	if n != 2 || uah != 4_050_000 {
		t.Fatalf("day1 回填后 = (n=%d,uah=%d), want (2,4050000)", n, uah)
	}
	if got := countRows(t, st, "daily"); got != 2 {
		t.Fatalf("daily 行数 = %d, want 2（两天）", got)
	}
	// 幂等：第二次调用（标记已落）不再改写。
	if err := st.BackfillDaily(); err != nil {
		t.Fatalf("第二次 BackfillDaily: %v", err)
	}
	if err := st.db.QueryRow(`SELECT n, uah FROM daily WHERE day = ?`, day1).Scan(&n, &uah); err != nil {
		t.Fatalf("再读 day1: %v", err)
	}
	if n != 2 || uah != 4_050_000 {
		t.Fatalf("回填应幂等, got (n=%d,uah=%d)", n, uah)
	}
}

// estimates 明细与 daily 档案必须全有或全无：daily 写失败要回滚 estimates。
func TestInsertEstimateRollsBackEstimatesOnDailyFailure(t *testing.T) {
	st := openTestStore(t, filepath.Join(t.TempDir(), "battery.db"))
	t.Cleanup(func() { _ = st.Close() })
	mustExec(t, st, `DROP TABLE daily`)
	if err := st.InsertEstimate(1_760_000_000, 4_000_000); err == nil {
		t.Fatal("daily 缺失时 InsertEstimate 应返回错误")
	}
	if n := countRows(t, st, "estimates"); n != 0 {
		t.Fatalf("档案写失败时 estimates 应回滚, rows=%d", n)
	}
	mustExec(t, st, schemaDDL)
	if err := st.InsertEstimate(1_760_000_000, 4_000_000); err != nil {
		t.Fatalf("重建 daily 后应成功: %v", err)
	}
	if n := countRows(t, st, "estimates"); n != 1 {
		t.Fatalf("estimates 行数 = %d, want 1", n)
	}
	if n := countRows(t, st, "daily"); n != 1 {
		t.Fatalf("daily 行数 = %d, want 1", n)
	}
}

// ML 离群锁死自愈：连续 N 次 IQR 离群说明基线已漂移到新工况（单位纠正/换电池/
// 校准拖动 EMA）。旧实现只拒收不学习，hist 与 EMA 都不变，之后每次都会同样离群
// （永久拒收）。达阈值后应放行一次并重置比值历史。
func TestLearningOutlierLockoutEscapes(t *testing.T) {
	est, st := newTestLearning(t)
	seedML(t, est)
	feed := func(permille int64) (EstUpdate, error) {
		e := kvInt(st, kvKeyEmaUAh)
		sr := SettledSession{Session: baseSession(), AccUAs: e * 2160 * permille / 1000, DesignUAh: 4000000}
		return est.OnSession(sr)
	}
	// 8 条比值完全一致的会话：hist 收敛到很窄的 IQR 带。
	for i := 0; i < 8; i++ {
		if _, err := feed(1003); err != nil {
			t.Fatalf("近同值会话 %d 应接受: %v", i, err)
		}
	}
	rejects := 0
	accepted := false
	for i := 0; i < 5; i++ {
		if _, err := feed(1050); err == nil {
			accepted = true
			break
		} else {
			var re *RejectError
			if !errors.As(err, &re) || re.Result.Reason != "outlier" {
				t.Fatalf("阶跃会话应判 outlier, got %v", err)
			}
			rejects++
		}
	}
	if !accepted {
		t.Fatalf("连续 %d 次离群后仍锁死，模型无法恢复", rejects)
	}
	if rejects != outlierEscape-1 {
		t.Fatalf("离群拒收次数 = %d, want %d（第 %d 次应放行并重置）", rejects, outlierEscape-1, outlierEscape)
	}
	// 逃逸后新工况的会话继续被接受。
	if _, err := feed(1003); err != nil {
		t.Fatalf("逃逸后应恢复接受: %v", err)
	}
}

func TestDisCounterJumpResync(t *testing.T) {
	cases := []struct {
		d, dt int64
		want  bool
	}{
		{-150_000, 60, true},
		{150_000, 60, true},
		{-150_000, disResyncMaxDt, true},
		{-58_333, 60, false},   // 短拍正常大电流放电（~1A×60s）
		{-150_000, 61, true},   // 61s 也在短拍窗口内
		{-150_000, 300, false}, // 长间隙可累积真实放电，交给 25A 上界
		{-400_000, 600, false},
		{0, 60, false},
		{-150_000, 0, false},
	}
	for _, c := range cases {
		if got := disCounterJumpResync(c.d, c.dt); got != c.want {
			t.Fatalf("disCounterJumpResync(%d,%d) = %v, want %v", c.d, c.dt, got, c.want)
		}
	}
}

// 短拍巨幅电量计回修只重定基线，不能计入放出电量（旧实现会多算 150mAh）。
func TestDischargeSkipsShortTickResyncJump(t *testing.T) {
	r := newPipeRig(t)
	cc := int64(5_000_000)
	putCounter(t, r, cc)
	r.put(80, -500_000, 3_800_000)
	r.step("Discharging") // 首拍只重定基线

	step := int64(8_333)
	for i := 0; i < 3; i++ { // 3 拍正常放电（≈0.5A×60s）
		cc -= step
		putCounter(t, r, cc)
		r.put(78-int64(i), -500_000, 3_800_000)
		r.step("Discharging")
	}
	// 1 拍回修跳变：-150mAh/60s（等效 9A，短拍不可能持续），应被跳过。
	cc -= 150_000
	putCounter(t, r, cc)
	r.put(74, -500_000, 3_800_000)
	r.step("Discharging")
	// 再 3 拍正常放电。
	for i := 0; i < 3; i++ {
		cc -= step
		putCounter(t, r, cc)
		r.put(72-int64(i), -500_000, 3_800_000)
		r.step("Discharging")
	}
	r.put(66, 3_000_000, 3_700_000)
	r.step("Charging") // 结算放电会话

	rows, err := r.st.RecentDischarge(1)
	if err != nil || len(rows) != 1 {
		t.Fatalf("RecentDischarge = (%v,%v), want 1 行", rows, err)
	}
	wantUAh := 6 * step // 首拍基线不计；1 拍回修跳变跳过
	if rows[0].UAh != wantUAh {
		t.Fatalf("放电电量 = %d, want %d（回修跳变不得计入）", rows[0].UAh, wantUAh)
	}
}

// 整窗在单个采样间隔内被跨过时没有窗内锚点，耗时会退化成 1 拍且不可信，不能采信。
func TestLocateWindowCrossRejectsSingleIntervalCrossing(t *testing.T) {
	seg := CCSeg{FromTs: 0, ToTs: 30}
	degenerate := []SampleRow{
		{TS: 0, UV: 4_150_000},
		{TS: 15, UV: 4_350_000}, // 首个 ≥WinLo 的样本已越过 WinHi
		{TS: 30, UV: 4_360_000},
	}
	if _, _, cross := locateWindowCross(degenerate, seg); cross {
		t.Fatal("单拍整窗跨越不应被采信")
	}
	normal := []SampleRow{
		{TS: 0, UV: 4_150_000},
		{TS: 15, UV: 4_250_000},
		{TS: 30, UV: 4_350_000},
	}
	lo, hi, cross := locateWindowCross(normal, seg)
	if !cross || lo.UV != 4_250_000 || hi.UV != 4_350_000 {
		t.Fatalf("正常跨窗应命中窗内锚点, got lo=%+v hi=%+v cross=%v", lo, hi, cross)
	}
}

// ICA 不能把恒流段之间的长间隙按「UA×gap」累进到达电压桶：600s×2A≈330mAh 足以
// 伪造主峰，首个会话还会把伪峰写成 ica_peak_base。
func TestFindPeakIgnoresSegmentGap(t *testing.T) {
	rows := []SampleRow{
		{TS: 0, UA: 2_000_000, UV: 3_800_000},
		{TS: 15, UA: 2_000_000, UV: 3_815_000},
		{TS: 30, UA: 2_000_000, UV: 3_830_000},
		{TS: 45, UA: 2_000_000, UV: 3_845_000},
		// 段间 600s 非 CC 间隙（recordCCCT 会把多段拼成同一片交给 ICA）。
		{TS: 645, UA: 2_000_000, UV: 4_100_000},
		{TS: 660, UA: 2_000_000, UV: 4_115_000},
		{TS: 675, UA: 2_000_000, UV: 4_130_000},
		{TS: 690, UA: 2_000_000, UV: 4_145_000},
	}
	_, h, ok := FindPeak(rows)
	// 段内每拍 dQ = 2A×15s = 3e7；跨段跳变若被计入会多出 1.2e9。
	if ok && h > 1e8 {
		t.Fatalf("主峰高 %.3g 明显包含跨段间隙电量（应 ≤1e8）", h)
	}
}

// 设计容量在 daemon 运行中重新裁决：design set 覆盖文件变化后必须同步给
// app 与正在采样的 Pipeline，否则描述/概览与估算口径会来回跳。
func TestReloadDesignPicksUpManualOverride(t *testing.T) {
	dir := t.TempDir()
	st := openTestStore(t, filepath.Join(dir, "battery.db"))
	t.Cleanup(func() { _ = st.Close() })
	a := &app{moddir: dir, st: st, fs: SysFS{Base: filepath.Join(dir, "sys")}}
	p := NewPipeline(a.fs, st, NewStable(st), 0, 1, time.Now)
	dataDir := filepath.Join(dir, "data")
	if err := writeDesignOverride(dataDir, 3220); err != nil {
		t.Fatalf("写覆盖: %v", err)
	}
	a.reloadDesign(p)
	if a.designUAh != 3_220_000 || p.designUAh != 3_220_000 {
		t.Fatalf("首次 reloadDesign = (app=%d,pipeline=%d), want 3220000", a.designUAh, p.designUAh)
	}
	if a.designInfo.Source != designSrcManual {
		t.Fatalf("design source = %q, want manual", a.designInfo.Source)
	}
	if err := writeDesignOverride(dataDir, 4000); err != nil {
		t.Fatalf("改写覆盖: %v", err)
	}
	a.reloadDesign(p)
	if a.designUAh != 4_000_000 || p.designUAh != 4_000_000 {
		t.Fatalf("覆盖值变化后 = (app=%d,pipeline=%d), want 4000000", a.designUAh, p.designUAh)
	}
}

// 原子写多进程并发：固定 "<path>.tmp" 会互相截断/rename，唯一临时文件并发写
// 必须都成功且最终内容是其中一份完整数据、不残留临时文件。
func TestAtomicWriteFileConcurrent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "module.prop")
	const workers = 8
	errCh := make(chan error, workers)
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			errCh <- atomicWriteFile(path, []byte("v"+strconv.Itoa(i)), 0o644)
		}(i)
	}
	wg.Wait()
	close(errCh)
	for err := range errCh {
		if err != nil {
			t.Fatalf("并发原子写失败: %v", err)
		}
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("读结果: %v", err)
	}
	if !strings.HasPrefix(string(data), "v") {
		t.Fatalf("最终内容异常: %q", string(data))
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("读目录: %v", err)
	}
	for _, e := range entries {
		if e.Name() != "module.prop" {
			t.Fatalf("残留临时文件: %s", e.Name())
		}
	}
}
