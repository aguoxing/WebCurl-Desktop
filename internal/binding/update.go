package binding

import (
	"webcurl-desktop/internal/config"
	"webcurl-desktop/internal/service"
)

// UpdateServiceBinding 是 service.UpdateService 面向前端的包装。
//
// 这里已经没有"接线专用"的方法了：更新回调在构造时注入，
// 因此本类型的每一个导出方法都是真正给 webview 用的。
type UpdateServiceBinding struct {
	inner *service.UpdateService
}

// NewUpdateServiceBinding 围绕一个 service.UpdateService 构造绑定。
// checkFn 为执行完整更新流程的回调，由 app 层提供。
func NewUpdateServiceBinding(cfg *config.Config, checkFn func() error) *UpdateServiceBinding {
	return &UpdateServiceBinding{inner: service.NewUpdateService(cfg, checkFn)}
}

// GetVersion 返回当前运行的应用版本。
func (s *UpdateServiceBinding) GetVersion() string { return s.inner.GetVersion() }

// CheckAndInstall 执行更新流程，返回 ""（已是最新）、
// service.RestartingCode（已就绪、应用正在重启）或错误信息。
func (s *UpdateServiceBinding) CheckAndInstall() string { return s.inner.CheckAndInstall() }
