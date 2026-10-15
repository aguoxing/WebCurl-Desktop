// Command updater 是 webcurl-desktop 的更新助手程序（update helper）。
//
// 主程序无法覆盖正在运行的自身二进制：Windows 会直接拒绝写入；类 Unix 上覆盖
// 运行中的文件虽然可行，却会让进程处于未定义状态。因此更新流程的最后一跳交给本程序：
//
//  1. 等待主进程退出（--pid）
//  2. 把解压好的新文件替换到目标目录（--source → --target）
//  3. 重新拉起应用
//
// 它与主程序放在同一目录，由 internal/updater.Apply 以脱离 stdio 的方式启动：
//
//	updater --source <解压目录> --target <可执行文件目录> --exe <文件名> --pid <PID>
//
// 可选参数：--timeout、--no-restart、--log。退出码 0 表示成功，1 表示替换过程失败，
// 2 表示参数解析失败（沿用 flag 包的惯例）。失败原因追加写入日志文件
// （默认位于系统临时目录）。
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
func main() {
	opts, err := parseOptions(os.Args[1:], os.Stderr)
	switch {
	case errors.Is(err, flag.ErrHelp):
		os.Exit(0)
	case err != nil:
		os.Exit(2)
	}

	log := newLogger(opts.logPath)

	if err := opts.validate(); err != nil {
		log.Printf("参数错误: %v", err)
		log.Close()
		os.Exit(1)
	}

	log.Printf("助手启动: source=%s target=%s exe=%s pid=%d timeout=%s",
		opts.source, opts.target, opts.exe, opts.pid, opts.timeout)

	if err := run(opts, log); err != nil {
		log.Printf("更新失败: %v", err)
		log.Close()
		os.Exit(1)
	}
	log.Printf("更新完成")
	log.Close()
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
