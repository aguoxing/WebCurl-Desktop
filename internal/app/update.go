package app

import (
	"errors"
	"time"

	"webcurl-desktop/internal/config"
	"webcurl-desktop/internal/logger"
	"webcurl-desktop/internal/updater"
)

// checkForUpdates 按配置的更新源执行一次完整的检查 + 安装流程。
//
// 当更新已就绪、助手程序已启动时返回 updater.ErrRestartRequired，
// 并在短暂延迟后退出进程以便替换二进制；没有新版本时返回 nil。
// 它会同时被菜单项、前端按钮和启动后的自动检查调用。
func (a *App) checkForUpdates() error {
	uc := a.cfg.Update

	var (
		info *updater.UpdateInfo
		err  error
	)

	switch uc.Provider {
	case config.UpdateProviderGitHub:
		info, err = updater.CheckGitHub(uc.APIURL, uc.GitHubToken, a.cfg.App.Version)

	case config.UpdateProviderCustom:
		// 自定义源没配地址等同于未启用，静默跳过而不是报错。
		if uc.APIURL == "" {
			return nil
		}
		info, err = updater.Check(uc.APIURL, a.cfg.App.Version)

	default:
		return nil
	}

	if err != nil || info == nil {
		return err
	}

	err = updater.Apply(info, updater.TrustedPublicKey(uc.PublicKey))
	if errors.Is(err, updater.ErrRestartRequired) {
		logger.Info().
			Str("version", info.Version).
			Msg("updater: helper launched, shutting down to finish the update")
		// 留出时间让 IPC 回执送达 webview，然后退出。
		go func() {
			time.Sleep(700 * time.Millisecond)
			a.Quit()
		}()
	}
	return err
}
