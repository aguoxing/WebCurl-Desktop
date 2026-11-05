package updater

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// newHTTPClient 构造更新流程使用的 HTTP 客户端。
//
// proxy 为空时沿用 http.DefaultTransport 的行为，即只认 HTTP_PROXY /
// HTTPS_PROXY / NO_PROXY 环境变量。Windows 控制面板里的「系统代理」开关 Go
// 不会自动读取，于是出现一种很典型的症状：本地代理客户端开着、系统代理也开着，
// 浏览器能开 GitHub，而更新检查直连 release-assets.githubusercontent.com 后超时。
// 设置页因此提供一个显式代理输入框。
//
// 每次都 Clone DefaultTransport 而不是复用同一个 Transport：调用方各自持有
// 不同超时的客户端，共用 Transport 会让连接池与超时互相牵连。
func newHTTPClient(timeout time.Duration, proxy string) (*http.Client, error) {
	transport := http.DefaultTransport.(*http.Transport).Clone()

	if proxy = strings.TrimSpace(proxy); proxy != "" {
		// 只填 "127.0.0.1:7890" 是很常见的写法，补默认协议再解析。
		if !strings.Contains(proxy, "://") {
			proxy = "http://" + proxy
		}
		target, err := url.Parse(proxy)
		if err != nil {
			return nil, fmt.Errorf("parse proxy %q: %w", proxy, err)
		}
		if target.Host == "" {
			return nil, fmt.Errorf("proxy %q has no host", proxy)
		}
		transport.Proxy = http.ProxyURL(target)
	}

	return &http.Client{Timeout: timeout, Transport: transport}, nil
}

// metadataAttempts 是元数据请求（SHA256SUMS、签名）的总尝试次数。
const metadataAttempts = 2

// metadataRetryDelay 是两次尝试之间的等待。链路抖动通常持续不到一秒，
// 而这里取的元数据只有几百字节，短暂退避后重试基本就能过。
const metadataRetryDelay = 300 * time.Millisecond

// getMetadata 发起一次元数据 GET，失败时重试。
//
// 只对传输层错误重试：HTTP 状态码是服务端的明确回答（404 就是没有），重试没有意义。
// 之所以需要重试，是因为 SHA256SUMS 与签名都经 release-assets.githubusercontent.com
// 分发，这条链路时不时在建立连接阶段卡住十几秒；一次抖动就让整次更新检查失败，
// 代价远大于多等 300 毫秒。
func getMetadata(client *http.Client, op, target string) (*http.Response, error) {
	var lastErr error

	for attempt := 0; attempt < metadataAttempts; attempt++ {
		if attempt > 0 {
			time.Sleep(metadataRetryDelay)
		}

		req, err := http.NewRequest(http.MethodGet, target, nil)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", op, err)
		}
		resp, err := client.Do(req)
		if err != nil {
			lastErr = transportError(op, err)
			continue
		}
		return resp, nil
	}

	return nil, lastErr
}
