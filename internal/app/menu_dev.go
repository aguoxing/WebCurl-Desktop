//go:build !production || devtools

package app

import "github.com/wailsapp/wails/v3/pkg/application"

// addDebugMenuItems 追加只对开发构建有意义的项。
//
// 刷新与开发者工具不该出现在交付给用户的菜单里：刷新会让 webview 重载并
// 丢掉全部前端状态，而终端用户点开控制台只会困惑。构建标签与 Wails 自身
// 处理开发者工具的方式保持一致（见 application 包的 roles_dev.go）。
func addDebugMenuItems(menu *application.Menu, text menuText) {
	menu.AddSeparator()
	addRole(menu, application.Reload, text.reload)
	addRole(menu, application.ForceReload, text.forceReload)
	addRole(menu, application.OpenDevTools, text.devTools)
}
