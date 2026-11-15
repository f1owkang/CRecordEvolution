//go:build !linux

package main

import (
	"os"
)

// 非 linux 只在开发机（Windows/macOS）上跑单测：标准库没有 flock，用「独占创建
// 旁路文件」模拟互斥，保证本机测试能验证「第二个 daemon 拿不到锁」。正式产物
// 只发布 linux/arm64，走 singleton_linux.go 的 flock。副作用是崩溃后可能残留
// .excl 文件，dev 环境手动删掉即可，不影响设备。
func lockFileExclusive(f *os.File) error {
	excl, err := os.OpenFile(f.Name()+".excl", os.O_CREATE|os.O_EXCL|os.O_RDWR, 0o644)
	if err != nil {
		if os.IsExist(err) {
			return errDaemonLocked
		}
		return err
	}
	_ = excl.Close()
	return nil
}

// unlockFile 删除旁路文件；文件不存在也算释放成功。
func unlockFile(f *os.File) error {
	err := os.Remove(f.Name() + ".excl")
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// terminateProcess/killProcess 在开发机上退化为 os.Process.Kill（没有信号语义，
// 单测通过注入 reclaimOps 覆盖回收顺序，不依赖这两个默认实现）。
func terminateProcess(pid int) error {
	p, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	return p.Kill()
}

func killProcess(pid int) error { return terminateProcess(pid) }
