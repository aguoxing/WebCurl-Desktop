package logger

import "log/slog"

// KVLogger 以"消息 + 键值对"的形式写日志，供那些按 log/slog 风格调用的组件使用。
//
// 本应用的日志后端是 zerolog，而请求执行器与 Mock 引擎沿用 slog 的调用习惯
// （slog.Info("xxx", "key", value)）。让它们各自 import 一次日志库，
// 就会在同一个进程里出现两套格式、两个输出目标。这个适配器把两者接起来：
// 上层只管按 slog 的风格写，落到哪里由这里决定。
type KVLogger struct {
	l *slog.Logger
}

// NewKVLogger 返回写入应用日志器的 KVLogger。
func NewKVLogger() KVLogger {
	return KVLogger{l: slog.New(NewSlogHandler())}
}

// Slog 返回底层 *slog.Logger，供需要直接传 Logger 的组件使用。
func (k KVLogger) Slog() *slog.Logger { return k.l }

func (k KVLogger) Info(msg string, args ...any)  { k.l.Info(msg, args...) }
func (k KVLogger) Warn(msg string, args ...any)  { k.l.Warn(msg, args...) }
func (k KVLogger) Error(msg string, args ...any) { k.l.Error(msg, args...) }
