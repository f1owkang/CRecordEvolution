package main

import (
	"errors"
	"fmt"
	"math"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"unicode"
)

// ErrNoData 表示没有任何可用电池数据，无法组装描述。
var ErrNoData = errors.New("无可用电池数据")

// Design 为内核口径的电池静态信息（容量单位 mAh，Pct 为内核健康百分比）。
type Design struct {
	DesignMah int64
	HasDesign bool
	FullMah   int64
	HasFull   bool
	Cycles    int64
	HasCycles bool
	Pct       int64
	HasPct    bool
}

// Snapshot 为估算出口快照：EstUAh 为实测满容量（µAh），其余为显示层派生量。
type Snapshot struct {
	// Version 为模块版本（"1.3.6(35)"），供 WebUI 关于页展示；读不到时为空。
	Version    string
	EstUAh     *int64
	Samples    int64
	CycleEquiv float64
	RMoh       *float64
	TempC      *float64
	SigmaMah   *float64

	TrendMahPerWeek *float64
	TrendState      string
	TrendSpanDay    *int64
	// WeeksTo80 按显著衰减斜率推算降到设计容量 80% 的周数；趋势不显著、
	// 未采信估算或方向不为衰减时为 nil（JSON 省略，前端不显示）。
	WeeksTo80 *int64

	// Quality 通道互检结果（各路容量口径互相印证），无从互检时为 nil
	Quality *Quality
	// Calib 电流刻度校准诊断（设置页展示），节点全缺失时为 nil
	Calib *CalibInfo
	// SessionsN 为 sessions 表总会话数，供 WebUI 设置页展示；0 表示未知/无会话。
	SessionsN int64
}

// 描述在 module.prop 里只能占一行：真实换行会把后半段写成非法键。两行换行只能
// 靠 properties 转义，而各管理器对转义的支持不同，所以按管理器分流：
//
//   - KernelSU（ksud）与 APatch（apd）都用 java-properties 的 PropertiesIter，
//     "\n" 会被还原成真实 LF；Android 的 PrecomputedText.createMeasuredParagraphs
//     只按 '\n'（LINE_FEED）切段，Minikin 的贪心断行也只按宽度断行，所以只有
//     转义的 \n 能稳定分成两行（U+2028 不切段）。
//   - Magisk 用 line.split("=", 2).map { it.trim() }，不还原转义，写成 \n 会
//     原样显示两个字符，因此退化为单行，用「 · 」连接后交给自然折行。
//
// 注意：这里是反斜杠 + n 两个字符，不是真实换行；真实 CR/LF 由
// WriteModuleProp 的 sanitizeDescription 清洗掉。
const descLineBreakEscaped = `\n`

// 管理器类型：按数据目录探测，测试可用 managerKindForDesc 覆盖。
const (
	managerKernelSU = "kernelsu"
	managerAPatch   = "apatch"
	managerMagisk   = "magisk"
	managerUnknown  = "unknown"
)

// managerKindForDesc 非空时强制管理器类型（测试/调试用）。
var managerKindForDesc string

// managerRoots 是各 root 管理器的数据目录；KernelSU/APatch 排在前面，避免
// 残留的 /data/adb/magisk 与原管理器共存时误判。
var managerRoots = []struct {
	kind string
	path string
}{
	{managerKernelSU, "/data/adb/ksu"},
	{managerAPatch, "/data/adb/apatch"},
	{managerMagisk, "/data/adb/magisk"},
}

// descManagerKind 返回当前 root 管理器类型；未知时按「不能还原转义」处理，
// 走单行兜底，避免把字面量 \n 显示给用户。
func descManagerKind() string {
	if managerKindForDesc != "" {
		return managerKindForDesc
	}
	for _, m := range managerRoots {
		if fi, err := os.Stat(m.path); err == nil && fi.IsDir() {
			return m.kind
		}
	}
	return managerUnknown
}

// descTypography 返回管理器描述正文的字号与字距（Compose Material3 默认）：
// KernelSU/Magisk 用 bodyMedium（14sp，letterSpacing 0.25sp），
// APatch 用 bodySmall（12sp，letterSpacing 0.4sp）。字距会加宽每个字符，
// 英文标签宽度差与补位空格都受影响，所以必须一起算。
func descTypography() (fontSp, lsSp float64) {
	if descManagerKind() == managerAPatch {
		return 12, 0.4
	}
	return 14, 0.25
}

// descLetterEm 返回每个字符的 letterSpacing（em 千分位）。
func descLetterEm() int {
	fontSp, lsSp := descTypography()
	if fontSp <= 0 {
		return 0
	}
	return int(math.Round(lsSp / fontSp * 1000))
}

// descEm* 是对齐补位用的字宽（em 千分位）。Roboto Regular：空格 248、
// 数字 562、% 732；CJK/全角/emoji 按 1000。补位空格数按整套字宽换算，
// 所以同一列的数值起点能在不同数字位数下对齐。
const (
	descEmSpace = 248
	descEmCJK   = 1000
)

// descSpaceEm 返回一个空格的渲染宽度（含 letterSpacing），是补位的单位。
func descSpaceEm() int {
	return descTextEm(" ")
}

// descRuneEm 是 Roboto Regular 下 ASCII 与描述常用符号的字宽（em 千分位）。
// 只影响补位空格数，不追求像素级排版；不在表里的字符按 500 估算。
var descRuneEm = map[rune]int{
	' ': 248, '!': 257, '"': 320, '#': 616, '$': 562, '%': 732, '&': 622,
	'\'': 174, '(': 342, ')': 348, '*': 431, '+': 567, ',': 196, '-': 276,
	'.': 263, '/': 412,
	'0': 562, '1': 562, '2': 562, '3': 562, '4': 562, '5': 562, '6': 562,
	'7': 562, '8': 562, '9': 562,
	':': 242, ';': 211, '<': 508, '=': 549, '>': 522, '?': 472, '@': 898,
	'A': 652, 'B': 623, 'C': 651, 'D': 656, 'E': 568, 'F': 553, 'G': 681,
	'H': 713, 'I': 272, 'J': 552, 'K': 627, 'L': 538, 'M': 873, 'N': 713,
	'O': 688, 'P': 631, 'Q': 688, 'R': 616, 'S': 593, 'T': 597, 'U': 648,
	'V': 636, 'W': 887, 'X': 627, 'Y': 601, 'Z': 599,
	'[': 265, '\\': 410, ']': 265, '^': 418, '_': 451, '`': 309,
	'a': 544, 'b': 561, 'c': 523, 'd': 564, 'e': 530, 'f': 347, 'g': 561,
	'h': 551, 'i': 243, 'j': 239, 'k': 507, 'l': 243, 'm': 876, 'n': 552,
	'o': 570, 'p': 561, 'q': 568, 'r': 338, 's': 516, 't': 327, 'u': 551,
	'v': 484, 'w': 751, 'x': 496, 'y': 473, 'z': 496,
	'{': 338, '|': 244, '}': 338, '~': 680,
	'±': 534, '°': 374, '·': 261, '•': 337,
}

// descTextEm 返回文本宽度（em 千分位）。
func descTextEm(s string) int {
	total := 0
	ls := descLetterEm()
	for _, r := range s {
		switch {
		case r == '\ufe0f' || r == '\u200d' || unicode.Is(unicode.Mn, r):
			// 零宽：变体选择符 / ZWJ / 组合符（不额外占位）
			continue
		case isFullWidth(r):
			total += descEmCJK
		default:
			if em, ok := descRuneEm[r]; ok {
				total += em
			} else {
				total += 500
			}
		}
		total += ls
	}
	return total
}

// descIconEm 是 emoji 图标宽度；Noto Color Emoji 各字形按 1 em 计。
const descIconEm = descEmCJK

// descMetric 是描述里的一格：图标 + 标签 + 数值，如「💚 内核 : 88% - 6700 mAh」。
type descMetric struct {
	icon  string
	label string
	value string
}

// descRow 是一行里的格（1~2 格）。
type descRow []descMetric

// descMetricLabelEm 返回「图标 标签 :」的宽度。
func descMetricLabelEm(m descMetric) int {
	return descIconEm + descSpaceEm() + descTextEm(m.label) + descSpaceEm() + descTextEm(":")
}

// BuildDescription 组装两行描述：第 1 行内核侧（健康度/容量）+ 循环，第 2 行
// 实测侧（实测/设计）+ 温度；缺哪个字段就省略哪个格、哪一行全空就省略哪一行。
//
//	内核健康 = charge_full / charge_full_design（系统电量计读数）
//	实测健康 = 本模块估算容量 / 设计容量
//
// 有实测值时满充/设计等中间量留给 WebUI；没有实测时才回落设计容量。
// 文案按界面语言输出（中文分支保持原字节不变，英文分支见 i18n.go）。
func BuildDescription(d Design, snap Snapshot) (string, error) {
	row1, row2 := descRows(d, snap)
	desc := joinDescRows(row1, row2)
	if desc == "" {
		return "", ErrNoData
	}
	return desc, nil
}

// descRows 把可用数据归到两行：第 1 行内核侧 + 循环，第 2 行实测侧 + 温度。
func descRows(d Design, snap Snapshot) (descRow, descRow) {
	var row1, row2 descRow
	if d.HasPct {
		val := fmt.Sprintf("%d%%", d.Pct)
		if d.HasFull && d.FullMah > 0 {
			val = fmt.Sprintf("%d%% - %d mAh", d.Pct, d.FullMah)
		}
		row1 = append(row1, descMetric{icon: "💚", label: uiText("内核", "Kernel"), value: val})
	} else if snap.EstUAh == nil && d.HasFull {
		// 没有内核健康百分比、也没有实测值时用满充容量兜底；内核健康已带
		// 满充 mAh 时不再重复「满充」。
		row1 = append(row1, descMetric{icon: "🔋", label: uiText("满充", "Full charge"), value: fmt.Sprintf("%d mAh", d.FullMah)})
	}
	if d.HasCycles {
		row1 = append(row1, descMetric{
			icon:  "♻️",
			label: uiText("循环", "Cycles"),
			value: fmt.Sprintf("%d%s", d.Cycles, uiText("次", "")),
		})
	}

	if snap.EstUAh != nil {
		estMah := *snap.EstUAh / 1000
		body := fmt.Sprintf("%d mAh", estMah)
		if channel == "ml" && snap.SigmaMah != nil && *snap.SigmaMah > 0 {
			body = fmt.Sprintf("%d±%d mAh", estMah, int64(math.Round(*snap.SigmaMah)))
		}
		label := uiText("实测", "Actual")
		if d.HasDesign && d.DesignMah > 0 {
			pct := int64(math.Round(float64(estMah) / float64(d.DesignMah) * 100))
			row2 = append(row2, descMetric{icon: "📏", label: label, value: fmt.Sprintf("%d%% - %s", pct, body)})
		} else {
			// 缺设计容量时无法折算百分比，只报实测容量
			row2 = append(row2, descMetric{icon: "📏", label: label, value: body})
		}
	} else if d.HasDesign {
		row2 = append(row2, descMetric{icon: "📐", label: uiText("设计", "Design"), value: fmt.Sprintf("%d mAh", d.DesignMah)})
	}
	if snap.TempC != nil {
		if t := *snap.TempC; !math.IsNaN(t) && !math.IsInf(t, 0) {
			row2 = append(row2, descMetric{
				icon:  "🌡️",
				label: uiText("温度", "Temp"),
				value: uiText(fmt.Sprintf("%.1f℃", t), fmt.Sprintf("%.1f°C", t)),
			})
		}
	}
	return row1, row2
}

// descAvailableEmOverride 非零时强制可用宽度（测试用）。
var descAvailableEmOverride int

// 可用宽度缓存：wm 是 app_process 命令，每次刷新都调太贵；进程内算一次即可
// （改字体缩放后重启 daemon 或跑一次 once）。
var (
	descAvailableMu    sync.Mutex
	descAvailableSet   bool
	descAvailableValue int
)

// resetDescAvailableCache 清空缓存，仅供测试。
func resetDescAvailableCache() {
	descAvailableMu.Lock()
	descAvailableSet = false
	descAvailableMu.Unlock()
}

// descCardPaddingDp 是管理器模块卡正文的左右内边距估算：KernelSU 列表 16+16、
// 卡片 16+16；APatch 量级相近。正文按 14sp（bodyMedium/bodySmall 附近）换算。
const (
	descCardPaddingDp = 64
	descBodySp        = 14
	descFallbackDp    = 360
)

// 测试可替换的设备查询。
var (
	descWmOutput = func(args ...string) (string, error) {
		out, err := exec.Command("/system/bin/wm", args...).Output()
		return string(out), err
	}
	descFontScaleSource = func() float64 {
		out, err := exec.Command("/system/bin/settings", "get", "system", "font_scale").Output()
		if err != nil {
			return 1
		}
		v, err := strconv.ParseFloat(strings.TrimSpace(string(out)), 64)
		if err != nil || v <= 0 {
			return 1
		}
		return v
	}
)

// parseWmSize 从 `wm size` 输出解析宽度像素：优先 Override size。
func parseWmSize(out string) (int, bool) {
	phys, over := 0, 0
	for _, line := range strings.Split(out, "\n") {
		w := 0
		for _, f := range strings.Fields(line) {
			i := strings.Index(f, "x")
			if i <= 0 {
				continue
			}
			if v, err := strconv.Atoi(f[:i]); err == nil && v > 0 {
				w = v
			}
		}
		if w <= 0 {
			continue
		}
		if strings.Contains(strings.ToLower(line), "override") {
			over = w
		} else {
			phys = w
		}
	}
	if over > 0 {
		return over, true
	}
	if phys > 0 {
		return phys, true
	}
	return 0, false
}

// parseWmDensity 从 `wm density` 输出解析 dpi：优先 Override density。
func parseWmDensity(out string) (int, bool) {
	phys, over := 0, 0
	for _, line := range strings.Split(out, "\n") {
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		v, err := strconv.Atoi(fields[len(fields)-1])
		if err != nil || v <= 0 {
			continue
		}
		if strings.Contains(strings.ToLower(line), "override") {
			over = v
		} else {
			phys = v
		}
	}
	if over > 0 {
		return over, true
	}
	if phys > 0 {
		return phys, true
	}
	return 0, false
}

// descScreenDp 返回屏幕逻辑宽度（dp）；读不到时按 360dp 估。
func descScreenDp() float64 {
	size, err := descWmOutput("size")
	if err != nil {
		return descFallbackDp
	}
	w, ok := parseWmSize(size)
	if !ok || w <= 0 {
		return descFallbackDp
	}
	density, err := descWmOutput("density")
	if err != nil {
		return descFallbackDp
	}
	dpi, ok := parseWmDensity(density)
	if !ok || dpi <= 0 {
		return descFallbackDp
	}
	return float64(w) * 160 / float64(dpi)
}

// descAvailableEm 返回管理器描述正文可用的宽度（em 千分位）：
// （逻辑宽度 − 卡片内边距）÷ 字体缩放 ÷ 14sp。字体调大时同一行能放的文字变少，
// 所以要把 font_scale 除进去。
func descAvailableEm() int {
	if descAvailableEmOverride > 0 {
		return descAvailableEmOverride
	}
	descAvailableMu.Lock()
	defer descAvailableMu.Unlock()
	if !descAvailableSet {
		dp := descScreenDp()
		scale := descFontScaleSource()
		if scale <= 0 {
			scale = 1
		}
		sp := (dp - descCardPaddingDp) / scale
		if sp <= 0 {
			sp = descFallbackDp
		}
		fontSp, _ := descTypography()
		if fontSp <= 0 {
			fontSp = descBodySp
		}
		descAvailableValue = int(sp / fontSp * 1000)
		descAvailableSet = true
	}
	return descAvailableValue
}

// descLabelMode 决定描述里保留多少文字标签：宽度不够时逐级裁剪，
// 保证两行仍能放下、不发生折行。
type descLabelMode int

const (
	descLabelsFull    descLabelMode = iota // 全部标签
	descLabelsCompact                      // 只保留第一格（内核/实测/满充/设计）标签
	descLabelsNone                         // 只留图标
)

// joinDescRows 拼出最终描述，并按屏幕可用宽度自适应标签：
// 先试全标签，放不下就去掉第二格标签（循环/温度只留图标），再不行只留图标。
//
// KernelSU/APatch：转义 \n 分两行，并按 Roboto 字宽补位对齐；
// Magisk：不还原 properties 转义，退化成单行「行1 · 行2」且不补位。
func joinDescRows(rows ...descRow) string {
	nonEmpty := make([]descRow, 0, len(rows))
	for _, row := range rows {
		if len(row) > 0 {
			nonEmpty = append(nonEmpty, row)
		}
	}
	if len(nonEmpty) == 0 {
		return ""
	}
	cellSep := uiText(" • ", " • ")
	if kind := descManagerKind(); kind == managerMagisk || kind == managerUnknown {
		texts := make([]string, len(nonEmpty))
		for i, row := range nonEmpty {
			parts := make([]string, len(row))
			for j, m := range row {
				parts[j] = m.icon + " " + m.label + " : " + m.value
			}
			texts[i] = strings.Join(parts, cellSep)
		}
		return strings.Join(texts, uiText(" · ", " · "))
	}
	avail := descAvailableEm()
	for _, mode := range []descLabelMode{descLabelsFull, descLabelsCompact, descLabelsNone} {
		if texts, maxEm := layoutDescRows(nonEmpty, mode); maxEm <= avail {
			return strings.Join(texts, descLineBreakEscaped)
		}
	}
	texts, _ := layoutDescRows(nonEmpty, descLabelsNone)
	return strings.Join(texts, descLineBreakEscaped)
}

// layoutDescRows 按标签模式渲染并对齐，返回各行文本与最大行宽
// （em 千分位，含行尾补位）。补位只加普通空格，不写真实换行。
func layoutDescRows(rows []descRow, mode descLabelMode) ([]string, int) {
	cellSep := uiText(" • ", " • ")
	type renderedCell struct {
		text     string
		em       int
		labelEm  int
		hasLabel bool
	}
	cells := make([][]renderedCell, len(rows))
	maxLabel := make([]int, 2)
	for i, row := range rows {
		for c, m := range row {
			rc := renderedCell{}
			if mode == descLabelsFull || (mode == descLabelsCompact && c == 0) {
				rc.hasLabel = true
				rc.labelEm = descMetricLabelEm(m)
				if rc.labelEm > maxLabel[c] {
					maxLabel[c] = rc.labelEm
				}
			} else {
				rc.labelEm = descIconEm + descSpaceEm()
			}
			cells[i] = append(cells[i], rc)
		}
	}
	for i, row := range rows {
		for c, m := range row {
			rc := &cells[i][c]
			if rc.hasLabel {
				pad := 1 + int(math.Round(float64(maxLabel[c]-rc.labelEm)/float64(descSpaceEm())))
				if pad < 1 {
					pad = 1
				}
				rc.text = m.icon + " " + m.label + " :" + strings.Repeat(" ", pad) + m.value
				rc.em = rc.labelEm + pad*descSpaceEm() + descTextEm(m.value)
			} else {
				rc.text = m.icon + " " + m.value
				rc.em = rc.labelEm + descTextEm(m.value)
			}
		}
	}
	// 第一格补到等宽，让「 • 」与第二格起点同列。
	maxFirst := 0
	for _, row := range cells {
		if len(row) >= 2 && row[0].em > maxFirst {
			maxFirst = row[0].em
		}
	}
	if maxFirst > 0 {
		for _, row := range cells {
			if len(row) < 2 {
				continue
			}
			if add := int(math.Round(float64(maxFirst-row[0].em) / float64(descSpaceEm()))); add > 0 {
				row[0].text += strings.Repeat(" ", add)
				row[0].em += add * descSpaceEm()
			}
		}
	}
	sepEm := descTextEm(cellSep)
	texts := make([]string, len(cells))
	ems := make([]int, len(cells))
	maxTotal := 0
	for i, row := range cells {
		parts := make([]string, len(row))
		em := 0
		for j, c := range row {
			if j > 0 {
				em += sepEm
			}
			parts[j] = c.text
			em += c.em
		}
		texts[i] = strings.Join(parts, cellSep)
		ems[i] = em
		if em > maxTotal {
			maxTotal = em
		}
	}
	for i := range texts {
		if add := int(math.Round(float64(maxTotal-ems[i]) / float64(descSpaceEm()))); add > 0 {
			texts[i] += strings.Repeat(" ", add)
			ems[i] += add * descSpaceEm()
		}
	}
	// ML 实验通道不再往描述前缀加文字：通道标识放在 module.prop 的 version
	// 后缀（X.Y.Z-ML）里，模块描述始终是纯状态卡，和 stable 保持一致。
	return texts, maxTotal
}

// isFullWidth 判断 rune 是否占一个整 em（East Asian Width 的 W/F 与 emoji）。
// 只覆盖描述文案会用到的区间，不引入 golang.org/x/text 依赖。
func isFullWidth(r rune) bool {
	switch {
	case r == 0x2103, // ℃：CJK 字体按全宽渲染
		r >= 0x1100 && r <= 0x115F,   // 谚文字母
		r >= 0x2E80 && r <= 0xA4CF,   // 部首、假名、汉字、彝文、全角标点
		r >= 0xAC00 && r <= 0xD7A3,   // 谚文音节
		r >= 0xF900 && r <= 0xFAFF,   // CJK 兼容汉字
		r >= 0xFE10 && r <= 0xFE19,   // 竖排标点
		r >= 0xFE30 && r <= 0xFE6F,   // CJK 兼容标点
		r >= 0xFF00 && r <= 0xFF60,   // 全角 ASCII/标点
		r >= 0xFFE0 && r <= 0xFFE6,   // 全角符号
		r >= 0x1F000 && r <= 0x1FAFF, // emoji
		r >= 0x2600 && r <= 0x27BF:   // ♻ 等杂项符号（带 VS16 时为彩色 emoji）
		return true
	}
	return false
}

// sanitizeDescription 清掉真实换行：module.prop 是逐行 key=value，真实换行会把
// 后半段写成非法键，管理器的 properties 解析还可能直接报错。行分隔只允许
// 转义 \n（KernelSU/APatch 会还原成真实 LF）。
func sanitizeDescription(s string) string {
	return strings.NewReplacer("\r", " ", "\n", " ").Replace(s)
}

// WriteModuleProp 用临时文件 + rename 原子替换 module.prop 的 description 行。
func WriteModuleProp(propPath, newDescription string) error {
	newDescription = sanitizeDescription(newDescription)
	data, err := os.ReadFile(propPath)
	if err != nil {
		return err
	}
	lines := strings.Split(string(data), "\n")
	found := false
	for i, line := range lines {
		if strings.HasPrefix(line, "description=") {
			lines[i] = "description=" + newDescription
			found = true
		}
	}
	if !found {
		if n := len(lines); n > 0 && lines[n-1] == "" {
			lines = append(lines[:n-1], "description="+newDescription, "")
		} else {
			lines = append(lines, "description="+newDescription)
		}
	}
	tmp := propPath + ".tmp"
	if err := os.WriteFile(tmp, []byte(strings.Join(lines, "\n")), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, propPath)
}
