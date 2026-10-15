// webcurl-desktop 是 WebCurl 的桌面版：HTTP 请求调试工作台 + Mock 服务。
//
// 入口按固定顺序完成三件事：定位并加载配置 → 初始化日志 → 装配并运行应用。
// 顺序不可调换：日志参数来自配置，而应用装配依赖前两者。
package main

import (
	"embed"
	"webcurl-desktop/internal/app"
	"webcurl-desktop/internal/config"
	"webcurl-desktop/internal/logger"
	"webcurl-desktop/internal/version"
)

// assets 内嵌构建好的前端产物。frontend/dist 由前端构建流程生成，
// 开发模式下由 Wails 的 dev server 接管，因此目录即使为空也不影响编译。
//
//go:embed all:frontend/dist
var assets embed.FS

func main() {
	cfgPath := config.ResolveConfigPath()

	cfg, err := config.Load(cfgPath)
	if err != nil {
		// 日志尚未初始化，只能直接 panic。
		panic("load config: " + err.Error())
	}

	logger.Init(logger.Config{
		Level:      cfg.Log.Level,
		File:       cfg.Log.File,
		WailsFile:  cfg.Log.WailsFile,
		MaxSize:    cfg.Log.MaxSize,
		MaxBackups: cfg.Log.MaxBackups,
		MaxAge:     cfg.Log.MaxAge,
		Compress:   cfg.Log.Compress,
		Console:    cfg.Log.Console,
	})

	defer logger.Sync()

	logger.Info().
		Str("version", version.String()).
		Str("config", cfgPath).
		Str("log_file", cfg.Log.File).
		Str("wails_log_file", cfg.Log.WailsFile).
		Msg("application starting")

	if err := app.New(assets, cfg).Run(); err != nil {
		logger.Fatal().Err(err).Msg("application exited with error")
	}
}
