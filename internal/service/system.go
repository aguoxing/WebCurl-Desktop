package service

// RuntimeInfo 汇总"关于"页面要展示的运行环境信息。
//
// 这些值只能在运行期确定：webview 版本取决于用户机器装了什么运行时，
// 因此由 app 层探测后注入，本包不 import Wails，保持可直接单测。
type RuntimeInfo struct {
	// WailsVersion 是实际链接进二进制的 Wails 框架版本，例如 "v3.0.0-beta.28"。
	// 构建元数据被剥离（如 garble）时为空串。
	WailsVersion string `json:"wailsVersion"`
	// WebviewName 是 webview 实现名，如 "WebView2"、"WebKitGTK 6"、"WKWebView"。
	// server 构建没有 webview，此处为空串。
	WebviewName string `json:"webviewName"`
	// WebviewVersion 是运行期探测到的 webview 版本；取不到时为空串 ——
	// 例如 macOS 的 WKWebView 随系统更新，Wails 不暴露其版本号。
	WebviewVersion string `json:"webviewVersion"`
	// OS 与 Arch 是运行平台，取值同 runtime.GOOS / runtime.GOARCH。
	OS   string `json:"os"`
	Arch string `json:"arch"`
}

// webviewKeys 把 Wails 各平台上报的键名映射为面向用户的实现名。
// 顺序即优先级：Linux 上 GTK4 与 GTK3 只会出现一个，但显式定序
// 可以避免 map 遍历的随机性影响结果。
var webviewKeys = []struct{ key, name string }{
	{"WebView2", "WebView2"},
	{"webkitgtk6-runtime", "WebKitGTK 6"},
	{"webkit2gtk-runtime", "WebKitGTK"},
}

// WebviewFromPlatform 从 Wails 的平台环境信息中提取 webview 实现名与版本。
//
// 各平台上报的键名并不统一（Windows 用 "WebView2"，Linux 用
// "webkit2gtk-runtime" / "webkitgtk6-runtime"，macOS 干脆不报），
// 这里做一次归一化，让前端只需拼接 name 与 version。
func WebviewFromPlatform(platform map[string]any, goos string) (name, version string) {
	// server 构建没有 webview：Wails 在这里只上报 mode=server。
	// 必须先判它，否则会按 GOOS 误报成桌面平台的 webview。
	if mode, _ := platform["mode"].(string); mode == "server" {
		return "", ""
	}

	for _, c := range webviewKeys {
		if v, ok := platform[c.key].(string); ok && v != "" {
			return c.name, v
		}
	}

	// macOS 走系统内置的 WKWebView，其版本随系统更新且不被 Wails 暴露。
	if goos == "darwin" {
		return "WKWebView", ""
	}

	return "", ""
}

// SystemService 向前端暴露运行环境信息。
//
// snapshot 由 app 层注入：它在 application.New 之后才能取到 Wails 实例，
// 而服务是在那之前构造的，所以这里只保存取值函数，真正调用发生在前端请求时。
type SystemService struct {
	snapshot func() RuntimeInfo
}

// NewSystemService 返回一个运行环境服务。snapshot 为 nil 时
// GetRuntimeInfo 返回零值，前端据此隐藏对应行，而不是拿到一个假版本号。
func NewSystemService(snapshot func() RuntimeInfo) *SystemService {
	return &SystemService{snapshot: snapshot}
}

// GetRuntimeInfo 返回当前运行环境信息。
func (s *SystemService) GetRuntimeInfo() RuntimeInfo {
	if s.snapshot == nil {
		return RuntimeInfo{}
	}
	return s.snapshot()
}
