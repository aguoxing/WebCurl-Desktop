package main

import (
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// logger 把更新过程追加写入日志文件。
//
// 本程序是被主程序以脱离 stdio 的方式启动的，输出没有任何接收方。
// 因此必须自带落盘能力，否则"更新失败了"将完全无从排查。
type logger struct {
	file *os.File
}

// defaultLogPath 返回默认日志路径。选系统临时目录是因为安装目录可能是只读的，
// 这里几乎总是可写。
func defaultLogPath() string {
	return filepath.Join(os.TempDir(), "webcurl-desktop-updater.log")
}

// newLogger 打开（或创建）日志文件。打不开时退化为写 stderr：
// 记不了日志不应该让更新直接失败。
func newLogger(path string) *logger {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return &logger{}
	}
	return &logger{file: f}
}

// Printf 写入一行带时间戳的日志。
// 每次都 Sync —— 助手可能在任意时刻被系统回收，留在缓冲区里的日志等于没写。
func (l *logger) Printf(format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	line := fmt.Sprintf("%s %s\n", time.Now().Format(time.RFC3339), msg)

	if l.file != nil {
		_, _ = l.file.WriteString(line)
		_ = l.file.Sync()
		return
	}
	_, _ = os.Stderr.WriteString(line)
}

// Close 关闭日志文件。
func (l *logger) Close() {
	if l.file != nil {
		_ = l.file.Close()
	}
}
