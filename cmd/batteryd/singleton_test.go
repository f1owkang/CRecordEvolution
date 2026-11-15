package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

// 同一路径第二次抢锁必须失败，释放后可重新拿到：这是 daemon 单实例的根基，
// 否则升级后新旧进程仍会同时写库、产生重复会话（issue #12）。
func TestDaemonLockExcludesSecondInstance(t *testing.T) {
	path := filepath.Join(t.TempDir(), "data", daemonLockFile)
	first, err := acquireDaemonLock(path)
	if err != nil {
		t.Fatalf("首次抢锁: %v", err)
	}
	data, rerr := os.ReadFile(path)
	if rerr != nil {
		t.Fatalf("读锁文件: %v", rerr)
	}
	if got := strings.TrimSpace(string(data)); got != strconv.Itoa(os.Getpid()) {
		t.Fatalf("锁文件应记录持有者 pid = %d, got %q", os.Getpid(), got)
	}

	if second, err := acquireDaemonLock(path); !errors.Is(err, errDaemonLocked) {
		if second != nil {
			_ = second.Close()
		}
		t.Fatalf("第二次抢锁 err = %v, want errDaemonLocked", err)
	}

	if err := first.Close(); err != nil {
		t.Fatalf("释放锁: %v", err)
	}
	third, err := acquireDaemonLock(path)
	if err != nil {
		t.Fatalf("释放后应能重新抢到锁: %v", err)
	}
	_ = third.Close()
}

// fakeProc 在临时目录里造一个 /proc 条目：comm 写文本，exe 写成「链接目标」的
// 普通文件（测试注入 readProcExeLink 读取，避开 Windows 建符号链接的权限限制）。
func fakeProc(t *testing.T, root, pid, comm, exeTarget string) {
	t.Helper()
	dir := filepath.Join(root, pid)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(dir, "comm"), comm+"\n")
	if exeTarget != "" {
		writeFile(t, filepath.Join(dir, "exe"), exeTarget)
	}
}

// 只回收「同名可执行文件」的进程：别的 batteryd、同名但别的 exe、自身都要排除；
// 旧版二进制被替换后 exe 链接带 " (deleted)" 后缀，仍须识别。
func TestFindOtherDaemonPids(t *testing.T) {
	old := readProcExeLink
	readProcExeLink = func(path string) (string, error) {
		data, err := os.ReadFile(path)
		if err != nil {
			return "", err
		}
		return strings.TrimSpace(string(data)), nil
	}
	t.Cleanup(func() { readProcExeLink = old })

	root := t.TempDir()
	exe := filepath.Join(t.TempDir(), "bin", "batteryd")
	writeFile(t, exe, "x")
	other := filepath.Join(t.TempDir(), "bin", "batteryd")
	writeFile(t, other, "x")
	self := os.Getpid()

	fakeProc(t, root, "101", "batteryd", exe)
	fakeProc(t, root, "102", "batteryd", other)
	fakeProc(t, root, "103", "sh", exe)
	fakeProc(t, root, "104", "batteryd", exe+" (deleted)")
	fakeProc(t, root, strconv.Itoa(self), "batteryd", exe)
	fakeProc(t, root, "105", "batteryd", "") // exe 读不到：跳过
	if err := os.Mkdir(filepath.Join(root, "notapid"), 0o755); err != nil {
		t.Fatal(err)
	}

	got := findOtherDaemonPids(root, exe, self)
	want := []int{101, 104}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("findOtherDaemonPids = %v, want %v", got, want)
	}
	if findOtherDaemonPids(filepath.Join(root, "missing"), exe, self) != nil {
		t.Fatal("proc 根不存在时应返回 nil")
	}
}

// 回收顺序：先全部 SIGTERM，等不到退出才对仍活着的进程 SIGKILL；
// 已经退出的进程不能被补刀。
func TestReclaimPidsTerminatesThenKillsStuck(t *testing.T) {
	alive := map[int]bool{7: true, 8: false}
	var events []string
	ops := reclaimOps{
		alive:     func(pid int) bool { return alive[pid] },
		terminate: func(pid int) error { events = append(events, fmt.Sprintf("term:%d", pid)); return nil },
		kill:      func(pid int) error { events = append(events, fmt.Sprintf("kill:%d", pid)); return nil },
		sleep:     func(time.Duration) { events = append(events, "sleep") },
		attempts:  3,
		interval:  time.Millisecond,
	}
	got := reclaimPids([]int{7, 8}, ops)
	if fmt.Sprint(got) != "[7 8]" {
		t.Fatalf("reclaimPids = %v, want [7 8]", got)
	}
	want := []string{"term:7", "term:8", "sleep", "sleep", "sleep", "kill:7"}
	if fmt.Sprint(events) != fmt.Sprint(want) {
		t.Fatalf("回收动作 = %v, want %v", events, want)
	}
	if reclaimPids(nil, ops) != nil {
		t.Fatal("空列表应返回 nil")
	}
}
