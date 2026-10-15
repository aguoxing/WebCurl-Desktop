package app

import (
	"errors"
	"fmt"
	"path/filepath"
	"runtime"

	"webcurl-desktop/internal/logger"
	"webcurl-desktop/internal/updater"

	"github.com/wailsapp/wails/v3/pkg/application"
)

// 前端与 Go 之间的事件名。与 frontend/src/hooks/event/useNativeEvent.ts 的
// NativeEvent 表一一对应，改名必须两边同时改。
const (
	// eventNavigate 由 Go 发往前端：请求界面跳转到负载给出的路由。
	eventNavigate = "nav:goto"

	// eventLanguage 由前端发往 Go：负载是当前界面语言（如 "zh-CN"）。
	// 菜单文案由 Go 侧持有，而界面语言只有前端知道，因此需要这条回传。
	eventLanguage = "app:language"
)

// defaultMenuLocale 是收到前端语言之前使用的菜单语言。
// 前端挂载后会立刻下发真实语言，这个默认值通常只存在几十毫秒。
const defaultMenuLocale = "en"

// menuText 是一套完整的菜单文案。
//
// 菜单结构在代码里是固定的，所以用结构体而不是 map 承载文案：
// 少写一项编译器就会报错，而 map 只会在运行时显示成空白。
type menuText struct {
	aboutApp   string // 形如 "关于 %s"，%s 由应用名填充
	settings   string
	services   string
	hideApp    string
	hideOthers string
	showAll    string
	quitApp    string

	file           string
	checkUpdate    string
	closeWindow    string
	quit           string
	openDataDir    string
	settingsWindow string

	edit       string
	undo       string
	redo       string
	cut        string
	copy       string
	paste      string
	pastePlain string
	deleteItem string
	selectAll  string

	view        string
	reload      string
	forceReload string
	devTools    string
	resetZoom   string
	zoomIn      string
	zoomOut     string
	fullscreen  string

	window     string
	minimise   string
	zoom       string
	bringFront string

	help     string
	about    string
	openLogs string
}

// menuTexts 是菜单文案表，键与前端 locales 目录下的语言标识一致。
var menuTexts = map[string]menuText{
	"zh-CN": {
		aboutApp:   "关于 %s",
		settings:   "设置",
		services:   "服务",
		hideApp:    "隐藏 %s",
		hideOthers: "隐藏其他",
		showAll:    "全部显示",
		quitApp:    "退出 %s",

		file:           "文件",
		checkUpdate:    "检查更新",
		closeWindow:    "关闭窗口",
		quit:           "退出",
		openDataDir:    "打开数据目录",
		settingsWindow: "设置窗口",

		edit:       "编辑",
		undo:       "撤销",
		redo:       "重做",
		cut:        "剪切",
		copy:       "复制",
		paste:      "粘贴",
		pastePlain: "粘贴为纯文本",
		deleteItem: "删除",
		selectAll:  "全选",

		view:        "视图",
		reload:      "重新加载",
		forceReload: "强制重新加载",
		devTools:    "开发者工具",
		resetZoom:   "实际大小",
		zoomIn:      "放大",
		zoomOut:     "缩小",
		fullscreen:  "切换全屏",

		window:     "窗口",
		minimise:   "最小化",
		zoom:       "缩放",
		bringFront: "前置全部窗口",

		help:     "帮助",
		about:    "关于",
		openLogs: "打开日志目录",
	},
	"en": {
		aboutApp:   "About %s",
		settings:   "Settings",
		services:   "Services",
		hideApp:    "Hide %s",
		hideOthers: "Hide Others",
		showAll:    "Show All",
		quitApp:    "Quit %s",

		file:           "File",
		checkUpdate:    "Check for Updates",
		closeWindow:    "Close Window",
		quit:           "Quit",
		openDataDir:    "Open Data Folder",
		settingsWindow: "Settings Window",

		edit:       "Edit",
		undo:       "Undo",
		redo:       "Redo",
		cut:        "Cut",
		copy:       "Copy",
		paste:      "Paste",
		pastePlain: "Paste and Match Style",
		deleteItem: "Delete",
		selectAll:  "Select All",

		view:        "View",
		reload:      "Reload",
		forceReload: "Force Reload",
		devTools:    "Developer Tools",
		resetZoom:   "Actual Size",
		zoomIn:      "Zoom In",
		zoomOut:     "Zoom Out",
		fullscreen:  "Toggle Full Screen",

		window:     "Window",
		minimise:   "Minimise",
		zoom:       "Zoom",
		bringFront: "Bring All to Front",

		help:     "Help",
		about:    "About",
		openLogs: "Open Log Folder",
	},
}

// menuTextFor 返回指定语言的菜单文案。
// 未支持的语言回退到英文，而不是返回零值——零值会让整个菜单变成空白。
func menuTextFor(locale string) menuText {
	if text, ok := menuTexts[locale]; ok {
		return text
	}
	return menuTexts[defaultMenuLocale]
}

// setupMenu 构建原生应用菜单，并订阅前端下发的界面语言。
//
// 菜单文案必须与界面语言一致：界面是中文而菜单是英文，用户立刻就能看出
// 这是"外挂"上去的。语言只有前端掌握（保存在 webview 的本地缓存里），
// 因此这里订阅 eventLanguage，收到后整体重建。
func (a *App) setupMenu() {
	a.applyMenu(defaultMenuLocale)

	a.Event.On(eventLanguage, func(event *application.CustomEvent) {
		locale := localeFromEvent(event)
		if locale == "" {
			return
		}
		// 菜单只能在 UI 线程上操作，而事件回调可能跑在任意 goroutine 上；
		// 把语言比较也放进同一个回调里，这样 a.menuLocale 只被主线程读写，
		// 不需要额外加锁。
		application.InvokeSync(func() {
			if locale == a.menuLocale {
				return
			}
			a.menuLocale = locale
			a.applyMenu(locale)
		})
	})
}

// applyMenu 按给定语言重建整个应用菜单。
//
// 重建而不是就地改标签：菜单项的语言、平台差异（macOS 的应用菜单、
// 开发构建才有的调试项）交织在一起，整体重建比逐项打补丁更容易保证正确。
func (a *App) applyMenu(locale string) {
	text := menuTextFor(locale)
	name := a.cfg.App.Name

	// 先释放上一个菜单，避免反复切换语言时在原生侧堆积不可见的菜单对象。
	if a.menu != nil {
		a.menu.Destroy()
	}

	appMenu := a.Menu.New()

	// macOS 要求第一个子菜单是"应用程序菜单"：系统会把它折叠到菜单栏的应用名
	// 之下，并从这里接管 Cmd+Q / Cmd+H / Cmd+, 与 About。缺少它这些快捷键
	// 全部失效，About 也不会出现在应用菜单里。
	if runtime.GOOS == "darwin" {
		a.fillDarwinAppMenu(appMenu.AddSubmenu(name), text, name)
	}

	a.fillFileMenu(appMenu.AddSubmenu(text.file), text)
	a.fillEditMenu(appMenu.AddSubmenu(text.edit), text)
	a.fillViewMenu(appMenu.AddSubmenu(text.view), text)
	a.fillWindowMenu(appMenu.AddSubmenu(text.window), text)
	a.fillHelpMenu(appMenu.AddSubmenu(text.help), text)

	a.Menu.SetApplicationMenu(appMenu)
	a.menu = appMenu

	// 菜单语言来自前端回传，链路一旦断掉只会表现为"菜单语言不对"，
	// 打包后很难排查；留一条日志让这件事可观测。
	logger.Info().Str("locale", locale).Msg("menu: application menu built")
}

// fillDarwinAppMenu 填充 macOS 专有的应用菜单。非 darwin 平台不会调用它。
func (a *App) fillDarwinAppMenu(menu *application.Menu, text menuText, name string) {
	addItem(menu, fmt.Sprintf(text.aboutApp, name), func() { a.openSettings("/about") })
	menu.AddSeparator()
	addItem(menu, text.settings+"…", func() { a.openSettings("/setting") })
	menu.AddSeparator()
	addRole(menu, application.ServicesMenu, text.services)
	menu.AddSeparator()
	addRole(menu, application.Hide, fmt.Sprintf(text.hideApp, name))
	addRole(menu, application.HideOthers, text.hideOthers)
	addRole(menu, application.UnHide, text.showAll)
	menu.AddSeparator()
	addRole(menu, application.Quit, fmt.Sprintf(text.quitApp, name))
}

// fillFileMenu 填充文件菜单。
//
// 这里没有"新建/打开/保存"：本应用不管理文档。文件菜单只放
// 打开数据目录、检查更新与退出这类真正的应用级动作，避免出现点不动的占位项。
func (a *App) fillFileMenu(menu *application.Menu, text menuText) {
	// 数据目录排在最前：本应用把历史、集合、环境变量都存成本地 JSON，
	// 用户需要随时能看到、备份、甚至手工编辑它们。
	addItem(menu, text.openDataDir, func() {
		if err := a.workspaceSvc.OpenDataDir(); err != nil {
			logger.Error().Err(err).Msg("menu: 打开数据目录失败")
		}
	})
	menu.AddSeparator()

	// 只在配置了更新源时才给"检查更新"，否则用户会点到一个必然报
	// "未配置"的菜单项。
	if a.cfg.Update.Provider != "" {
		addItem(menu, text.checkUpdate+"…", func() {
			// 菜单回调在 UI 线程上执行，网络请求必须挪到后台。
			go func() {
				// 更新就绪导致的"重启"不是错误，不记录为失败。
				if err := a.checkForUpdates(); err != nil && !errors.Is(err, updater.ErrRestartRequired) {
					logger.Error().Err(err).Msg("updater: check failed")
				}
			}()
		})
		menu.AddSeparator()
	}

	// 与 Wails 内置 File 菜单保持同一套平台约定：macOS 的 Cmd+W 只关窗口
	// （退出在应用菜单里），其它平台把退出放在文件菜单。
	if runtime.GOOS == "darwin" {
		addRole(menu, application.CloseWindow, text.closeWindow)
	} else {
		addRole(menu, application.Quit, text.quit)
	}
}

// fillEditMenu 填充编辑菜单。
//
// 这不是"顺手加上"的装饰：webview 里的剪贴板快捷键由菜单角色分发，
// 缺少 Edit 菜单时 macOS 上的 Cmd+C / Cmd+V 根本不工作。
func (a *App) fillEditMenu(menu *application.Menu, text menuText) {
	addRole(menu, application.Undo, text.undo)
	addRole(menu, application.Redo, text.redo)
	menu.AddSeparator()
	addRole(menu, application.Cut, text.cut)
	addRole(menu, application.Copy, text.copy)
	addRole(menu, application.Paste, text.paste)
	if runtime.GOOS == "darwin" {
		addRole(menu, application.PasteAndMatchStyle, text.pastePlain)
	}
	menu.AddSeparator()
	addRole(menu, application.Delete, text.deleteItem)
	addRole(menu, application.SelectAll, text.selectAll)
}

// fillViewMenu 填充视图菜单。
func (a *App) fillViewMenu(menu *application.Menu, text menuText) {
	addRole(menu, application.ResetZoom, text.resetZoom)
	addRole(menu, application.ZoomIn, text.zoomIn)
	addRole(menu, application.ZoomOut, text.zoomOut)
	menu.AddSeparator()
	addRole(menu, application.ToggleFullscreen, text.fullscreen)

	// 刷新与开发者工具只在开发构建里出现，由构建标签决定（见 menu_dev.go）。
	addDebugMenuItems(menu, text)
}

// fillWindowMenu 填充窗口菜单。
func (a *App) fillWindowMenu(menu *application.Menu, text menuText) {
	// 桌面设置窗口是应用里的第二个窗口，容易出现"关掉后不知道去哪找回"，
	// 因此在窗口菜单里常驻一个入口。
	addItem(menu, text.settingsWindow, func() { a.openSettings("/setting") })
	menu.AddSeparator()
	addRole(menu, application.Minimise, text.minimise)
	addRole(menu, application.Zoom, text.zoom)
	menu.AddSeparator()
	if runtime.GOOS == "darwin" {
		addRole(menu, application.Front, text.bringFront)
	} else {
		addRole(menu, application.CloseWindow, text.closeWindow)
	}
}

// fillHelpMenu 填充帮助菜单。
func (a *App) fillHelpMenu(menu *application.Menu, text menuText) {
	addItem(menu, text.about, func() { a.openSettings("/about") })
	menu.AddSeparator()
	addItem(menu, text.openLogs, a.openLogDirectory)
}

// addItem 添加一个普通菜单项。
func addItem(menu *application.Menu, label string, action func()) {
	menu.Add(label).OnClick(func(*application.Context) { action() })
}

// addRole 添加一个角色项并覆盖它的标签。
//
// Wails 的角色项自带英文标签（Cut / Zoom In / …），而界面语言可能是中文，
// 因此这里统一改写；FindByRole 用来取回刚加进去的那一项。
// 角色项的系统行为（快捷键、与系统服务对接）不受标签影响。
func addRole(menu *application.Menu, role application.Role, label string) {
	menu.AddRole(role)

	if label == "" {
		return
	}
	if item := menu.FindByRole(role); item != nil {
		item.SetLabel(label)
	}
}

// navigateTo 通知前端跳转到指定路由。
//
// 路径与页面的映射表由前端持有（Vue Router），Go 侧不复制一份，
// 只把目标路径发过去；前端收到后自行决定怎么跳。
func (a *App) navigateTo(path string) {
	// 返回值表示事件是否被监听方取消。跳转没有取消语义，忽略即可。
	_ = a.Event.Emit(eventNavigate, path)

	// 事件是单向的：前端没监听、或监听得太晚，这里都不会报错。
	// 留一条日志，让"点了菜单没反应"这件事至少能被查到。
	logger.Info().Str("path", path).Msg("menu: navigation requested")
}

// openLogDirectory 在系统文件管理器里打开日志所在目录。
// 日志路径来自配置，为空表示只输出到控制台，此时不做任何事。
func (a *App) openLogDirectory() {
	if a.cfg.Log.File == "" {
		return
	}

	dir := filepath.Dir(a.cfg.Log.File)
	if err := a.Env.OpenFileManager(dir, false); err != nil {
		logger.Error().Err(err).Str("dir", dir).Msg("menu: open log directory failed")
	}
}

// localeFromEvent 从事件负载里取出语言标识。
// 前端用 Events.Emit(eventLanguage, "zh-CN") 发送，负载经 JSON 往返后是字符串；
// 非字符串负载一律视为"没带语言"，而不是猜一个默认值。
func localeFromEvent(event *application.CustomEvent) string {
	locale, ok := event.Data.(string)
	if !ok {
		return ""
	}
	return locale
}
