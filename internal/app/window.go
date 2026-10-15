package app

import (
	"strconv"

	"webcurl-desktop/internal/config"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
)

// newMainWindow 按配置创建主窗口，startURL 是它一开始要加载的地址。
//
// 主窗口加载的就是应用自己的前端（见 frontend/dist），没有本地 HTTP 服务参与，
// 因此这里传入的只是应用内路由。
//
// macOS 上使用隐藏式标题栏 + 半透明背景，让内容与系统外观衔接。
//
// 这里刻意不设起始尺寸，窗口沿用框架默认值；最小尺寸默认也取该值，因此窗口
// 不能被拖得比打开时更小。下限与打开尺寸是同一个值，就不会出现"起始尺寸小于
// 下限、第一次拖拽反而跳大"的中间态；把下限调大即可抬高窗口能缩到的最小尺寸。
func newMainWindow(wailsApp *application.App, cfg *config.Config, startURL string) *application.WebviewWindow {
	return wailsApp.Window.NewWithOptions(application.WebviewWindowOptions{
		Title:     cfg.Window.Title,
		MinWidth:  cfg.Window.MinWidth,
		MinHeight: cfg.Window.MinHeight,
		Mac: application.MacWindow{
			InvisibleTitleBarHeight: 50,
			Backdrop:                application.MacBackdropTranslucent,
			TitleBar:                application.MacTitleBarHiddenInset,
		},
		BackgroundColour: parseHexColour(cfg.Window.BackgroundColor),
		URL:              startURL,
	})
}

// applyCloseBehaviour 按配置决定关闭窗口的行为：
// 配置为 quit 时保持默认（直接退出），否则拦截关闭事件改为隐藏窗口。
//
// 这里取消的是默认关闭动作，而不是关闭窗口本身：托盘驻留的应用需要
// 主窗口在"看起来关掉了"之后仍然可以被托盘重新唤起。
func (a *App) applyCloseBehaviour() {
	if a.cfg.Tray.CloseBehaviour == config.CloseBehaviourQuit {
		return
	}
	a.MainWindow.RegisterHook(events.Common.WindowClosing, func(e *application.WindowEvent) {
		a.MainWindow.Hide()
		e.Cancel()
	})
}

// parseHexColour 把 "#RRGGBB" 解析为 RGBA；格式非法时返回黑色。
// 颜色来自用户可编辑的配置文件，因此这里必须容错而不是 panic。
func parseHexColour(hex string) application.RGBA {
	if len(hex) != 7 || hex[0] != '#' {
		return application.NewRGB(0, 0, 0)
	}
	val, err := strconv.ParseUint(hex[1:], 16, 24)
	if err != nil {
		return application.NewRGB(0, 0, 0)
	}
	return application.NewRGB(uint8(val>>16), uint8(val>>8), uint8(val))
}
