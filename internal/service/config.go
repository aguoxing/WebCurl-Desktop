// Package service 承载应用的业务逻辑。服务都是不依赖 Wails 的普通 Go 类型，
// 因此可以直接做单元测试。
//
// 它们在被交给前端之前会由 binding 包包装一层；并且它们从不 import app 包：
// 当服务需要回调运行中的应用（开机自启、检查更新）时，回调经构造函数注入并保存为
// 结构体字段。这样依赖箭头始终单向 —— app → service —— 既避免循环导入，
// 也不存在包级可变状态带来的初始化顺序问题，多个实例之间也不会互相串扰。
package service

import "webcurl-desktop/internal/config"

// ConfigService 负责读写持久化的配置。它持有应用所拥有的那个 *config.Config，
// 因此此处的修改无需重新从磁盘加载就能被各处看到。
type ConfigService struct {
	cfg *config.Config

	// applyAutostart 由 app 层注入：true 注册开机登录项，false 注销。
	// 存成字段而不是包级变量，服务实例之间就不会互相串扰，
	// 也不再依赖"必须先 SetXxx 再使用"的隐式初始化顺序。允许为 nil。
	applyAutostart func(enabled bool) error
}

// NewConfigService 返回一个基于 cfg 工作的服务。
// applyAutostart 由 app 层提供，可为 nil —— 此时 UpdateStartup 只持久化配置。
func NewConfigService(cfg *config.Config, applyAutostart func(enabled bool) error) *ConfigService {
	return &ConfigService{cfg: cfg, applyAutostart: applyAutostart}
}

// GetAllConfig 返回脱敏副本：前端永远不会拿到密钥。
func (c *ConfigService) GetAllConfig() config.Config { return c.cfg.Redacted() }

// GetApp 返回只读的应用段落。它没有对应的 Update，
// 因为名称与版本由构建流程管理。
func (c *ConfigService) GetApp() config.AppConfig { return c.cfg.App }

// GetWindow / UpdateWindow 读取并持久化窗口段落。
func (c *ConfigService) GetWindow() config.WindowConfig            { return c.cfg.Window }
func (c *ConfigService) UpdateWindow(v config.WindowConfig) string { c.cfg.Window = v; return c.save() }

// GetTray / UpdateTray 读取并持久化托盘段落。
func (c *ConfigService) GetTray() config.TrayConfig            { return c.cfg.Tray }
func (c *ConfigService) UpdateTray(v config.TrayConfig) string { c.cfg.Tray = v; return c.save() }

// GetStartup / UpdateStartup 读取并持久化开机自启段落。
// 登录项与存储值同步切换；切换失败被忽略，因为下次启动时以配置文件内容为准。
func (c *ConfigService) GetStartup() config.StartupConfig { return c.cfg.Startup }
func (c *ConfigService) UpdateStartup(v config.StartupConfig) string {
	c.cfg.Startup = v
	if c.applyAutostart != nil {
		_ = c.applyAutostart(v.Enabled != 0)
	}
	return c.save()
}

// GetLog / UpdateLog 读取并持久化日志段落。
// 改动在下次启动时生效：日志器只在 main.go 中初始化一次。
func (c *ConfigService) GetLog() config.LogConfig            { return c.cfg.Log }
func (c *ConfigService) UpdateLog(v config.LogConfig) string { c.cfg.Log = v; return c.save() }

// GetRequest / UpdateRequest 读取并持久化请求调试的默认参数。
// 改动对"之后新建的请求"生效，已有的请求不受影响。
func (c *ConfigService) GetRequest() config.RequestConfig { return c.cfg.Request }
func (c *ConfigService) UpdateRequest(v config.RequestConfig) string {
	c.cfg.Request = v
	return c.save()
}

// GetMock / UpdateMock 读取并持久化 Mock 引擎段落。
// Enabled 的改动在下次启动应用时生效。
func (c *ConfigService) GetMock() config.MockConfig { return c.cfg.Mock }
func (c *ConfigService) UpdateMock(v config.MockConfig) string {
	c.cfg.Mock = v
	return c.save()
}

// GetStorage / UpdateStorage 读取并持久化数据目录设置。
// 改动在下次启动时生效：存储层在启动阶段一次性打开。
func (c *ConfigService) GetStorage() config.StorageConfig { return c.cfg.Storage }
func (c *ConfigService) UpdateStorage(v config.StorageConfig) string {
	c.cfg.Storage = v
	return c.save()
}

// GetUpdate 返回脱敏后的更新段落。
func (c *ConfigService) GetUpdate() config.UpdateConfig { return c.cfg.Update.Redacted() }

// UpdateUpdate 合并前端提交的 payload，而不是整段替换：
// 界面回传的脱敏令牌会让已存储的密钥保持原样。
func (c *ConfigService) UpdateUpdate(v config.UpdateConfig) string {
	c.cfg.Update.MergeFrom(v)
	return c.save()
}

// save 持久化当前配置，成功返回 ""，失败返回错误文本。
// 返回 string 是 Wails 的要求：被绑定的方法必须返回 JavaScript 侧可检查的值。
func (c *ConfigService) save() string {
	if err := config.Save(config.ResolveConfigPath(), c.cfg); err != nil {
		return err.Error()
	}
	return ""
}
