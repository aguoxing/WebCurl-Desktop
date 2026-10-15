package binding

import "webcurl-desktop/internal/service"

// SystemServiceBinding 是 service.SystemService 面向前端的包装。
type SystemServiceBinding struct {
	inner *service.SystemService
}

// NewSystemServiceBinding 围绕一个 service.SystemService 构造绑定。
// snapshot 为 app 层提供的运行环境取值函数，可为 nil。
func NewSystemServiceBinding(snapshot func() service.RuntimeInfo) *SystemServiceBinding {
	return &SystemServiceBinding{inner: service.NewSystemService(snapshot)}
}

// GetRuntimeInfo 返回 Wails 版本、webview 实现与版本、运行平台等信息。
func (s *SystemServiceBinding) GetRuntimeInfo() service.RuntimeInfo {
	return s.inner.GetRuntimeInfo()
}
