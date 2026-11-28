// 守护进程单实例保护：同一模块同时只允许一个 batteryd daemon 写库。
//
// 背景（issue #12）：模块升级后旧 daemon 仍在内存里跑（心跳 stale=1），若管理器
// 或用户又拉起一个新 daemon，两个进程会各自从 kv 恢复同一个充电会话并分别结算，
// 历史里出现完全重复的会话/放电行，samples 与 charged_ua_total 也会成倍累加。
// pipeline 的结算幂等只在单进程内成立（先置 sess_active=0 再落库），挡不住跨进程
// 重复，所以必须在进程入口加锁。
//
// 两层保护：
//  1. <moddir>/data/batteryd.lock 上的独占锁：拿不到锁的进程直接退出，进程崩溃时
//     内核自动释放（linux/Android 用 flock）；
//  2. 旧版本根本没有这把锁，升级后可能还在跑：启动时按 /proc 找出本模块除自身外的
//     batteryd 进程并结束掉，避免新旧两套口径同时写库。
package main

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

// daemonLockFile 是单实例锁文件名（位于 <moddir>/data/）。
const daemonLockFile = "batteryd.lock"

// errDaemonLocked 表示已有 daemon 持有锁；调用方据此安静退出而不是当失败。
var errDaemonLocked = errors.New("已有 batteryd daemon 在运行")

// daemonLock 持有独占锁；Close 时释放（进程崩溃由内核回收）。
type daemonLock struct{ f *os.File }

// acquireDaemonLock 取得守护进程单实例锁。拿不到时返回 errDaemonLocked，其它错误
// 按真实故障上抛（例如无权创建 data 目录）。
func acquireDaemonLock(path string) (*daemonLock, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("创建锁目录失败：%w", err)
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return nil, fmt.Errorf("打开锁文件失败：%w", err)
	}
	if err := lockFileExclusive(f); err != nil {
		_ = f.Close()
		if errors.Is(err, errDaemonLocked) {
			return nil, errDaemonLocked
		}
		return nil, fmt.Errorf("获取守护进程锁失败：%w", err)
	}
	// 锁文件里记下持有者 pid 便于手动排查；写失败不影响锁语义（flock 已生效）。
	_ = f.Truncate(0)
	_, _ = f.Seek(0, io.SeekStart)
	_, _ = fmt.Fprintf(f, "%d\n", os.Getpid())
	return &daemonLock{f: f}, nil
}

// Close 释放锁并关闭文件；重复调用安全（nil 也不 panic）。
func (l *daemonLock) Close() error {
	if l == nil || l.f == nil {
		return nil
	}
	_ = unlockFile(l.f)
	return l.f.Close()
}

// procRoot 是 /proc 根路径，单测可替换成临时目录。测试不要并行改它。
var procRoot = "/proc"

// readProcExeLink 读取 /proc/<pid>/exe 的链接目标；单测注入（Windows 开发机上
// 普通用户没有创建符号链接的权限）。
var readProcExeLink = os.Readlink

// isDaemonCmdline 判断 /proc/<pid>/cmdline 对应的进程是否是守护进程：argv[0] 之后
// 缺省（旧版 service.sh 直接 exec 无参数）或为 "daemon" 才算。json/once/diag/
// history/design 是同一二进制的子命令，comm 与 exe 都与 daemon 相同；启动瞬间
// 如果 WebUI 正在轮询，把读命令当成旧 daemon 杀掉会让该次取数失败/超时。
func isDaemonCmdline(raw []byte) bool {
	args := bytes.Split(bytes.TrimRight(raw, "\x00"), []byte{0})
	if len(args) <= 1 {
		return true
	}
	return string(args[1]) == "daemon"
}

// findOtherDaemonPids 在 procRoot 下找出「除 self 外、同名可执行文件」的 batteryd
// 守护进程：先按 comm 粗筛（避免对每个进程都读 exe 链接），再核对
// /proc/<pid>/exe 是否指向本模块的 bin/batteryd，最后按 cmdline 排除同二进制
// 子命令。旧版本升级后二进制被替换，exe 链接可能带 " (deleted)" 后缀，比较前
// 去掉。读不到的进程直接跳过（进程可能刚退出）。
func findOtherDaemonPids(procRoot, exePath string, self int) []int {
	entries, err := os.ReadDir(procRoot)
	if err != nil {
		return nil
	}
	want := exePath
	if resolved, rerr := filepath.EvalSymlinks(exePath); rerr == nil {
		want = resolved
	}
	want = filepath.Clean(want)
	var pids []int
	for _, e := range entries {
		pid, perr := strconv.Atoi(e.Name())
		if perr != nil || pid == self {
			continue
		}
		dir := filepath.Join(procRoot, e.Name())
		comm, cerr := os.ReadFile(filepath.Join(dir, "comm"))
		if cerr != nil || strings.TrimSpace(string(comm)) != "batteryd" {
			continue
		}
		target, lerr := readProcExeLink(filepath.Join(dir, "exe"))
		if lerr != nil {
			continue
		}
		target = strings.TrimSuffix(target, " (deleted)")
		if filepath.Clean(target) != want {
			continue
		}
		cmdline, mErr := os.ReadFile(filepath.Join(dir, "cmdline"))
		if mErr != nil || !isDaemonCmdline(cmdline) {
			continue
		}
		pids = append(pids, pid)
	}
	sort.Ints(pids)
	return pids
}

// reclaimOps 把回收旧 daemon 依赖的副作用做成可注入的，便于单测验证「先 TERM、
// 等不到退出才 KILL」的顺序。
type reclaimOps struct {
	alive     func(pid int) bool
	terminate func(pid int) error
	kill      func(pid int) error
	sleep     func(d time.Duration)
	attempts  int
	interval  time.Duration
}

// reclaimPids 回收一批旧 daemon 进程：先全部 SIGTERM，轮询等退出；仍活着的再
// SIGKILL。返回实际处理过的 pid（供日志）。
func reclaimPids(pids []int, ops reclaimOps) []int {
	if len(pids) == 0 {
		return nil
	}
	for _, pid := range pids {
		_ = ops.terminate(pid)
	}
	for i := 0; i < ops.attempts; i++ {
		anyAlive := false
		for _, pid := range pids {
			if ops.alive(pid) {
				anyAlive = true
				break
			}
		}
		if !anyAlive {
			return pids
		}
		ops.sleep(ops.interval)
	}
	for _, pid := range pids {
		if ops.alive(pid) {
			_ = ops.kill(pid)
		}
	}
	return pids
}

// reclaimOtherDaemons 在拿到单实例锁之后调用：结束升级前遗留的旧版 daemon。
// 本次进程已在锁内，所以回收不会误伤自己；旧进程里未落盘的一拍状态由 kv 恢复。
func reclaimOtherDaemons(exePath string) []int {
	self := os.Getpid()
	pids := findOtherDaemonPids(procRoot, exePath, self)
	if len(pids) == 0 {
		return nil
	}
	return reclaimPids(pids, reclaimOps{
		alive:     processAlive,
		terminate: terminateProcess,
		kill:      killProcess,
		sleep:     time.Sleep,
		attempts:  6,
		interval:  250 * time.Millisecond,
	})
}

// processAlive 用 /proc/<pid> 是否存在判断进程是否还活着（僵尸进程也算存在，
// 但这里只用于旧 daemon，僵尸不会有 /proc/<pid>/exe 可写，回收逻辑不依赖它）。
func processAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	_, err := os.Stat(filepath.Join(procRoot, strconv.Itoa(pid)))
	return err == nil
}
