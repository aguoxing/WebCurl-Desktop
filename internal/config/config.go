// Package config 负责加载、合并与持久化应用配置。
//
// 加载遵循"默认值 + 文件覆盖"的策略：先取 Default()，再用 YAML 反序列化覆盖它，
// 因此配置文件缺字段不会导致启动失败。路径优先级依次为
// {可执行文件目录}/configs/config.yaml → XDG 用户配置目录 → 内置默认值。
//
// 密钥永远不会要求写进被版本控制的文件：TokenEnvVar 环境变量优先于 YAML，
// 且来自环境变量的令牌不会被 Save 写回磁盘。
//
// 版本号同理但不完全一样：构建时注入的版本（见 internal/version）优先于 YAML，
// 且同样不会被 Save 固化到磁盘上。
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/adrg/xdg"
	"gopkg.in/yaml.v3"

	"webcurl-desktop/internal/version"
)

const configFileName = "config.yaml"

const (
	CloseBehaviourQuit     = "quit"
	CloseBehaviourMinimize = "minimize"
)

// RedactedToken 在把配置送往前端时代替真实令牌。
// 前端把原样回传即可保持已存储的值不变（见 UpdateConfig.MergeFrom）。
const RedactedToken = "***"

// TokenEnvVar 在配置文件解析后读取。一旦设置，其优先级高于 YAML 中的 github_token，
// 因此密钥无需存放在被跟踪的文件里。
const TokenEnvVar = "GOWAILS_GITHUB_TOKEN"

// Config 是应用的完整配置，既用于 YAML 序列化，也作为绑定层返回给前端的结构。
type Config struct {
	App     AppConfig     `yaml:"app"     json:"app"`
	Window  WindowConfig  `yaml:"window"  json:"window"`
	Tray    TrayConfig    `yaml:"tray"    json:"tray"`
	Startup StartupConfig `yaml:"startup" json:"startup"`
	Log     LogConfig     `yaml:"log"     json:"log"`
	Update  UpdateConfig  `yaml:"update"  json:"update"`
	Server  ServerConfig  `yaml:"server"  json:"server"`
	Request RequestConfig `yaml:"request" json:"request"`
	Mock    MockConfig    `yaml:"mock"    json:"mock"`
	Storage StorageConfig `yaml:"storage" json:"storage"`

	// tokenFromEnv 记录 GitHubToken 来自 TokenEnvVar。
	// 这类令牌永远不会被写回磁盘。非导出字段，因此不会被序列化。
	tokenFromEnv bool

	// fileVersion 保存配置文件中原本的 app.version。
	// 仅当构建注入版本生效时才被填充，供 Save 还原使用（见 Save 说明）。
	fileVersion string
}

// AppConfig 描述应用自身的标识信息，由构建流程维护，前端只读。
type AppConfig struct {
	Name        string `yaml:"name"        json:"name"`
	Description string `yaml:"description" json:"description"`
	// Version 是当前运行版本。构建注入版本存在时它会被覆盖（见 Load），
	// 因此 YAML 里这一项只在未注入版本时（go run、本地调试）生效。
	Version string `yaml:"version"     json:"version"`
}

// WindowConfig 描述主窗口的初始外观。URL 为前端路由入口。
type WindowConfig struct {
	Title           string `yaml:"title"             json:"title"`
	URL             string `yaml:"url"               json:"url"`
	BackgroundColor string `yaml:"background_colour" json:"backgroundColor"`

	// MinWidth / MinHeight 是窗口能被拖拽到的最小尺寸，0 表示不限制。
	//
	// 默认取框架自身的默认窗口尺寸（800×600）：窗口以该尺寸打开，且不能再被拖小。
	// 下限与打开尺寸取同一个值，就不存在"起始尺寸小于下限、第一次拖拽反而跳大"
	// 那种别扭的中间态。想放宽或收紧下限，改这两个值即可。
	MinWidth  int `yaml:"min_width"  json:"minWidth"`
	MinHeight int `yaml:"min_height" json:"minHeight"`
}

// TrayConfig 描述系统托盘行为。
type TrayConfig struct {
	// "quit" = 直接退出应用，不保留托盘图标；"minimize" = 关闭窗口时隐藏到托盘。
	CloseBehaviour string `yaml:"close_behaviour" json:"closeBehaviour"`
	Tooltip        string `yaml:"tooltip"         json:"tooltip"`
}

// StartupConfig 描述开机自启开关。0 关闭，非 0 开启。
type StartupConfig struct {
	Enabled int `yaml:"enabled" json:"enabled"`
}

const (
	UpdateProviderGitHub = "github"
	UpdateProviderCustom = "custom"
)

// UpdateConfig 描述更新源。Provider 为空表示关闭更新功能。
//
// 注意 json 标签沿用 camelCase：Wails 的绑定生成器按 json 标签产出前端模型，
// 改动这里会直接改掉前端的字段名。
type UpdateConfig struct {
	AutoCheck   int    `yaml:"auto_check"   json:"autoCheck"`   // 0/1
	Provider    string `yaml:"provider"     json:"provider"`    // "" | "github" | "custom"
	APIURL      string `yaml:"api_url"      json:"apiUrl"`      // github 填 "owner/repo"，custom 填完整地址
	GitHubToken string `yaml:"github_token" json:"githubToken"` // github 可选 PAT

	// PublicKey 是用于校验发布包签名的 Ed25519 公钥（base64/hex 原始 32 字节，或 PEM）。
	//
	// 它通常不需要在这里配：正式构建会把公钥通过 -ldflags 注入到
	// internal/updater.EmbeddedPublicKey，且注入值优先于本项 —— 内嵌在二进制里的
	// 信任锚无法被"改配置文件"绕过。这一节是给不重新构建二进制的自定义源部署留的出口。
	//
	// 一旦配置（或内嵌）了公钥，更新就必须带签名，缺失或验签失败都会中止安装。
	PublicKey string `yaml:"public_key" json:"publicKey"`
}

// Redacted 返回可安全交给前端的副本：令牌被替换为 RedactedToken，绝不会进入 webview。
func (u UpdateConfig) Redacted() UpdateConfig {
	if u.GitHubToken != "" {
		u.GitHubToken = RedactedToken
	}
	return u
}

// MergeFrom 应用 src 中由前端可编辑的字段。GitHubToken 仅在 src 携带真实值时才被覆盖：
// 空串或脱敏占位符都表示"保持已存储的值"，因此把脱敏后的配置原样回传
// 永远不会清空或破坏密钥。
func (u *UpdateConfig) MergeFrom(src UpdateConfig) {
	if src.GitHubToken != "" && src.GitHubToken != RedactedToken {
		u.GitHubToken = src.GitHubToken
	}
	u.AutoCheck = src.AutoCheck
	u.Provider = src.Provider
	u.APIURL = src.APIURL
	// 公钥不是机密，原样透传即可（前端拿到的也没有被脱敏）。
	u.PublicKey = src.PublicKey
}

// Redacted 返回可安全交给前端的整份配置副本。
func (c Config) Redacted() Config {
	c.Update = c.Update.Redacted()
	return c
}

// ServerConfig 描述 server 模式（以 `-tags server` 构建的无界面 HTTP 服务）的监听参数。
//
// 这一节只在 server 构建里被读取：桌面构建不会启动 HTTP 服务，改动它没有任何作用。
// 容器部署时可用环境变量临时覆盖 —— WAILS_SERVER_HOST / WAILS_SERVER_PORT 的
// 优先级高于此处，这样同一份镜像无需重新构建就能改绑定地址（见 build/docker）。
type ServerConfig struct {
	// Host 是监听地址。默认 localhost，仅在宿主机内可见；
	// 容器内需要改为 0.0.0.0 才能被外部访问。
	Host string `yaml:"host" json:"host"`
	// Port 是监听端口。0 表示使用内置默认值 8080。
	Port int `yaml:"port" json:"port"`
}

// RequestConfig 是请求调试的默认参数。
//
// 这一节只决定"新建请求时勾选什么"，不改写已保存的请求：用户在某个请求里
// 关掉重定向、加了 30 秒超时，那是这条请求自身的属性，不该被全局默认值覆盖。
// 数值型开关统一用 0/1 表示布尔值，与 YAML 里更直观的写法保持一致。
type RequestConfig struct {
	// DefaultTimeoutSec 是新建请求的默认超时（秒）；0 表示不设超时。
	DefaultTimeoutSec int `yaml:"default_timeout_sec" json:"defaultTimeoutSec"`

	// DefaultRetryCount 是新建请求的默认重试次数（不含首次尝试）。
	DefaultRetryCount int `yaml:"default_retry_count" json:"defaultRetryCount"`

	// DefaultRetryDelaySec 是重试间隔（秒）。
	DefaultRetryDelaySec int `yaml:"default_retry_delay_sec" json:"defaultRetryDelaySec"`

	// FollowRedirect 为 1 时新建请求默认跟随 3XX。
	FollowRedirect int `yaml:"follow_redirect" json:"followRedirect"`

	// VerifySSL 为 1 时新建请求默认校验 TLS 证书。
	//
	// 默认开启：关掉校验会让中间人攻击与自签证书都变得无法区分，
	// 只有在调试自签环境的当场才该临时关掉。
	VerifySSL int `yaml:"verify_ssl" json:"verifySSL"`

	// HistoryLimit 是历史记录保留条数上限。
	HistoryLimit int `yaml:"history_limit" json:"historyLimit"`
}

// MockConfig 描述内置 Mock 引擎。
//
// Mock 引擎是所有能力里唯一"必须真实监听端口"的部分：它的用途就是被别的
// 程序访问，因此这些监听是产品的一部分，不受"桌面版不再内嵌 HTTP 服务"的影响。
type MockConfig struct {
	// Enabled 为 1 时随应用启动引擎；为 0 时只在界面上手动启动。
	Enabled int `yaml:"enabled" json:"enabled"`

	// ConfigPath 是 mock.json 的路径；为空时取数据目录下的 mock.json。
	//
	// 放在数据目录而不是可执行文件同目录：安装到 Program Files 之后那里不可写，
	// 而 Mock 的配置是用户会频繁编辑的内容。
	ConfigPath string `yaml:"config_path" json:"configPath"`
}

// StorageConfig 描述本地数据的存放位置（历史、集合、环境变量）。
//
// 默认放在用户配置目录而不是可执行文件同目录：桌面应用可能被装到
// Program Files 这类只读位置，把用户数据写在那里会在提权/多用户场景下失败。
type StorageConfig struct {
	// Dir 是数据目录；为空时使用「用户配置目录/webcurl-desktop/data」。
	Dir string `yaml:"dir" json:"dir"`
}

// LogConfig 描述日志输出。数值型开关统一用 0/1 表示布尔值，
// 以便与 YAML 中更直观的写法保持一致。
type LogConfig struct {
	Level      string `yaml:"level"       json:"level"`
	File       string `yaml:"file"        json:"file"`
	WailsFile  string `yaml:"wails_file"  json:"wailsFile"`
	MaxSize    int    `yaml:"max_size"    json:"maxSize"`
	MaxBackups int    `yaml:"max_backups" json:"maxBackups"`
	MaxAge     int    `yaml:"max_age"     json:"maxAge"`
	Compress   int    `yaml:"compress"    json:"compress"`
	Console    int    `yaml:"console"     json:"console"`
}

// Default 返回内置默认配置，作为文件缺失或字段缺省时的兜底。
func Default() *Config {
	return &Config{
		App: AppConfig{
			Name:        "WebCurl",
			Description: "网页版 API 调试与 Mock 工具的桌面版",
			Version:     "0.0.1",
		},
		Window: WindowConfig{
			Title:           "WebCurl",
			URL:             "/",
			BackgroundColor: "#1B2636",
			MinWidth:        800,
			MinHeight:       600,
		},
		Tray: TrayConfig{
			CloseBehaviour: CloseBehaviourMinimize,
			Tooltip:        "WebCurl",
		},
		Startup: StartupConfig{
			Enabled: 0,
		},
		Log: LogConfig{
			Level:      "debug",
			File:       defaultLogPath(),
			MaxSize:    10,
			MaxBackups: 5,
			MaxAge:     30,
			Compress:   1,
			Console:    1,
		},
		Update: UpdateConfig{
			AutoCheck: 0,
			Provider:  "",
		},
		Server: ServerConfig{
			Host: "localhost",
			Port: 8080,
		},
		Request: RequestConfig{
			DefaultTimeoutSec:    30,
			DefaultRetryCount:    0,
			DefaultRetryDelaySec: 1,
			FollowRedirect:       1,
			VerifySSL:            1,
			HistoryLimit:         500,
		},
		Mock: MockConfig{
			Enabled: 1,
		},
		Storage: StorageConfig{},
	}
}

// exeDir 返回当前可执行文件所在目录；定位失败时退回当前工作目录。
func exeDir() string {
	exe, err := os.Executable()
	if err != nil {
		return "."
	}
	return filepath.Dir(exe)
}

// DefaultDataDir 返回数据目录的默认位置（用户配置目录下的 data 子目录）。
//
// 与配置文件同处一个父目录：用户想备份或迁移"我的接口集合"时，
// 只需要拷走 webcurl-desktop 这一个目录。
func DefaultDataDir() string {
	if dir, err := xdg.DataFile(filepath.Join("webcurl-desktop", "data")); err == nil {
		return dir
	}
	return filepath.Join(exeDir(), "data")
}

// DataDir 返回实际使用的数据目录：配置里有就用配置的，否则用默认位置。
func (c *Config) DataDir() string {
	if c.Storage.Dir != "" {
		return c.Storage.Dir
	}
	return DefaultDataDir()
}

// MockConfigPath 返回实际使用的 mock.json 路径：配置里有就用配置的，
// 否则放在数据目录下——安装到只读目录后也能正常读写。
func (c *Config) MockConfigPath() string {
	if c.Mock.ConfigPath != "" {
		return c.Mock.ConfigPath
	}
	return filepath.Join(c.DataDir(), "mock.json")
}

// defaultLogPath 返回默认日志文件路径（可执行文件同级的 logs/app.log）。
func defaultLogPath() string {
	return filepath.Join(exeDir(), "logs", "app.log")
}

// Load 读取 path 并把它合并到 Default() 之上；文件不存在时返回 Default()。
// 环境变量覆盖最后应用（见 applyEnvOverrides）。
func Load(path string) (*Config, error) {
	cfg := Default()

	data, err := os.ReadFile(path)
	switch {
	case err == nil:
		if err := yaml.Unmarshal(data, cfg); err != nil {
			return nil, fmt.Errorf("parse config: %w", err)
		}
	case os.IsNotExist(err):
		// 文件不存在，直接沿用 Default()。
	default:
		return nil, fmt.Errorf("read config: %w", err)
	}

	// YAML 里显式写 `log.file: ""` 时，不能让日志系统失去文件输出。
	if cfg.Log.File == "" {
		cfg.Log.File = defaultLogPath()
	}

	applyBuildVersion(cfg)
	applyEnvOverrides(cfg)

	return cfg, nil
}

// applyBuildVersion 让构建注入的版本优先于配置文件。
//
// 版本号是二进制自身的属性：若把它交给用户可编辑的 YAML，改一行配置就能让
// 更新检查的"当前版本"漂移，从而误判有无新版本。因此这里做一次覆盖，
// 同时记下文件中原有的值，交给 Save 还原。
func applyBuildVersion(cfg *Config) {
	if !version.IsInjected() {
		return
	}
	cfg.fileVersion = cfg.App.Version
	cfg.App.Version = version.Version
}

// applyEnvOverrides 让环境变量优先于配置文件，
// 从而使密钥不会滞留在任何可能进入版本控制的文件里。
func applyEnvOverrides(cfg *Config) {
	if v := strings.TrimSpace(os.Getenv(TokenEnvVar)); v != "" {
		cfg.Update.GitHubToken = v
		cfg.tokenFromEnv = true
	}
}

// LocalConfigPath 返回可执行文件同目录下的配置文件路径。
func LocalConfigPath() string {
	return filepath.Join(exeDir(), "configs", configFileName)
}

// XDGConfigPath 返回 XDG 用户配置目录下的配置文件路径。
func XDGConfigPath() (string, error) {
	return xdg.ConfigFile(filepath.Join("webcurl-desktop", configFileName))
}

// ResolveConfigPath 返回要使用的配置路径。优先级：本地 configs/ 目录 >
// XDG 用户目录 > 本地路径（不存在时 Load 会返回默认值）。
func ResolveConfigPath() string {
	if _, err := os.Stat(LocalConfigPath()); err == nil {
		return LocalConfigPath()
	}
	if p, err := XDGConfigPath(); err == nil {
		return p
	}
	return LocalConfigPath()
}

// Save 以仅属主可读写的权限把 cfg 写入 path。
//
// 两类"来自二进制而非文件"的值会被刻意还原，避免被持久化：
//   - 来自环境变量的令牌；
//   - 构建注入的版本号 —— 否则用户点一次"保存设置"就会把当时的版本
//     固化进配置，之后换成别的二进制也读不回来。
func Save(path string, cfg *Config) error {
	out := *cfg
	if out.tokenFromEnv {
		out.Update.GitHubToken = ""
	}
	if out.fileVersion != "" {
		out.App.Version = out.fileVersion
	}

	data, err := yaml.Marshal(&out)
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}
	return os.WriteFile(path, data, 0o600)
}
