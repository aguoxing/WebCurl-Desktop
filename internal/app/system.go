package app

import (
	"runtime"
	"runtime/debug"

	"webcurl-desktop/internal/service"
)

// wailsModulePath 是 Wails 框架的 Go module 路径，用来从构建元数据里
// 找出实际链接进来的框架版本。
//
// 这里刻意读 debug.ReadBuildInfo 而不是引 wails 的内部 version 包：
// internal/version 属于框架私有实现，跨版本随时可能变动；
// 构建元数据是 Go 工具链的稳定契约，且 production 构建同样可用。
const wailsModulePath = "github.com/wailsapp/wails/v3"

// runtimeInfo 汇总运行环境信息，供"关于"页面展示。
//
// 这个方法值被注入给 system 服务，只有前端请求时才真正求值 ——
// 此时 application.New 早已返回，a.Env 已就绪。
func (a *App) runtimeInfo() service.RuntimeInfo {
	info := service.RuntimeInfo{
		WailsVersion: linkedWailsVersion(),
		OS:           runtime.GOOS,
		Arch:         runtime.GOARCH,
	}

	// server 构建同样会初始化 Env（platformEnvironment 只上报 mode=server），
	// 但仍保留判空，避免装配顺序变化时 panic。
	if a.Env != nil {
		info.WebviewName, info.WebviewVersion = service.WebviewFromPlatform(
			a.Env.Info().PlatformInfo, runtime.GOOS,
		)
	}
	return info
}

// linkedWailsVersion 从当前二进制的构建元数据里取出 Wails 的 module 版本。
//
// 拿不到时返回空串（例如用 garble 混淆、或构建元数据被剥离），
// 由界面显示为占位符，而不是编造一个版本号。
func linkedWailsVersion() string {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return ""
	}
	return wailsVersionFrom(info)
}

// wailsVersionFrom 从构建元数据中找出 Wails 的 module 版本。
// 独立成函数是为了让判断逻辑可直接单测 —— go test 生成的二进制
// 不会记录依赖列表，测试里只能靠构造函数化的 *debug.BuildInfo 来覆盖。
func wailsVersionFrom(info *debug.BuildInfo) string {
	if info == nil {
		return ""
	}

	for _, dep := range info.Deps {
		if dep.Path != wailsModulePath {
			continue
		}
		// 本地 replace（开发 Wails 本身）时 Replace.Version 为空，
		// 此时 dep.Version 只是占位伪版本，报出来反而误导，故返回空串。
		if dep.Replace != nil {
			return dep.Replace.Version
		}
		return dep.Version
	}
	return ""
}
