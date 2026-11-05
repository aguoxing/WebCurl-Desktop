package updater

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// TestNewHTTPClientRoutesThroughProxy 验证显式代理确实被用上。
//
// 目标地址用的是解析不了的主机名，因此请求能成功只可能来自「走了代理」：
// 直连会在 Dial 阶段就失败。这一条对应的真实场景是本地代理客户端开着、
// 系统代理也开着，而 Go 不会读系统代理，更新于是直连
// release-assets.githubusercontent.com 超时。
func TestNewHTTPClientRoutesThroughProxy(t *testing.T) {
	var hits int32

	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		// 正向代理收到的是绝对 URL 形式的请求行，这是它区别于「被访问的站点」的标志。
		if !r.URL.IsAbs() {
			t.Errorf("代理收到的请求行不是绝对 URL：%s", r.RequestURI)
		}
		_, _ = w.Write([]byte("via proxy"))
	}))
	defer proxy.Close()

	client, err := newHTTPClient(5*time.Second, proxy.URL)
	if err != nil {
		t.Fatalf("构造客户端失败: %v", err)
	}

	resp, err := client.Get("http://asset.invalid/webcurl-desktop.zip")
	if err != nil {
		t.Fatalf("请求应当经代理成功，却失败了: %v", err)
	}
	defer resp.Body.Close()

	if got := atomic.LoadInt32(&hits); got != 1 {
		t.Errorf("代理被访问 %d 次，期望 1 次", got)
	}
}

// TestNewHTTPClientAcceptsBareHostPort 覆盖「只填 host:port」这种最常见的手填形式。
func TestNewHTTPClientAcceptsBareHostPort(t *testing.T) {
	client, err := newHTTPClient(time.Second, "127.0.0.1:7890")
	if err != nil {
		t.Fatalf("不接受 host:port 形式: %v", err)
	}

	transport, ok := client.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("Transport 类型为 %T，期望 *http.Transport", client.Transport)
	}
	req, err := http.NewRequest(http.MethodGet, "https://api.github.com/x", nil)
	if err != nil {
		t.Fatalf("构造请求失败: %v", err)
	}
	proxyURL, err := transport.Proxy(req)
	if err != nil {
		t.Fatalf("解析代理失败: %v", err)
	}
	if proxyURL == nil || proxyURL.String() != "http://127.0.0.1:7890" {
		t.Errorf("代理 = %v，期望 http://127.0.0.1:7890", proxyURL)
	}
}

// TestNewHTTPClientRejectsBadProxy 保证填错时立刻报错，而不是等请求超时。
func TestNewHTTPClientRejectsBadProxy(t *testing.T) {
	for _, in := range []string{"http://", "://nohost", "http://%zz"} {
		if _, err := newHTTPClient(time.Second, in); err == nil {
			t.Errorf("代理 %q 应当被拒绝", in)
		}
	}
}

// TestNewHTTPClientEmptyProxyKeepsEnvironment 覆盖「留空跟随环境变量」。
//
// 这里不断言最终代理值：它取决于运行环境是否设了 HTTP_PROXY，断言具体值会让
// 测试在开发机与 CI 上表现不同。要守的是留空不报错、且仍走 DefaultTransport 的
// ProxyFromEnvironment（而不是被误设成「不走代理」）。
func TestNewHTTPClientEmptyProxyKeepsEnvironment(t *testing.T) {
	client, err := newHTTPClient(time.Second, "   ")
	if err != nil {
		t.Fatalf("空代理不该报错: %v", err)
	}
	transport, ok := client.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("Transport 类型为 %T，期望 *http.Transport", client.Transport)
	}
	if transport.Proxy == nil {
		t.Fatal("Proxy 函数为 nil，等于强制直连，环境变量会被忽略")
	}

	// 环回地址一律直连（Go 内建规则），用它确认 Proxy 仍是环境变量那一套实现。
	req, _ := http.NewRequest(http.MethodGet, "http://127.0.0.1:9/x", nil)
	if u, err := transport.Proxy(req); err != nil || u != nil {
		t.Errorf("环回地址应当直连，得到 %v（err=%v）", u, err)
	}
}

// TestTransportErrorHidesSignedURL 守错误信息：GitHub 的附件地址是一长串签名
// URL，不能整段摊到界面上。
//
// 直接构造 url.Error 而不是真发请求：这是 http.Client 在超时时返回的类型，
// 而真发请求会让用例依赖外网可达性。
func TestTransportErrorHidesSignedURL(t *testing.T) {
	const signed = "https://release-assets.githubusercontent.com/github-production-release-asset/123?sp=r&sig=SECRET"

	cases := []struct {
		name   string
		err    error
		expect string
	}{
		{
			name:   "超时",
			err:    &url.Error{Op: "Get", URL: signed, Err: context.DeadlineExceeded},
			expect: "连接超时",
		},
		{
			// Go 在 Client.Timeout 触发时给的是带这句文案的错误，不是裸的 context 错误。
			name:   "客户端超时",
			err:    &url.Error{Op: "Get", URL: signed, Err: errors.New("net/http: request canceled while awaiting headers (Client.Timeout exceeded while awaiting headers)")},
			expect: "连接超时",
		},
		{
			name:   "域名解析失败",
			err:    &url.Error{Op: "Get", URL: signed, Err: errors.New("dial tcp: lookup release-assets.githubusercontent.com: no such host")},
			expect: "域名解析失败",
		},
	}

	for _, c := range cases {
		msg := transportError("checksum request", c.err).Error()
		if strings.Contains(msg, "SECRET") || strings.Contains(msg, "sp=r") {
			t.Errorf("%s：错误信息泄漏了签名串: %s", c.name, msg)
		}
		if !strings.Contains(msg, "release-assets.githubusercontent.com") {
			t.Errorf("%s：错误信息丢了主机名，无法判断是哪台服务器不通: %s", c.name, msg)
		}
		if !strings.Contains(msg, c.expect) {
			t.Errorf("%s：错误信息里没有 %q: %s", c.name, c.expect, msg)
		}
	}
}

// TestTransportErrorKeepsNonURLDetail 确认非 url.Error 的情形不被吞掉细节。
func TestTransportErrorKeepsNonURLDetail(t *testing.T) {
	msg := transportError("checksum request", errors.New("签名校验失败")).Error()
	if msg != "checksum request: 签名校验失败" {
		t.Errorf("得到 %q", msg)
	}
}

// flakyTransport 前 fail 次返回传输层错误，之后成功。
//
// 用假 Transport 而不是真连接：真连接下 Go 的 Transport 自己也可能重试一次，
// 那样测试就分不清成功来自我们的重试还是标准库的。
type flakyTransport struct {
	remaining int32
	hits      int32
}

func (f *flakyTransport) RoundTrip(*http.Request) (*http.Response, error) {
	atomic.AddInt32(&f.hits, 1)
	if atomic.AddInt32(&f.remaining, -1) >= 0 {
		return nil, &url.Error{Op: "Get", URL: "https://example.invalid/x", Err: errors.New("connection reset by peer")}
	}
	return &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader("sha256:abc")),
	}, nil
}

// TestGetMetadataRetriesTransportError 覆盖链路抖动：一次传输失败之后重试成功，
// 整次更新检查不该因此作废。截图里那次超时正是这种情形。
func TestGetMetadataRetriesTransportError(t *testing.T) {
	transport := &flakyTransport{remaining: 1}
	client := &http.Client{Transport: transport}

	resp, err := getMetadata(client, "checksum request", "https://example.invalid/x")
	if err != nil {
		t.Fatalf("重试应当成功，却返回错误: %v", err)
	}
	defer resp.Body.Close()

	if got := atomic.LoadInt32(&transport.hits); got != 2 {
		t.Errorf("请求次数 = %d，期望 2（一次失败 + 一次重试）", got)
	}
}

// TestGetMetadataGivesUpAfterAttempts 确认重试次数有上限，不会无限重试。
func TestGetMetadataGivesUpAfterAttempts(t *testing.T) {
	transport := &flakyTransport{remaining: metadataAttempts + 5}
	client := &http.Client{Transport: transport}

	_, err := getMetadata(client, "checksum request", "https://example.invalid/x")
	if err == nil {
		t.Fatal("一直失败时应当返回错误")
	}
	if got := atomic.LoadInt32(&transport.hits); got != metadataAttempts {
		t.Errorf("请求次数 = %d，期望 %d", got, metadataAttempts)
	}
	// 传输错误的形状要保留可读性：说明是哪个环节、哪台主机。
	if !strings.Contains(err.Error(), "checksum request") {
		t.Errorf("错误信息缺少环节说明: %v", err)
	}
	if !strings.Contains(err.Error(), "example.invalid") {
		t.Errorf("错误信息缺少主机名: %v", err)
	}
}

// TestGetMetadataDoesNotRetryHTTPError 确认状态码不触发重试。
func TestGetMetadataDoesNotRetryHTTPError(t *testing.T) {
	var hits int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	client, err := newHTTPClient(5*time.Second, "")
	if err != nil {
		t.Fatalf("构造客户端失败: %v", err)
	}

	resp, err := getMetadata(client, "checksum request", server.URL)
	if err != nil {
		t.Fatalf("404 是服务端的明确回答，不该作为传输错误返回: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("状态码 = %d，期望 404", resp.StatusCode)
	}
	if got := atomic.LoadInt32(&hits); got != 1 {
		t.Errorf("请求次数 = %d，期望 1（状态码不该重试）", got)
	}
}
