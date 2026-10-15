//go:build !windows

package main

import (
	"errors"
	"os"
	"syscall"
)

// processAlive 通过信号 0 探测主进程是否存活。
//
// 类 Unix 上替换运行中的文件是被允许的（rename 换的是目录项，旧 inode 会随进程退出
// 而释放），所以这里以进程状态为准：只有主进程真的退出了才开始替换，
// 否则新旧两个实例会同时存在。
func processAlive(pid int, _ string) bool {
	if pid <= 0 {
		// 没给 PID 就无从判断，按"可以继续"处理。
		return false
	}
	p, err := os.FindProcess(pid)
	if err != nil {
		return false
	}

	err = p.Signal(syscall.Signal(0))
	switch {
	case err == nil:
		return true
	case errors.Is(err, os.ErrProcessDone), errors.Is(err, syscall.ESRCH):
		return false
	default:
		// 例如 EPERM：进程存在，但当前用户无权向它发信号。
		return true
	}
}
