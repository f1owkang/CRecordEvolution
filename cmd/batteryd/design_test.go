package main

import (
	"path/filepath"
	"strings"
	"testing"
)

// 设备树容量探针的替身：记录调用次数，按预设值返回。
// 测试结束后由 defer 还原，避免污染其它用例。
func stubDesignDtbProbe(t *testing.T, value int64, err error) *int {
	t.Helper()
	old := designDtbProbe
	calls := 0
	designDtbProbe = func() (int64, error) {
		calls++
		return value, err
	}
	t.Cleanup(func() { designDtbProbe = old })
	return &calls
}

// issue #14 现场：vivo V2463A 的 charge_full_design 是电池标签容量 4508mAh，
// 设备树给出与 charge_full/charge_counter 同口径的 3220mAh，实测 2825mAh。
// 裁决必须取设备树值，否则健康度被打成 2692/4508≈60%。
func TestPickDesignUAhIssue14(t *testing.T) {
	got := pickDesignUAh(4_508_000, 3_220_000, 2_825_000)
	if !got.Mismatch {
		t.Fatal("4508 vs 3220 应判定为口径分歧")
	}
	if got.Source != designSrcDTB || got.UAh != 3_220_000 {
		t.Fatalf("应采纳设备树 3220000µAh, got source=%s UAh=%d", got.Source, got.UAh)
	}
	// 采纳后健康度应落在合理区间（87%），而不是 62%。
	if pct := healthPct(2_825_000, got.UAh); pct != 87 {
		t.Fatalf("内核健康度应为 87%%, got %d%%", pct)
	}
}

// 分歧阈值必须以较大值为分母：4508 vs 3220 相对较大值是 28.6%，相对较小值
// 是 40%。若按小值算，阈值 20% 依然成立，但阈值取 30% 时会漏判——这正是
// 最容易写反的地方，单独钉住判定口径。
func TestDesignMismatchUsesLargerAsBase(t *testing.T) {
	if !designMismatch(4_508_000, 3_220_000) {
		t.Fatal("4508 vs 3220（28.6% of 4508）应判为分歧")
	}
	if designMismatch(4_508_000, 4_500_000) {
		t.Fatal("4508 vs 4500 是同一口径，不应判分歧")
	}
	// 恰好 20% 不算分歧（严格大于才触发），21% 触发。
	if designMismatch(1_000_000, 800_000) {
		t.Fatal("恰好 20% 不应判分歧")
	}
	if !designMismatch(1_000_000, 790_000) {
		t.Fatal("21% 应判分歧")
	}
}

// 两侧一致时保持 sysfs 值：既有设备的健康度/窗口口径完全不变。
func TestPickDesignUAhAgreementKeepsSysfs(t *testing.T) {
	got := pickDesignUAh(4_508_000, 4_500_000, 3_000_000)
	if got.Mismatch || got.Source != designSrcSysfs || got.UAh != 4_508_000 {
		t.Fatalf("一致时应保持 sysfs 值, got %+v", got)
	}
}

func TestPickDesignUAhSingleSide(t *testing.T) {
	if got := pickDesignUAh(0, 0, 3_000_000); got.Source != designSrcMissing || got.UAh != 0 {
		t.Fatalf("两侧都缺失应报 missing, got %+v", got)
	}
	if got := pickDesignUAh(0, 3_220_000, 2_825_000); got.Source != designSrcDTB || got.UAh != 3_220_000 {
		t.Fatalf("只有设备树值时应采纳它, got %+v", got)
	}
	if got := pickDesignUAh(4_508_000, 0, 2_825_000); got.Source != designSrcSysfs || got.UAh != 4_508_000 {
		t.Fatalf("只有 sysfs 值时应采纳它, got %+v", got)
	}
}

// 口径分歧但没有实测锚时无从定向：保持 sysfs，但把分歧标记出来供 diag 观察。
func TestPickDesignUAhMismatchWithoutAnchor(t *testing.T) {
	got := pickDesignUAh(4_508_000, 3_220_000, 0)
	if !got.Mismatch {
		t.Fatal("应标记口径分歧")
	}
	if got.Source != designSrcSysfs || got.UAh != 4_508_000 {
		t.Fatalf("无锚时应保持 sysfs, got %+v", got)
	}
}

// 有分歧但实测锚更接近 sysfs 时不能盲目切到设备树（反向回归）。
func TestPickDesignUAhAnchorCloserToSysfs(t *testing.T) {
	got := pickDesignUAh(4_508_000, 3_220_000, 4_400_000)
	if got.Source != designSrcSysfs || got.UAh != 4_508_000 {
		t.Fatalf("锚更接近 sysfs 时应保留 sysfs, got %+v", got)
	}
}

// 物理门控：设备树值比实测满充容量还小，采纳会把健康度算成 >110%，
// 说明它不是本机的设计容量（例如是别的电池型号的出厂值），保留 sysfs。
func TestPickDesignUAhRejectsDesignBelowFullCharge(t *testing.T) {
	got := pickDesignUAh(5_000_000, 3_000_000, 3_500_000)
	if got.Source != designSrcSysfs || got.UAh != 5_000_000 {
		t.Fatalf("设计容量低于实测满充容量时应保留 sysfs, got %+v", got)
	}
	if !got.Mismatch {
		t.Fatal("口径分歧仍应标记出来")
	}
}

func TestReadDesignOverrideUAh(t *testing.T) {
	dir := t.TempDir()
	if v, err := readDesignOverrideUAh(dir); v != 0 || err != nil {
		t.Fatalf("文件缺失应返回 (0,nil), got (%d,%v)", v, err)
	}
	cases := []struct {
		content string
		wantUAh int64
		wantErr bool
	}{
		{"3220", 3_220_000, false},
		{" 3220 \r\n", 3_220_000, false},
		{"", 0, false},
		{"abc", 0, true},
		{"0", 0, true},
		{"-3220", 0, true},
		{"99999", 0, true},
	}
	for _, tc := range cases {
		t.Run(strings.ReplaceAll(tc.content, "\n", `\n`), func(t *testing.T) {
			writeFile(t, filepath.Join(dir, designOverrideFile), tc.content)
			v, err := readDesignOverrideUAh(dir)
			if (err != nil) != tc.wantErr {
				t.Fatalf("content=%q: err=%v, wantErr=%v", tc.content, err, tc.wantErr)
			}
			if v != tc.wantUAh {
				t.Fatalf("content=%q: got %d, want %d", tc.content, v, tc.wantUAh)
			}
		})
	}
}

// 设备树容量只探一次：命中后走 kv 缓存，未命中按天节流重试。
func TestCachedDTBatteryCapacity(t *testing.T) {
	st := openTestStore(t, filepath.Join(t.TempDir(), "battery.db"))
	defer st.Close()
	const now = int64(1_800_000_000)

	calls := stubDesignDtbProbe(t, 3_220_000, nil)
	if v := cachedDTBatteryCapacity(st, now); v != 3_220_000 {
		t.Fatalf("首次探测应返回 3220000, got %d", v)
	}
	if v := cachedDTBatteryCapacity(st, now+10); v != 3_220_000 || *calls != 1 {
		t.Fatalf("命中缓存不应再探测, got %d calls=%d", v, *calls)
	}
}

func TestCachedDTBatteryCapacityRetriesAfterMiss(t *testing.T) {
	st := openTestStore(t, filepath.Join(t.TempDir(), "battery.db"))
	defer st.Close()
	const now = int64(1_800_000_000)

	calls := stubDesignDtbProbe(t, 0, nil)
	if v := cachedDTBatteryCapacity(st, now); v != 0 {
		t.Fatalf("未找到时应返回 0, got %d", v)
	}
	if v := cachedDTBatteryCapacity(st, now+3600); v != 0 || *calls != 1 {
		t.Fatalf("重试窗口内应直接返回 0 不再探测, got %d calls=%d", v, *calls)
	}
	// 过了重试间隔再探一次（节点可能后来才就绪/换了机型对应树）。
	cachedDTBatteryCapacity(st, now+designDtbRetrySecs+1)
	if *calls != 2 {
		t.Fatalf("超过重试间隔应再探测一次, calls=%d", *calls)
	}
}

func TestResolveDesignCapacityManualWins(t *testing.T) {
	st := openTestStore(t, filepath.Join(t.TempDir(), "battery.db"))
	defer st.Close()
	dataDir := t.TempDir()
	writeFile(t, filepath.Join(dataDir, designOverrideFile), "3305")

	calls := stubDesignDtbProbe(t, 3_220_000, nil)
	got := resolveDesignCapacity(st, dataDir, 4_508_000, 2_825_000, 1_800_000_000)
	if got.Source != designSrcManual || got.UAh != 3_305_000 {
		t.Fatalf("手动覆盖应优先, got %+v", got)
	}
	if *calls != 0 {
		t.Fatalf("手动覆盖时不应再跑设备树 find, calls=%d", *calls)
	}
}

func TestResolveDesignCapacityBadOverride(t *testing.T) {
	st := openTestStore(t, filepath.Join(t.TempDir(), "battery.db"))
	defer st.Close()
	dataDir := t.TempDir()
	writeFile(t, filepath.Join(dataDir, designOverrideFile), "oops")

	stubDesignDtbProbe(t, 0, nil)
	got := resolveDesignCapacity(st, dataDir, 4_508_000, 2_825_000, 1_800_000_000)
	if got.Source != designSrcSysfs || got.UAh != 4_508_000 {
		t.Fatalf("覆盖文件非法时应退回自动裁决, got %+v", got)
	}
	if got.Note != "override_bad" {
		t.Fatalf("非法覆盖应标记 override_bad, got %q", got.Note)
	}
}

func TestDesignChoiceDiagNote(t *testing.T) {
	// 未裁决的零值不产生 diag 行（测试/极旧库构造的 app 走不到这里）。
	if note := (DesignChoice{}).DiagNote(); note != "" {
		t.Fatalf("零值不应有 design 行, got %q", note)
	}
	// 常规设备不产生额外 diag 行。
	plain := pickDesignUAh(4_000_000, 0, 3_900_000)
	if note := plain.DiagNote(); note != "" {
		t.Fatalf("常规设备不应有 design 行, got %q", note)
	}
	issue := pickDesignUAh(4_508_000, 3_220_000, 2_825_000)
	note := issue.DiagNote()
	for _, want := range []string{"src=dtb", "design=3220mAh", "mismatch=1", "sysfs=4508mAh", "dtb=3220mAh", "anchor=2825mAh"} {
		if !strings.Contains(note, want) {
			t.Fatalf("diag 裁决行缺 %q: %q", want, note)
		}
	}
}

// 端到端口径：裁决出的设计容量就是描述里的健康度分母，也是第 2 行回落的
// 「设计」值（issue #14 用户可见的结果）。
func TestIssue14DescriptionUsesTrustedDesign(t *testing.T) {
	choice := pickDesignUAh(4_508_000, 3_220_000, 2_825_000)
	d := Design{
		DesignMah: choice.UAh / 1000, HasDesign: true,
		FullMah: 2_825, HasFull: true,
		Pct: healthPct(2_825_000, choice.UAh), HasPct: true,
	}
	desc, err := BuildDescription(d, Snapshot{})
	if err != nil {
		t.Fatal(err)
	}
	flat := canonicalDesc(desc)
	if !strings.Contains(flat, "87%") {
		t.Fatalf("健康度应按 3220mAh 口径显示 87%%, got %q", flat)
	}
	if strings.Contains(flat, "62%") || strings.Contains(flat, "60%") {
		t.Fatalf("不应再出现 4508mAh 口径的 62%%/60%%, got %q", flat)
	}
	if !strings.Contains(flat, "3220mAh") {
		t.Fatalf("设计容量应展示裁决后的 3220mAh, got %q", flat)
	}
}

// 事件去重：json/once 每次启动都会重新裁决，结果不变时不能再写事件。
func TestLogDesignChoiceDedupes(t *testing.T) {
	st := openTestStore(t, filepath.Join(t.TempDir(), "battery.db"))
	defer st.Close()
	choice := pickDesignUAh(4_508_000, 3_220_000, 2_825_000)
	for i := 0; i < 3; i++ {
		logDesignChoice(st, choice)
	}
	if n := countRows(t, st, "events"); n != 1 {
		t.Fatalf("裁决结果不变时应只记一条事件, got %d", n)
	}
	// 结果变化（这里换成缺失）应再记一条。
	logDesignChoice(st, DesignChoice{Source: designSrcMissing})
	if n := countRows(t, st, "events"); n != 2 {
		t.Fatalf("裁决结果变化应补记事件, got %d", n)
	}
}
