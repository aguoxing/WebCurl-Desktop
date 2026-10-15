// Package app 负责装配并运行 Wails 应用：创建应用与主窗口、按配置挂载系统托盘、
// 菜单、开机自启与自动更新，并把配置注入到绑定层。
//
// 它是依赖关系的终点：app 依赖 service/binding/config/logger/updater，
// 反之不成立。需要反向调用的三处（开机自启、更新检查、Mock 日志推送）以方法值的
// 形式注入给绑定层，以免形成循环导入。
//
// 同一份装配代码支撑两种形态，由 application.System.IsServer() 分流：
// 桌面构建（macOS/Windows/Linux）创建窗口、托盘与菜单；server 构建
// （`-tags server`）只拉起 HTTP 服务，窗口/托盘/菜单/自启全部跳过。
//
// 运行时的服务实例（请求执行器、工作区存储、Mock 引擎）都在这里一次性建好，
// 再交给绑定层暴露给前端。它们是本应用里唯一的"有状态资源"，
// 生命周期与进程一致：启动时打开、退出时关闭。
//
// 关注点按文件拆分：
//
//	app.go               装配与生命周期（含桌面/server 分流）
//	window.go            主窗口创建、关闭行为、颜色解析
//	tray.go              系统托盘
//	menu.go              应用菜单与菜单事件（含多语言文案）
//	menu_dev.go          开发构建才有的菜单项（刷新、开发者工具）
//	menu_production.go   上述项在生产构建下的空实现
//	autostart.go         开机自启
//	update.go            更新检查与安装
//	system.go            运行环境信息（Wails 版本、webview 版本）
//	mock.go              Mock 引擎的启动/停止、事件推送与系统对话框
package app

import (
	"embed"
	"log/slog"
	"time"

	"webcurl-desktop/internal/assets"
	"webcurl-desktop/internal/binding"
	"webcurl-desktop/internal/config"
	"webcurl-desktop/internal/logger"
	"webcurl-desktop/internal/service"
	"webcurl-desktop/internal/store"

	"github.com/wailsapp/wails/v3/pkg/application"
)

// App 持有 Wails 应用实例与共享资源。
type App struct {
	*application.App

	cfg *config.Config

	// MainWindow 与 Tray 只在桌面构建里被赋值：server 模式（-tags server）
	// 没有原生窗口与系统托盘，这两个字段保持为 nil。
	MainWindow *application.WebviewWindow
	Tray       *application.SystemTray

	// menu 是当前生效的应用菜单，重建前先销毁它，避免反复切换语言时
	// 在原生侧堆积不可见的菜单对象。
	menu *application.Menu

	// menuLocale 是当前菜单所用语言。只在 UI 线程读写（见 setupMenu）。
	menuLocale string

	// mockSvc 托管内置 Mock 引擎。引擎必须真实监听端口，因此它是本应用里
	// 唯一长期持有网络资源的组件。
	mockSvc *service.MockService

	// workspaceSvc 读写历史、集合、环境变量。托盘上的"打开数据目录"会用到它。
	workspaceSvc *service.WorkspaceService

	// store 是历史/集合/环境变量的落盘存储。
	store *store.Store

	// settingsWindow 是桌面设置窗口（配置 / 关于）。按需创建，关闭后仍保留实例。
	settingsWindow *application.WebviewWindow
}

// New 装配应用：准备服务实例、创建 Wails 实例与主窗口，并按配置挂载托盘、自启、更新与菜单。
// cfg 为 nil 时使用 config.Default()。frontendFS 是内嵌的前端产物。
//
// 所有依赖都在这里一次性组装，方法之外不存在任何包级可变状态：
// 绑定层需要的回调以方法值形式注入，它们都通过 *App 指针延迟取用 Wails 实例，
// 因此可以在 application.New 之前构造服务 —— 真正被调用时 a.App 早已就绪。
//
// 同一条装配流程要同时支撑两种形态，靠 application.System.IsServer() 分流：
// 桌面构建创建窗口/托盘/菜单并处理关闭行为，server 构建只拉起 HTTP 服务。
// 这个判断是编译期常量，因此两条分支各自只会有一条被链接进最终二进制。
func New(frontendFS embed.FS, cfg *config.Config) *App {
	if cfg == nil {
		cfg = config.Default()
	}

	a := &App{cfg: cfg}

	// server 模式下没有原生窗口与托盘，窗口/托盘/菜单/自启都不应被创建，
	// 否则只会构造出一批无界面也无从触达的对象。
	headless := application.System.IsServer()

	// 存储打不开就退回内存态（tempDir），而不是让应用起不来：
	// 用户至少还能进来改设置里的数据目录，把问题修回去。
	st, err := store.Open(store.Options{
		Dir:          cfg.DataDir(),
		HistoryLimit: cfg.Request.HistoryLimit,
	})
	if err != nil {
		logger.Error().Err(err).Msg("打开数据目录失败，本次运行不持久化历史与集合")
		st, _ = store.Open(store.Options{Dir: "", HistoryLimit: cfg.Request.HistoryLimit})
	}
	a.store = st

	requestSvc := service.NewRequestService(0, logger.NewKVLogger())
	workspaceSvc := service.NewWorkspaceService(st)
	a.workspaceSvc = workspaceSvc
	mockSvc := service.NewMockService(
		cfg.MockConfigPath(),
		a.emitMockLog,
		logger.NewKVLogger().Slog(),
	)
	a.mockSvc = mockSvc

	opts := application.Options{
		Name:        cfg.App.Name,
		Description: cfg.App.Description,
		Services: binding.AllServices(cfg, binding.Deps{
			ApplyAutostart:  a.applyAutostart,
			CheckForUpdates: a.checkForUpdates,
			RuntimeInfo:     a.runtimeInfo,

			PickFiles:    a.pickFiles,
			PickSavePath: a.pickSavePath,
			PickOpenPath: a.pickOpenPath,

			EmitMockLog:   a.emitMockLog,
			ClearMockLogs: a.emitMockLogsCleared,
		}, binding.Services{
			Request:   requestSvc,
			Workspace: workspaceSvc,
			Mock:      mockSvc,
		}),
		Logger: slog.New(logger.NewSlogHandler()),
		Assets: application.AssetOptions{
			Handler: application.AssetFileServerFS(frontendFS),
		},
		// 只有 server 构建会读取这一节；桌面构建下它被忽略。
		Server: application.ServerOptions{
			Host: cfg.Server.Host,
			Port: cfg.Server.Port,
		},
	}

	// 单实例：Mock 引擎会占用用户配置的端口，两个实例会直接撞端口；
	// 而且托盘图标出现两个也让人分不清哪个是"当前那个"。
	// 第二个实例把参数交给第一个实例后自行退出，由第一个实例把窗口拉到前台。
	//
	// UUID 用固定的反向域名而非应用名：应用名是可以改的（配置里就能改），
	// 而单实例的锁必须跨版本稳定，否则改个名字就又能开出第二个实例。
	opts.SingleInstance = &application.SingleInstanceOptions{
		UniqueID: "com.webcurl.desktop",
		OnSecondInstanceLaunch: func(application.SecondInstanceData) {
			application.InvokeSync(func() {
				if a.MainWindow != nil {
					a.MainWindow.Show().Focus()
				}
			})
		},
	}

	// 采用"关闭即隐藏到托盘"时，窗口全关不应退出应用；
	// macOS 上还要切到附件策略，避免占用 Dock 图标。
	// 这些平台选项只对窗口有意义，headless 时不设置。
	trayActive := false
	if !headless {
		trayActive = cfg.Tray.CloseBehaviour == config.CloseBehaviourMinimize

		macPolicy := application.ActivationPolicyRegular
		if trayActive {
			macPolicy = application.ActivationPolicyAccessory
		}
		opts.Mac = application.MacOptions{
			ApplicationShouldTerminateAfterLastWindowClosed: !trayActive,
			ActivationPolicy: macPolicy,
		}
		opts.Windows = application.WindowsOptions{
			DisableQuitOnLastWindowClosed: trayActive,
		}
		opts.Linux = application.LinuxOptions{
			DisableQuitOnLastWindowClosed: trayActive,
		}
	}

	wailsApp := application.New(opts)
	a.App = wailsApp

	// 进程退出时关停 Mock 引擎：它在事件循环之外持有监听套接字，
	// 不显式关掉就可能让进程迟迟退不出（Windows 上表现为"窗口关了但进程还在"）。
	a.OnShutdown(a.stopMock)

	// 引擎要在 application.New 之后启动：日志是经 Wails 事件推给界面的，
	// 应用没起来之前发出去的事件没人接收。
	a.startMock()

	// server 模式到此为止：HTTP 监听由 application.Run 拉起，
	// 再往下都是桌面专属装配。
	if headless {
		return a
	}

	a.MainWindow = newMainWindow(wailsApp, cfg, cfg.Window.URL)

	// 配置里已经开启了自启时，确保登录项与之一致。
	a.syncAutostart()

	if trayActive && len(assets.Icon) > 0 {
		a.setupTray()
	}
	a.applyCloseBehaviour()

	if cfg.Update.Provider != "" && cfg.Update.AutoCheck != 0 {
		// 自动检查延后 2 秒，让窗口先呈现，避免启动即被网络请求拖慢。
		go func() {
			time.Sleep(2 * time.Second)
			_ = a.checkForUpdates()
		}()
	}

	a.setupMenu()

	return a
}

// Config 返回应用持有的配置对象。
func (a *App) Config() *config.Config { return a.cfg }
