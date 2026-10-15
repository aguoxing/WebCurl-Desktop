// Package logger 封装应用的日志基础设施。
//
// 内部使用 zerolog，并按大小/份数/天数做文件轮转（lumberjack），可选同时输出到控制台。
// Wails 自身通过 log/slog 记录日志，因此本包还提供了一个 slog → zerolog 的桥接器
// （NewSlogHandler），把 Wails 的日志统一汇入同一个输出通道。
package logger

import (
	"context"
	"io"
	"log/slog"
	"os"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/rs/zerolog/pkgerrors"
	"gopkg.in/natefinch/lumberjack.v2"
)

// Config 是日志初始化的输入参数。数值型开关统一用 0/1 表示布尔值，
// 与配置文件的写法保持一致。
type Config struct {
	Level      string // 日志级别，如 debug/info/warn/error
	File       string // 主日志文件路径，为空则不写文件
	WailsFile  string // Wails 内部日志文件路径，为空则与主日志合并
	MaxSize    int    // 单个日志文件的最大体积（MB）
	MaxBackups int    // 保留的历史文件份数
	MaxAge     int    // 历史文件保留天数
	Compress   int    // 是否压缩历史文件（0/1）
	Console    int    // 是否同时输出到控制台（0/1）
}

// Init 按 cfg 初始化全局日志器。它必须在任何日志调用之前执行一次。
// 若既未配置文件也未开启控制台，则回退到仅控制台输出，避免日志凭空消失。
func Init(cfg Config) {
	zerolog.TimeFieldFormat = time.RFC3339
	zerolog.ErrorStackMarshaler = pkgerrors.MarshalStack

	var writers []io.Writer

	if cfg.File != "" {
		lj := &lumberjack.Logger{
			Filename:   cfg.File,
			MaxSize:    cfg.MaxSize,
			MaxBackups: cfg.MaxBackups,
			MaxAge:     cfg.MaxAge,
			Compress:   cfg.Compress != 0,
		}
		lumberjackFile = lj
		writers = append(writers, lj)
	}

	if cfg.Console != 0 {
		writers = append(writers, zerolog.ConsoleWriter{
			Out:        os.Stderr,
			TimeFormat: time.RFC3339,
		})
	}

	if len(writers) == 0 {
		writers = append(writers, zerolog.ConsoleWriter{
			Out:        os.Stderr,
			TimeFormat: time.RFC3339,
		})
	}

	var writer io.Writer
	if len(writers) == 1 {
		writer = writers[0]
	} else {
		writer = io.MultiWriter(writers...)
	}

	level, err := zerolog.ParseLevel(cfg.Level)
	if err != nil {
		level = zerolog.DebugLevel
	}

	zerolog.SetGlobalLevel(level)
	log.Logger = zerolog.New(writer).With().Timestamp().Caller().Logger()

	// 为 Wails 内部日志单独开一个输出文件。
	if cfg.WailsFile != "" && cfg.WailsFile != cfg.File {
		wailsLj := &lumberjack.Logger{
			Filename:   cfg.WailsFile,
			MaxSize:    cfg.MaxSize,
			MaxBackups: cfg.MaxBackups,
			MaxAge:     cfg.MaxAge,
			Compress:   cfg.Compress != 0,
		}
		wailsLumberjackFile = wailsLj
		wZlog := zerolog.New(wailsLj).With().Timestamp().Logger()
		wailsLogger = &wZlog
	}
}

// 保存轮转文件的句柄，供 Sync 在退出前关闭。
var lumberjackFile io.Closer
var wailsLumberjackFile io.Closer
var wailsLogger *zerolog.Logger

// Sync 关闭文件写入器，冲刷缓冲。通常以 defer 方式在 main 中调用。
func Sync() {
	if lumberjackFile != nil {
		lumberjackFile.Close()
	}
	if wailsLumberjackFile != nil {
		wailsLumberjackFile.Close()
	}
}

// 全局日志事件的便捷入口，等价于 zerolog 的 log.Debug() 等。
func Debug() *zerolog.Event { return log.Debug() }
func Info() *zerolog.Event  { return log.Info() }
func Warn() *zerolog.Event  { return log.Warn() }
func Error() *zerolog.Event { return log.Error() }
func Fatal() *zerolog.Event { return log.Fatal() }
func Panic() *zerolog.Event { return log.Panic() }

// SetLevel 在运行时调整全局日志级别。
func SetLevel(level string) error {
	l, err := zerolog.ParseLevel(level)
	if err != nil {
		return err
	}
	zerolog.SetGlobalLevel(l)
	return nil
}

// --- slog → zerolog 桥接 ---

// wailsSlogHandler 把 slog 记录转发到 zerolog，
// 使 Wails 内部日志与业务日志落在同一个文件、同一套格式里。
type wailsSlogHandler struct {
	logger *zerolog.Logger
	attrs  []slog.Attr
}

// NewSlogHandler 返回可交给 Wails 使用的 slog.Handler。
// 配置了独立的 Wails 日志文件时写入该文件，否则并入主日志。
func NewSlogHandler() slog.Handler {
	l := &log.Logger
	if wailsLogger != nil {
		l = wailsLogger
	}
	return &wailsSlogHandler{logger: l}
}

func (h *wailsSlogHandler) Enabled(_ context.Context, level slog.Level) bool {
	return level >= slog.LevelDebug
}

func (h *wailsSlogHandler) Handle(_ context.Context, r slog.Record) error {
	evt := levelToZerolog(h.logger, r.Level)
	if evt == nil {
		return nil
	}
	r.Attrs(func(a slog.Attr) bool {
		if a.Equal(slog.Attr{}) {
			return true
		}
		evt = addSlogAttr(evt, a)
		return true
	})
	for _, a := range h.attrs {
		evt = addSlogAttr(evt, a)
	}
	evt.Msg(r.Message)
	return nil
}

func (h *wailsSlogHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	cp := make([]slog.Attr, len(h.attrs), len(h.attrs)+len(attrs))
	copy(cp, h.attrs)
	cp = append(cp, attrs...)
	return &wailsSlogHandler{logger: h.logger, attrs: cp}
}

func (h *wailsSlogHandler) WithGroup(name string) slog.Handler { return h }

// levelToZerolog 把 slog 级别映射到对应的 zerolog 事件构造器。
func levelToZerolog(logger *zerolog.Logger, level slog.Level) *zerolog.Event {
	switch {
	case level < slog.LevelInfo:
		return logger.Debug()
	case level < slog.LevelWarn:
		return logger.Info()
	case level < slog.LevelError:
		return logger.Warn()
	default:
		return logger.Error()
	}
}

// addSlogAttr 把单个 slog 属性附加到事件上，保持其原始类型。
func addSlogAttr(evt *zerolog.Event, a slog.Attr) *zerolog.Event {
	switch a.Value.Kind() {
	case slog.KindString:
		return evt.Str(a.Key, a.Value.String())
	case slog.KindInt64:
		return evt.Int64(a.Key, a.Value.Int64())
	case slog.KindUint64:
		return evt.Uint64(a.Key, a.Value.Uint64())
	case slog.KindFloat64:
		return evt.Float64(a.Key, a.Value.Float64())
	case slog.KindBool:
		return evt.Bool(a.Key, a.Value.Bool())
	case slog.KindDuration:
		return evt.Dur(a.Key, a.Value.Duration())
	case slog.KindTime:
		return evt.Time(a.Key, a.Value.Time())
	default:
		return evt.Any(a.Key, a.Value.Any())
	}
}
