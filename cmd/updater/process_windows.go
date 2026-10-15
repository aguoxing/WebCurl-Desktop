//go:build windows

package main

import "os"

// processAlive 判断主进程是否仍然占用目标可执行文件。
//
// Windows 上无法用信号探测进程存活，但这里真正关心的只有一件事：
// 目标文件能不能被替换。正在运行的映像被映射进了进程地址空间，
// 以可写方式打开必然失败（共享冲突）；一旦能打开，就说明可以安全替换了。
// 所以 --pid 在 Windows 上只用于日志，实际依据是文件占用状态。
func processAlive(_ int, targetExe string) bool {
	f, err := os.OpenFile(targetExe, os.O_RDWR, 0)
	if err != nil {
		// 文件不存在说明无需等待；其余错误（正被占用、无权限）一律按"还没好"处理。
		return !os.IsNotExist(err)
	}
	_ = f.Close()
	return false
}
