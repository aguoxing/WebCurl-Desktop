// Package httpclient 是桌面版的 HTTP 请求执行引擎。
//
// 上游 WebCurl 把"发请求"实现在一个 HTTP 处理器里（handleForward）：参数从
// multipart 表单里解出来，结果直接写回 http.ResponseWriter。桌面版没有这层
// HTTP 中转，请求由前端通过 Wails 绑定发起，因此这里把同一套语义重写成纯粹的
// 函数：输入 Request，输出 Response，不碰任何 http.ResponseWriter。
//
// 与上游保持一致的部分（这些行为差异应视为 bug）：
//   - 超时、重试次数、重试间隔的语义：重试在"网络出错"或"状态码 ≥ 500"时发生；
//   - 跳过证书校验、不跟随 3XX 这两个开关（上游是 verify_ssl=N / follow_redirect=N，
//     这里改用 SkipSSLVerify / NoFollowRedirect 表达）;
//   - Body 类型的划分：none / json / xml / text / html /
//     x-www-form-urlencoded / form-data / binary。
//
// 刻意改进的部分：
//   - 文件上传按本地路径读取（上游要把文件 multipart 传到服务端再落临时目录），
//     因此支持任意大小且不做中转；
//   - 禁用传输层的自动 gzip 解压：调试工具应当看到服务端真正发出的字节与头部，
//     否则 Content-Length / Content-Encoding 会被悄悄抹掉；
//   - 响应体读取有上限（Config.MaxBodyBytes），避免一次误请求把内存吃满。
package httpclient

import (
	"fmt"
	"net/http"
	"strings"
	"time"
)

// BodyKind 是请求体类型的枚举，取值与上游 form 字段 body_type 一致。
type BodyKind string

const (
	// BodyNone 表示不带请求体。
	BodyNone BodyKind = "none"
	// BodyJSON / BodyXML / BodyText / BodyHTML 是四种原始文本体。
	BodyJSON BodyKind = "json"
	BodyXML  BodyKind = "xml"
	BodyText BodyKind = "text"
	BodyHTML BodyKind = "html"
	// BodyFormURLEncoded 是 application/x-www-form-urlencoded。
	BodyFormURLEncoded BodyKind = "x-www-form-urlencoded"
	// BodyFormData 是 multipart/form-data，可携带文件。
	BodyFormData BodyKind = "form-data"
	// BodyBinary 表示以单个文件内容作为整个请求体。
	BodyBinary BodyKind = "binary"
)

// KV 是一个可启用的键值对，用于 Params / Headers / Cookies / 表单字段。
//
// Enabled 为 false 的条目在界面上保留但不参与请求——这是调试工具的常见用法：
// 临时关掉一个头而不用删掉它。
type KV struct {
	Name    string `json:"name"`
	Value   string `json:"value"`
	Enabled bool   `json:"enabled"`
}

// FileField 描述 form-data 里的一个文件字段。
//
// 与普通字段分开建模是因为界面上的编辑方式完全不同：文件要选路径、要显示大小。
type FileField struct {
	FieldName   string `json:"fieldName"`
	Path        string `json:"path"`
	ContentType string `json:"contentType,omitempty"`
	Enabled     bool   `json:"enabled"`
}

// AuthKind 是认证方式的枚举。
type AuthKind string

const (
	AuthNone   AuthKind = "none"
	AuthBasic  AuthKind = "basic"
	AuthBearer AuthKind = "bearer"
	// AuthAPIKey 通过自定义 Header 或查询参数传递密钥，由 ParamIn 决定位置。
	AuthAPIKey AuthKind = "api-key"
)

// Auth 描述请求认证配置。
type Auth struct {
	Kind AuthKind `json:"kind"`

	// Username / Password 用于 basic。
	Username string `json:"username,omitempty"`
	Password string `json:"password,omitempty"`

	// Token 用于 bearer。
	Token string `json:"token,omitempty"`

	// APIKeyName / APIKeyValue / ParamIn 用于 api-key，ParamIn 取 "header" 或 "query"。
	APIKeyName  string `json:"apiKeyName,omitempty"`
	APIKeyValue string `json:"apiKeyValue,omitempty"`
	ParamIn     string `json:"paramIn,omitempty"`
}

// Request 是一次待发送的请求。字段与界面上的各个面板一一对应。
type Request struct {
	Method string `json:"method"`
	URL    string `json:"url"`

	Params  []KV `json:"params,omitempty"`
	Headers []KV `json:"headers,omitempty"`
	Cookies []KV `json:"cookies,omitempty"`

	BodyKind BodyKind    `json:"bodyKind,omitempty"`
	Body     string      `json:"body,omitempty"`
	Form     []KV        `json:"form,omitempty"`
	Files    []FileField `json:"files,omitempty"`

	// BinaryPath 是 BodyBinary 时读取的文件路径。
	BinaryPath string `json:"binaryPath,omitempty"`

	Auth Auth `json:"auth,omitempty"`

	// TimeoutSec 为 0 表示不设整体超时（由调用方 context 兜底）。
	TimeoutSec int `json:"timeoutSec,omitempty"`
	// RetryCount 是"额外"重试次数，总尝试次数为 RetryCount+1。
	RetryCount    int `json:"retryCount,omitempty"`
	RetryDelaySec int `json:"retryDelaySec,omitempty"`

	// SkipSSLVerify 为 true 时跳过 TLS 证书校验。
	//
	// 刻意用"反向"命名：这样零值 false 就等于"校验证书"，即安全的默认行为。
	// 若写成 VerifySSL，零值会让忘记赋值的调用方悄悄跳过校验——调试工具里
	// 这属于最不该出现的意外。
	SkipSSLVerify bool `json:"skipSslVerify,omitempty"`

	// NoFollowRedirect 为 true 时不跟随 3XX。零值 false = 跟随（与浏览器一致）。
	NoFollowRedirect bool `json:"noFollowRedirect,omitempty"`

	// ProxyURL 为空表示走环境变量里的代理设置。
	ProxyURL string `json:"proxyUrl,omitempty"`

	// AcceptEncoding 非空时作为 Accept-Encoding 显式发送。
	AcceptEncoding string `json:"acceptEncoding,omitempty"`
}

// Response 是一次请求的完整结果，可直接序列化给前端。
//
// Body 是原始字节：调用方决定是当文本展示还是当二进制处理（见 IsText）。
type Response struct {
	Status     int    `json:"status"`
	StatusText string `json:"statusText"`
	Proto      string `json:"proto"`

	// Headers 保留服务端返回的顺序，同名多值会展开成多行。
	Headers []KV `json:"headers"`
	Cookies []KV `json:"cookies"`

	// Body 是完整响应体（受 Config.MaxBodyBytes 限制，超出时 Truncated 为 true）。
	Body []byte `json:"-"`

	// IsText 表示 Body 可以按文本展示。
	IsText bool `json:"isText"`

	Size int `json:"size"`
	// Truncated 表示响应体超过了读取上限，Body 是截断后的内容。
	Truncated bool `json:"truncated"`

	DurationMs int64 `json:"durationMs"`
	// Attempts 是实际发出的请求次数（含重试）。
	Attempts int `json:"attempts"`

	FinalURL   string    `json:"finalUrl"`
	Redirects  int       `json:"redirects"`
	ReceivedAt time.Time `json:"receivedAt"`
}

// Config 是执行器的可调参数。
type Config struct {
	// MaxBodyBytes 是单次响应体读取上限，0 表示使用 DefaultMaxBodyBytes。
	MaxBodyBytes int64

	// Logger 用于记录每次请求的结果，可为 nil。
	Logger Logger
}

// Logger 是执行器需要的最小日志接口，避免把具体日志库绑进这个包。
type Logger interface {
	Info(msg string, args ...any)
	Warn(msg string, args ...any)
	Error(msg string, args ...any)
}

// DefaultMaxBodyBytes 是响应体读取的默认上限（64MB）。
//
// 取这个值是在"够用"和"别把桌面进程撑爆"之间折中：接口调试极少有超过它的响应，
// 而真要下载大文件时会走响应另存为，届时按需放开。
const DefaultMaxBodyBytes = 64 << 20

// Client 是请求执行器。零值可用，但建议经 New 构造以带上配置。
type Client struct {
	cfg Config
}

// New 构造执行器。
func New(cfg Config) *Client {
	if cfg.MaxBodyBytes <= 0 {
		cfg.MaxBodyBytes = DefaultMaxBodyBytes
	}
	return &Client{cfg: cfg}
}

// Normalize 补齐请求里的缺省值，使零值 Request 也能表达"GET 无体请求"。
//
// 前端传入的对象可能省略字段（尤其是布尔开关），因此这里把"未填写"翻译成
// 明确语义，避免零值 false 被当成用户主动关闭了 SSL 校验。
func (r *Request) Normalize() {
	r.Method = strings.ToUpper(strings.TrimSpace(r.Method))
	if r.Method == "" {
		r.Method = http.MethodGet
	}
	if r.BodyKind == "" {
		r.BodyKind = BodyNone
	}
	if r.Auth.Kind == "" {
		r.Auth.Kind = AuthNone
	}
	if r.TimeoutSec < 0 {
		r.TimeoutSec = 0
	}
	if r.RetryCount < 0 {
		r.RetryCount = 0
	}
	if r.RetryDelaySec < 0 {
		r.RetryDelaySec = 0
	}
}

// Validate 检查请求是否具备发送条件。它只管"结构上能不能发"，
// 目标主机的可达性由实际请求负责报错。
func (r *Request) Validate() error {
	if strings.TrimSpace(r.URL) == "" {
		return fmt.Errorf("请求地址不能为空")
	}
	switch r.Method {
	case http.MethodGet, http.MethodPost, http.MethodPut, http.MethodPatch,
		http.MethodDelete, http.MethodHead, http.MethodOptions, http.MethodTrace:
		return nil
	default:
		return fmt.Errorf("不支持的请求方法: %s", r.Method)
	}
}

// enabled 过滤出启用的键值对。
func enabled(list []KV) []KV {
	out := make([]KV, 0, len(list))
	for _, kv := range list {
		if !kv.Enabled || kv.Name == "" {
			continue
		}
		out = append(out, kv)
	}
	return out
}
