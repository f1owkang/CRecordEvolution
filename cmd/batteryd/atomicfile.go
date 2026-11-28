package main

import (
	"os"
	"path/filepath"
	"time"
)

// atomicWriteFile 用同目录唯一临时文件 + rename 原子替换目标文件。
//
// 临时文件名必须唯一（os.CreateTemp）：写者是多进程的——daemon 的状态刷新、
// action 触发的 once、design set 子命令都可能同时写 module.prop / design_mah。
// 固定 "<path>.tmp" 会在并发时互相截断或 rename 走对方文件，表现为「刷新描述
// 失败」误报，极端时序下旧内容还会覆盖新内容。同目录保证 rename 不跨文件系统。
func atomicWriteFile(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	f, err := os.CreateTemp(dir, filepath.Base(path)+".tmp*")
	if err != nil {
		return err
	}
	tmp := f.Name()
	cleanup := func() { _ = os.Remove(tmp) }
	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		cleanup()
		return err
	}
	// CreateTemp 默认 0600：按目标文件权限显式 chmod（module.prop 是 0644）。
	if err := f.Chmod(perm); err != nil {
		_ = f.Close()
		cleanup()
		return err
	}
	if err := f.Close(); err != nil {
		cleanup()
		return err
	}
	// rename 在 Android 上是原子的；Windows 开发机上杀毒/索引偶发持有目标文件
	// 句柄会让 Rename 短暂报 Access denied，做几次短重试（重试幂等且目标不变）。
	var rerr error
	for attempt := 0; attempt < 5; attempt++ {
		if rerr = os.Rename(tmp, path); rerr == nil {
			return nil
		}
		time.Sleep(time.Duration(attempt+1) * 10 * time.Millisecond)
	}
	cleanup()
	return rerr
}
