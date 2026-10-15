package binding

import (
	"webcurl-desktop/internal/mock"
	"webcurl-desktop/internal/service"
)

// MockServiceBinding 是 service.MockService 面向前端的包装。
//
// 界面上的 Mock 控制台（启停、编辑配置、看日志）全部经这里。
// 请求日志是"推"给前端的：后端通过 app 层把每条日志发成 Wails 事件，
// 这里的 Logs 只用于页面刚打开时补一段历史。
type MockServiceBinding struct {
	inner *service.MockService

	// clearLogs 由 app 层注入，用于连同界面上的日志一起清干净。
	clearLogs func()
}

// NewMockServiceBinding 构造 Mock 服务绑定。
func NewMockServiceBinding(inner *service.MockService, clearLogs func()) *MockServiceBinding {
	return &MockServiceBinding{inner: inner, clearLogs: clearLogs}
}

// Status 返回引擎状态（是否在跑、监听在哪、最近一次错误）。
func (b *MockServiceBinding) Status() mock.EngineStatus { return b.inner.Status() }

// Start / Stop / Restart 控制引擎生命周期。
func (b *MockServiceBinding) Start() error   { return b.inner.Start() }
func (b *MockServiceBinding) Stop() error    { return b.inner.Stop() }
func (b *MockServiceBinding) Restart() error { return b.inner.Restart() }

// GetConfig 返回 mock.json 原文。
func (b *MockServiceBinding) GetConfig() (string, error) { return b.inner.Config() }

// SaveConfig 保存配置并立即热加载。
func (b *MockServiceBinding) SaveConfig(raw string) error { return b.inner.SaveConfig(raw) }

// Reload 用给定配置热加载（不写盘）。
func (b *MockServiceBinding) Reload(raw string) error { return b.inner.Reload(raw) }

// ConfigPath 返回 mock.json 的位置。
func (b *MockServiceBinding) ConfigPath() string { return b.inner.ConfigPath() }

// RecentLogs 返回最近的请求日志，供控制台打开时补历史。
func (b *MockServiceBinding) RecentLogs(limit int) []mock.RequestLog { return b.inner.Logs(limit) }

// ClearLogs 清空日志。
func (b *MockServiceBinding) ClearLogs() {
	b.inner.ClearLogs()
	if b.clearLogs != nil {
		b.clearLogs()
	}
}
