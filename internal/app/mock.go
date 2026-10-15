package app

import (
	"fmt"
	"path/filepath"
	"strings"

	"webcurl-desktop/internal/logger"
	"webcurl-desktop/internal/mock"

	"github.com/wailsapp/wails/v3/pkg/application"
)

// 事件名由后端发往前端。集中在这里声明，避免同一个名字在 Go 与 TS 两侧各写一遍
// 还拼错——前端侧的同名常量见 frontend/src/events.ts。
const (
	// eventMockLog 每来一条 Mock 请求日志就发一次。
	eventMockLog = "mock:log"
	// eventMockStatus 引擎状态变化时发送（启停、重启、加载配置失败）。
	eventMockStatus = "mock:status"
	// eventMockLogsCleared 日志被清空时发送。
	eventMockLogsCleared = "mock:logs-cleared"
)

// startMock 按配置启动 Mock 引擎。
//
// 失败不阻塞应用启动：用户完全可能是在"引擎起不来"的状态下打开应用去改配置的，
// 因此这里只记日志，状态由 Mock 控制台从 Status() 读出并展示。
func (a *App) startMock() {
	if a.cfg.Mock.Enabled == 0 {
		logger.Info().Msg("mock: 配置中未开启随应用启动，跳过")
		return
	}
	if err := a.mockSvc.Start(); err != nil {
		logger.Error().Err(err).Msg("mock: 引擎启动失败")
	}
	a.emitMockStatus()
}

// stopMock 停止 Mock 引擎。由 OnShutdown 调用：引擎持有监听套接字，
// 不显式关掉会让进程迟迟退不出（Windows 上表现为"窗口关了但进程还在"）。
func (a *App) stopMock() {
	if a.mockSvc == nil {
		return
	}
	if err := a.mockSvc.Stop(); err != nil {
		logger.Warn().Err(err).Msg("mock: 停止引擎时出错")
	}
}

// emitMockStatus 把引擎状态推给前端。
func (a *App) emitMockStatus() {
	if a.Event == nil {
		return
	}
	_ = a.Event.Emit(eventMockStatus, a.mockSvc.Status())
}

// emitMockLog 把一条 Mock 请求日志推给前端。
//
// 这是引擎与控制台之间唯一的实时通道：没有它，界面只能靠轮询，
// 而 Mock 日志的价值恰恰在于"请求打进来的瞬间就看见"。
func (a *App) emitMockLog(entry mock.RequestLog) {
	if a.Event == nil {
		return
	}
	_ = a.Event.Emit(eventMockLog, entry)
}

// emitMockLogsCleared 通知前端清空日志列表。
//
// 后端缓冲已由绑定层清掉，前端如果还留着旧列表，就会出现
// "清空后刷新页面日志又回来了"的错觉。
func (a *App) emitMockLogsCleared() {
	if a.Event == nil {
		return
	}
	_ = a.Event.Emit(eventMockLogsCleared)
}

// --- 系统对话框 ---
//
// 这些回调注入给绑定层。它们必须延迟取用 a.App：绑定层在 application.New 之前
// 就被构造好了，而真正被调用时（用户点按钮）Wails 实例早已就绪。

// pickFiles 弹出文件选择对话框，返回选中的绝对路径。
func (a *App) pickFiles(multiple bool) ([]string, error) {
	if a.App == nil {
		return nil, fmt.Errorf("应用尚未就绪")
	}
	dialog := a.Dialog.OpenFile().
		SetTitle("选择文件").
		SetButtonText("选择")

	if multiple {
		return dialog.PromptForMultipleSelection()
	}
	path, err := dialog.PromptForSingleSelection()
	if err != nil {
		return nil, err
	}
	if path == "" {
		return nil, nil
	}
	return []string{path}, nil
}

// pickSavePath 弹出"另存为"对话框。
func (a *App) pickSavePath(suggested string) (string, error) {
	if a.App == nil {
		return "", fmt.Errorf("应用尚未就绪")
	}
	name := ""
	if suggested != "" {
		name = sanitizeSuggestedName(suggested)
	}
	return a.Dialog.SaveFileWithOptions(&application.SaveFileDialogOptions{
		Title:    "保存到文件",
		Filename: name,
	}).PromptForSingleSelection()
}

// pickOpenPath 弹出"打开文件"对话框（用于导入 curl 命令或导出的配置）。
func (a *App) pickOpenPath() (string, error) {
	if a.App == nil {
		return "", fmt.Errorf("应用尚未就绪")
	}
	return a.Dialog.OpenFile().
		SetTitle("选择要导入的文件").
		SetButtonText("导入").
		PromptForSingleSelection()
}

// sanitizeSuggestedName 清理"另存为"的默认文件名。
//
// 系统对话框对非法字符的处理各不相同（有的直接报错、有的静默替换），
// 而这里的文件名来自 URL 与 Content-Disposition，可能带斜杠、冒号、问号。
// 统一清洗成安全字符，避免用户看到"随时打不开"的对话框。
func sanitizeSuggestedName(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return "response"
	}
	// 只保留路径分隔符之后的最后一段，杜绝目录穿越与非法分隔符。
	name = filepath.Base(name)

	replacer := strings.NewReplacer(
		"/", "_", "\\", "_", ":", "_", "*", "_",
		"?", "_", `"`, "_", "<", "_", ">", "_", "|", "_",
	)
	name = replacer.Replace(name)
	if name == "" || name == "." || name == ".." {
		return "response"
	}
	return name
}
