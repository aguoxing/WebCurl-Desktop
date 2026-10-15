// Package binding 集中注册所有通过 Wails 绑定机制暴露给前端的服务。
//
// 每个服务都有一层专属包装（如 ConfigServiceBinding），只挑选该给前端用的方法暴露出去。
// 这样底层服务上的内部方法就不会被生成绑定。
// Wails 按导出的类型名而非文件名生成 JavaScript，因此本目录下的文件直接按
// 主题命名（config.go、request.go），与 internal/service 下的同名文件一一对应。
//
// 运行时依赖由 AllServices 的参数一次性传入，本包不保存任何包级状态：
// 服务实例与它拿到的回调一一对应，重复调用也不会互相覆盖，
// 更不存在"先 New 再 SetChecker，顺序错了回调就丢失"的隐患。
//
// 新增一个可绑定服务的步骤：
//  1. 在本包下新建包装文件（如 foo.go）
//  2. 若它需要 app 层提供能力，在 Deps 中补一个字段
//  3. 在下面的 AllServices() 中注册
package binding

import (
	"errors"

	"webcurl-desktop/internal/config"
	"webcurl-desktop/internal/mock"
	"webcurl-desktop/internal/service"

	"github.com/wailsapp/wails/v3/pkg/application"
)

// errNoDialog 表示当前运行形态下拿不到系统对话框。
//
// 无界面构建（server 模式）与单元测试里不会注入对话框回调，此时调用相关方法
// 应当明确报错，而不是静默返回空值——否则界面上会表现为"点了按钮没反应"。
var errNoDialog = errors.New("当前运行环境下不支持系统对话框")

// Deps 汇总绑定层需要由 app 层提供的运行时能力。
// 用值传递而不是包级变量：调用方一眼能看到服务究竟依赖了什么。
type Deps struct {
	// ApplyAutostart 开关开机登录项（true 注册，false 注销）。可为 nil。
	ApplyAutostart func(enabled bool) error

	// CheckForUpdates 执行一次完整的更新检查与安装流程。
	// 返回 updater.ErrRestartRequired 表示更新已就绪、进程需要退出。
	CheckForUpdates func() error

	// RuntimeInfo 读取运行环境信息（Wails 版本、webview 版本等）。
	// 它在 application.New 之后才会被调用，因此实现里可以安全使用 Wails 实例。可为 nil。
	RuntimeInfo func() service.RuntimeInfo

	// PickFiles 弹出文件选择对话框（多选或单选），返回路径列表。可为 nil。
	PickFiles func(multiple bool) ([]string, error)

	// PickSavePath 弹出"另存为"对话框，返回目标路径。可为 nil。
	PickSavePath func(suggested string) (string, error)

	// PickOpenPath 弹出"打开文件"对话框，返回选中路径。可为 nil。
	PickOpenPath func() (string, error)

	// EmitMockLog 把一条 Mock 请求日志推给前端。可为 nil。
	EmitMockLog func(mock.RequestLog)

	// ClearMockLogs 通知前端清空日志列表（后端缓冲已一并清空）。可为 nil。
	ClearMockLogs func()
}

// Services 汇总由 app 层构造好的服务实例。
//
// 这些实例都持有运行时资源（连接、文件句柄、引擎），因此由 app 层创建并管理
// 生命周期，绑定层只负责把它们的方法暴露出去。
type Services struct {
	// Request 执行 HTTP 请求并缓存响应体。
	Request *service.RequestService

	// Workspace 读写历史、集合、环境变量。
	Workspace *service.WorkspaceService

	// Mock 托管内置 Mock 引擎。
	Mock *service.MockService
}

// AllServices 返回需要绑定到前端的完整服务列表。
// 顺序对 Wails 没有影响；唯一的要求是返回的这些包装类型，
// 其每一个导出方法都可以安全地被 webview 调用。
func AllServices(cfg *config.Config, deps Deps, svc Services) []application.Service {
	return []application.Service{
		application.NewService(NewConfigServiceBinding(cfg, deps.ApplyAutostart)),
		application.NewService(NewUpdateServiceBinding(cfg, deps.CheckForUpdates)),
		application.NewService(NewSystemServiceBinding(deps.RuntimeInfo)),
		application.NewService(NewRequestServiceBinding(
			svc.Request,
			deps.PickFiles,
			deps.PickSavePath,
		)),
		application.NewService(NewWorkspaceServiceBinding(
			svc.Workspace,
			deps.PickSavePath,
			deps.PickOpenPath,
		)),
		application.NewService(NewMockServiceBinding(svc.Mock, deps.ClearMockLogs)),
	}
}
