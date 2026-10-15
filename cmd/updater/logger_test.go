package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestDefaultLogPathIsAbsolute 验证默认日志路径为绝对路径：
// 助手由主程序以任意工作目录启动，相对路径会落到不可预期的位置。
func TestDefaultLogPathIsAbsolute(t *testing.T) {
	path := defaultLogPath()
	if !filepath.IsAbs(path) {
		t.Fatalf("defaultLogPath() = %q, want an absolute path", path)
	}
}

// TestLoggerAppendsWithTimestamp 验证日志追加写入且每行带时间戳。
// 助手可能在任意时刻被系统回收，因此"写了就落盘"是它能被事后排查的前提。
func TestLoggerAppendsWithTimestamp(t *testing.T) {
	path := filepath.Join(t.TempDir(), "updater.log")

	first := newLogger(path)
	first.Printf("第一次")
	first.Close()

	// 第二次打开必须追加而不是截断：否则失败原因会被下一次运行冲掉。
	second := newLogger(path)
	second.Printf("第二次")
	second.Close()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)

	for _, want := range []string{"第一次", "第二次"} {
		if !strings.Contains(content, want) {
			t.Fatalf("日志缺少 %q：\n%s", want, content)
		}
	}
	lines := strings.Split(strings.TrimSpace(content), "\n")
	if len(lines) != 2 {
		t.Fatalf("got %d lines, want 2 (追加写入失败)：\n%s", len(lines), content)
	}
	// 时间戳格式为 RFC3339，行首应当以四位年份开头。
	if !strings.HasPrefix(lines[0], "20") {
		t.Fatalf("日志行缺少时间戳前缀：%q", lines[0])
	}
}

// TestLoggerFallsBackToStderr 验证日志文件不可写时不会中断更新。
// 路径指向一个目录，从而必然打开失败，进而走到 stderr 兜底分支。
func TestLoggerFallsBackToStderr(t *testing.T) {
	l := newLogger(t.TempDir()) // 目录不可作为文件打开
	if l.file != nil {
		t.Fatal("打开一个目录竟然成功了，用例前提不成立")
	}

	// 兜底分支写 stderr，这里只验证它不会 panic。
	l.Printf("兜底输出")
	l.Close()
}
