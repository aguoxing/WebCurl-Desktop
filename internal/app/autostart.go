package app

import "webcurl-desktop/internal/logger"

// syncAutostart 在启动时让登录项与配置保持一致。
// 失败只记录警告：登录项不是关键路径，配置文件里的值仍是权威来源。
func (a *App) syncAutostart() {
	if a.cfg.Startup.Enabled == 0 {
		return
	}
	if err := a.applyAutostart(true); err != nil {
		logger.Warn().Err(err).Msg("autostart: enable failed")
	}
}

// applyAutostart 开关开机登录项，并把它作为回调注入给 service.ConfigService。
// 因此方法签名必须保持 func(bool) error。
func (a *App) applyAutostart(enabled bool) error {
	if enabled {
		return a.Autostart.Enable()
	}
	return a.Autostart.Disable()
}
