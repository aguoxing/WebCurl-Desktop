package app

import (
	"webcurl-desktop/internal/assets"
	"webcurl-desktop/internal/logger"

	"github.com/wailsapp/wails/v3/pkg/application"
)

// setupTray 创建系统托盘图标与托盘菜单，并启动其事件循环。
// 调用方负责判断是否需要托盘（见 New 中的 trayActive）。
//
// 托盘承载的是"没有视图归属的动作"：窗口隐藏后的唤起、启停 Mock、
// 打开数据目录、退出。请求调试、Mock 控制台这类有页面归属的入口只放在应用菜单里，
// 不在这里重复。
func (a *App) setupTray() {
	a.Tray = a.SystemTray.New()
	a.Tray.SetIcon(assets.Icon)
	a.Tray.SetTooltip(a.cfg.Tray.Tooltip)

	// 左键点击切换主窗口显示/隐藏。
	a.Tray.OnClick(a.toggleWindow)

	trayMenu := a.NewMenu()
	trayMenu.Add("显示 / 隐藏窗口").OnClick(func(*application.Context) { a.toggleWindow() })
	trayMenu.AddSeparator()

	// 状态直接写进菜单项，省得用户去翻页面确认引擎到底起没起。
	trayMenu.Add("Mock：" + a.mockStatusLabel()).SetEnabled(false)

	// 分开启停两项而不是"切换"：菜单项在原生侧不便于动态改标签（重建整个托盘菜单
	// 会闪），而两项永远可用、也不会出现"点了一下不知道是开还是关"的歧义。
	trayMenu.Add("启动 Mock").OnClick(func(*application.Context) {
		if err := a.startMockFromShell(); err != nil {
			logger.Error().Err(err).Msg("tray: 启动 Mock 失败")
		}
	})
	trayMenu.Add("停止 Mock").OnClick(func(*application.Context) {
		if err := a.stopMockFromShell(); err != nil {
			logger.Error().Err(err).Msg("tray: 停止 Mock 失败")
		}
	})
	trayMenu.AddSeparator()

	trayMenu.Add("打开数据目录").OnClick(func(*application.Context) {
		if err := a.workspaceSvc.OpenDataDir(); err != nil {
			logger.Error().Err(err).Msg("tray: 打开数据目录失败")
		}
	})
	trayMenu.AddSeparator()

	trayMenu.Add("设置…").OnClick(func(*application.Context) { a.openSettings("/setting") })
	trayMenu.Add("退出").OnClick(func(*application.Context) { a.Quit() })
	a.Tray.SetMenu(trayMenu)

	a.Tray.Run()
}

// mockStatusLabel 生成托盘里那行状态文案。
//
// 有监听时给出具体地址（用户经常要把它粘到被测程序里），
// 没在跑时区分"没配置"和"配置了但没启动"——这两种情况的下一步动作完全不同。
func (a *App) mockStatusLabel() string {
	if a.mockSvc == nil {
		return "未初始化"
	}
	st := a.mockSvc.Status()
	if st.Running && len(st.Listeners) > 0 {
		l := st.Listeners[0]
		return formatListenAddr(l.Host, l.Port)
	}
	if st.LastError != "" {
		return "未运行（" + st.LastError + "）"
	}
	return "已停止"
}

// toggleWindow 在主窗口的显示与隐藏之间切换。
// 重新显示时一并请求焦点，否则窗口会出现在其它应用之后。
func (a *App) toggleWindow() {
	if a.MainWindow == nil {
		return
	}
	if a.MainWindow.IsVisible() {
		a.MainWindow.Hide()
		return
	}
	a.MainWindow.Show().Focus()
}
