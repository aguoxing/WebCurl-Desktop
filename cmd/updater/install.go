package main

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

const (
	// replaceAttempts 与 replaceRetryInterval 针对 Windows：主进程刚退出时，
	// 它的映像可能还没被系统完全释放，rename 会短暂失败。重试几次即可，
	// 比"失败就放弃"友好得多。
	replaceAttempts      = 20
	replaceRetryInterval = 250 * time.Millisecond
)

// run 完成"等主进程退出 → 替换文件 → 重新拉起应用"三步。
func run(opts options, log *logger) error {
	targetExe := filepath.Join(opts.target, opts.exe)

	waitForRelease(opts.timeout, func() bool { return processAlive(opts.pid, targetExe) }, time.Sleep, log)

	files, err := collectPayload(opts.source, opts.target)
	if err != nil {
		return err
	}
	// 压缩包里只有一个文件时，它就是要安装的可执行文件：强制落到 --exe 上。
	// 这样即使用户把主程序改过名，升级之后仍然是同一个文件名。
	if len(files) == 1 {
		files[0].dest = targetExe
	}

	for _, f := range files {
		if err := install(f, log); err != nil {
			return err
		}
	}

	if opts.noRestart {
		log.Printf("已跳过重启（--no-restart）")
		return nil
	}
	return relaunch(targetExe, opts.target, log)
}

// waitForRelease 等到主进程不再占用目标可执行文件，或超过 timeout。
//
// alive 与 sleep 通过参数注入，而不是直接调用 processAlive / time.Sleep：
// 这样等待逻辑可以脱离真实进程与真实时钟被测试 —— 前者要杀掉进程才能覆盖，
// 后者会让每个用例都要真等上几百毫秒。
//
// 等待时长按"轮询次数"而非墙上时钟折算：`timeout / pollInterval` 次轮询之后
// 仍未观察不到释放，就记一条超时日志并继续替换（多数情况下仍会成功，
// 直接放弃只会把用户卡在旧版本上）。返回 false 表示是超时退出。
func waitForRelease(timeout time.Duration, alive func() bool, sleep func(time.Duration), log *logger) bool {
	attempts := int(timeout / pollInterval)

	for i := 0; ; i++ {
		if !alive() {
			log.Printf("目标已可替换，开始安装")
			return true
		}
		if i >= attempts {
			log.Printf("等待主进程退出超时（%s），仍继续尝试替换", timeout)
			return false
		}
		sleep(pollInterval)
	}
}

// payloadFile 描述一个待安装的文件。
type payloadFile struct {
	// rel 是相对目标目录的路径。
	rel string
	// src 是解压目录里的绝对路径。
	src string
	// dest 是最终要写入的绝对路径。
	dest string
}

// collectPayload 枚举解压目录下的所有普通文件，并推导出各自的目标路径。
//
// 符号链接、设备文件等一律跳过：发布包里不该出现它们，
// 而跟随链接复制等于把安装目录之外的任意文件搬进来。
func collectPayload(source, target string) ([]payloadFile, error) {
	var files []payloadFile

	err := filepath.WalkDir(source, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.Type().IsRegular() {
			return nil
		}
		rel, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		files = append(files, payloadFile{
			rel:  rel,
			src:  path,
			dest: filepath.Join(target, rel),
		})
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("读取解压目录 %s: %w", source, err)
	}
	if len(files) == 0 {
		return nil, fmt.Errorf("解压目录 %s 中没有可安装的文件", source)
	}
	return files, nil
}

// install 把单个文件安装到目标位置。
//
// 先写同目录下的临时文件再整体改名：同一文件系统内的 rename 是原子的，
// 因此不存在"替换到一半"的中间状态，中途断电也不会留下半个可执行文件。
func install(f payloadFile, log *logger) error {
	log.Printf("安装 %s → %s", f.src, f.dest)

	dir := filepath.Dir(f.dest)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("创建目录 %s: %w", dir, err)
	}

	tmp := f.dest + ".new"
	if err := copyFile(f.src, tmp); err != nil {
		return err
	}
	if err := replaceFile(tmp, f.dest); err != nil {
		// 失败时清掉临时文件，避免它被误当成下一次更新的负载。
		_ = os.Remove(tmp)
		return err
	}
	return nil
}

// copyFile 把 src 复制为 dst，并统一给可执行权限。
// 压缩包记录的文件权限并不可靠，而主程序必然需要可执行位。
func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("打开 %s: %w", src, err)
	}
	defer in.Close()

	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
	if err != nil {
		return fmt.Errorf("创建 %s: %w", dst, err)
	}

	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		return fmt.Errorf("写入 %s: %w", dst, err)
	}
	if err := out.Close(); err != nil {
		return fmt.Errorf("关闭 %s: %w", dst, err)
	}
	// Windows 上只读属性会让后续 rename 失败，显式清掉。
	if err := os.Chmod(dst, 0o755); err != nil {
		return fmt.Errorf("设置权限 %s: %w", dst, err)
	}
	return nil
}

// replaceFile 把 src 改名到 dst，失败时重试若干次。
func replaceFile(src, dst string) error {
	var err error
	for attempt := 0; attempt < replaceAttempts; attempt++ {
		if err = os.Rename(src, dst); err == nil {
			return nil
		}
		time.Sleep(replaceRetryInterval)
	}
	return fmt.Errorf("替换 %s: %w", dst, err)
}

// relaunch 启动替换后的可执行文件。
func relaunch(exePath, workDir string, log *logger) error {
	cmd := exec.Command(exePath)
	// 以安装目录为工作目录，保证相对路径（如 configs/config.yaml）仍然可用。
	cmd.Dir = workDir
	// 标准流保持为空：主程序本来就是以 nil stdio 启动本程序的，
	// 再往下传递只会得到无效句柄。
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("重新拉起 %s: %w", exePath, err)
	}
	log.Printf("已重新拉起 %s（PID %d）", exePath, cmd.Process.Pid)

	// 本程序马上就要退出，Release 让子进程与它彻底脱钩，避免留下僵尸进程。
	return cmd.Process.Release()
}
