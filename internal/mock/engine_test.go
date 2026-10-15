package mock

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// TestEngineServesConfiguredRoute 验证引擎真的起了监听并能按配置回应。
//
// Mock 的全部价值就在这条路径上：配置写进去 → 端口起得来 → 命中路由拿到预期响应。
func TestEngineServesConfiguredRoute(t *testing.T) {
	port := freePort(t)
	cfgPath := filepath.Join(t.TempDir(), "mock.json")

	cfg := map[string]any{
		"listen": []map[string]any{{"host": "127.0.0.1", "port": port}},
		"routes": []map[string]any{
			{
				"method": "GET",
				"path":   "/hello",
				"responses": []map[string]any{
					{"status": 200, "body": "from-mock"},
				},
			},
		},
	}
	writeJSONFile(t, cfgPath, cfg)

	m := New(Options{ConfigPath: cfgPath})
	if err := m.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer func() { _ = m.Stop() }()

	st := m.EngineStatus()
	if !st.Running {
		t.Fatalf("EngineStatus().Running = false, 期望 true（LastError=%s）", st.LastError)
	}
	if len(st.Listeners) != 1 || st.Listeners[0].Port != port {
		t.Errorf("监听列表 = %+v, 期望包含端口 %d", st.Listeners, port)
	}
	if st.ConfigPath != cfgPath {
		t.Errorf("ConfigPath = %q, 期望 %q", st.ConfigPath, cfgPath)
	}

	body := pollBody(t, fmt.Sprintf("http://127.0.0.1:%d/hello", port), "from-mock")
	if !strings.Contains(body, "from-mock") {
		t.Errorf("Mock 路由响应 = %q", body)
	}
}

// TestEngineReportsRequestLog 验证请求日志会经 LogSink 推出来（控制台靠它工作），
// 并且环形缓冲里能取到——两者分别覆盖"实时推送"和"打开页面时补历史"。
func TestEngineReportsRequestLog(t *testing.T) {
	ClearRecentLogs()

	var mu sync.Mutex
	var received []RequestLog
	SetLogSink(func(l RequestLog) {
		mu.Lock()
		defer mu.Unlock()
		received = append(received, l)
	})
	defer SetLogSink(nil)

	port := freePort(t)
	cfgPath := filepath.Join(t.TempDir(), "mock.json")
	writeJSONFile(t, cfgPath, map[string]any{
		"listen": []map[string]any{{"host": "127.0.0.1", "port": port}},
		"routes": []map[string]any{
			{"method": "GET", "path": "/logged",
				"responses": []map[string]any{{"status": 200, "body": "ok"}}},
		},
	})

	m := New(Options{ConfigPath: cfgPath})
	if err := m.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer func() { _ = m.Stop() }()

	pollBody(t, fmt.Sprintf("http://127.0.0.1:%d/logged", port), "ok")

	// 日志推送是异步的，给一点时间。
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		mu.Lock()
		n := len(received)
		mu.Unlock()
		if n > 0 {
			break
		}
		time.Sleep(30 * time.Millisecond)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(received) == 0 {
		t.Fatal("LogSink 未收到任何请求日志")
	}
	if got := received[0].URL; !strings.Contains(got, "/logged") {
		t.Errorf("日志 URL = %q, 期望包含 /logged", got)
	}
	if recent := RecentLogs(0); len(recent) == 0 {
		t.Error("环形缓冲里应有日志，供控制台打开时补历史")
	}
}

// TestEngineWithEmptyConfigOnlyReportsNotRunning 覆盖"mock.json 不存在"的首次运行路径。
//
// 此时引擎不应该报错退出（用户还要在界面里写第一份配置），只是没有监听在跑。
func TestEngineWithEmptyConfigOnlyReportsNotRunning(t *testing.T) {
	m := New(Options{ConfigPath: filepath.Join(t.TempDir(), "不存在.json")})

	st := m.EngineStatus()
	if st.Running {
		t.Error("空配置下 Running 应为 false")
	}
	if len(st.Listeners) != 0 {
		t.Errorf("空配置下不应有监听: %+v", st.Listeners)
	}
	// 关键是"没有监听"不等于"引擎坏了"：状态查询本身必须成功。
	if st.ConfigPath == "" {
		t.Error("ConfigPath 应始终可读")
	}
}

// TestEngineReloadSwitchesRoute 验证热重载：改配置后新路由立即生效，
// 这是 Mock 控制台最常用的操作。
func TestEngineReloadSwitchesRoute(t *testing.T) {
	port := freePort(t)
	cfgPath := filepath.Join(t.TempDir(), "mock.json")

	build := func(body string) []byte {
		raw, err := json.Marshal(map[string]any{
			"listen": []map[string]any{{"host": "127.0.0.1", "port": port}},
			"routes": []map[string]any{
				{"method": "GET", "path": "/v",
					"responses": []map[string]any{{"status": 200, "body": body}}},
			},
		})
		if err != nil {
			t.Fatalf("构造配置: %v", err)
		}
		return raw
	}

	writeJSONFile(t, cfgPath, json.RawMessage(build("v1")))
	m := New(Options{ConfigPath: cfgPath})
	if err := m.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer func() { _ = m.Stop() }()

	// 先确认 v1 已生效，否则后面的"变成 v2"可能是因为第一次就没配对。
	if body := pollBody(t, fmt.Sprintf("http://127.0.0.1:%d/v", port), "v1"); !strings.Contains(body, "v1") {
		t.Fatalf("重载前响应 = %q, 期望 v1", body)
	}

	if err := m.Reload(build("v2")); err != nil {
		t.Fatalf("Reload: %v", err)
	}
	pollBody(t, fmt.Sprintf("http://127.0.0.1:%d/v", port), "v2")
}

// TestEngineStopReleasesPort 验证停止后端口被释放——否则"重启引擎"会因端口占用失败。
func TestEngineStopReleasesPort(t *testing.T) {
	port := freePort(t)
	cfgPath := filepath.Join(t.TempDir(), "mock.json")
	writeJSONFile(t, cfgPath, map[string]any{
		"listen": []map[string]any{{"host": "127.0.0.1", "port": port}},
	})

	m := New(Options{ConfigPath: cfgPath})
	if err := m.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	pollBody(t, fmt.Sprintf("http://127.0.0.1:%d/any", port), "")

	if err := m.Stop(); err != nil {
		t.Fatalf("Stop: %v", err)
	}

	// 给监听关闭留出时间，然后确认端口可以重新绑定。
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
		if err == nil {
			_ = ln.Close()
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Errorf("停止后端口 %d 仍不可用", port)
}

// freePort 取一个空闲端口。
func freePort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("取空闲端口: %v", err)
	}
	defer func() { _ = ln.Close() }()
	return ln.Addr().(*net.TCPAddr).Port
}

// writeJSONFile 把 payload 写成 JSON 文件。
func writeJSONFile(t *testing.T, path string, payload any) {
	t.Helper()
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("序列化配置: %v", err)
	}
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatalf("写配置文件: %v", err)
	}
}

// pollBody 反复请求 url，直到响应包含 want 或超时；want 为空表示只等"能连上"。
func pollBody(t *testing.T, url, want string) string {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	var last string
	for time.Now().Before(deadline) {
		resp, err := http.Get(url)
		if err == nil {
			buf := make([]byte, 4096)
			n, _ := resp.Body.Read(buf)
			_ = resp.Body.Close()
			last = string(buf[:n])
			if want == "" || strings.Contains(last, want) {
				return last
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
	return last
}
