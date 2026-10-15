//go:build production && !devtools

package app

import "github.com/wailsapp/wails/v3/pkg/application"

// addDebugMenuItems 在生产构建下是空实现：刷新与开发者工具仅对开发者有意义。
// 开发构建下的实现在 menu_dev.go。
func addDebugMenuItems(*application.Menu, menuText) {}
