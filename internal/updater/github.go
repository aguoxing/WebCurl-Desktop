package updater

import (
	"bufio"
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"webcurl-desktop/internal/logger"
)

// githubRelease 是 GitHub Releases API 响应中我们会用到的字段子集。
type githubRelease struct {
	TagName string        `json:"tag_name"`
	Name    string        `json:"name"`
	Body    string        `json:"body"`
	Assets  []githubAsset `json:"assets"`
}

type githubAsset struct {
	Name               string `json:"name"`
	Size               int64  `json:"size"`
	BrowserDownloadURL string `json:"browser_download_url"`
}

// 更新源的端点前缀。抽成变量是为了让测试能把它们指向本地服务 ——
// CheckGitHub 的两条路径（REST API 与公开网页）都值得端到端覆盖。
var (
	githubAPIBase = "https://api.github.com"
	githubWebBase = "https://github.com"
)

// CheckGitHub 查询 GitHub Releases 是否有新版本，repo 形如 "owner/repo"。
//
// 当 release 里带有 SHA256SUMS 附件时，会从中读出所选附件的摘要；
// 否则 Checksum 保持为空，记录一条警告后按"不校验"继续（见 Apply）。
// 公开仓库的 token 可为空。
func CheckGitHub(repo, token, currentVersion string) (*UpdateInfo, error) {
	if repo == "" {
		return nil, fmt.Errorf("github repository not configured")
	}

	endpoint := fmt.Sprintf("%s/repos/%s/releases/latest", githubAPIBase, repo)
	req, err := http.NewRequest(http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("build github request: %w", err)
	}
	// 固定 API 版本，避免 GitHub 将来更换默认值导致响应结构悄悄变化。
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("github request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		// 把响应体带上：GitHub 会在里面说明限流或凭据无效的原因。
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		text := strings.TrimSpace(string(body))

		// 匿名额度用尽不是错误，而是可以绕开的情况：改走公开页面与下载路径。
		if isRateLimited(resp, text) {
			logger.Warn().
				Str("repo", repo).
				Msg("updater: github API rate limit reached, falling back to the public release pages")
			return checkGitHubViaWeb(repo, currentVersion)
		}
		return nil, fmt.Errorf("github API HTTP %d: %s", resp.StatusCode, text)
	}

	var rel githubRelease
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxAPIResponseSize)).Decode(&rel); err != nil {
		return nil, fmt.Errorf("decode github response: %w", err)
	}

	tag := strings.TrimPrefix(rel.TagName, "v")
	if tag == "" || !versionNewer(tag, currentVersion) {
		return nil, nil
	}

	asset, ok := pickAsset(rel.Assets)
	if !ok {
		// 以前这里会退回到 zipball_url，也就是源码压缩包，
		// 它随后会被 rename 覆盖正在运行的二进制。宁可跳过。
		logger.Warn().
			Str("repo", repo).
			Str("version", tag).
			Str("platform", runtime.GOOS+"/"+runtime.GOARCH).
			Msg("updater: release has no asset matching this platform, skipping")
		return nil, nil
	}

	info := &UpdateInfo{
		HasUpdate:    true,
		Version:      tag,
		URL:          asset.BrowserDownloadURL,
		Size:         asset.Size,
		ReleaseNotes: rel.Body,
		AssetName:    asset.Name,
	}

	sumsURL := findChecksumAsset(rel.Assets)
	if sumsURL == "" {
		logger.Warn().
			Str("repo", repo).
			Str("version", tag).
			Str("asset", asset.Name).
			Msg("updater: release has no SHA256SUMS asset, update will not be verified")
	} else {
		checksum, err := fetchChecksum(client, sumsURL, asset.Name)
		if err != nil {
			// 清单存在，说明这次发布声称可校验：读不到就应当报错，
			// 而不是降级为"不校验"。
			return nil, fmt.Errorf("read SHA256SUMS for %s: %w", asset.Name, err)
		}
		info.Checksum = checksum
	}

	// 签名附件是可选发布的：拿不到就留空，是否因此拒绝安装由 Apply 按
	// "有没有钉住公钥"决定。
	if sigURL := findSignatureAsset(rel.Assets, asset.Name); sigURL != "" {
		signature, err := fetchSignature(client, sigURL)
		if err != nil {
			// 与摘要同理：附件存在就意味着这次发布声称有签名，
			// 读不到必须报错，不能让网络故障降级成"未签名"。
			return nil, fmt.Errorf("read signature for %s: %w", asset.Name, err)
		}
		info.Signature = signature
	}

	return info, nil
}

// pickAsset 为当前平台挑选 release 附件，校验清单与签名永远不会被选中。
func pickAsset(assets []githubAsset) (githubAsset, bool) {
	for _, a := range assets {
		if matchesPlatform(a.Name) {
			return a, true
		}
	}
	return githubAsset{}, false
}

// matchesPlatform 判断一个发布包的文件名是否属于当前平台。
//
// 匹配刻意放宽，因为发布包名称是手工起的：名字里要包含操作系统，以及 Go 的
// 架构名或它的常见别名（x86_64 / aarch64）。校验清单与签名永不匹配。
//
// 单独抽出来是因为它有两个调用方：API 路径从附件列表里挑，公开下载路径从
// SHA256SUMS 的条目里挑（见 pickPlatformAsset）。
func matchesPlatform(name string) bool {
	lower := strings.ToLower(name)
	if isChecksumAsset(lower) || isSignatureAsset(lower) || !strings.Contains(lower, runtime.GOOS) {
		return false
	}
	switch {
	case strings.Contains(lower, runtime.GOARCH):
	case runtime.GOARCH == "amd64" && strings.Contains(lower, "x86_64"):
	case runtime.GOARCH == "arm64" && strings.Contains(lower, "aarch64"):
	default:
		return false
	}
	return true
}

// findChecksumAsset 返回 release 中校验清单的下载地址；没有则返回空串。
func findChecksumAsset(assets []githubAsset) string {
	for _, a := range assets {
		if isChecksumAsset(strings.ToLower(a.Name)) {
			return a.BrowserDownloadURL
		}
	}
	return ""
}

// isChecksumAsset 判断 name（已转小写）是否像 SHA256SUMS 这类清单文件。
// 判定需要足够严格，不能误伤任何平台发布包。
func isChecksumAsset(lowerName string) bool {
	return strings.HasPrefix(lowerName, "sha256sums") ||
		strings.HasSuffix(lowerName, ".sha256") ||
		strings.HasSuffix(lowerName, "sha256sums.txt")
}

// signatureCandidates 列出发布包签名可能使用的文件名，按试探顺序。
//
// 命名有两种常见写法，都接受：
//   - `foo.zip` → `foo.sig`（发布脚本采用这种，把 .sig 追加在去掉扩展名的名字后）；
//   - `foo.zip` → `foo.zip.sig`。
//
// 顺序固定（先 .sig 后 .signature），这样同一份 release 上的结果稳定，
// 而不像 map 遍历那样随机；公开下载路径也靠它逐个试探。
func signatureCandidates(assetName string) []string {
	lowerAsset := strings.ToLower(assetName)
	base := strings.TrimSuffix(lowerAsset, filepath.Ext(lowerAsset))
	return []string{
		base + ".sig",
		base + ".signature",
		lowerAsset + ".sig",
		lowerAsset + ".signature",
	}
}

// findSignatureAsset 返回 release 中发布包签名的下载地址；没有则返回空串。
func findSignatureAsset(assets []githubAsset, assetName string) string {
	for _, want := range signatureCandidates(assetName) {
		for _, a := range assets {
			if strings.EqualFold(a.Name, want) {
				return a.BrowserDownloadURL
			}
		}
	}
	return ""
}

// fetchSignature 下载签名附件并返回其文本表示。
//
// 发布脚本写出的是裸二进制 `.sig`（Ed25519 固定 64 字节），而更新清单里的
// Signature 字段承载的是文本。这里做一个归一化：附件本身已经是 hex/base64 文本就
// 原样保留，否则按裸二进制转成 base64。这样后续只需一套解码逻辑。
func fetchSignature(client *http.Client, url string) (string, error) {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return "", fmt.Errorf("build signature request: %w", err)
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("signature request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("signature HTTP %d", resp.StatusCode)
	}

	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxSignatureFileSize+1))
	if err != nil {
		return "", fmt.Errorf("read signature: %w", err)
	}
	if len(raw) > maxSignatureFileSize {
		return "", fmt.Errorf("signature file exceeds %d bytes", maxSignatureFileSize)
	}

	// 先按文本尝试：首尾去空白后能解出 64 字节，说明附件本身就是文本。
	//
	// 注意不能对 raw 本身做裁剪再回退 —— 裸二进制签名的首/末字节完全可能
	// 恰是空白字节（0x20/0x09/0x0a 等），裁剪会把签名改坏，
	// 并且这种损坏要到验签时才暴露，排查成本很高。
	text := bytes.TrimSpace(raw)
	if _, err := decodeSignature(string(text)); err == nil {
		return string(text), nil
	}
	return base64.StdEncoding.EncodeToString(raw), nil
}

// fetchChecksum 下载清单并取出 assetName 对应的摘要。
func fetchChecksum(client *http.Client, url, assetName string) (string, error) {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return "", fmt.Errorf("build checksum request: %w", err)
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("checksum request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("checksum HTTP %d", resp.StatusCode)
	}
	return parseChecksumFile(io.LimitReader(resp.Body, maxChecksumFileSize), assetName)
}

// parseChecksumFile 解析 `sha256sum` 的输出 —— "<hex>  <文件名>"，
// 兼容二进制模式的 "*" 前缀 —— 并返回 assetName 对应的 "sha256:<hex>"。
// 只比较文件名部分，因此清单里写路径也能匹配上。
func parseChecksumFile(r io.Reader, assetName string) (string, error) {
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) < 2 {
			continue
		}
		name := strings.TrimPrefix(fields[len(fields)-1], "*")
		if !strings.EqualFold(filepath.Base(name), assetName) {
			continue
		}
		checksum, err := normalizeChecksum(fields[0])
		if err != nil {
			return "", err
		}
		return checksum, nil
	}
	if err := scanner.Err(); err != nil {
		return "", err
	}
	return "", fmt.Errorf("no checksum entry for %q", assetName)
}

// ── 公开页面路径 ─────────────────────────────────────────────────────────────
//
// GitHub 的 REST API 对匿名请求只给 60 次/小时，额度按**出口 IP** 计算 ——
// 公司 NAT、机房代理这类共享出口很容易被别人用光，表现为 403 rate limit。
// 下面这组函数用 github.com 的普通网页与下载路径完成同一次检查，完全不经过
// API，因此没有这个上限。
//
// 代价是拿不到 release 正文，而「这次发布了哪些文件」改从 SHA256SUMS 里读回来：
// 那份清单本来就要下载，正好可以兼作附件清单。

// isRateLimited 判断这次失败是不是「额度用尽」。
//
// 429 的语义本身就是「请求过多」，直接判定；403 则要看证据。证据优先取响应头
// X-RateLimit-Remaining: 0 —— 它是稳定的契约，而正文措辞会随 GitHub 改文案而
// 失效，所以正文里的 "rate limit" 只作兜底。
func isRateLimited(resp *http.Response, body string) bool {
	if resp.StatusCode == http.StatusTooManyRequests {
		return true
	}
	if resp.StatusCode != http.StatusForbidden {
		return false
	}
	if resp.Header.Get("X-RateLimit-Remaining") == "0" {
		return true
	}
	return strings.Contains(strings.ToLower(body), "rate limit")
}

// checkGitHubViaWeb 走公开页面与下载路径完成一次检查。
func checkGitHubViaWeb(repo, currentVersion string) (*UpdateInfo, error) {
	client := &http.Client{Timeout: 15 * time.Second}

	tag, err := latestReleaseTag(client, repo)
	if err != nil {
		return nil, err
	}

	version := strings.TrimPrefix(tag, "v")
	if version == "" || !versionNewer(version, currentVersion) {
		return nil, nil
	}

	base := fmt.Sprintf("%s/%s/releases/download/%s", githubWebBase, repo, tag)

	// 资产名从清单里挑，而不是按约定硬拼 —— 命名规则只存在于发布脚本里，
	// 在这里再复制一份迟早会漂移。
	const checksumName = "SHA256SUMS"
	entries, err := fetchChecksumEntries(client, base+"/"+checksumName)
	if err != nil {
		return nil, fmt.Errorf("read %s for %s: %w", checksumName, tag, err)
	}

	assetName, checksum := pickPlatformAsset(entries)
	if assetName == "" {
		logger.Warn().
			Str("repo", repo).
			Str("version", version).
			Str("platform", runtime.GOOS+"/"+runtime.GOARCH).
			Msg("updater: release has no asset matching this platform, skipping")
		return nil, nil
	}

	info := &UpdateInfo{
		HasUpdate: true,
		Version:   version,
		URL:       base + "/" + assetName,
		Checksum:  checksum,
		AssetName: assetName,
	}

	// 签名是可选发布的：逐个试探候选文件名，都 404 就当作未签名，
	// 是否因此拒绝安装交给 Apply 按「有没有钉住公钥」判断。
	sigURL, err := findSignatureViaWeb(client, base, assetName)
	if err != nil {
		return nil, err
	}
	if sigURL != "" {
		signature, err := fetchSignature(client, sigURL)
		if err != nil {
			// 与 API 路径同理：附件在就说明这次发布声称有签名，读不到必须报错。
			return nil, fmt.Errorf("read signature for %s: %w", assetName, err)
		}
		info.Signature = signature
	}

	return info, nil
}

// latestReleaseTag 从公开的 /releases/latest 端点解析出最新 release 的 tag。
//
// 这个端点不经过 API：github.com 回一个 302 到 /releases/tag/<tag>，因此不消耗
// 匿名额度。这里刻意**不跟随重定向** —— 需要的只是 Location 头里的 tag，抓下整个
// HTML 页面既慢又没必要，还会多出一个「页面渲染失败」的失败面。
//
// 仓库还没有任何 release 时不会出现 /releases/tag/ 段，此时返回空串。
func latestReleaseTag(client *http.Client, repo string) (string, error) {
	endpoint := fmt.Sprintf("%s/%s/releases/latest", githubWebBase, repo)

	noRedirect := *client
	noRedirect.CheckRedirect = func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}

	resp, err := noRedirect.Get(endpoint)
	if err != nil {
		return "", fmt.Errorf("github release page: %w", err)
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))

	// 200 表示没有最新 release（github.com 直接渲染了 releases 列表）。
	if resp.StatusCode == http.StatusOK {
		return "", nil
	}
	if resp.StatusCode < 300 || resp.StatusCode >= 400 {
		return "", fmt.Errorf("github release page HTTP %d", resp.StatusCode)
	}

	location := resp.Header.Get("Location")
	if location == "" {
		return "", fmt.Errorf("github release page: redirect without a Location header")
	}

	const marker = "/releases/tag/"
	idx := strings.LastIndex(location, marker)
	if idx < 0 {
		return "", nil
	}

	tag := location[idx+len(marker):]
	if cut := strings.IndexAny(tag, "?#"); cut >= 0 {
		tag = tag[:cut]
	}
	return tag, nil
}

// pickPlatformAsset 从「文件名 → 摘要」里挑出属于当前平台的那个，同时返回
// 它的名字与摘要。按文件名排序后取第一个，让结果与 map 的遍历顺序无关。
//
// 返回的文件名保留清单里的原始大小写 —— 下载 URL 是大小写敏感的。
func pickPlatformAsset(entries map[string]string) (string, string) {
	names := make([]string, 0, len(entries))
	for name := range entries {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		if matchesPlatform(name) {
			return name, entries[name]
		}
	}
	return "", ""
}

// findSignatureViaWeb 在公开下载路径下逐个试探签名的候选文件名，
// 全部不存在时返回空串。
func findSignatureViaWeb(client *http.Client, base, assetName string) (string, error) {
	for _, name := range signatureCandidates(assetName) {
		url := base + "/" + name
		found, err := urlExists(client, url)
		if err != nil {
			return "", err
		}
		if found {
			return url, nil
		}
	}
	return "", nil
}

// urlExists 探测公开下载路径上是否存在该文件。
//
// 只有明确的 200 / 404 才算结论，其余状态码一律当作出错：拿不准时宁可报出来，
// 也不要让一个 429 或 5xx 被误解成「没有签名」而静默降级。
func urlExists(client *http.Client, url string) (bool, error) {
	req, err := http.NewRequest(http.MethodHead, url, nil)
	if err != nil {
		return false, fmt.Errorf("build request for %s: %w", url, err)
	}

	resp, err := client.Do(req)
	if err != nil {
		return false, fmt.Errorf("request %s: %w", url, err)
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))

	switch resp.StatusCode {
	case http.StatusOK:
		return true, nil
	case http.StatusNotFound:
		return false, nil
	default:
		return false, fmt.Errorf("HEAD %s: HTTP %d", url, resp.StatusCode)
	}
}

// fetchChecksumEntries 下载清单并返回「文件名 → 摘要」。
func fetchChecksumEntries(client *http.Client, url string) (map[string]string, error) {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("build checksum request: %w", err)
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("checksum request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("checksum HTTP %d", resp.StatusCode)
	}
	return parseChecksumEntries(io.LimitReader(resp.Body, maxChecksumFileSize))
}

// parseChecksumEntries 解析 `sha256sum` 的输出，返回「文件名 → sha256:<hex>」。
// 键保留清单里的原始大小写（下载 URL 大小写敏感），只取路径的文件名部分。
func parseChecksumEntries(r io.Reader) (map[string]string, error) {
	entries := make(map[string]string)

	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) < 2 {
			continue
		}
		name := filepath.Base(strings.TrimPrefix(fields[len(fields)-1], "*"))
		checksum, err := normalizeChecksum(fields[0])
		if err != nil {
			return nil, err
		}
		entries[name] = checksum
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return entries, nil
}
