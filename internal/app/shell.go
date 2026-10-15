package app

import (
	"net"
	"strconv"

	"github.com/wailsapp/wails/v3/pkg/application"
)

// settingsURL 拼出设置窗口的地址。
//
// 前端用的是 hash 路由（见 frontend/src/router/index.ts）：页面由 Wails 内嵌资源
// 服务加载，history 模式下刷新或深链会直接落到不存在的服务端路径上。
func (a *App) settingsURL(route string) string {
	base := a.cfg.Window.URL
	if base == "" {
		base = "/"
	}
	return base + "#" + route
}

// openSettings 打开（或唤起）桌面设置窗口，并跳转到指定路由。
//
// 设置/关于这类"应用级"页面放在独立窗口里，而不是挤进主窗口的标签页：
// 主窗口是用户正在调接口的工作台，把请求上下文推走只为改个选项，代价太大。
func (a *App) openSettings(route string) {
	// 窗口只能在 UI 线程上创建/操作，而本方法可能由菜单回调、绑定调用触发。
	application.InvokeSync(func() {
		if a.settingsWindow == nil {
			a.settingsWindow = a.Window.NewWithOptions(application.WebviewWindowOptions{
				Title:            a.cfg.App.Name + " 设置",
				Width:            920,
				Height:           700,
				MinWidth:         720,
				MinHeight:        520,
				URL:              a.settingsURL(route),
				BackgroundColour: parseHexColour(a.cfg.Window.BackgroundColor),
				Mac: application.MacWindow{
					InvisibleTitleBarHeight: 50,
					Backdrop:                application.MacBackdropTranslucent,
					TitleBar:                application.MacTitleBarHiddenInset,
				},
			})
			return
		}
		a.settingsWindow.Show().Focus()
		_ = a.Event.Emit(eventNavigate, route)
	})
}

// --- 托盘 / 菜单里的引擎操作 ---
//
// 这些动作与 Mock 控制台页面上的按钮走同一套服务，但入口在原生侧，
// 因此需要在事后主动把状态推给界面（页面上的按钮有返回值可以自己刷新，
// 菜单点击没有）。

// startMockFromShell 启动引擎并把状态推给界面。
func (a *App) startMockFromShell() error {
	if a.mockSvc == nil {
		return nil
	}
	err := a.mockSvc.Start()
	a.emitMockStatus()
	return err
}

// stopMockFromShell 停止引擎并把状态推给界面。
func (a *App) stopMockFromShell() error {
	if a.mockSvc == nil {
		return nil
	}
	err := a.mockSvc.Stop()
	a.emitMockStatus()
	return err
}

// formatListenAddr 把监听地址拼成可以直接访问的形式。
//
// 监听在通配地址上时不能照抄（0.0.0.0:8080 不是能粘进浏览器的地址），
// 换成回环地址才是用户实际会用的那个。
func formatListenAddr(host string, port int) string {
	if host == "" || host == "0.0.0.0" || host == "::" {
		host = "127.0.0.1"
	}
	return net.JoinHostPort(host, strconv.Itoa(port))
}
