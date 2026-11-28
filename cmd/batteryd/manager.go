package main

import "os"

// root 管理器类型。描述排版（module.prop 的换行/字号/字距）与诊断文本共用同一
// 套判定，避免两边各写一份路径表而漂移。
const (
	managerKernelSU = "kernelsu"
	managerAPatch   = "apatch"
	managerMagisk   = "magisk"
	managerUnknown  = "unknown"
)

// managerRoots 是各 root 管理器的数据目录，按优先级排列：KernelSU/APatch 排在
// Magisk 之前，避免残留的 /data/adb/magisk 与原管理器共存时误判。
// APatch 的工作目录是 /data/adb/ap（apd 的 WORKING_DIR，busybox 在
// /data/adb/ap/bin）；/data/adb/apatch 是历史文档误写，仅作兼容回退保留。
var managerRoots = []struct {
	kind string
	path string
}{
	{managerKernelSU, "/data/adb/ksu"},
	{managerAPatch, "/data/adb/ap"},
	{managerAPatch, "/data/adb/apatch"},
	{managerMagisk, "/data/adb/magisk"},
}

// detectManagerKind 按固定数据目录探测 root 管理器；只 stat，不执行命令
// （diag 不读取任何可能涉及个人数据的命令输出）。
func detectManagerKind() string {
	for _, m := range managerRoots {
		if fi, err := os.Stat(m.path); err == nil && fi.IsDir() {
			return m.kind
		}
	}
	return managerUnknown
}

// managerDisplayName 返回诊断文本展示用的管理器名。
func managerDisplayName(kind string) string {
	switch kind {
	case managerKernelSU:
		return "KernelSU"
	case managerAPatch:
		return "APatch"
	case managerMagisk:
		return "Magisk"
	default:
		return "unknown"
	}
}
