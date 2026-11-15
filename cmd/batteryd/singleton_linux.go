//go:build linux

package main

import (
	"errors"
	"os"
	"syscall"
)

// lockFileExclusive 用 flock 对锁文件加独占非阻塞锁：进程退出/崩溃时内核自动
// 释放，不会留下需要人工清理的陈旧锁。Android 的 /data 分区（ext4/f2fs）支持。
func lockFileExclusive(f *os.File) error {
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		if errors.Is(err, syscall.EWOULDBLOCK) || errors.Is(err, syscall.EAGAIN) {
			return errDaemonLocked
		}
		return err
	}
	return nil
}

// unlockFile 释放 flock；进程退出时本就会释放，这里只是让正常路径更明确。
func unlockFile(f *os.File) error {
	return syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
}

// terminateProcess 发 SIGTERM：旧 daemon 未注册信号处理器，会立即退出，所在库
// 由 WAL 保证一致（会话状态每拍落 kv）。
func terminateProcess(pid int) error { return syscall.Kill(pid, syscall.SIGTERM) }

// killProcess 对 SIGTERM 无响应的旧 daemon 强制回收。
func killProcess(pid int) error { return syscall.Kill(pid, syscall.SIGKILL) }
