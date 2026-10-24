package main

import (
	"math"
	"os"
	"path/filepath"
	"testing"
)

// TestMain 把测试默认界面语言固定为中文：既保护 describe_test.go 等既有中文
// 断言不受开发机/设备真实语言影响，也让「缓存后的默认路径」确定；需要英文的
// 用例一律用 withUILang 显式覆盖（cleanup 会还原成这里的中文）。
func TestMain(m *testing.M) {
	uiLangOverride = langZh
	os.Exit(m.Run())
}

// withUILang 临时覆盖界面语言；cleanup 还原，用例之间互不污染。
func withUILang(t *testing.T, lang string) {
	t.Helper()
	old := uiLangOverride
	uiLangOverride = lang
	t.Cleanup(func() { uiLangOverride = old })
}

// withLocaleSources 替换三个语言探测来源（环境变量/getprop/属性文件），
// cleanup 还原默认真机路径。
func withLocaleSources(t *testing.T, env func() string, prop func(string) (string, bool), file func() (string, bool)) {
	t.Helper()
	oldEnv, oldProp, oldFile := localeEnvSource, localePropSource, localeFileSource
	localeEnvSource, localePropSource, localeFileSource = env, prop, file
	t.Cleanup(func() {
		localeEnvSource, localePropSource, localeFileSource = oldEnv, oldProp, oldFile
	})
}

func TestLangFromLocale(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"zh", langZh},
		{"zh-CN", langZh},
		{"ZH-cn", langZh},
		{"zh_CN", langZh},
		{" zh-Hans-CN ", langZh},
		{"en-US", langEn},
		{"en", langEn},
		{"fr-FR", langEn},
		{"ja-JP", langEn},
		{"und", langEn}, // 非空但非中文：按需求一律归英文
		{"C", langEn},   // 非法/占位 locale 也不崩，归英文
		{"", ""},        // 未探测到
		{"   ", ""},
	}
	for _, c := range cases {
		if got := langFromLocale(c.in); got != c.want {
			t.Errorf("langFromLocale(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestLangFromEnv(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"en", langEn},
		{"EN-us", langEn},
		{"zh", langZh},
		{"zh-CN", langZh},
		{"  en  ", langEn},
		{"", ""},
		{"   ", ""},
		{"fr-FR", ""}, // 非法取值忽略，继续系统探测
	}
	for _, c := range cases {
		if got := langFromEnv(c.in); got != c.want {
			t.Errorf("langFromEnv(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestResolveUILangPriority(t *testing.T) {
	t.Run("环境变量最优先", func(t *testing.T) {
		withLocaleSources(t,
			func() string { return "en-US" },
			func(string) (string, bool) { t.Fatal("环境变量命中后不应再读 getprop"); return "", false },
			func() (string, bool) { t.Fatal("环境变量命中后不应再读文件"); return "", false })
		if got := resolveUILang(); got != langEn {
			t.Fatalf("resolveUILang = %q, want %q", got, langEn)
		}
	})

	t.Run("非法环境变量被忽略", func(t *testing.T) {
		withLocaleSources(t,
			func() string { return "fr-FR" },
			func(prop string) (string, bool) {
				if prop == "persist.sys.locale" {
					return "zh-CN", true
				}
				return "", false
			},
			func() (string, bool) { return "", false })
		if got := resolveUILang(); got != langZh {
			t.Fatalf("非法环境变量应继续系统探测, got %q, want %q", got, langZh)
		}
	})

	t.Run("persist.sys.locale 优先于其它属性", func(t *testing.T) {
		var seen []string
		withLocaleSources(t,
			func() string { return "" },
			func(prop string) (string, bool) {
				seen = append(seen, prop)
				if prop == "persist.sys.locale" {
					return "zh-CN", true
				}
				return "en-US", true // 后续属性即使可读也不该被读取
			},
			func() (string, bool) { t.Fatal("getprop 命中后不应再读文件"); return "", false })
		if got := resolveUILang(); got != langZh {
			t.Fatalf("resolveUILang = %q, want %q", got, langZh)
		}
		if len(seen) != 1 || seen[0] != "persist.sys.locale" {
			t.Fatalf("应只读取第一个属性, got %v", seen)
		}
	})

	t.Run("ro.product.locale 次选", func(t *testing.T) {
		withLocaleSources(t,
			func() string { return "" },
			func(prop string) (string, bool) {
				if prop == "ro.product.locale" {
					return "en-US", true
				}
				return "", false
			},
			func() (string, bool) { t.Fatal("getprop 命中后不应再读文件"); return "", false })
		if got := resolveUILang(); got != langEn {
			t.Fatalf("resolveUILang = %q, want %q", got, langEn)
		}
	})

	t.Run("persist.sys.language 旧机兜底", func(t *testing.T) {
		withLocaleSources(t,
			func() string { return "" },
			func(prop string) (string, bool) {
				if prop == "persist.sys.language" {
					return "zh", true
				}
				return "", false
			},
			func() (string, bool) { return "", false })
		if got := resolveUILang(); got != langZh {
			t.Fatalf("resolveUILang = %q, want %q", got, langZh)
		}
	})

	t.Run("空属性值视为未探测到", func(t *testing.T) {
		withLocaleSources(t,
			func() string { return "" },
			func(prop string) (string, bool) {
				if prop == "persist.sys.locale" {
					return "", true // 属性存在但为空
				}
				if prop == "ro.product.locale" {
					return "en-US", true
				}
				return "", false
			},
			func() (string, bool) { return "", false })
		if got := resolveUILang(); got != langEn {
			t.Fatalf("空属性应顺延, got %q, want %q", got, langEn)
		}
	})

	t.Run("非中文 locale 归英文", func(t *testing.T) {
		withLocaleSources(t,
			func() string { return "" },
			func(prop string) (string, bool) {
				if prop == "persist.sys.locale" {
					return "fr-FR", true
				}
				return "", false
			},
			func() (string, bool) { return "", false })
		if got := resolveUILang(); got != langEn {
			t.Fatalf("resolveUILang = %q, want %q", got, langEn)
		}
	})

	t.Run("属性文件兜底", func(t *testing.T) {
		withLocaleSources(t,
			func() string { return "" },
			func(string) (string, bool) { return "", false },
			func() (string, bool) { return "en-US", true })
		if got := resolveUILang(); got != langEn {
			t.Fatalf("resolveUILang = %q, want %q", got, langEn)
		}
	})

	t.Run("全部来源失败默认中文", func(t *testing.T) {
		withLocaleSources(t,
			func() string { return "" },
			func(string) (string, bool) { return "", false },
			func() (string, bool) { return "", false })
		if got := resolveUILang(); got != langZh {
			t.Fatalf("resolveUILang = %q, want %q", got, langZh)
		}
	})
}

func TestUILangOverrideWins(t *testing.T) {
	resetUILangCache()
	t.Cleanup(resetUILangCache)
	withLocaleSources(t,
		func() string { return "zh" },
		func(string) (string, bool) { return "zh-CN", true },
		func() (string, bool) { return "zh-CN", true })
	withUILang(t, langEn)
	if got := uiLang(); got != langEn {
		t.Fatalf("override 应压过探测结果, got %q, want %q", got, langEn)
	}
	withUILang(t, langZh)
	if got := uiLang(); got != langZh {
		t.Fatalf("override 切换应立即生效, got %q, want %q", got, langZh)
	}
}

// withUILangPrefFile 写入临时语言偏好文件并指向它；cleanup 还原路径与缓存。
func withUILangPrefFile(t *testing.T, content string) string {
	t.Helper()
	old := uiLangPrefFile
	p := filepath.Join(t.TempDir(), "lang")
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatalf("写语言偏好文件: %v", err)
	}
	uiLangPrefFile = p
	t.Cleanup(func() {
		uiLangPrefFile = old
		resetUILangCache()
	})
	return p
}

// TestUILangPrefFileLive 覆盖安装器/WebUI 写入的 data/lang：优先级高于系统
// 语言，且文件改写后无需重启进程就能生效（WebUI 切换语言即可刷新描述）。
// 文件内容非法时回退系统语言。
func TestUILangPrefFileLive(t *testing.T) {
	resetUILangCache()
	withUILang(t, "")
	withLocaleSources(t,
		func() string { return "" },
		func(prop string) (string, bool) {
			if prop == "persist.sys.locale" {
				return "en-US", true
			}
			return "", false
		},
		func() (string, bool) { return "", false })
	p := withUILangPrefFile(t, "zh\n")
	if got := uiLang(); got != langZh {
		t.Fatalf("偏好文件应压过系统语言, got %q, want %q", got, langZh)
	}
	// 模拟 WebUI 切换语言：直接改写文件，下一次调用立即生效
	if err := os.WriteFile(p, []byte("en\n"), 0o644); err != nil {
		t.Fatalf("改写语言偏好文件: %v", err)
	}
	if got := uiLang(); got != langEn {
		t.Fatalf("偏好文件改写后应立即生效, got %q, want %q", got, langEn)
	}
	// 非法内容忽略，回退系统语言 en-US
	if err := os.WriteFile(p, []byte("klingon\n"), 0o644); err != nil {
		t.Fatalf("写非法语言偏好文件: %v", err)
	}
	if got := uiLang(); got != langEn {
		t.Fatalf("非法偏好应回退系统语言, got %q, want %q", got, langEn)
	}
}

// TestUILangEnvBeatsPrefFile 锁定 CRECORD_LANG 调试覆盖仍然最高优先级。
func TestUILangEnvBeatsPrefFile(t *testing.T) {
	resetUILangCache()
	withUILang(t, "")
	withLocaleSources(t,
		func() string { return "en-US" },
		func(string) (string, bool) { return "", false },
		func() (string, bool) { return "", false })
	withUILangPrefFile(t, "zh\n")
	if got := uiLang(); got != langEn {
		t.Fatalf("环境变量应压过偏好文件, got %q, want %q", got, langEn)
	}
}

// TestUILangCachedOnce 锁定：系统语言探测（getprop 3 次 + 属性文件 1 次）只做
// 一次；环境变量与语言偏好文件每次调用都会重新读取（后者要支持 WebUI 实时切换）。
func TestUILangCachedOnce(t *testing.T) {
	resetUILangCache()
	t.Cleanup(resetUILangCache)
	withUILang(t, "") // 清掉 TestMain 的中文覆盖，走真实缓存路径
	var propCalls, fileCalls int
	withLocaleSources(t,
		func() string { return "" },
		func(string) (string, bool) { propCalls++; return "", false },
		func() (string, bool) { fileCalls++; return "", false })
	first := uiLang()
	if first != langZh {
		t.Fatalf("全部来源失败应默认中文, got %q", first)
	}
	if propCalls != 3 || fileCalls != 1 {
		t.Fatalf("首次系统探测应为 3 个属性 + 1 次文件, got prop=%d file=%d", propCalls, fileCalls)
	}
	if got := uiLang(); got != first {
		t.Fatalf("第二次调用返回值应一致, got %q", got)
	}
	if got := uiLang(); got != first {
		t.Fatalf("第三次调用返回值应一致, got %q", got)
	}
	if propCalls != 3 || fileCalls != 1 {
		t.Fatalf("系统语言探测应缓存, got prop=%d file=%d", propCalls, fileCalls)
	}
}

func TestBuildDescriptionEnglish(t *testing.T) {
	t.Run("有实测与设计容量", func(t *testing.T) {
		withUILang(t, langEn)
		estUA := int64(3_600_000)
		got, err := BuildDescription(fullDesign(), Snapshot{EstUA: &estUA})
		if err != nil {
			t.Fatalf("BuildDescription: %v", err)
		}
		want := "Kernel health 96% (3850 mAh) | Measured health 90% (3600 mAh) | Cycles 210"
		if got != want {
			t.Fatalf("got %q, want %q", got, want)
		}
	})

	t.Run("无实测时的中间量", func(t *testing.T) {
		withUILang(t, langEn)
		got, err := BuildDescription(fullDesign(), Snapshot{})
		if err != nil {
			t.Fatalf("BuildDescription: %v", err)
		}
		want := "Kernel health 96% (3850 mAh) | Design 4000 mAh | Cycles 210"
		if got != want {
			t.Fatalf("got %q, want %q", got, want)
		}
	})

	t.Run("温度", func(t *testing.T) {
		withUILang(t, langEn)
		temp := 31.5
		got, err := BuildDescription(fullDesign(), Snapshot{TempC: &temp})
		if err != nil {
			t.Fatalf("BuildDescription: %v", err)
		}
		want := "Kernel health 96% (3850 mAh) | Design 4000 mAh | Cycles 210 | Temp 31.5°C"
		if got != want {
			t.Fatalf("got %q, want %q", got, want)
		}
	})

	t.Run("缺设计容量只报实测", func(t *testing.T) {
		withUILang(t, langEn)
		estUA := int64(3_600_000)
		d := Design{FullMah: 3850, HasFull: true, Cycles: 210, HasCycles: true, Pct: 96, HasPct: true}
		got, err := BuildDescription(d, Snapshot{EstUA: &estUA})
		if err != nil {
			t.Fatalf("BuildDescription: %v", err)
		}
		want := "Kernel health 96% (3850 mAh) | Measured 3600 mAh | Cycles 210"
		if got != want {
			t.Fatalf("got %q, want %q", got, want)
		}
	})

	t.Run("部分字段降级", func(t *testing.T) {
		withUILang(t, langEn)
		d := Design{FullMah: 3850, HasFull: true, Cycles: 210, HasCycles: true}
		got, err := BuildDescription(d, Snapshot{})
		if err != nil {
			t.Fatalf("BuildDescription: %v", err)
		}
		want := "Full charge 3850 mAh | Cycles 210"
		if got != want {
			t.Fatalf("got %q, want %q", got, want)
		}
	})

	t.Run("ML 前缀与区间", func(t *testing.T) {
		withUILang(t, langEn)
		old := channel
		channel = "ml"
		defer func() { channel = old }()
		estUA := int64(3_600_000)
		sigma := 15.6
		got, err := BuildDescription(fullDesign(), Snapshot{EstUA: &estUA, SigmaMah: &sigma})
		if err != nil {
			t.Fatalf("BuildDescription: %v", err)
		}
		want := "[ML experimental] Kernel health 96% (3850 mAh) | Measured health 90% (3600±16 mAh) | Cycles 210"
		if got != want {
			t.Fatalf("got %q, want %q", got, want)
		}
	})

	t.Run("无数据返回 ErrNoData", func(t *testing.T) {
		withUILang(t, langEn)
		if _, err := BuildDescription(Design{}, Snapshot{}); err == nil {
			t.Fatal("无任何数据时应返回 ErrNoData")
		}
	})
}

// TestBuildDescriptionChineseDefault 锁定中文输出字节（英文支持不得改变默认行为）。
func TestBuildDescriptionChineseDefault(t *testing.T) {
	withUILang(t, langZh)
	estUA := int64(3_600_000)
	got, err := BuildDescription(fullDesign(), Snapshot{EstUA: &estUA})
	if err != nil {
		t.Fatalf("BuildDescription: %v", err)
	}
	want := "内核健康 96%（3850 mAh）｜实测健康 90%（3600 mAh）｜循环 210次"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}

	old := channel
	channel = "ml"
	defer func() { channel = old }()
	estUA = int64(3_600_000)
	got, err = BuildDescription(fullDesign(), Snapshot{EstUA: &estUA})
	if err != nil {
		t.Fatalf("BuildDescription: %v", err)
	}
	want = "[ML实验版]内核健康 96%（3850 mAh）｜实测健康 90%（3600 mAh）｜循环 210次"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

// onceFixture 组装一份覆盖所有字段的 once 输出数据。
func onceFixture() (Design, Snapshot, *DischargeRow) {
	estUA := int64(3_600_000)
	sigma := 15.6
	rMoh := 23.4
	tempC := 32.5
	implied := int64(4_500_000)
	d := fullDesign()
	snap := Snapshot{
		EstUA:      &estUA,
		Samples:    5,
		CycleEquiv: 1.25,
		RMoh:       &rMoh,
		TempC:      &tempC,
		SigmaMah:   &sigma,
	}
	dis := &DischargeRow{StartCap: 90, EndCap: 40, Uah: 2_400_000, Implied: &implied}
	return d, snap, dis
}

func TestOnceTextEnglish(t *testing.T) {
	withUILang(t, langEn)
	d, snap, dis := onceFixture()
	got := onceText(d, snap, dis)
	want := "Design capacity: 4000 mAh\n" +
		"Full charge capacity: 3850 mAh\n" +
		"Cycle count: 210\n" +
		"Kernel health: 96%\n" +
		"Measured estimate: 3600 mAh (5 valid sessions)\n" +
		"Estimate uncertainty: ±16 mAh\n" +
		"Cycle equivalent: 1.25\n" +
		"Internal resistance: 23.4 mΩ\n" +
		"Discharge record: 90→40% released 2400 mAh (implied 4500 mAh)\n" +
		"Battery temperature: 32.5°C\n"
	if got != want {
		t.Fatalf("got:\n%s\nwant:\n%s", got, want)
	}

	// 学习期 + 无内阻/温度/放电记录时的降级文案
	learning := onceText(fullDesign(), Snapshot{CycleEquiv: math.NaN()}, nil)
	wantLearning := "Design capacity: 4000 mAh\n" +
		"Full charge capacity: 3850 mAh\n" +
		"Cycle count: 210\n" +
		"Kernel health: 96%\n" +
		"Measured estimate: Learning period\n" +
		"Cycle equivalent: --\n" +
		"Internal resistance: Collecting\n"
	if learning != wantLearning {
		t.Fatalf("got:\n%s\nwant:\n%s", learning, wantLearning)
	}
}

// TestOnceTextEnglishSingularSession 锁定英文 once 单复数：1 次有效会话不能写
// "1 valid sessions"（首装后第一次采信即会遇到）。
func TestOnceTextEnglishSingularSession(t *testing.T) {
	withUILang(t, langEn)
	estUA := int64(3_600_000)
	got := onceText(Design{DesignMah: 4000, HasDesign: true}, Snapshot{EstUA: &estUA, Samples: 1, CycleEquiv: 1.25}, nil)
	want := "Design capacity: 4000 mAh\n" +
		"Measured estimate: 3600 mAh (1 valid session)\n" +
		"Cycle equivalent: 1.25\n" +
		"Internal resistance: Collecting\n"
	if got != want {
		t.Fatalf("got:\n%s\nwant:\n%s", got, want)
	}
}

// TestOnceTextChineseDefault 锁定既有中文 once 输出字节。
func TestOnceTextChineseDefault(t *testing.T) {
	withUILang(t, langZh)
	d, snap, dis := onceFixture()
	got := onceText(d, snap, dis)
	want := "出厂设计容量：4000 mAh\n" +
		"满充容量：3850 mAh\n" +
		"循环次数：210\n" +
		"内核健康：96%\n" +
		"实测估算：3600 mAh（5次有效会话）\n" +
		"估计不确定度：±16 mAh\n" +
		"循环当量：1.25\n" +
		"内阻：23.4 mΩ\n" +
		"放电记录：90→40% 放出 2400 mAh（隐含 4500 mAh）\n" +
		"电池温度：32.5℃\n"
	if got != want {
		t.Fatalf("got:\n%s\nwant:\n%s", got, want)
	}
}
