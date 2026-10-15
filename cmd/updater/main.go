// Command updater 是 webcurl-desktop 的更新助手程序（update helper）。
//
// 主程序无法覆盖正在运行的自身二进制：Windows 会直接拒绝写入；类 Unix 上覆盖
// 运行中的文件虽然可行，却会让进程处于未定义状态。因此更新流程的最后一跳交给本程序：
//
//  1. 等待主进程退出（--pid）
//  2. 把解压好的新文件替换到目标目录（--source → --target）
//  3. 重新拉起应用
//
// 它与主程序放在同一目录，由 internal/updater.Apply 启动 —— 后者会先把本程序
// 复制到临时目录再执行，这样安装目录里那一份在替换阶段不是运行中的映像，
// 可以被升级包里附带的新版本覆盖：
//
//	updater --source <解压目录> --target <可执行文件目录> --exe <文件名> --pid <PID>
//
// 可选参数：--timeout、--no-restart、--log。退出码 0 表示成功，1 表示替换过程失败，
// 2 表示参数解析失败（沿用 flag 包的惯例）。失败原因追加写入日志文件
// （默认位于系统临时目录）。
//
// 环境变量 GOWS_UPDATE_CLEANUP 指向主程序托付的工作目录，本程序退出前会删掉它
// （见 cleanupWorkDir）。
//
// 本程序刻意不依赖项目内部任何包：它必须能被单独交叉编译到任意平台，
// 并且要在主程序已经退出之后独立跑完整个替换过程。
package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	// defaultTimeout 是等待主进程退出的默认上限。超过之后不再等待，
	// 而是直接尝试替换 —— 多数情况下仍会成功（类 Unix 上一定成功），
	// 直接放弃只会把用户卡在旧版本上。
	defaultTimeout = 60 * time.Second

	// pollInterval 是轮询主进程状态的间隔。取小值是为了让替换尽早开始，
	// 太大用户会明显感到"更新卡住"。
	pollInterval = 250 * time.Millisecond
)

// options 汇总命令行参数。
type options struct {
	source    string        // 解压后存放新文件的目录
	target    string        // 要替换的文件所在目录
	exe       string        // 要替换的文件名（不含路径）
	pid       int           // 等待其退出的主进程 PID，0 表示不等待
	timeout   time.Duration // 等待主进程退出的最长时间
	noRestart bool          // 替换完成后不重新拉起应用
	logPath   string        // 日志文件路径
}

// validate 校验参数。助手是被程序启动的，参数错误只可能来自调用方改动，
// 因此这里宁可明确报错，也不要带着错误参数去动用户的安装目录。
func (o options) validate() error {
	switch {
	case o.source == "":
		return errors.New("缺少 --source")
	case o.target == "":
		return errors.New("缺少 --target")
	case o.exe == "":
		return errors.New("缺少 --exe")
	}
	if filepath.Base(o.exe) != o.exe {
		return fmt.Errorf("--exe 只能是文件名，不能包含路径: %s", o.exe)
	}
	if o.timeout <= 0 {
		return fmt.Errorf("--timeout 必须为正数，当前为 %s", o.timeout)
	}
	if _, err := os.Stat(o.source); err != nil {
		return fmt.Errorf("--source 不可用: %w", err)
	}
	return nil
}

// main 是入口。退出码约定：0 成功，1 替换过程失败，2 参数解析失败
// （沿用 flag 包 ExitOnError 的惯例，`-h` 仍为 0）。
//
// 各分支都改成经 runMain 返回退出码、而不是各自 os.Exit，这样工作目录的清理
// 能在退出前统一执行一次 —— 成功和失败都必须走到它。
func main() {
	code := runMain(os.Args[1:], os.Stderr)
	cleanupWorkDir()
	os.Exit(code)
}

// runMain 执行一次完整的更新流程，返回进程退出码。
func runMain(args []string, errOut io.Writer) int {
	opts, err := parseOptions(args, errOut)
	switch {
	case errors.Is(err, flag.ErrHelp):
		return 0
	case err != nil:
		return 2
	}

	log := newLogger(opts.logPath)

	if err := opts.validate(); err != nil {
		log.Printf("参数错误: %v", err)
		log.Close()
		return 1
	}

	log.Printf("助手启动: source=%s target=%s exe=%s pid=%d timeout=%s",
		opts.source, opts.target, opts.exe, opts.pid, opts.timeout)

	if err := run(opts, log); err != nil {
		log.Printf("更新失败: %v", err)
		log.Close()
		return 1
	}
	log.Printf("更新完成")
	log.Close()
	return 0
}

// parseOptions 解析命令行参数。
//
// 使用独立的 FlagSet 而不是包级 flag.CommandLine，有两个原因：
//   - 出错时返回 error 而不是直接终止进程，因此参数契约可以被测试覆盖
//     （助手是被程序启动的，参数名写错会让更新在最后一跳静默失败）；
//   - 不污染全局标志集，便于在测试进程内直接调用。
//
// usage 输出到 errOut，方便调用方把用法引导到正确的流。
func parseOptions(args []string, errOut io.Writer) (options, error) {
	fs := flag.NewFlagSet("updater", flag.ContinueOnError)
	fs.SetOutput(errOut)

	var opts options
	fs.StringVar(&opts.source, "source", "", "解压后存放新文件的目录（必填）")
	fs.StringVar(&opts.target, "target", "", "要替换的文件所在目录（必填）")
	fs.StringVar(&opts.exe, "exe", "", "要替换的文件名，不含路径（必填）")
	fs.IntVar(&opts.pid, "pid", 0, "等待其退出的主进程 PID；0 表示不等待")
	fs.DurationVar(&opts.timeout, "timeout", defaultTimeout, "等待主进程退出的最长时间")
	fs.BoolVar(&opts.noRestart, "no-restart", false, "替换完成后不重新拉起应用")
	fs.StringVar(&opts.logPath, "log", defaultLogPath(), "日志文件路径")

	if err := fs.Parse(args); err != nil {
		return options{}, err
	}
	return opts, nil
}

// cleanupEnvVar 是主程序用来托付「本次更新的工作目录」的环境变量名。
//
// 主程序刻意不自己删这个目录：它必须先把控制权交出来并退出，本程序才能开始
// 替换文件，所以只有这里才是安全的删除时机。
//
// 用环境变量而不是新增命令行参数，是为了向前兼容 —— 更新包里的本程序可能与
// 正在运行的主程序不是同一批发布的，而旧版主程序遇到陌生 flag 会让本程序以
// 退出码 2 直接拒绝启动，环境变量则会被安静地忽略。
const cleanupEnvVar = "GOWS_UPDATE_CLEANUP"

// cleanupWorkDir 删除主程序托付的工作目录，尽力而为。
//
// 失败不报错：此时目录里通常只剩正在运行的自己 —— Windows 不允许删除运行中的
// 映像，这是预期内的残留，主程序会在下一次更新前再清一遍
// （见 internal/updater 的 cleanupStaleWorkDirs）。
func cleanupWorkDir() {
	dir := os.Getenv(cleanupEnvVar)
	if dir == "" {
		return
	}

	// 环境变量是进程外部可改写的，因此限定只能删除系统临时目录下的路径：
	// 否则它就成了一个「删除任意目录」的通道。
	cleaned := filepath.Clean(dir)
	tempDir := filepath.Clean(os.TempDir())
	if !filepath.IsAbs(cleaned) ||
		!strings.HasPrefix(strings.ToLower(cleaned), strings.ToLower(tempDir)+string(os.PathSeparator)) {
		return
	}
	_ = os.RemoveAll(cleaned)
}
