package httpclient

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/base64"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"
)

// Do 执行请求并返回结果。
//
// 重试策略与上游一致：网络出错或响应状态码 ≥ 500 时重试，直到用完 RetryCount。
// 这里与上游有两点不同，都是刻意的：
//   - 响应体在重试用尽后一定返回给调用方（哪怕是 500），上游会把它当失败丢掉，
//     而调试工具恰恰需要看到失败响应长什么样；
//   - 网络错误在重试用尽后以 error 返回，但若期间拿到过任何响应，则返回该响应。
func (c *Client) Do(ctx context.Context, req Request) (*Response, error) {
	req.Normalize()
	if err := req.Validate(); err != nil {
		return nil, err
	}

	bodyBytes, contentType, err := buildBody(req)
	if err != nil {
		return nil, err
	}

	targetURL, err := buildURL(req)
	if err != nil {
		return nil, err
	}

	client, redirects := newHTTPClient(req)

	var (
		lastResp *Response
		lastErr  error
	)

	for attempt := 0; attempt <= req.RetryCount; attempt++ {
		if attempt > 0 && req.RetryDelaySec > 0 {
			select {
			case <-ctx.Done():
				return lastResp, ctx.Err()
			case <-time.After(time.Duration(req.RetryDelaySec) * time.Second):
			}
		}

		resp, err := c.attempt(ctx, client, req, targetURL, bodyBytes, contentType)
		if err != nil {
			lastErr = err
			c.log().Warn("请求失败", "attempt", attempt+1, "url", targetURL.String(), "err", err)
			continue
		}

		resp.Attempts = attempt + 1
		resp.Redirects = *redirects
		lastResp = resp

		if resp.Status < 500 {
			c.log().Info("请求完成", "method", req.Method, "url", targetURL.String(),
				"status", resp.Status, "duration_ms", resp.DurationMs)
			return resp, nil
		}

		// 5xx 视为可重试，但保留响应，以便重试用尽后仍能展示。
		c.log().Warn("服务端错误，准备重试", "status", resp.Status, "attempt", attempt+1)
		lastErr = nil
		*redirects = 0
	}

	if lastResp != nil {
		// 重试用尽仍有响应：返回它，让用户看到真实的失败现场。
		lastResp.Attempts = req.RetryCount + 1
		return lastResp, nil
	}
	return nil, lastErr
}

// attempt 发出一次请求并读全响应体。
func (c *Client) attempt(
	ctx context.Context,
	client *http.Client,
	req Request,
	targetURL *url.URL,
	bodyBytes []byte,
	contentType string,
) (*Response, error) {
	var reader io.Reader
	if bodyBytes != nil {
		reader = bytes.NewReader(bodyBytes)
	}

	httpReq, err := http.NewRequestWithContext(ctx, req.Method, targetURL.String(), reader)
	if err != nil {
		return nil, fmt.Errorf("构造请求失败: %w", err)
	}

	if contentType != "" {
		httpReq.Header.Set("Content-Type", contentType)
	}
	applyHeaders(httpReq, req)
	applyAuth(httpReq, req)
	if req.AcceptEncoding != "" {
		httpReq.Header.Set("Accept-Encoding", req.AcceptEncoding)
	}

	start := time.Now()
	httpResp, err := client.Do(httpReq)
	duration := time.Since(start)
	if err != nil {
		return nil, fmt.Errorf("发送请求失败: %w", err)
	}
	defer httpResp.Body.Close()

	// 多读一个字节：用来判断是否被上限截断。
	limit := c.cfg.MaxBodyBytes
	raw, err := io.ReadAll(io.LimitReader(httpResp.Body, limit+1))
	if err != nil {
		return nil, fmt.Errorf("读取响应体失败: %w", err)
	}
	truncated := int64(len(raw)) > limit
	if truncated {
		raw = raw[:limit]
	}

	resp := &Response{
		Status:     httpResp.StatusCode,
		StatusText: strings.TrimSpace(strings.TrimPrefix(httpResp.Status, fmt.Sprintf("%d", httpResp.StatusCode))),
		Proto:      httpResp.Proto,
		Headers:    flattenHeaders(httpResp.Header),
		Cookies:    flattenCookies(httpResp),
		Body:       raw,
		IsText:     isTextual(httpResp.Header.Get("Content-Type"), raw),
		Size:       len(raw),
		Truncated:  truncated,
		DurationMs: duration.Milliseconds(),
		FinalURL:   httpResp.Request.URL.String(),
		ReceivedAt: time.Now(),
	}
	return resp, nil
}

func (c *Client) log() Logger {
	if c.cfg.Logger != nil {
		return c.cfg.Logger
	}
	return nopLogger{}
}

type nopLogger struct{}

func (nopLogger) Info(string, ...any)  {}
func (nopLogger) Warn(string, ...any)  {}
func (nopLogger) Error(string, ...any) {}

// newHTTPClient 按请求选项构造客户端。
//
// 每次请求新建一个客户端而不是复用：选项（超时、TLS、重定向策略）都挂在
// client/Transport 上，而调试工具里相邻两次请求的选项常常不同，复用反而要
// 处理缓存失效。这里的开销只是几行结构体分配，连接池的收益对交互式调用没有意义。
//
// 返回的计数指针记录本次请求经历的重定向次数：Go 的 http.Response 不直接暴露
// 这个数字，而界面上显示"经过 3 次跳转"对排查问题很有用，所以自己数。
func newHTTPClient(req Request) (*http.Client, *int) {
	tr := &http.Transport{
		// 用调试工具的人看的就是服务端发出的原始字节，自动解压会把
		// Content-Encoding / Content-Length 抹掉，反而让人怀疑服务端没压缩。
		DisableCompression: true,
		Proxy:              http.ProxyFromEnvironment,
	}
	if !req.SkipSSLVerify {
		tr.TLSClientConfig = &tls.Config{MinVersion: tls.VersionTLS12}
	} else {
		tr.TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
	}
	if req.ProxyURL != "" {
		if u, err := url.Parse(req.ProxyURL); err == nil {
			tr.Proxy = http.ProxyURL(u)
		}
	}

	client := &http.Client{Transport: tr}
	if req.TimeoutSec > 0 {
		client.Timeout = time.Duration(req.TimeoutSec) * time.Second
	}

	redirects := new(int)
	if req.NoFollowRedirect {
		client.CheckRedirect = func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		}
	} else {
		client.CheckRedirect = func(*http.Request, []*http.Request) error {
			*redirects++
			return nil
		}
	}
	// 调试工具要看到服务端下发的 Cookie，但不该把它们记住并在后续请求里自动带上，
	// 否则用户无从判断某个 Cookie 到底来自哪次响应。因此给每次请求一个干净的空 jar。
	if jar, err := cookiejar.New(nil); err == nil {
		client.Jar = jar
	}
	return client, redirects
}

// buildURL 解析目标地址并合并查询参数。
//
// 已存在于 URL 里的查询串会被保留，界面上的参数追加其后——用户从浏览器
// 复制地址时经常是一整条带参数的 URL，直接重排会让人认不出自己填了什么。
func buildURL(req Request) (*url.URL, error) {
	u, err := url.Parse(strings.TrimSpace(req.URL))
	if err != nil {
		return nil, fmt.Errorf("无效的请求地址: %w", err)
	}
	if u.Scheme == "" {
		// 只写 example.com/path 时按最常见的调试场景补 https。
		u.Scheme = "https"
	}
	if u.Host == "" {
		return nil, fmt.Errorf("无效的请求地址: 缺少主机名")
	}

	params := enabled(req.Params)
	if req.Auth.Kind == AuthAPIKey && req.Auth.ParamIn == "query" && req.Auth.APIKeyName != "" {
		params = append(params, KV{Name: req.Auth.APIKeyName, Value: req.Auth.APIKeyValue, Enabled: true})
	}
	if len(params) > 0 {
		q := u.Query()
		for _, kv := range params {
			q.Add(kv.Name, kv.Value)
		}
		u.RawQuery = q.Encode()
	}
	return u, nil
}

// applyHeaders 写入用户自定义请求头与 Cookie。
func applyHeaders(httpReq *http.Request, req Request) {
	for _, kv := range enabled(req.Headers) {
		// Set 而不是 Add：界面上的同名头由用户自己决定保留哪一个，
		// 而重复行往往是无意留下的，Add 会让服务端收到两个值。
		httpReq.Header.Set(kv.Name, kv.Value)
	}
	for _, c := range enabled(req.Cookies) {
		httpReq.AddCookie(&http.Cookie{Name: c.Name, Value: c.Value})
	}
}

// applyAuth 按认证配置改写请求头/参数。
// 认证不覆盖用户已经手填的同名头：显式配置优先于认证面板的自动生成。
func applyAuth(httpReq *http.Request, req Request) {
	switch req.Auth.Kind {
	case AuthBasic:
		if httpReq.Header.Get("Authorization") == "" {
			creds := base64.StdEncoding.EncodeToString([]byte(req.Auth.Username + ":" + req.Auth.Password))
			httpReq.Header.Set("Authorization", "Basic "+creds)
		}
	case AuthBearer:
		if httpReq.Header.Get("Authorization") == "" && req.Auth.Token != "" {
			httpReq.Header.Set("Authorization", "Bearer "+req.Auth.Token)
		}
	case AuthAPIKey:
		if req.Auth.ParamIn == "header" && req.Auth.APIKeyName != "" && httpReq.Header.Get(req.Auth.APIKeyName) == "" {
			httpReq.Header.Set(req.Auth.APIKeyName, req.Auth.APIKeyValue)
		}
	}
}

// buildBody 按 BodyKind 构造请求体，并返回应设置的 Content-Type。
func buildBody(req Request) ([]byte, string, error) {
	switch req.BodyKind {
	case BodyNone, "":
		return nil, "", nil

	case BodyJSON:
		return []byte(req.Body), "application/json", nil
	case BodyXML:
		return []byte(req.Body), "application/xml", nil
	case BodyText:
		return []byte(req.Body), "text/plain", nil
	case BodyHTML:
		return []byte(req.Body), "text/html", nil

	case BodyFormURLEncoded:
		form := url.Values{}
		for _, kv := range enabled(req.Form) {
			form.Add(kv.Name, kv.Value)
		}
		return []byte(form.Encode()), "application/x-www-form-urlencoded", nil

	case BodyFormData:
		buf := &bytes.Buffer{}
		w := multipart.NewWriter(buf)
		for _, kv := range enabled(req.Form) {
			if err := w.WriteField(kv.Name, kv.Value); err != nil {
				return nil, "", fmt.Errorf("写入表单字段 %s 失败: %w", kv.Name, err)
			}
		}
		for _, f := range req.Files {
			if !f.Enabled || f.Path == "" {
				continue
			}
			fieldName := f.FieldName
			if fieldName == "" {
				fieldName = "file"
			}
			if err := writeFilePart(w, fieldName, f); err != nil {
				return nil, "", err
			}
		}
		if err := w.Close(); err != nil {
			return nil, "", fmt.Errorf("结束 multipart 写入失败: %w", err)
		}
		return buf.Bytes(), w.FormDataContentType(), nil

	case BodyBinary:
		if req.BinaryPath == "" {
			return nil, "", fmt.Errorf("二进制请求体需要选择文件")
		}
		data, err := os.ReadFile(req.BinaryPath)
		if err != nil {
			return nil, "", fmt.Errorf("读取文件失败: %w", err)
		}
		ct := mime.TypeByExtension(filepath.Ext(req.BinaryPath))
		if ct == "" {
			ct = "application/octet-stream"
		}
		return data, ct, nil

	default:
		return nil, "", fmt.Errorf("不支持的请求体类型: %s", req.BodyKind)
	}
}

// writeFilePart 把一个本地文件写进 multipart。
//
// 先落盘再读而不是流式拷贝：同一个请求可能因重试发送多次，而重试时要重放请求体，
// 流式读只能消费一次。文件体积对表单上传而言无所谓，换来的是重试语义正确。
func writeFilePart(w *multipart.Writer, fieldName string, f FileField) error {
	fh, err := os.Open(f.Path)
	if err != nil {
		return fmt.Errorf("打开文件 %s 失败: %w", f.Path, err)
	}
	defer fh.Close()

	contentType := f.ContentType
	if contentType == "" {
		contentType = mime.TypeByExtension(filepath.Ext(f.Path))
	}

	part, err := w.CreatePart(filePartHeader(fieldName, filepath.Base(f.Path), contentType))
	if err != nil {
		return fmt.Errorf("创建文件表单段失败: %w", err)
	}
	if _, err := io.Copy(part, fh); err != nil {
		return fmt.Errorf("写入文件 %s 失败: %w", f.Path, err)
	}
	return nil
}

// flattenHeaders 展开响应头，保留服务端顺序与同名多值。
func flattenHeaders(h http.Header) []KV {
	out := make([]KV, 0, len(h))
	for _, name := range sortedHeaderNames(h) {
		for _, v := range h.Values(name) {
			out = append(out, KV{Name: name, Value: v, Enabled: true})
		}
	}
	return out
}

// flattenCookies 展开 Set-Cookie。
func flattenCookies(resp *http.Response) []KV {
	cookies := resp.Cookies()
	out := make([]KV, 0, len(cookies))
	for _, c := range cookies {
		out = append(out, KV{Name: c.Name, Value: c.Value, Enabled: true})
	}
	return out
}

// isTextual 判断响应体能否按文本展示。
//
// 判定顺序：先看 Content-Type 是否属于文本族；类型缺失或过于笼统时（比如
// application/octet-stream 被滥用），再看字节是否构成合法 UTF-8 且不含 NUL。
// 只看 Content-Type 会漏掉大量不设类型的接口，只看字节又会把压缩数据误判成文本。
func isTextual(contentType string, body []byte) bool {
	ct := strings.ToLower(strings.TrimSpace(contentType))
	if i := strings.Index(ct, ";"); i >= 0 {
		ct = strings.TrimSpace(ct[:i])
	}

	switch {
	case strings.HasPrefix(ct, "text/"):
		return true
	case ct == "application/json", ct == "application/xml", ct == "application/xhtml+xml",
		ct == "application/javascript", ct == "application/x-javascript",
		ct == "application/x-www-form-urlencoded", ct == "application/graphql",
		ct == "application/ld+json", ct == "application/problem+json",
		ct == "application/vnd.api+json", ct == "application/x-ndjson":
		return true
	case strings.HasSuffix(ct, "+json"), strings.HasSuffix(ct, "+xml"):
		return true
	}

	if len(body) == 0 {
		// 空响应体当成文本处理：界面上显示空内容比显示"二进制"更自然。
		return true
	}
	if bytes.IndexByte(body, 0) >= 0 {
		return false
	}
	return utf8.Valid(body)
}
