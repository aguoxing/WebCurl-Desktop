// Package assets 内嵌由 Go 代码直接读取的静态资源。
//
// 图标放在这里、而不是散落在 internal/app 下，是为了让"哪些资源被编进二进制"
// 一目了然；将来要新增字体、模板、数据库迁移脚本等嵌入资源，也统一归到这里。
//
// 注意：build/appicon.png 与本文件是同内容的两份拷贝，无法合并 ——
// Wails 的图标流水线（wails3 generate icons）只读取 build/ 目录下的文件。
package assets

import _ "embed"

// Icon 是托盘共用的应用图标（PNG）。
//
//go:embed appicon.png
var Icon []byte
