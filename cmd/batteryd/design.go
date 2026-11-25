// 设计容量口径裁决：charge_full_design 并不总是与 charge_full 同口径。
// 部分 vivo/iQOO 双电芯机把电池标签容量写进该节点（issue #14 的 V2463A：
// charge_full_design=4508mAh，uevent 的 MODEL_NAME 含 4500MAH），而
// charge_full / charge_counter / 各路估算都是「双电芯串联的单芯电荷量」
// （同一块电池的 3220mAh 口径）。直接相除会把健康度打成 ~60%。
//
// 设备树的 vivo,bat-capacity-mah 由电量计配置给出，与 charge_counter 同口径，
// 可用作交叉校验；读不到时允许用户在 data/design_mah 手动指定。
//
// 裁决原则（顺序即优先级）：
//  1. 手动覆盖（data/design_mah）优先，用户显式指定的值不再参与自动裁决；
//  2. sysfs 与设备树都没有有效值时按缺失处理（描述/JSON 逐段降级）；
//  3. 只有一侧有值时用该侧；
//  4. 两侧都有且分歧 ≤designMismatchPct 时信 sysfs（ABI 值，不改变既有设备行为）；
//  5. 分歧过大时按「与实测锚最接近的一侧」裁决——实测锚是 charge_full（电量计
//     满充容量，与设计容量同口径），缺失时退回已采信估算。这是两种口径之间
//     唯一可定向的观测证据；没有锚则保持 sysfs 值并标记 mismatch 供诊断。
//
// 裁决只改「设计容量」这一个量：健康度百分比、capacityInWindow 窗口、
// cycleEquiv 循环当量、weeks_to_80 全部共用它；不动采样/结算口径。

package main

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const (
	// designOverrideFile 手动覆盖文件（相对 <moddir>/data/）。内容为纯数字
	// 容量（mAh），用于自动裁决不可靠的机型；改完重启模块生效。
	designOverrideFile = "design_mah"
	// 覆盖值的合理范围（mAh）：超出视为写错（例如把 µAh 当 mAh 填）。
	designOverrideMinMah = 100
	designOverrideMaxMah = 30000

	// designMismatchPct 判「sysfs 与设备树不是同一口径」的相对分歧阈值（%），
	// 分母取两者中较大的值。同口径的两次读数典型差异 <5%（都是设计值，不随
	// 老化/温度变化），口径错位则是几十个百分点：issue #14 的 4508 vs 3220
	// 相对较大值是 28.6%——按较小值算会得到 40%，方向反了就会漏判，故这里
	// 明确用大值做分母并留 20% 的门槛，兼顾「不漏判口径错位」和「不误动正常机」。
	designMismatchPct = 20
	// designMaxImpliedHealth 是「采纳某个设计容量后允许的实测/设计比」上限。
	// 电池不可能充进比设计容量多一成的电（少量出厂冗余除外），所以设备树值
	// 若比实测满充容量还小，说明它不是本机的设计容量，不能采纳。
	designMaxImpliedHealth = 1.1

	// 设计容量来源取值（进事件 detail 与 diag，不直接面向用户）。
	designSrcSysfs   = "sysfs"
	designSrcDTB     = "dtb"
	designSrcManual  = "manual"
	designSrcMissing = "missing"

	// kvDesignDtbUAh 设备树容量缓存（µAh）：设备树在开机后不会变，缓存后
	// WebUI 的 15s/60s 轮询不再跑 find。
	kvDesignDtbUAh = "design_dtb_ua"
	// kvDesignDtbTs 上次设备树探测时刻：未找到时按 designDtbRetrySecs 重试，
	// 避免每次 json 调用都空跑一遍 find。
	kvDesignDtbTs      = "design_dtb_ts"
	designDtbRetrySecs = 86400
	// kvDesignLog 上次落事件的裁决状态：json/once 每次启动都会重新裁决，
	// 不比对就会把 events 表写满。
	kvDesignLog = "design_log"
)

// designDtbProbe 读设备树容量（µAh）；可注入供测试替换。
var designDtbProbe = readDTBatteryCapacity

// DesignChoice 是一次设计容量裁决的结果。UAh 为最终采用值，Source 为来源
// （sysfs/dtb/manual/missing），SysfsUAh/DtbUAh/AnchorUAh 保留原始读数供事件
// 与诊断，Mismatch 表示两侧口径分歧已触发裁决，Note 是附加说明。
type DesignChoice struct {
	UAh       int64
	Source    string
	SysfsUAh  int64
	DtbUAh    int64
	AnchorUAh int64
	Mismatch  bool
	Note      string
}

// pickDesignUAh 在 sysfs 的 charge_full_design 与设备树容量之间裁决可信设计
// 容量（µAh）。anchorUAh 为实测锚（charge_full 或已采信估算，µAh），≤0 表示
// 没有可用锚。纯函数，便于覆盖全部裁决分支。
func pickDesignUAh(sysfsUAh, dtbUAh, anchorUAh int64) DesignChoice {
	r := DesignChoice{SysfsUAh: sysfsUAh, DtbUAh: dtbUAh, AnchorUAh: anchorUAh}
	switch {
	case sysfsUAh <= 0 && dtbUAh <= 0:
		r.Source = designSrcMissing
		return r
	case sysfsUAh <= 0:
		// 内核没把设计容量暴露到 sysfs（vivo/iQOO 常见）：设备树是唯一来源。
		r.UAh, r.Source = dtbUAh, designSrcDTB
		return r
	case dtbUAh <= 0:
		r.UAh, r.Source = sysfsUAh, designSrcSysfs
		return r
	}
	r.Mismatch = designMismatch(sysfsUAh, dtbUAh)
	if !r.Mismatch {
		// 两侧一致：信 ABI 值，保证既有设备行为完全不变。
		r.UAh, r.Source = sysfsUAh, designSrcSysfs
		return r
	}
	if anchorUAh <= 0 {
		// 口径分歧但没有实测锚：无法定向，保持 sysfs，只把分歧留给诊断。
		r.UAh, r.Source = sysfsUAh, designSrcSysfs
		return r
	}
	// 按与实测满充容量的相对偏差取更接近的一侧；偏差相同（对称情形）时
	// 取 sysfs，保持「自动裁决尽量不动 ABI 值」的倾向。
	if designDeviation(dtbUAh, anchorUAh) < designDeviation(sysfsUAh, anchorUAh) {
		// 采纳设备树值前先过物理门控：设计容量不能小于实测满充容量太多，
		// 否则会把一块正常的电池算成 >110% 健康度（设备树里是别的容量）。
		if float64(anchorUAh) <= float64(dtbUAh)*designMaxImpliedHealth {
			r.UAh, r.Source = dtbUAh, designSrcDTB
			return r
		}
	}
	r.UAh, r.Source = sysfsUAh, designSrcSysfs
	return r
}

// designMismatch 判两个设计容量是否分歧过大（相对较大值超过阈值）。
func designMismatch(a, b int64) bool {
	lo, hi := a, b
	if lo > hi {
		lo, hi = hi, lo
	}
	if hi <= 0 {
		return false
	}
	return (hi-lo)*100 > hi*designMismatchPct
}

// designDeviation 返回候选值相对实测锚的偏差比例；anchorUAh ≤0 时返回 +Inf
// （由调用方先行排除）。
func designDeviation(candidateUAh, anchorUAh int64) float64 {
	if anchorUAh <= 0 {
		return math.Inf(1)
	}
	return math.Abs(float64(candidateUAh-anchorUAh)) / float64(anchorUAh)
}

// readDesignOverrideUAh 读 <dataDir>/design_mah（mAh）并折算 µAh。文件不存在
// 返回 (0, nil)；存在但内容非法/超范围时返回带上下文的错误，由调用方按
// 「不覆盖」降级，并把问题留给 diag。
func readDesignOverrideUAh(dataDir string) (int64, error) {
	data, err := os.ReadFile(filepath.Join(dataDir, designOverrideFile))
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, fmt.Errorf("读取 %s 失败：%w", designOverrideFile, err)
	}
	text := strings.TrimSpace(string(data))
	if text == "" {
		return 0, nil
	}
	mah, err := strconv.ParseInt(text, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("%s=%q 不是整数：%w", designOverrideFile, text, err)
	}
	if mah < designOverrideMinMah || mah > designOverrideMaxMah {
		return 0, fmt.Errorf("%s=%d 超出合理范围 [%d, %d] mAh",
			designOverrideFile, mah, designOverrideMinMah, designOverrideMaxMah)
	}
	return mah * 1000, nil
}

// cachedDTBatteryCapacity 读设备树容量，结果按 kv 缓存一次。未找到时按
// designDtbRetrySecs 节流重试（节点缺失通常是机型不支持，不必每次轮询都跑
// find）；找到后永久使用，不需要重试。now 为 Unix 秒，供测试注入。
func cachedDTBatteryCapacity(st KVStore, now int64) int64 {
	if v := kvInt(st, kvDesignDtbUAh); v > 0 {
		return v
	}
	if ts := kvInt(st, kvDesignDtbTs); ts > 0 && now-ts < designDtbRetrySecs {
		return 0
	}
	// 先落探测时刻再跑 find：并发/崩溃时最多漏读一天，不会反复空转。
	_ = st.KVSet(kvDesignDtbTs, strconv.FormatInt(now, 10))
	v, err := designDtbProbe()
	if err != nil || v <= 0 {
		return 0
	}
	// 缓存失败只影响下次多跑一遍 find，本次仍用读到的值。
	_ = st.KVSet(kvDesignDtbUAh, strconv.FormatInt(v, 10))
	return v
}

// resolveDesignCapacity 组装可信设计容量：手动覆盖 > sysfs/设备树裁决。
// 所有子步骤失败都只降级，不阻断启动（设计容量缺失时描述与估算按可用字段
// 降级，见 newApp 注释）。
func resolveDesignCapacity(st *Store, dataDir string, sysfsUAh, anchorUAh, now int64) DesignChoice {
	overrideUAh, oerr := readDesignOverrideUAh(dataDir)
	if overrideUAh > 0 {
		return DesignChoice{
			UAh:       overrideUAh,
			Source:    designSrcManual,
			SysfsUAh:  sysfsUAh,
			AnchorUAh: anchorUAh,
			Note:      "override",
		}
	}
	r := pickDesignUAh(sysfsUAh, cachedDTBatteryCapacity(st, now), anchorUAh)
	if oerr != nil {
		r.Note = "override_bad" // 覆盖文件写错：不覆盖，但要让 diag 能看出来
	}
	return r
}

// logDesignChoice 把裁决结果落一次事件：只在结果变化时写，避免 json/once
// 轮询重复刷 events。写库失败静默（与其它诊断路径一致）。
func logDesignChoice(st *Store, r DesignChoice) {
	key := fmt.Sprintf("%s|%d|%d|%d|%t|%s",
		r.Source, r.UAh, r.SysfsUAh, r.DtbUAh, r.Mismatch, r.Note)
	if prev, ok := st.KVGet(kvDesignLog); ok && prev == key {
		return
	}
	if err := st.KVSet(kvDesignLog, key); err != nil {
		return // 记不下状态就不写事件：否则每次启动都会重复写
	}
	if r.UAh <= 0 {
		_ = st.InsertEvent("design_missing", "charge_full_design 与设备树均无有效值，实测估算停用")
		return
	}
	_ = st.InsertEvent("design_pick", r.eventDetail())
}

// eventDetail 是 design_pick 事件的排障明细（中文说明 + 原值，单位 µAh）。
func (r DesignChoice) eventDetail() string {
	var b strings.Builder
	fmt.Fprintf(&b, "设计容量裁决 采纳=%dµAh", r.UAh)
	if r.Source != "" {
		fmt.Fprintf(&b, " 来源=%s", r.Source)
	}
	if r.SysfsUAh > 0 {
		fmt.Fprintf(&b, " sysfs=%dµAh", r.SysfsUAh)
	}
	if r.DtbUAh > 0 {
		fmt.Fprintf(&b, " dtb=%dµAh", r.DtbUAh)
	}
	if r.AnchorUAh > 0 {
		fmt.Fprintf(&b, " 实测锚=%dµAh", r.AnchorUAh)
	}
	if r.Mismatch {
		b.WriteString(" 口径分歧=1")
	}
	if r.Note != "" {
		fmt.Fprintf(&b, " 备注=%s", r.Note)
	}
	return b.String()
}

// DiagNote 返回 diag 用的裁决摘要（mAh）。完全常规的情形（sysfs 值、无分歧、
// 无备注）返回空串，让绝大多数设备的 diag 输出保持原样；零值（未裁决）也省略。
func (r DesignChoice) DiagNote() string {
	if r.Source == "" || (r.Source == designSrcSysfs && !r.Mismatch && r.Note == "") {
		return ""
	}
	var b strings.Builder
	fmt.Fprintf(&b, "src=%s", r.Source)
	if r.Source == designSrcMissing {
		return b.String()
	}
	fmt.Fprintf(&b, " design=%dmAh", r.UAh/1000)
	if r.Mismatch {
		b.WriteString(" mismatch=1")
	}
	if r.SysfsUAh > 0 {
		fmt.Fprintf(&b, " sysfs=%dmAh", r.SysfsUAh/1000)
	}
	if r.DtbUAh > 0 {
		fmt.Fprintf(&b, " dtb=%dmAh", r.DtbUAh/1000)
	}
	if r.AnchorUAh > 0 {
		fmt.Fprintf(&b, " anchor=%dmAh", r.AnchorUAh/1000)
	}
	if r.Note != "" {
		fmt.Fprintf(&b, " %s", r.Note)
	}
	return b.String()
}
