package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func fullDesign() Design {
	return Design{DesignMah: 4000, HasDesign: true, FullMah: 3850, HasFull: true, Cycles: 210, HasCycles: true, Pct: 96, HasPct: true}
}

// descJoin 用转义行分隔符拼多行（KernelSU/APatch 读到的是真实换行）。
func descJoin(lines ...string) string {
	return strings.Join(lines, descLineBreakEscaped)
}

// canonicalDesc 去掉对齐补位空格后比较内容；精确补位由专门用例断言。
func canonicalDesc(s string) string {
	return strings.ReplaceAll(s, " ", "")
}

// withDescManager 临时覆盖管理器类型（KernelSU/APatch 走两行，Magisk 走单行）。
func withDescManager(t *testing.T, kind string) {
	t.Helper()
	old := managerKindForDesc
	managerKindForDesc = kind
	t.Cleanup(func() { managerKindForDesc = old })
}

// descRows 把两行描述拆开（按转义 \n）。
func splitDesc(desc string) []string {
	return strings.Split(desc, descLineBreakEscaped)
}

// valueStartEm 返回一格内「标签冒号 + 补位空格」的宽度，即数值起点列。
func valueStartEm(t *testing.T, cell string) int {
	t.Helper()
	idx := strings.Index(cell, ":")
	if idx < 0 {
		t.Fatalf("格内没有标签冒号: %q", cell)
	}
	rest := cell[idx+1:]
	trimmed := strings.TrimLeft(rest, " ")
	spaces := len(rest) - len(trimmed)
	return descTextEm(cell[:idx+1]) + spaces*descEmSpace
}

// absInt 返回整数绝对值（补位残差断言用）。
func absInt(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

func TestBuildDescriptionContent(t *testing.T) {
	estUAh := int64(3_600_000)
	temp := 31.5
	sigma := 15.6
	zero := 0.0

	cases := []struct {
		name   string
		design Design
		snap   Snapshot
		ml     bool
		want   string
	}{
		{
			name:   "无实测：第 2 行回落设计容量",
			design: fullDesign(),
			snap:   Snapshot{},
			want: descJoin(
				"💚 内核 : 96% - 3850 mAh • ♻️ 循环 : 210次",
				"📐 设计 : 4000 mAh",
			),
		},
		{
			name:   "温度",
			design: fullDesign(),
			snap:   Snapshot{TempC: &temp},
			want: descJoin(
				"💚 内核 : 96% - 3850 mAh • ♻️ 循环 : 210次",
				"📐 设计 : 4000 mAh • 🌡️ 温度 : 31.5℃",
			),
		},
		{
			name:   "有实测",
			design: fullDesign(),
			snap:   Snapshot{EstUAh: &estUAh},
			want: descJoin(
				"💚 内核 : 96% - 3850 mAh • ♻️ 循环 : 210次",
				"📏 实测 : 90% - 3600 mAh",
			),
		},
		{
			name:   "只有满充与循环",
			design: Design{FullMah: 3850, HasFull: true, Cycles: 210, HasCycles: true},
			snap:   Snapshot{},
			want:   "🔋 满充 : 3850 mAh • ♻️ 循环 : 210次",
		},
		{
			name:   "有实测但缺设计容量",
			design: Design{FullMah: 3850, HasFull: true, Cycles: 210, HasCycles: true, Pct: 96, HasPct: true},
			snap:   Snapshot{EstUAh: &estUAh},
			want: descJoin(
				"💚 内核 : 96% - 3850 mAh • ♻️ 循环 : 210次",
				"📏 实测 : 3600 mAh",
			),
		},
		{
			name:   "有百分比但无满充容量",
			design: Design{Pct: 96, HasPct: true, Cycles: 210, HasCycles: true},
			snap:   Snapshot{},
			want:   "💚 内核 : 96% • ♻️ 循环 : 210次",
		},
		{
			name:   "只有温度",
			design: Design{},
			snap:   Snapshot{TempC: &temp},
			want:   "🌡️ 温度 : 31.5℃",
		},
		{
			name:   "ML 前缀 + 区间",
			design: fullDesign(),
			snap:   Snapshot{EstUAh: &estUAh, SigmaMah: &sigma},
			ml:     true,
			want: "[ML实验版]" + descJoin(
				"💚 内核 : 96% - 3850 mAh • ♻️ 循环 : 210次",
				"📏 实测 : 90% - 3600±16 mAh",
			),
		},
		{
			name:   "ML 零区间不追加 ±",
			design: fullDesign(),
			snap:   Snapshot{EstUAh: &estUAh, SigmaMah: &zero},
			ml:     true,
			want: "[ML实验版]" + descJoin(
				"💚 内核 : 96% - 3850 mAh • ♻️ 循环 : 210次",
				"📏 实测 : 90% - 3600 mAh",
			),
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			old := channel
			if c.ml {
				channel = "ml"
			} else {
				channel = "stable"
			}
			t.Cleanup(func() { channel = old })
			got, err := BuildDescription(c.design, c.snap)
			if err != nil {
				t.Fatalf("BuildDescription: %v", err)
			}
			if canonicalDesc(got) != canonicalDesc(c.want) {
				t.Fatalf("got %q, want %q", got, c.want)
			}
		})
	}
}

// TestBuildDescriptionExact 锁定精确输出（含标签补位与行尾对齐空格）。
func TestBuildDescriptionExact(t *testing.T) {
	estUAh := int64(3_600_000)
	temp := 31.5
	desc, err := BuildDescription(fullDesign(), Snapshot{EstUAh: &estUAh, TempC: &temp})
	if err != nil {
		t.Fatalf("BuildDescription: %v", err)
	}
	want := descJoin(
		"💚 内核 : 96% - 3850 mAh • ♻️ 循环 : 210次 ",
		"📏 实测 : 90% - 3600 mAh • 🌡️ 温度 : 31.5℃",
	)
	if desc != want {
		t.Fatalf("got %q, want %q", desc, want)
	}
}

// withAvailableEm 临时把可用宽度设为 em（宽度自适应测试用）。
func withAvailableEm(t *testing.T, em int) {
	t.Helper()
	old := descAvailableEmOverride
	descAvailableEmOverride = em
	t.Cleanup(func() { descAvailableEmOverride = old })
}

func TestParseWmSize(t *testing.T) {
	cases := []struct {
		name   string
		in     string
		want   int
		wantOK bool
	}{
		{"仅物理尺寸", "Physical size: 1080x2400", 1080, true},
		{"物理+覆盖尺寸", "Physical size: 1080x2400\nOverride size: 720x1600", 720, true},
		{"空输出", "", 0, false},
		{"异常输出", "error: no display", 0, false},
	}
	for _, c := range cases {
		got, ok := parseWmSize(c.in)
		if got != c.want || ok != c.wantOK {
			t.Fatalf("%s: got (%d,%v), want (%d,%v)", c.name, got, ok, c.want, c.wantOK)
		}
	}
}

func TestParseWmDensity(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want int
	}{
		{"仅物理密度", "Physical density: 440", 440},
		{"物理+覆盖密度", "Physical density: 440\nOverride density: 420", 420},
		{"空输出", "", 0},
	}
	for _, c := range cases {
		if got, _ := parseWmDensity(c.in); got != c.want {
			t.Fatalf("%s: got %d, want %d", c.name, got, c.want)
		}
	}
}

// TestDescAvailableEm 验证按 wm size/density 与 font_scale 折算可用宽度。
func TestDescAvailableEm(t *testing.T) {
	oldWm, oldFs := descWmOutput, descFontScaleSource
	oldOverride := descAvailableEmOverride
	t.Cleanup(func() {
		descWmOutput, descFontScaleSource = oldWm, oldFs
		descAvailableEmOverride = oldOverride
		resetDescAvailableCache()
	})
	descAvailableEmOverride = 0
	descWmOutput = func(args ...string) (string, error) {
		if args[0] == "size" {
			return "Physical size: 1080x2400\nOverride size: 720x1600", nil
		}
		return "Physical density: 440\nOverride density: 420", nil
	}
	descFontScaleSource = func() float64 { return 1.0 }
	resetDescAvailableCache()
	// dp = 720*160/420 ≈ 274.3；可用 em = (274.3-64)/14*1000 ≈ 15020
	if got, want := descAvailableEm(), 15020; got != want {
		t.Fatalf("可用宽度 got %d, want %d", got, want)
	}
	descFontScaleSource = func() float64 { return 1.3 }
	resetDescAvailableCache()
	if got := descAvailableEm(); got >= 15020 || got < 11000 {
		t.Fatalf("字体放大后可用宽度应变小, got %d", got)
	}
}

// TestBuildDescriptionAdaptiveLabels 验证按宽度自适应裁标签：
// 宽屏全标签，中宽去掉循环/温度标签，窄屏只留图标。
func TestBuildDescriptionAdaptiveLabels(t *testing.T) {
	withUILang(t, langEn)
	estUAh := int64(6_300_000)
	temp := 41.1
	d := fullDesign()
	d.Pct, d.FullMah, d.Cycles, d.DesignMah = 92, 6700, 301, 7000
	build := func(t *testing.T) string {
		t.Helper()
		desc, err := BuildDescription(d, Snapshot{EstUAh: &estUAh, TempC: &temp})
		if err != nil {
			t.Fatalf("BuildDescription: %v", err)
		}
		return desc
	}
	// 全标签需要 22488（英文）/18957（中文）
	withAvailableEm(t, 23500)
	if got := build(t); !strings.Contains(got, "Cycles :") || !strings.Contains(got, "Temp :") {
		t.Fatalf("宽屏应保留全部标签: %q", got)
	}
	// 中宽：英文放不下全标签（22488），但放得下紧凑（18690）
	withAvailableEm(t, 21000)
	got := build(t)
	if strings.Contains(got, "Cycles :") || strings.Contains(got, "Temp :") {
		t.Fatalf("中宽应去掉循环/温度标签: %q", got)
	}
	if !strings.Contains(got, "Kernel :") {
		t.Fatalf("中宽应保留内核标签: %q", got)
	}
	// 窄屏：连紧凑都放不下（18690），只留图标
	withAvailableEm(t, 15000)
	got = build(t)
	for _, label := range []string{"Kernel", "Measured", "Cycles", "Temp"} {
		if strings.Contains(got, label) {
			t.Fatalf("窄屏不应保留标签 %q: %q", label, got)
		}
	}
	if !strings.Contains(got, "92% - 6700 mAh") {
		t.Fatalf("窄屏仍应显示数值: %q", got)
	}
}

// TestBuildDescriptionAdaptiveChinese 中文阈值：全标签 18957、紧凑 16219、仅图标 13481。
func TestBuildDescriptionAdaptiveChinese(t *testing.T) {
	withUILang(t, langZh)
	estUAh := int64(6_300_000)
	temp := 41.1
	d := fullDesign()
	d.Pct, d.FullMah, d.Cycles, d.DesignMah = 92, 6700, 301, 7000
	build := func(t *testing.T) string {
		t.Helper()
		desc, err := BuildDescription(d, Snapshot{EstUAh: &estUAh, TempC: &temp})
		if err != nil {
			t.Fatalf("BuildDescription: %v", err)
		}
		return desc
	}
	withAvailableEm(t, 18000)
	got := build(t)
	if strings.Contains(got, "循环 :") || strings.Contains(got, "温度 :") || !strings.Contains(got, "内核 :") {
		t.Fatalf("18k em 应走中文紧凑模式: %q", got)
	}
	withAvailableEm(t, 14000)
	got = build(t)
	if strings.Contains(got, "内核") || strings.Contains(got, "实测") {
		t.Fatalf("14k em 应只留图标: %q", got)
	}
}

func TestBuildDescriptionNoData(t *testing.T) {
	if _, err := BuildDescription(Design{}, Snapshot{}); err == nil {
		t.Fatal("无任何数据时应返回 ErrNoData")
	}
}

// TestBuildDescriptionLineOrder 锁定两行分组：内核侧+循环在第一行，
// 实测侧+温度在第二行，且每行只有一个健康度。
func TestBuildDescriptionLineOrder(t *testing.T) {
	estUAh := int64(3_600_000)
	temp := 32.5
	got, err := BuildDescription(fullDesign(), Snapshot{EstUAh: &estUAh, TempC: &temp})
	if err != nil {
		t.Fatalf("BuildDescription: %v", err)
	}
	lines := splitDesc(got)
	if len(lines) != 2 {
		t.Fatalf("应为两行, got %d: %q", len(lines), got)
	}
	if strings.Count(lines[0], "💚") != 1 || !strings.Contains(lines[0], "♻️") {
		t.Fatalf("第 1 行应为「内核 + 循环」: %q", lines[0])
	}
	if strings.Count(lines[1], "📏") != 1 || !strings.Contains(lines[1], "🌡️") {
		t.Fatalf("第 2 行应为「实测 + 温度」: %q", lines[1])
	}
	if strings.Contains(lines[0], "🌡️") || strings.Contains(lines[1], "♻️") {
		t.Fatalf("辅助指标放错行: %q", got)
	}
}

// TestDescLineBreakIsEscaped 行分隔必须是转义的两个字符：真实换行会写坏
// module.prop 的 key=value 结构。
func TestDescLineBreakIsEscaped(t *testing.T) {
	if descLineBreakEscaped != `\n` {
		t.Fatalf("行分隔应为转义 \\n，当前 %q", descLineBreakEscaped)
	}
	if strings.ContainsAny(descLineBreakEscaped, "\r\n") {
		t.Fatalf("行分隔不得是真实换行: %q", descLineBreakEscaped)
	}
}

// TestBuildDescriptionMagiskSingleLine 验证 Magisk 兜底：不还原 properties 转义，
// 写 \n 会显示字面量，所以拼成单行、用「 · 」连接，且不做对齐补位。
func TestBuildDescriptionMagiskSingleLine(t *testing.T) {
	withDescManager(t, managerMagisk)
	estUAh := int64(3_600_000)
	temp := 31.5
	desc, err := BuildDescription(fullDesign(), Snapshot{EstUAh: &estUAh, TempC: &temp})
	if err != nil {
		t.Fatalf("BuildDescription: %v", err)
	}
	if strings.Contains(desc, descLineBreakEscaped) {
		t.Fatalf("Magisk 下不应出现字面量 \\n: %q", desc)
	}
	if !strings.Contains(desc, " · ") {
		t.Fatalf("Magisk 下两行应用「 · 」连接: %q", desc)
	}
	if strings.ContainsAny(desc, "\r\n") {
		t.Fatalf("描述不得包含真实 CR/LF: %q", desc)
	}
	want := canonicalDesc("💚 内核 : 96% - 3850 mAh • ♻️ 循环 : 210次 · 📏 实测 : 90% - 3600 mAh • 🌡️ 温度 : 31.5℃")
	if canonicalDesc(desc) != want {
		t.Fatalf("got %q, want %q", desc, want)
	}
}

// TestDescManagerKindDetection 验证目录探测顺序：KernelSU/APatch 优先于残留的
// Magisk 目录，全部缺失时退化为 unknown（走单行兜底）。
func TestDescManagerKindDetection(t *testing.T) {
	oldRoots := managerRoots
	oldKind := managerKindForDesc
	t.Cleanup(func() { managerRoots = oldRoots; managerKindForDesc = oldKind })
	managerKindForDesc = ""

	root := t.TempDir()
	ksu := filepath.Join(root, "ksu")
	apatch := filepath.Join(root, "apatch")
	magisk := filepath.Join(root, "magisk")
	for _, d := range []string{ksu, apatch, magisk} {
		if err := os.Mkdir(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	managerRoots = []struct {
		kind string
		path string
	}{
		{managerKernelSU, ksu},
		{managerAPatch, apatch},
		{managerMagisk, magisk},
	}
	if got := descManagerKind(); got != managerKernelSU {
		t.Fatalf("三个目录都在时应优先 KernelSU, got %q", got)
	}
	if err := os.Remove(ksu); err != nil {
		t.Fatal(err)
	}
	if got := descManagerKind(); got != managerAPatch {
		t.Fatalf("没有 ksu 时应识别 APatch, got %q", got)
	}
	if err := os.Remove(apatch); err != nil {
		t.Fatal(err)
	}
	if got := descManagerKind(); got != managerMagisk {
		t.Fatalf("只剩 magisk 时应识别 Magisk, got %q", got)
	}
	if err := os.Remove(magisk); err != nil {
		t.Fatal(err)
	}
	if got := descManagerKind(); got != managerUnknown {
		t.Fatalf("都没有时应为 unknown, got %q", got)
	}
}

// TestBuildDescriptionAlignedColumns 锁定对齐：同一列的标签补齐后数值同列，
// 第一格补齐后「 • 」同列，两行总宽一致。
func TestBuildDescriptionAlignedColumns(t *testing.T) {
	d := fullDesign()
	d.Pct = 9 // 一位数百分比放大差异
	estUAh := int64(3_600_000)
	temp := 31.5
	desc, err := BuildDescription(d, Snapshot{EstUAh: &estUAh, TempC: &temp})
	if err != nil {
		t.Fatalf("BuildDescription: %v", err)
	}
	rows := splitDesc(desc)
	if len(rows) != 2 {
		t.Fatalf("应为两行: %q", desc)
	}
	split := func(row string) []string {
		parts := strings.Split(row, " • ")
		if len(parts) != 2 {
			t.Fatalf("行应有 2 格: %q", row)
		}
		return parts
	}
	r0, r1 := split(rows[0]), split(rows[1])

	if a, b := valueStartEm(t, r0[0]), valueStartEm(t, r1[0]); a != b {
		t.Fatalf("第 1 列数值起点应一致: %d vs %d (%q / %q)", a, b, r0[0], r1[0])
	}
	if a, b := valueStartEm(t, r0[1]), valueStartEm(t, r1[1]); a != b {
		t.Fatalf("第 2 列数值起点应一致: %d vs %d (%q / %q)", a, b, r0[1], r1[1])
	}
	// 补位只能以空格（约 1/4 em）为单位，允许少于一个空格的舍入残差
	if a, b := descTextEm(rows[0]), descTextEm(rows[1]); absInt(a-b) >= 2*descEmSpace {
		t.Fatalf("两行总宽应基本一致: %d vs %d (%q / %q)", a, b, rows[0], rows[1])
	}
}

// TestDescTextEm 校验字宽口径（em 千分位）。
func TestDescTextEm(t *testing.T) {
	cases := []struct {
		s    string
		want int
	}{
		{"内核", 2036},
		{"96%", (562+18)*2 + (732 + 18)},
		{"41.1℃", (562+18)*2 + (263 + 18) + (562 + 18) + (1000 + 18)},
		{"💚", 1018},
		{" ", 266},
	}
	for _, c := range cases {
		if got := descTextEm(c.s); got != c.want {
			t.Fatalf("descTextEm(%q) = %d, want %d", c.s, got, c.want)
		}
	}
	if got := descTextEm("Kernel"); got != 2928 {
		t.Fatalf("Kernel 标签宽度（含字距）应为 2928, got %d", got)
	}
}

// TestBuildDescriptionNoRealNewline 真实换行会让 module.prop 的后续行变成非法键。
func TestBuildDescriptionNoRealNewline(t *testing.T) {
	estUAh := int64(3_600_000)
	temp := 31.5
	desc, err := BuildDescription(fullDesign(), Snapshot{EstUAh: &estUAh, TempC: &temp})
	if err != nil {
		t.Fatalf("BuildDescription: %v", err)
	}
	if strings.ContainsAny(desc, "\r\n") {
		t.Fatalf("描述不得包含真实 CR/LF: %q", desc)
	}
}

func writeTestProp(t *testing.T, content string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "module.prop")
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatalf("写测试文件: %v", err)
	}
	return p
}

func readTestProp(t *testing.T, p string) string {
	t.Helper()
	out, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("读回文件: %v", err)
	}
	return string(out)
}

func TestWriteModulePropReplacesExistingLine(t *testing.T) {
	p := writeTestProp(t, "id=Charging_Record\nname=Charging Record\nversion=1.1\ndescription=旧的描述\nauthor=f1owkang\n")
	if err := WriteModuleProp(p, "新的描述"); err != nil {
		t.Fatalf("WriteModuleProp: %v", err)
	}
	want := "id=Charging_Record\nname=Charging Record\nversion=1.1\ndescription=新的描述\nauthor=f1owkang\n"
	if got := readTestProp(t, p); got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
	if _, err := os.Stat(p + ".tmp"); !os.IsNotExist(err) {
		t.Fatalf("临时文件应已被 rename 消费, stat err=%v", err)
	}
}

func TestWriteModulePropAppendsWhenMissing(t *testing.T) {
	p := writeTestProp(t, "id=Charging_Record\nname=Charging Record\nversion=1.1\n")
	if err := WriteModuleProp(p, "新的描述"); err != nil {
		t.Fatalf("WriteModuleProp: %v", err)
	}
	want := "id=Charging_Record\nname=Charging Record\nversion=1.1\ndescription=新的描述\n"
	if got := readTestProp(t, p); got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestWriteModulePropKeepsLFAndLineCount(t *testing.T) {
	p := writeTestProp(t, "a=1\nb=2\ndescription=x\nc=3\n")
	if err := WriteModuleProp(p, "y"); err != nil {
		t.Fatalf("WriteModuleProp: %v", err)
	}
	out, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("读回文件: %v", err)
	}
	if bytes.ContainsRune(out, '\r') {
		t.Fatalf("输出中出现 CR: %q", out)
	}
	if got, want := bytes.Count(out, []byte("\n")), 4; got != want {
		t.Fatalf("换行数 got %d, want %d", got, want)
	}
	if !strings.HasSuffix(string(out), "\n") {
		t.Fatalf("尾随换行丢失: %q", out)
	}
}

// TestWriteModulePropSanitizesRealNewline 防御真实换行写坏 module.prop：
// 换行被替换成空格，文件行数与其它键保持不变。
func TestWriteModulePropSanitizesRealNewline(t *testing.T) {
	p := writeTestProp(t, "id=x\ndescription=旧\nversion=1\n")
	if err := WriteModuleProp(p, "第一行\n第二行\r\n第三行"); err != nil {
		t.Fatalf("WriteModuleProp: %v", err)
	}
	out := readTestProp(t, p)
	if got, want := strings.Count(out, "\n"), 3; got != want {
		t.Fatalf("真实换行不应增加文件行数: got %d, want %d (%q)", got, want, out)
	}
	if want := "id=x\ndescription=第一行 第二行  第三行\nversion=1\n"; out != want {
		t.Fatalf("got %q, want %q", out, want)
	}
}
