// Package mock 是 WebCurl 的 Mock 引擎：按 mock.json 起若干个真实监听的 HTTP/HTTPS
// 服务，把请求按配置的路由规则回应出去。
//
// 与上游的差异只在"控制面"：
//   - 上游在主 HTTP 服务上挂了一组 /mock/api/* 路由 + 一个 mock.html 控制台页面，
//     靠 WebSocket 把请求日志推给页面。桌面版没有这层 HTTP 中转，控制面改为
//     直接方法调用（Start/Stop/Restart/Reload/Status/SaveConfig），日志通过
//     SetLogSink 注册的回调交给 Wails 事件。
//   - 被控面（也就是真正对外提供服务的那些监听）完全保留：Mock 的价值就在于
//     它必须是一个真实的服务器，客户端才连得上。
//
// 引擎内部实现（路由匹配、响应构造、压缩/分块/延时/计数器、WebSocket 与 SSE
// 剧本、静态目录、请求日志持久化）均按上游原样保留。
package mock

import "log/slog"

// maxMemory 是解析 multipart 请求时的内存阈值。
//
// 上游取 16GB，等价于"全部留在内存里，不落临时文件"。保留这个取值是因为
// Mock 服务多半跑在本机、请求体很小，而落盘后的清理逻辑（RemoveAll）反而
// 是额外的一处失败点。真要在 Mock 前面传大文件时再调小它更有意义。
const maxMemory int64 = 16 << 30

// Options 是构造引擎的入参。
type Options struct {
	// ConfigPath 是 mock.json 的路径。为空时取可执行文件同目录下的 mock.json。
	ConfigPath string

	// Logger 是引擎内部日志的去处。为空时使用 slog 默认日志器。
	//
	// 引擎内部大量使用包级 slog 调用（上游如此），逐个改签名既无收益也容易漏，
	// 因此这里通过 slog.SetDefault 把全局日志器指过来。
	Logger *slog.Logger

	// OnLog 接收每一次 Mock 请求的日志，可为 nil。
	OnLog LogSink
}

// New 构造引擎。此时尚未监听任何端口，需要再调用 Start。
//
// 构造与启动分开是为了让调用方能在启动前先把日志接收方挂好——
// 否则引擎起来的瞬间产生的日志会丢。
func New(opts Options) *AppManager {
	if opts.Logger != nil {
		slog.SetDefault(opts.Logger)
	}
	if opts.OnLog != nil {
		SetLogSink(opts.OnLog)
	}
	return NewAppManager(opts.ConfigPath)
}

// EngineStatus 是引擎对外的状态快照，供界面显示。
type EngineStatus struct {
	// Running 表示当前有监听在跑。
	Running bool `json:"running"`
	// ConfigPath 是配置文件的绝对路径，界面上显示"配置来自哪里"。
	ConfigPath string `json:"configPath"`
	// Listeners 是当前生效的监听地址，便于用户确认"该把请求发到哪"。
	Listeners []ListenerStatus `json:"listeners"`
	// LastError 是最近一次启停/加载的失败原因，为空表示一切正常。
	LastError string `json:"lastError,omitempty"`
}

// ListenerStatus 是单个监听的状态。
type ListenerStatus struct {
	Host      string   `json:"host"`
	Port      int      `json:"port"`
	Protocols []string `json:"protocols,omitempty"`
}

// EngineStatus 汇总引擎当前状态。
func (m *AppManager) EngineStatus() EngineStatus {
	running, _, err := m.Status()

	st := EngineStatus{
		Running:    running,
		ConfigPath: m.ConfigPath(),
		Listeners:  m.Listeners(),
	}
	if err != nil {
		st.LastError = err.Error()
	}
	return st
}

// ConfigPath 返回当前使用的配置文件路径。
//
// 暴露它是因为"配置到底读写在哪"对这个功能很关键：用户改了界面上的路由，
// 却想不起来对应的 JSON 在哪，就没法做版本管理或分享给同事。
func (m *AppManager) ConfigPath() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.cfgPath
}

// Listeners 返回当前生效的监听地址。
//
// 只在 running 时返回：未运行时 m.cfg 可能只是一份尚未生效的草稿，
// 把它当成"当前监听"会让用户以为请求已经能发过去了。
func (m *AppManager) Listeners() []ListenerStatus {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.running || m.cfg == nil {
		return nil
	}
	out := make([]ListenerStatus, 0, len(m.cfg.Listen))
	for _, l := range m.cfg.Listen {
		out = append(out, ListenerStatus{
			Host:      l.Host,
			Port:      l.Port,
			Protocols: l.Protocols,
		})
	}
	return out
}
