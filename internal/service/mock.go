package service

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"webcurl-desktop/internal/mock"
)

// MockService 托管内置 Mock 引擎的生命周期与配置读写。
//
// 引擎实例由本服务持有（而不是 app 层）：引擎的启停、配置读写、日志读取是
// 一组内聚的操作，散在 app 层会让"谁负责 Stop"变得含糊——而 Mock 引擎忘了
// Stop 的后果是进程退出时端口还占着。
type MockService struct {
	cfgPath string
	engine  *mock.AppManager

	mu      sync.Mutex
	lastErr string
}

// NewMockService 构造 Mock 服务并注册日志接收方。
//
// 构造时**不**启动引擎：启动时机由 app 层决定（配置里 Enabled 为 0 时就不该启动），
// 而且启动失败要能被界面看到，不能吞在构造函数里。
func NewMockService(cfgPath string, onLog mock.LogSink, logger *slog.Logger) *MockService {
	if logger != nil {
		slog.SetDefault(logger)
	}
	engine := mock.New(mock.Options{
		ConfigPath: cfgPath,
		Logger:     logger,
		OnLog:      onLog,
	})
	return &MockService{cfgPath: cfgPath, engine: engine}
}

// Start 启动引擎。
//
// 配置里没有任何监听时引擎会返回错误，这是正常的首次运行状态：
// 界面上应当提示"还没有配置监听"，而不是把它当成故障弹一堆红字。
// 因此这里把该错误记进 lastErr 但**不**向上传播为异常，返回的 error 只用于
// 真正需要用户知道的情况。
func (s *MockService) Start() error {
	err := s.engine.Start()

	s.mu.Lock()
	if err != nil {
		s.lastErr = err.Error()
	} else {
		s.lastErr = ""
	}
	s.mu.Unlock()

	if err != nil && isNoListenerConfig(err) {
		// 空配置不是故障，交给状态字段表达。
		return nil
	}
	return err
}

// Stop 停止引擎。
func (s *MockService) Stop() error {
	err := s.engine.Stop()
	s.mu.Lock()
	if err != nil {
		s.lastErr = err.Error()
	}
	s.mu.Unlock()
	return err
}

// Restart 重启引擎（改完监听配置后让新端口生效）。
func (s *MockService) Restart() error {
	err := s.engine.Restart()
	s.mu.Lock()
	if err != nil {
		s.lastErr = err.Error()
	} else {
		s.lastErr = ""
	}
	s.mu.Unlock()
	if err != nil && isNoListenerConfig(err) {
		return nil
	}
	return err
}

// Status 返回状态快照，并把上次失败原因并进去。
func (s *MockService) Status() mock.EngineStatus {
	st := s.engine.EngineStatus()

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.lastErr != "" && st.LastError == "" {
		st.LastError = s.lastErr
	}
	return st
}

// Config 返回 mock.json 的原文。文件不存在时返回空对象，让界面能直接编辑。
func (s *MockService) Config() (string, error) {
	raw, err := os.ReadFile(s.cfgPath)
	if errors.Is(err, os.ErrNotExist) {
		return "{}", nil
	}
	if err != nil {
		return "", fmt.Errorf("读取 Mock 配置失败: %w", err)
	}
	return string(raw), nil
}

// SaveConfig 把配置写入 mock.json，并立即热加载。
//
// 保存与生效合并成一步：用户点了保存就是想让它生效，中间再要求点一次"重载"
// 只会让人怀疑到底存没存上。写盘失败或热加载失败都会如实返回。
func (s *MockService) SaveConfig(raw string) error {
	if strings.TrimSpace(raw) == "" {
		return errors.New("配置不能为空")
	}
	if err := os.MkdirAll(filepath.Dir(s.cfgPath), 0o755); err != nil {
		return fmt.Errorf("创建配置目录失败: %w", err)
	}
	if err := os.WriteFile(s.cfgPath, []byte(raw), 0o600); err != nil {
		return fmt.Errorf("写入 mock.json 失败: %w", err)
	}
	return s.Reload(raw)
}

// Reload 用给定配置热加载引擎（不写盘）。
func (s *MockService) Reload(raw string) error {
	err := s.engine.Reload([]byte(raw))

	s.mu.Lock()
	if err != nil {
		s.lastErr = err.Error()
	} else {
		s.lastErr = ""
	}
	s.mu.Unlock()

	if err != nil && isNoListenerConfig(err) {
		return nil
	}
	return err
}

// Logs 返回最近的请求日志（供控制台打开时补历史）。
func (s *MockService) Logs(limit int) []mock.RequestLog { return mock.RecentLogs(limit) }

// ClearLogs 清空日志缓冲。
func (s *MockService) ClearLogs() { mock.ClearRecentLogs() }

// ConfigPath 返回 mock.json 的位置，界面上显示"配置存在哪"。
func (s *MockService) ConfigPath() string { return s.cfgPath }

// isNoListenerConfig 判断错误是否只是"配置里还没有监听"。
//
// 引擎用固定文案报这个错，这里按文案匹配确实脆弱，但引擎是随本项目一起演进的
// 内部实现（不是第三方依赖），把判断收敛在一个函数里，将来改文案只需改此处。
func isNoListenerConfig(err error) bool {
	return err != nil && strings.Contains(err.Error(), "没有可用的监听配置")
}
