package httpclient

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// TestDoGetWithParamsAndHeaders 覆盖最基本的一条路径：方法、查询参数、自定义头，
// 以及响应体的文本判定。
func TestDoGetWithParamsAndHeaders(t *testing.T) {
	var gotQuery, gotHeader string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		gotHeader = r.Header.Get("X-Debug")
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.Header().Set("X-Trace-Id", "abc123")
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()

	c := New(Config{})
	resp, err := c.Do(context.Background(), Request{
		Method: "get", // 小写也应被规整
		URL:    srv.URL + "/ping",
		Params: []KV{
			{Name: "a", Value: "1", Enabled: true},
			{Name: "skip", Value: "x", Enabled: false}, // 未启用不应出现在 URL 里
		},
		Headers: []KV{{Name: "X-Debug", Value: "yes", Enabled: true}},
	})
	if err != nil {
		t.Fatalf("Do: %v", err)
	}

	if resp.Status != 200 {
		t.Errorf("状态码 = %d, 期望 200", resp.Status)
	}
	if gotQuery != "a=1" {
		t.Errorf("查询串 = %q, 期望 a=1（未启用的参数不应被发送）", gotQuery)
	}
	if gotHeader != "yes" {
		t.Errorf("自定义头 = %q, 期望 yes", gotHeader)
	}
	if !resp.IsText {
		t.Error("JSON 响应应被判定为文本")
	}
	if string(resp.Body) != `{"ok":true}` {
		t.Errorf("响应体 = %q", resp.Body)
	}
	if !hasHeader(resp.Headers, "X-Trace-Id", "abc123") {
		t.Errorf("响应头里缺少 X-Trace-Id: %+v", resp.Headers)
	}
	if resp.DurationMs < 0 {
		t.Errorf("耗时 = %d, 不应为负", resp.DurationMs)
	}
}

// TestDoRetryOnServerError 验证 5xx 会触发重试，且重试用尽后仍返回响应本身——
// 这正是"调试工具要看到失败现场"与上游行为的差异点。
func TestDoRetryOnServerError(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&calls, 1)
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("boom " + string(rune('0'+n))))
	}))
	defer srv.Close()

	c := New(Config{})
	resp, err := c.Do(context.Background(), Request{
		Method:     "GET",
		URL:        srv.URL,
		RetryCount: 2,
	})
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	if calls != 3 {
		t.Errorf("实际请求次数 = %d, 期望 3（1 次 + 2 次重试）", calls)
	}
	if resp.Status != 500 {
		t.Errorf("状态码 = %d, 期望 500", resp.Status)
	}
	if resp.Attempts != 3 {
		t.Errorf("Attempts = %d, 期望 3", resp.Attempts)
	}
	if !strings.Contains(string(resp.Body), "boom") {
		t.Errorf("重试用尽后应仍返回响应体, 实际 %q", resp.Body)
	}
}

// TestDoRetryRecoversOnSecondAttempt 验证"先失败后成功"的重试恢复路径。
func TestDoRetryRecoversOnSecondAttempt(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&calls, 1) == 1 {
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		_, _ = w.Write([]byte("ok"))
	}))
	defer srv.Close()

	c := New(Config{})
	resp, err := c.Do(context.Background(), Request{Method: "GET", URL: srv.URL, RetryCount: 3})
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	if resp.Status != 200 || string(resp.Body) != "ok" {
		t.Errorf("重试恢复失败: status=%d body=%q", resp.Status, resp.Body)
	}
	if resp.Attempts != 2 {
		t.Errorf("Attempts = %d, 期望 2", resp.Attempts)
	}
}

// TestDoJSONBody 验证原始 JSON 体会带上正确的 Content-Type。
func TestDoJSONBody(t *testing.T) {
	var gotCT string
	var gotBody []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotCT = r.Header.Get("Content-Type")
		gotBody, _ = io.ReadAll(r.Body)
	}))
	defer srv.Close()

	c := New(Config{})
	_, err := c.Do(context.Background(), Request{
		Method:   "POST",
		URL:      srv.URL,
		BodyKind: BodyJSON,
		Body:     `{"name":"webcurl"}`,
	})
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	if gotCT != "application/json" {
		t.Errorf("Content-Type = %q", gotCT)
	}
	if string(gotBody) != `{"name":"webcurl"}` {
		t.Errorf("请求体 = %q", gotBody)
	}
}

// TestDoFormURLEncoded 验证表单编码。
func TestDoFormURLEncoded(t *testing.T) {
	var gotCT, gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotCT = r.Header.Get("Content-Type")
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
	}))
	defer srv.Close()

	c := New(Config{})
	_, err := c.Do(context.Background(), Request{
		Method:   "POST",
		URL:      srv.URL,
		BodyKind: BodyFormURLEncoded,
		Form: []KV{
			{Name: "user", Value: "张三", Enabled: true},
			{Name: "off", Value: "1", Enabled: false},
		},
	})
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	if gotCT != "application/x-www-form-urlencoded" {
		t.Errorf("Content-Type = %q", gotCT)
	}
	if !strings.Contains(gotBody, "user=") || strings.Contains(gotBody, "off=") {
		t.Errorf("表单体 = %q, 未启用的字段不应出现", gotBody)
	}
}

// TestDoFormDataWithFile 验证 multipart 上传：普通字段 + 文件字段，
// 且文件名与内容都要原样送达。
func TestDoFormDataWithFile(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "报告.txt")
	content := "文件内容-content"
	if err := os.WriteFile(filePath, []byte(content), 0o600); err != nil {
		t.Fatalf("准备文件: %v", err)
	}

	var gotField, gotFileName, gotFileBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			t.Errorf("解析 multipart: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		gotField = r.FormValue("title")
		f, fh, err := r.FormFile("doc")
		if err != nil {
			t.Errorf("取文件: %v", err)
			return
		}
		defer f.Close()
		gotFileName = fh.Filename
		b, _ := io.ReadAll(f)
		gotFileBody = string(b)
	}))
	defer srv.Close()

	c := New(Config{})
	_, err := c.Do(context.Background(), Request{
		Method:   "POST",
		URL:      srv.URL,
		BodyKind: BodyFormData,
		Form:     []KV{{Name: "title", Value: "标题", Enabled: true}},
		Files:    []FileField{{FieldName: "doc", Path: filePath, Enabled: true}},
	})
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	if gotField != "标题" {
		t.Errorf("普通字段 = %q", gotField)
	}
	if gotFileName != "报告.txt" {
		t.Errorf("文件名 = %q, 期望 报告.txt", gotFileName)
	}
	if gotFileBody != content {
		t.Errorf("文件内容 = %q, 期望 %q", gotFileBody, content)
	}
}

// TestDoBinaryBody 验证整文件作为请求体，并按扩展名推断 Content-Type。
func TestDoBinaryBody(t *testing.T) {
	filePath := filepath.Join(t.TempDir(), "payload.json")
	if err := os.WriteFile(filePath, []byte(`{"raw":1}`), 0o600); err != nil {
		t.Fatalf("准备文件: %v", err)
	}

	var gotCT string
	var gotBody []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotCT = r.Header.Get("Content-Type")
		gotBody, _ = io.ReadAll(r.Body)
	}))
	defer srv.Close()

	c := New(Config{})
	_, err := c.Do(context.Background(), Request{
		Method:     "PUT",
		URL:        srv.URL,
		BodyKind:   BodyBinary,
		BinaryPath: filePath,
	})
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	if !strings.HasPrefix(gotCT, "application/json") {
		t.Errorf("Content-Type = %q, 期望按 .json 推断", gotCT)
	}
	if string(gotBody) != `{"raw":1}` {
		t.Errorf("请求体 = %q", gotBody)
	}
}

// TestDoBearerAuth 验证认证注入，以及"手填头优先于认证面板"这条规则。
func TestDoBearerAuth(t *testing.T) {
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
	}))
	defer srv.Close()

	c := New(Config{})

	if _, err := c.Do(context.Background(), Request{
		Method: "GET", URL: srv.URL,
		Auth: Auth{Kind: AuthBearer, Token: "tok-1"},
	}); err != nil {
		t.Fatalf("Do: %v", err)
	}
	if gotAuth != "Bearer tok-1" {
		t.Errorf("Authorization = %q, 期望 Bearer tok-1", gotAuth)
	}

	// 手填 Authorization 时不应被认证面板覆盖。
	if _, err := c.Do(context.Background(), Request{
		Method: "GET", URL: srv.URL,
		Headers: []KV{{Name: "Authorization", Value: "Custom xyz", Enabled: true}},
		Auth:    Auth{Kind: AuthBearer, Token: "tok-2"},
	}); err != nil {
		t.Fatalf("Do: %v", err)
	}
	if gotAuth != "Custom xyz" {
		t.Errorf("Authorization = %q, 期望手填值优先", gotAuth)
	}
}

// TestDoBasicAuth 验证 Basic 认证的编码。
func TestDoBasicAuth(t *testing.T) {
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
	}))
	defer srv.Close()

	c := New(Config{})
	if _, err := c.Do(context.Background(), Request{
		Method: "GET", URL: srv.URL,
		Auth: Auth{Kind: AuthBasic, Username: "u", Password: "p"},
	}); err != nil {
		t.Fatalf("Do: %v", err)
	}
	// base64("u:p") = dTpw
	if gotAuth != "Basic dTpw" {
		t.Errorf("Authorization = %q, 期望 Basic dTpw", gotAuth)
	}
}

// TestDoAPIKeyInQuery 验证 api-key 走查询参数的形态。
func TestDoAPIKeyInQuery(t *testing.T) {
	var gotQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
	}))
	defer srv.Close()

	c := New(Config{})
	if _, err := c.Do(context.Background(), Request{
		Method: "GET", URL: srv.URL,
		Auth: Auth{Kind: AuthAPIKey, APIKeyName: "token", APIKeyValue: "v", ParamIn: "query"},
	}); err != nil {
		t.Fatalf("Do: %v", err)
	}
	if gotQuery != "token=v" {
		t.Errorf("查询串 = %q, 期望 token=v", gotQuery)
	}
}

// TestDoNoFollowRedirect 验证 follow_redirect=false 时不跟随跳转。
func TestDoNoFollowRedirect(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/final" {
			_, _ = w.Write([]byte("final"))
			return
		}
		http.Redirect(w, r, "/final", http.StatusFound)
	}))
	defer srv.Close()

	c := New(Config{})

	resp, err := c.Do(context.Background(), Request{Method: "GET", URL: srv.URL, NoFollowRedirect: true})
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	if resp.Status != http.StatusFound {
		t.Errorf("状态码 = %d, 期望 302", resp.Status)
	}
	if resp.Redirects != 0 {
		t.Errorf("Redirects = %d, 期望 0", resp.Redirects)
	}

	resp, err = c.Do(context.Background(), Request{Method: "GET", URL: srv.URL})
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	if resp.Status != 200 || string(resp.Body) != "final" {
		t.Errorf("跟随跳转后 status=%d body=%q", resp.Status, resp.Body)
	}
	if resp.Redirects != 1 {
		t.Errorf("Redirects = %d, 期望 1", resp.Redirects)
	}
}

// TestDoBinaryDetection 验证二进制内容不被当作文本。
func TestDoBinaryDetection(t *testing.T) {
	payload := []byte{0x89, 0x50, 0x4e, 0x47, 0x00, 0x01, 0x02}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(payload)
	}))
	defer srv.Close()

	c := New(Config{})
	resp, err := c.Do(context.Background(), Request{Method: "GET", URL: srv.URL})
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	if resp.IsText {
		t.Error("image/png 含 NUL 字节，不应被判定为文本")
	}
	if len(resp.Body) != len(payload) {
		t.Errorf("响应体长度 = %d, 期望 %d", len(resp.Body), len(payload))
	}
}

// TestDoTruncatesLargeBody 验证超过读取上限时截断并打标。
func TestDoTruncatesLargeBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write([]byte(strings.Repeat("x", 2048)))
	}))
	defer srv.Close()

	c := New(Config{MaxBodyBytes: 100})
	resp, err := c.Do(context.Background(), Request{Method: "GET", URL: srv.URL})
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	if !resp.Truncated {
		t.Error("超出上限时 Truncated 应为 true")
	}
	if len(resp.Body) != 100 {
		t.Errorf("响应体长度 = %d, 期望 100", len(resp.Body))
	}
}

// TestDoTimeout 验证超时参数生效。
func TestDoTimeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/slow" {
			time.Sleep(1500 * time.Millisecond)
		}
		_, _ = w.Write([]byte("late"))
	}))
	defer srv.Close()

	c := New(Config{})

	// 1 秒超时对 1.5 秒的响应应当失败。
	if _, err := c.Do(context.Background(), Request{Method: "GET", URL: srv.URL + "/slow", TimeoutSec: 1}); err == nil {
		t.Fatal("预期超时错误")
	}

	// 5 秒超时对同一响应应当成功，确认失败确实来自超时而非别的原因。
	resp, err := c.Do(context.Background(), Request{Method: "GET", URL: srv.URL + "/slow", TimeoutSec: 5})
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	if string(resp.Body) != "late" {
		t.Errorf("响应体 = %q", resp.Body)
	}
}

// TestDoValidation 验证非法输入的报错信息面向用户、可读。
func TestDoValidation(t *testing.T) {
	c := New(Config{})
	cases := []struct {
		name string
		req  Request
		want string
	}{
		{"空地址", Request{Method: "GET"}, "请求地址不能为空"},
		{"不支持的方法", Request{Method: "FETCH", URL: "http://example.com"}, "不支持的请求方法"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := c.Do(context.Background(), tc.req)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("err = %v, 期望包含 %q", err, tc.want)
			}
		})
	}

	// 缺少 scheme 时按 https 补全；缺少主机名才算错。
	if _, err := c.Do(context.Background(), Request{Method: "GET", URL: "/only/path"}); err == nil {
		t.Error("缺少主机名时应报错")
	}
}

// TestDoCookies 验证响应 Cookie 被解析出来。
func TestDoCookies(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.SetCookie(w, &http.Cookie{Name: "sid", Value: "abc"})
		http.SetCookie(w, &http.Cookie{Name: "theme", Value: "dark"})
		_, _ = w.Write([]byte("ok"))
	}))
	defer srv.Close()

	c := New(Config{})
	resp, err := c.Do(context.Background(), Request{Method: "GET", URL: srv.URL})
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	if !hasHeader(resp.Cookies, "sid", "abc") || !hasHeader(resp.Cookies, "theme", "dark") {
		t.Errorf("Cookie 解析不完整: %+v", resp.Cookies)
	}
}

// TestRequestNormalize 验证零值请求被补齐为 GET + none。
func TestRequestNormalize(t *testing.T) {
	r := Request{}
	r.Normalize()
	if r.Method != "GET" || r.BodyKind != BodyNone || r.Auth.Kind != AuthNone {
		t.Errorf("Normalize 结果 = %+v", r)
	}
}

// TestIsTextual 覆盖文本判定的边界：类型缺失时靠字节判断，压缩数据不被误判。
func TestIsTextual(t *testing.T) {
	cases := []struct {
		ct   string
		body []byte
		want bool
	}{
		{"text/html; charset=utf-8", []byte("<p>x</p>"), true},
		{"application/json", []byte(`{}`), true},
		{"application/vnd.api+json", []byte(`{}`), true},
		{"", []byte("中文内容"), true},
		{"application/octet-stream", []byte("普通文本"), true},
		{"application/octet-stream", []byte{0x1f, 0x8b, 0x00}, false},
		{"application/octet-stream", []byte{0xff, 0xfe, 0xfd}, false},
	}
	for _, tc := range cases {
		if got := isTextual(tc.ct, tc.body); got != tc.want {
			t.Errorf("isTextual(%q, %v) = %v, 期望 %v", tc.ct, tc.body, got, tc.want)
		}
	}
}

// TestDoReturnsUnmarshalableResponseStruct 确认响应结构可被序列化给前端
// （Wails 绑定会把返回值编成 JSON，字段标签写错会在此暴露）。
func TestDoReturnsUnmarshalableResponseStruct(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"n":1}`))
	}))
	defer srv.Close()

	c := New(Config{})
	resp, err := c.Do(context.Background(), Request{Method: "GET", URL: srv.URL})
	if err != nil {
		t.Fatalf("Do: %v", err)
	}

	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("序列化响应: %v", err)
	}
	var back map[string]any
	if err := json.Unmarshal(data, &back); err != nil {
		t.Fatalf("反序列化响应: %v", err)
	}
	if back["status"].(float64) != 200 {
		t.Errorf("status 字段 = %v", back["status"])
	}
}

// hasHeader 判断键值列表里是否存在指定条目。
func hasHeader(list []KV, name, value string) bool {
	for _, kv := range list {
		if strings.EqualFold(kv.Name, name) && kv.Value == value {
			return true
		}
	}
	return false
}
