package binding

import (
	"webcurl-desktop/internal/config"
	"webcurl-desktop/internal/service"
)

// ConfigServiceBinding 是 service.ConfigService 面向前端的包装。
// 它为每个子配置提供一对 Get/Update；由程序自己管理的段落（App）只读。
// 这里每个导出方法都会变成一次 JavaScript 调用，
// 因此不该被 webview 触达的逻辑，要下沉为被包装服务上的非导出方法。
type ConfigServiceBinding struct {
	inner *service.ConfigService
}

// NewConfigServiceBinding 围绕一个 service.ConfigService 构造绑定。
// applyAutostart 由 app 层提供（开关开机登录项），可为 nil。
func NewConfigServiceBinding(cfg *config.Config, applyAutostart func(enabled bool) error) *ConfigServiceBinding {
	return &ConfigServiceBinding{inner: service.NewConfigService(cfg, applyAutostart)}
}

// GetAllConfig 返回脱敏后的完整配置。
func (c *ConfigServiceBinding) GetAllConfig() config.Config { return c.inner.GetAllConfig() }

// GetApp 返回只读的应用段落。
func (c *ConfigServiceBinding) GetApp() config.AppConfig { return c.inner.GetApp() }

// GetWindow / UpdateWindow 读取并持久化窗口段落。
func (c *ConfigServiceBinding) GetWindow() config.WindowConfig { return c.inner.GetWindow() }
func (c *ConfigServiceBinding) UpdateWindow(v config.WindowConfig) string {
	return c.inner.UpdateWindow(v)
}

// GetTray / UpdateTray 读取并持久化托盘段落。
func (c *ConfigServiceBinding) GetTray() config.TrayConfig            { return c.inner.GetTray() }
func (c *ConfigServiceBinding) UpdateTray(v config.TrayConfig) string { return c.inner.UpdateTray(v) }

// GetStartup / UpdateStartup 读取并持久化开机自启段落，
// 更新时会同步切换登录项。
func (c *ConfigServiceBinding) GetStartup() config.StartupConfig { return c.inner.GetStartup() }
func (c *ConfigServiceBinding) UpdateStartup(v config.StartupConfig) string {
	return c.inner.UpdateStartup(v)
}

// GetLog / UpdateLog 读取并持久化日志段落。
func (c *ConfigServiceBinding) GetLog() config.LogConfig            { return c.inner.GetLog() }
func (c *ConfigServiceBinding) UpdateLog(v config.LogConfig) string { return c.inner.UpdateLog(v) }

// GetUpdate / UpdateUpdate 读取并持久化更新段落。
// 回传的 payload 中令牌是脱敏值，因此存储的密钥能在往返后保持原样。
func (c *ConfigServiceBinding) GetUpdate() config.UpdateConfig { return c.inner.GetUpdate() }
func (c *ConfigServiceBinding) UpdateUpdate(v config.UpdateConfig) string {
	return c.inner.UpdateUpdate(v)
}

// GetRequest / UpdateRequest 读取并持久化请求调试的默认参数。
func (c *ConfigServiceBinding) GetRequest() config.RequestConfig { return c.inner.GetRequest() }
func (c *ConfigServiceBinding) UpdateRequest(v config.RequestConfig) string {
	return c.inner.UpdateRequest(v)
}

// GetMock / UpdateMock 读取并持久化 Mock 引擎段落。
func (c *ConfigServiceBinding) GetMock() config.MockConfig { return c.inner.GetMock() }
func (c *ConfigServiceBinding) UpdateMock(v config.MockConfig) string {
	return c.inner.UpdateMock(v)
}

// GetStorage / UpdateStorage 读取并持久化数据目录设置。
func (c *ConfigServiceBinding) GetStorage() config.StorageConfig { return c.inner.GetStorage() }
func (c *ConfigServiceBinding) UpdateStorage(v config.StorageConfig) string {
	return c.inner.UpdateStorage(v)
}
