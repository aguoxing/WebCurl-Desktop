package service

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"webcurl-desktop/internal/httpclient"
)

// previewBytes 是发给界面的二进制响应体预览上限（1MB）。
//
// 为什么不让界面一次拿到全部：Wails 绑定走 JSON，二进制要 base64，体积会涨 1/3。
// 一个 64MB 的下载会变成 85MB 的字符串穿过 IPC，界面大概率直接卡死。
// 因此界面只拿预览，"下载完整响应"走 SaveBody 由后端直接写盘。
const previewBytes = 1 << 20

// bodyCacheLimit 是完整响应体的缓存条数上限。
//
// 只留最近几条：用户点"保存响应"时总是针对刚才那次请求，
// 缓存更多只意味着内存里长期押着几十 MB 没人要的字节。
const bodyCacheLimit = 8

// SendResult 是一次发送的结果，形状直接对齐界面需要展示的内容。
type SendResult struct {
	// ID 是本次响应的标识，供 SaveBody 取完整内容。
	ID string `json:"id"`

	// Error 非空表示请求没能完成（网络错误、参数非法等）。
	// 注意它是"字段"而不是 Go 的 error：Wails 绑定里一次调用要么返回结果、
	// 要么抛异常，而"请求失败"是需要连同请求信息一起展示给用户的正常结果。
	Error string `json:"error,omitempty"`

	Status     int             `json:"status"`
	StatusText string          `json:"statusText"`
	Proto      string          `json:"proto"`
	Headers    []httpclient.KV `json:"headers"`
	Cookies    []httpclient.KV `json:"cookies"`

	// IsText 为 true 时内容在 Text 里；否则二进制预览在 Base64 里。
	IsText bool   `json:"isText"`
	Text   string `json:"text,omitempty"`
	Base64 string `json:"base64,omitempty"`

	// Size 是响应体总字节数；预览被截断时 PreviewTruncated 为 true。
	Size             int  `json:"size"`
	PreviewTruncated bool `json:"previewTruncated"`
	// Truncated 表示响应体在后端就被读取上限截断了（见 httpclient.Config）。
	Truncated bool `json:"truncated"`

	DurationMs int64  `json:"durationMs"`
	Attempts   int    `json:"attempts"`
	FinalURL   string `json:"finalUrl"`
	Redirects  int    `json:"redirects"`

	// SuggestedFileName 是"另存为"时的默认文件名，由 URL 与 Content-Type 推断。
	SuggestedFileName string `json:"suggestedFileName"`
}

// RequestService 负责执行请求、管理取消、缓存完整响应体。
type RequestService struct {
	client *httpclient.Client

	mu      sync.Mutex
	cancels map[string]context.CancelFunc
	bodies  []cachedBody
}

// cachedBody 是一条被缓存下来的完整响应体。
type cachedBody struct {
	id   string
	data []byte
	// text 为 true 表示内容是文本，SaveBody 直接写原文即可。
	text bool
}

// NewRequestService 构造请求服务。maxBodyBytes <= 0 时用 httpclient 的默认上限。
func NewRequestService(maxBodyBytes int64, logger httpclient.Logger) *RequestService {
	return &RequestService{
		client:  httpclient.New(httpclient.Config{MaxBodyBytes: maxBodyBytes, Logger: logger}),
		cancels: make(map[string]context.CancelFunc),
	}
}

// Send 执行一次请求。requestID 由调用方给出，用于支持"发送中取消"。
//
// 无论成功失败都会返回 SendResult：失败信息放在 Error 字段里。让"请求失败"
// 成为一个正常返回值而不是抛错，界面才能把它和"调用本身出错"区分开——
// 前者要展示在响应面板里，后者才该弹提示。
func (s *RequestService) Send(requestID string, req httpclient.Request) SendResult {
	ctx, cancel := context.WithCancel(context.Background())
	if requestID != "" {
		s.mu.Lock()
		// 同一 ID 重复发送时先取消上一次，避免两个请求共用一个取消句柄。
		if prev, ok := s.cancels[requestID]; ok {
			prev()
		}
		s.cancels[requestID] = cancel
		s.mu.Unlock()
		defer func() {
			s.mu.Lock()
			// 只在仍是自己时删除：期间可能已被新的一次发送顶替。
			if cur, ok := s.cancels[requestID]; ok {
				_ = cur
				delete(s.cancels, requestID)
			}
			s.mu.Unlock()
			cancel()
		}()
	} else {
		defer cancel()
	}

	req.Normalize()
	resp, err := s.client.Do(ctx, req)
	if err != nil {
		return SendResult{ID: requestID, Error: friendlyError(ctx, err)}
	}

	out := SendResult{
		ID:         requestID,
		Status:     resp.Status,
		StatusText: resp.StatusText,
		Proto:      resp.Proto,
		Headers:    resp.Headers,
		Cookies:    resp.Cookies,
		Size:       resp.Size,
		Truncated:  resp.Truncated,
		DurationMs: resp.DurationMs,
		Attempts:   resp.Attempts,
		FinalURL:   resp.FinalURL,
		Redirects:  resp.Redirects,
		IsText:     resp.IsText,
	}
	out.SuggestedFileName = suggestFileName(resp)

	if resp.IsText {
		out.Text = string(resp.Body)
	} else if len(resp.Body) > previewBytes {
		out.Base64 = base64.StdEncoding.EncodeToString(resp.Body[:previewBytes])
		out.PreviewTruncated = true
	} else {
		out.Base64 = base64.StdEncoding.EncodeToString(resp.Body)
	}

	s.cacheBody(requestID, resp.Body, resp.IsText)
	return out
}

// Cancel 取消一次进行中的请求。找不到对应 ID 说明它已经结束，不算错误。
func (s *RequestService) Cancel(requestID string) {
	s.mu.Lock()
	cancel, ok := s.cancels[requestID]
	s.mu.Unlock()
	if ok {
		cancel()
	}
}

// SaveBody 把缓存的完整响应体写到 path。
//
// 由后端落盘而不是把字节回传前端再写：一是避免大响应穿过 IPC，
// 二是"另存为"本来就要拿到用户选定的路径，绕一圈没有意义。
func (s *RequestService) SaveBody(id, path string) error {
	if path == "" {
		return errors.New("未指定保存路径")
	}

	s.mu.Lock()
	var found *cachedBody
	for i := range s.bodies {
		if s.bodies[i].id == id {
			found = &s.bodies[i]
			break
		}
	}
	s.mu.Unlock()

	if found == nil {
		return errors.New("响应内容已过期，请重新发送请求")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("创建目录失败: %w", err)
	}
	if err := os.WriteFile(path, found.data, 0o644); err != nil {
		return fmt.Errorf("写入文件失败: %w", err)
	}
	return nil
}

// BodySize 返回缓存中该响应体的字节数，-1 表示缓存里没有。
func (s *RequestService) BodySize(id string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.bodies {
		if s.bodies[i].id == id {
			return len(s.bodies[i].data)
		}
	}
	return -1
}

// cacheBody 缓存完整响应体，超出条数上限时丢最旧的。
func (s *RequestService) cacheBody(id string, data []byte, isText bool) {
	if id == "" || data == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	// 同一 ID 重发时替换旧内容，否则列表里会堆一串同名条目。
	out := s.bodies[:0]
	for _, b := range s.bodies {
		if b.id != id {
			out = append(out, b)
		}
	}
	s.bodies = append(out, cachedBody{id: id, data: data, text: isText})
	if len(s.bodies) > bodyCacheLimit {
		s.bodies = s.bodies[len(s.bodies)-bodyCacheLimit:]
	}
}

// friendlyError 把底层错误翻译成用户能看懂的话。
//
// 界面上显示"context canceled"或一大串 url.Error 包装层没有任何帮助，
// 而"请求已取消"和"连接不上目标服务器"是两件完全不同的事，必须能分清。
func friendlyError(ctx context.Context, err error) string {
	if errors.Is(ctx.Err(), context.Canceled) {
		return "请求已取消"
	}
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return "请求超时"
	}

	msg := err.Error()
	msg = strings.TrimPrefix(msg, "发送请求失败: ")
	msg = strings.TrimPrefix(msg, "读取响应体失败: ")

	if strings.Contains(msg, "context deadline exceeded") {
		return "请求超时"
	}
	if strings.Contains(msg, "no such host") {
		return "域名解析失败: " + msg
	}
	if strings.Contains(msg, "connection refused") {
		return "连接被拒绝（目标服务未启动或端口不对）: " + msg
	}
	if strings.Contains(msg, "certificate") {
		return "证书校验失败（自签证书请在设置里临时关闭校验）: " + msg
	}
	return msg
}

// suggestFileName 依据 URL 与 Content-Type 推断"另存为"的默认文件名。
func suggestFileName(resp *httpclient.Response) string {
	// Content-Disposition 里的文件名是服务端明确表达的意图，优先采用。
	for _, kv := range resp.Headers {
		if !strings.EqualFold(kv.Name, "Content-Disposition") {
			continue
		}
		if name := fileNameFromDisposition(kv.Value); name != "" {
			return name
		}
	}

	base := "response"
	if resp.FinalURL != "" {
		if u, err := url.Parse(resp.FinalURL); err == nil && u.Path != "" && u.Path != "/" {
			base = filepath.Base(u.Path)
		}
	}

	if ext := extFromContentType(contentTypeOf(resp.Headers)); ext != "" &&
		filepath.Ext(base) == "" {
		base += ext
	}
	return base
}

// contentTypeOf 取响应头里的 Content-Type 原始值。
func contentTypeOf(headers []httpclient.KV) string {
	for _, kv := range headers {
		if strings.EqualFold(kv.Name, "Content-Type") {
			return kv.Value
		}
	}
	return ""
}

// extFromContentType 把常见 MIME 映射到扩展名，用于给下载文件起个像样的名字。
func extFromContentType(ct string) string {
	if i := strings.Index(ct, ";"); i >= 0 {
		ct = ct[:i]
	}
	ct = strings.TrimSpace(strings.ToLower(ct))

	switch ct {
	case "application/json":
		return ".json"
	case "application/xml", "text/xml":
		return ".xml"
	case "text/html":
		return ".html"
	case "text/plain":
		return ".txt"
	case "text/csv":
		return ".csv"
	case "application/pdf":
		return ".pdf"
	case "application/zip":
		return ".zip"
	case "image/png":
		return ".png"
	case "image/jpeg":
		return ".jpg"
	case "image/gif":
		return ".gif"
	case "image/svg+xml":
		return ".svg"
	}
	return ""
}

// fileNameFromDisposition 从 Content-Disposition 里抠出文件名。
func fileNameFromDisposition(v string) string {
	const marker = `filename=`
	i := strings.Index(strings.ToLower(v), marker)
	if i < 0 {
		return ""
	}
	rest := v[i+len(marker):]
	rest = strings.TrimSpace(rest)
	if rest == "" {
		return ""
	}
	if rest[0] == '"' {
		if end := strings.Index(rest[1:], `"`); end >= 0 {
			return filepath.Base(rest[1 : 1+end])
		}
		return ""
	}
	if end := strings.IndexAny(rest, ";"); end >= 0 {
		rest = rest[:end]
	}
	return filepath.Base(strings.TrimSpace(rest))
}
