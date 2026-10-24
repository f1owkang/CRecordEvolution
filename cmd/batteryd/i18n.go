package main

// 界面文案双语（zh/en）：只影响「面向用户」的出口——module.prop 描述行与
// `batteryd once` 输出；内部日志、events 表、stderr 错误一律保持中文。
//
// 默认中文：探测不到语言时回退 zh，保证既有中文用户零行为变化。
// 语言探测顺序（Android 权威源在前的理由见各来源注释）：
//  1. 环境变量 CRECORD_LANG（zh/en，调试用，可带地区后缀）
//  2. getprop persist.sys.locale   —— 当前用户语言（如 en-US），最权威
//  3. getprop ro.product.locale    —— 出厂/系统语言（旧机与部分 ROM）
//  4. getprop persist.sys.language —— 更老的 API（值如 zh/en）
//  5. /data/property/persist.sys.locale 文件直读 —— getprop 不可用时兜底
//  6. 兜底 zh
//
// 探测结果进程内缓存一次：getprop 是 fork+exec，daemon 每次刷新描述都重复探测
// 没有意义。所有读取函数做成可注入的 package var（同 main.go 里
// tzPropSource/tzFileSource 的既有写法），另留 uiLangOverride 供测试/调试覆盖，
// 保证单测不依赖设备、不受执行顺序影响。

import (
	"os"
	"os/exec"
	"strings"
	"sync"
)

// 界面语言取值。
const (
	langZh = "zh"
	langEn = "en"
)

// crLangEnv 调试/覆盖用环境变量；取 zh/en（大小写不敏感，允许 en-US 这类后缀）。
const crLangEnv = "CRECORD_LANG"

// localeProps 系统语言属性按优先级排列；前一个取不到（属性不存在或为空）才看下一个。
var localeProps = []string{
	"persist.sys.locale",
	"ro.product.locale",
	"persist.sys.language",
}

// localeFilePath 持久化属性文件：root 可直接读，覆盖 getprop 被 SELinux 拒绝
// 或属性服务未就绪的场景。
const localeFilePath = "/data/property/persist.sys.locale"

// localeEnvSource / localePropSource / localeFileSource 可注入，供单元测试替换；
// 默认走真机路径。
var (
	localeEnvSource  = func() string { return os.Getenv(crLangEnv) }
	localePropSource = func(prop string) (string, bool) {
		out, err := exec.Command("/system/bin/getprop", prop).Output()
		if err != nil {
			return "", false
		}
		if s := strings.TrimSpace(string(out)); s != "" {
			return s, true
		}
		return "", false
	}
	localeFileSource = func() (string, bool) {
		b, err := os.ReadFile(localeFilePath)
		if err != nil {
			return "", false
		}
		if s := strings.TrimSpace(string(b)); s != "" {
			return s, true
		}
		return "", false
	}
)

// langFromLocale 把 locale 字符串判定为界面语言，纯函数、无 IO、可单测：
//   - 空/全空白 → ""（视为未探测到，调用方继续看下一来源）
//   - 以 zh 开头 → zh（zh、zh-CN、zh_CN、ZH-cn……）
//   - 其余任何非空 locale → en（en-US 以及 fr-FR/ja-JP 等非中文均回退英文）
func langFromLocale(locale string) string {
	s := strings.ToLower(strings.TrimSpace(locale))
	if s == "" {
		return ""
	}
	if strings.HasPrefix(s, "zh") {
		return langZh
	}
	return langEn
}

// langFromEnv 解析 CRECORD_LANG：只接受 zh*/en*，非法取值返回 ""（忽略并继续
// 系统探测，避免一个手误的环境变量把整套文案锁死）。
func langFromEnv(v string) string {
	s := strings.ToLower(strings.TrimSpace(v))
	switch {
	case s == "":
		return ""
	case strings.HasPrefix(s, "zh"):
		return langZh
	case strings.HasPrefix(s, "en"):
		return langEn
	default:
		return ""
	}
}

// resolveUILang 执行一次完整探测（不缓存），返回 zh/en。
// 每次调用都会真实读环境/属性/文件，供单测逐层验证优先级。
func resolveUILang() string {
	if lang := langFromEnv(localeEnvSource()); lang != "" {
		return lang
	}
	if lang := readUILangPref(); lang != "" {
		return lang
	}
	return resolveSystemUILang()
}

// resolveSystemUILang 只按系统语言来源探测：属性（persist.sys.locale →
// ro.product.locale → persist.sys.language）→ 属性文件 → 兜底中文。
// 与语言偏好文件分开，便于 uiLang 只缓存这一层（偏好文件要实时读）。
func resolveSystemUILang() string {
	for _, prop := range localeProps {
		if v, ok := localePropSource(prop); ok {
			if lang := langFromLocale(v); lang != "" {
				return lang
			}
		}
	}
	if v, ok := localeFileSource(); ok {
		if lang := langFromLocale(v); lang != "" {
			return lang
		}
	}
	return langZh
}

// uiLangPrefFile 模块语言偏好文件（安装器与 WebUI 写入，内容 zh/en）。
// 由 newApp 按模块目录设置；空表示不读（单测直接调用描述函数时）。
var uiLangPrefFile string

// setUILangPrefFile 记录语言偏好文件路径；由 newApp 调用。
func setUILangPrefFile(path string) { uiLangPrefFile = path }

// readUILangPref 读语言偏好文件，返回 zh/en；文件不存在或内容非法时返回 ""。
// 每次 uiLang 都重新读：WebUI 里切换语言后无需重启 daemon，下一次刷新描述
// 或 once/json 输出就会使用新语言（一次 os.ReadFile 的成本可忽略）。
func readUILangPref() string {
	if uiLangPrefFile == "" {
		return ""
	}
	b, err := os.ReadFile(uiLangPrefFile)
	if err != nil {
		return ""
	}
	return langFromLocale(string(b))
}

// 系统语言探测结果缓存：sync.Once 保证进程内只探测一次（getprop 是 fork+exec，
// 每次刷新描述都重复探测没有意义）。语言偏好文件与 CRECORD_LANG 不走缓存，
// 以便 WebUI/安装器改完立即生效。
var (
	uiLangOnce   sync.Once
	uiLangCached string
)

// uiLangOverride 非空时强制界面语言（zh/en）。供测试覆盖与调试；优先级高于
// 所有探测来源，因此用例之间互不影响，也无需依赖真实设备语言。
var uiLangOverride string

// uiLang 返回当前界面语言（zh/en）。优先级：
//  1. uiLangOverride（测试/调试）
//  2. CRECORD_LANG 环境变量（调试）
//  3. data/lang 偏好文件（安装时选择 / WebUI 里切换）
//  4. 系统语言探测（缓存）
func uiLang() string {
	if uiLangOverride != "" {
		return uiLangOverride
	}
	if lang := langFromEnv(localeEnvSource()); lang != "" {
		return lang
	}
	if lang := readUILangPref(); lang != "" {
		return lang
	}
	uiLangOnce.Do(func() { uiLangCached = resolveSystemUILang() })
	return uiLangCached
}

// resetUILangCache 清空缓存，仅供测试使用：重置后下一次 uiLang 会重新探测。
// 测试串行执行（本包无 t.Parallel），不存在并发读写。
func resetUILangCache() {
	uiLangOnce = sync.Once{}
	uiLangCached = ""
}

// uiText 按当前界面语言二选一：默认中文。
func uiText(zh, en string) string {
	if uiLang() == langEn {
		return en
	}
	return zh
}

// uiKV 拼「标签：值」：中文用全角冒号，英文用半角冒号 + 空格。
func uiKV(zhLabel, enLabel, value string) string {
	if uiLang() == langEn {
		return enLabel + ": " + value
	}
	return zhLabel + "：" + value
}
