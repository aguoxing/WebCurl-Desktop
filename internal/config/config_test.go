package config

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"webcurl-desktop/internal/version"
)

// TestRedactedHidesToken 验证脱敏后前端拿到的是占位符，且原配置未被改动。
func TestRedactedHidesToken(t *testing.T) {
	cfg := &Config{Update: UpdateConfig{GitHubToken: "ghp_secret"}}

	redacted := cfg.Redacted()
	if redacted.Update.GitHubToken != RedactedToken {
		t.Fatalf("frontend saw %q, want %q", redacted.Update.GitHubToken, RedactedToken)
	}
	if cfg.Update.GitHubToken != "ghp_secret" {
		t.Fatal("redaction must not mutate the stored config")
	}
}

// TestMergeFromPreservesStoredToken 覆盖回写约定的三种输入：
// 占位符与空串保留原值，真实值才覆盖。
func TestMergeFromPreservesStoredToken(t *testing.T) {
	cases := []struct {
		name     string
		incoming string
		want     string
	}{
		{"占位符保留原令牌", RedactedToken, "ghp_stored"},
		{"空串保留原令牌", "", "ghp_stored"},
		{"真实值覆盖原令牌", "ghp_new", "ghp_new"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			stored := UpdateConfig{GitHubToken: "ghp_stored", APIURL: "old/repo"}
			stored.MergeFrom(UpdateConfig{GitHubToken: tc.incoming, Provider: "github", APIURL: "new/repo"})

			if stored.GitHubToken != tc.want {
				t.Fatalf("token = %q, want %q", stored.GitHubToken, tc.want)
			}
			if stored.Provider != "github" || stored.APIURL != "new/repo" {
				t.Fatalf("editable fields were not applied: %+v", stored)
			}
		})
	}
}

// TestLoadReadsTokenFromEnv 验证环境变量中的令牌会被读取（并去除首尾空白），
// 同时打上"来自环境变量"的标记。
func TestLoadReadsTokenFromEnv(t *testing.T) {
	t.Setenv(TokenEnvVar, "  ghp_from_env  ")

	cfg, err := Load(filepath.Join(t.TempDir(), "absent.yaml"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Update.GitHubToken != "ghp_from_env" {
		t.Fatalf("token = %q, want %q", cfg.Update.GitHubToken, "ghp_from_env")
	}
	if !cfg.tokenFromEnv {
		t.Fatal("a token read from the environment must be flagged as such")
	}
}

// TestLoadEnvTokenWinsOverFile 验证环境变量优先级高于配置文件。
func TestLoadEnvTokenWinsOverFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte("update:\n  github_token: \"ghp_from_file\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv(TokenEnvVar, "ghp_from_env")

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Update.GitHubToken != "ghp_from_env" {
		t.Fatalf("token = %q, want the environment to win", cfg.Update.GitHubToken)
	}
}

// TestSaveNeverPersistsEnvToken 验证来自环境变量的令牌绝不落盘，
// 并且文件权限为仅属主可读写（Windows 不建模 Unix 权限位，故跳过断言）。
func TestSaveNeverPersistsEnvToken(t *testing.T) {
	t.Setenv(TokenEnvVar, "ghp_from_env")

	cfg, err := Load(filepath.Join(t.TempDir(), "absent.yaml"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	path := filepath.Join(t.TempDir(), "nested", "config.yaml")
	if err := Save(path, cfg); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "ghp_from_env") {
		t.Fatal("a token sourced from the environment was written to disk")
	}

	// Windows 不建模 Unix 权限位，Stat 在那里会报 0666。
	if runtime.GOOS != "windows" {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if perm := info.Mode().Perm(); perm != 0o600 {
			t.Fatalf("config file permissions = %o, want 600", perm)
		}
	}
}

// TestSaveKeepsFileSourcedToken 验证来自文件的令牌在保存后依然存在。
func TestSaveKeepsFileSourcedToken(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte("update:\n  provider: \"github\"\n  github_token: \"ghp_from_file\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv(TokenEnvVar, "")

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := Save(path, cfg); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "ghp_from_file") {
		t.Fatal("a file-sourced token must survive a save")
	}
}

// TestLoadRestoresDefaultLogPath 验证 YAML 中显式的空 log.file 会回退到默认路径。
func TestLoadRestoresDefaultLogPath(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte("log:\n  file: \"\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Log.File == "" {
		t.Fatal("an empty log.file must fall back to the default path")
	}
}

// TestDefaultWindowMinSize 验证主窗口内置的最小尺寸：取框架默认窗口尺寸，
// 于是窗口不能被拖得比打开时更小。
func TestDefaultWindowMinSize(t *testing.T) {
	cfg := Default()

	if cfg.Window.MinWidth != 800 || cfg.Window.MinHeight != 600 {
		t.Fatalf("window min size = %dx%d, want 800x600", cfg.Window.MinWidth, cfg.Window.MinHeight)
	}
}

// TestLoadWindowMinSizeFromFile 验证 window 段的最小尺寸可被配置文件覆盖，
// 且未出现的字段保留默认值。
func TestLoadWindowMinSizeFromFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	body := "window:\n  min_width: 1600\n  min_height: 900\n"
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Window.MinWidth != 1600 || cfg.Window.MinHeight != 900 {
		t.Fatalf("window min size = %dx%d, want 1600x900", cfg.Window.MinWidth, cfg.Window.MinHeight)
	}
	// 缺省字段仍取默认值：窗口标题不在文件里。
	if cfg.Window.Title != Default().Window.Title {
		t.Fatalf("window.title = %q, want the default", cfg.Window.Title)
	}
}

// TestDefaultServerOptions 验证 server 模式的内置默认值：绑定回环地址与 8080 端口。
// 回环地址是刻意的安全默认 —— 不经配置就不会把无鉴权的 HTTP 服务暴露到外网。
func TestDefaultServerOptions(t *testing.T) {
	cfg := Default()

	if cfg.Server.Host != "localhost" {
		t.Fatalf("server.host = %q, want %q", cfg.Server.Host, "localhost")
	}
	if cfg.Server.Port != 8080 {
		t.Fatalf("server.port = %d, want 8080", cfg.Server.Port)
	}
}

// TestLoadServerSectionFromFile 验证 YAML 中的 server 段落会被读到，
// 且未在文件里出现的字段仍保留默认值（"默认值 + 文件覆盖"策略）。
func TestLoadServerSectionFromFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	body := "server:\n  host: \"0.0.0.0\"\n  port: 9090\n"
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Server.Host != "0.0.0.0" || cfg.Server.Port != 9090 {
		t.Fatalf("server = %+v, want {0.0.0.0 9090}", cfg.Server)
	}
}

// TestLoadServerPartialFileKeepsDefaults 验证只写 host 时 port 仍取默认值。
func TestLoadServerPartialFileKeepsDefaults(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte("server:\n  host: \"0.0.0.0\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Server.Host != "0.0.0.0" {
		t.Fatalf("server.host = %q, want the file value", cfg.Server.Host)
	}
	if cfg.Server.Port != 8080 {
		t.Fatalf("server.port = %d, want the default 8080", cfg.Server.Port)
	}
}

// writeVersionedConfig 生成一份带指定 app.version 的配置文件。
func writeVersionedConfig(t *testing.T, fileVersion string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	body := "app:\n  version: \"" + fileVersion + "\"\n"
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

// stubBuildVersion 模拟构建时注入的版本。
// 真实构建中它由 -ldflags -X 写入 internal/version.Version。
func stubBuildVersion(t *testing.T, v string) {
	t.Helper()
	old := version.Version
	version.Version = v
	t.Cleanup(func() { version.Version = old })
}

// TestLoadKeepsFileVersionWithoutInjection 覆盖本地运行场景（go run / go test）：
// 没有注入版本时，配置文件里的版本号才是当前版本。
func TestLoadKeepsFileVersionWithoutInjection(t *testing.T) {
	path := writeVersionedConfig(t, "0.0.1")

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.App.Version != "0.0.1" {
		t.Fatalf("version = %q, want the value from the file", cfg.App.Version)
	}
}

// TestLoadLetsBuildVersionWin 验证构建注入的版本优先于配置文件。
// 这关系到更新检查的正确性：用户改一行 YAML 不该让"当前版本"漂移。
func TestLoadLetsBuildVersionWin(t *testing.T) {
	path := writeVersionedConfig(t, "0.0.1")
	stubBuildVersion(t, "1.2.3")

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.App.Version != "1.2.3" {
		t.Fatalf("version = %q, want the injected %q", cfg.App.Version, "1.2.3")
	}
}

// TestSaveDoesNotPersistBuildVersion 验证"保存设置"不会把构建版本固化进用户配置，
// 否则升级后回退到别的二进制就会读出错误的版本号。
func TestSaveDoesNotPersistBuildVersion(t *testing.T) {
	path := writeVersionedConfig(t, "0.0.1")
	stubBuildVersion(t, "1.2.3")

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := Save(path, cfg); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "1.2.3") {
		t.Fatalf("the injected version leaked into the config file:\n%s", data)
	}
	if !strings.Contains(string(data), "0.0.1") {
		t.Fatalf("the file version was not preserved:\n%s", data)
	}
}
