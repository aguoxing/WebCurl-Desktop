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

// CheckGitHub 查询 GitHub Releases 是否有新版本，repo 形如 "owner/repo"。
//
// 当 release 里带有 SHA256SUMS 附件时，会从中读出所选附件的摘要；
// 否则 Checksum 保持为空，记录一条警告后按"不校验"继续（见 Apply）。
// 公开仓库的 token 可为空。
func CheckGitHub(repo, token, currentVersion string) (*UpdateInfo, error) {
	if repo == "" {
		return nil, fmt.Errorf("github repository not configured")
	}

	endpoint := fmt.Sprintf("https://api.github.com/repos/%s/releases/latest", repo)
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
		return nil, fmt.Errorf("github API HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
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
// 匹配刻意放宽，因为发布包名称是手工起的：名字里要包含操作系统，
// 以及 Go 的架构名或它的常见别名（x86_64 / aarch64）。
func pickAsset(assets []githubAsset) (githubAsset, bool) {
	for _, a := range assets {
		name := strings.ToLower(a.Name)
		if isChecksumAsset(name) || isSignatureAsset(name) || !strings.Contains(name, runtime.GOOS) {
			continue
		}
		switch {
		case strings.Contains(name, runtime.GOARCH):
		case runtime.GOARCH == "amd64" && strings.Contains(name, "x86_64"):
		case runtime.GOARCH == "arm64" && strings.Contains(name, "aarch64"):
		default:
			continue
		}
		return a, true
	}
	return githubAsset{}, false
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

// findSignatureAsset 返回 release 中发布包签名的下载地址；没有则返回空串。
//
// 命名有两种常见写法，都接受：
//   - `foo.zip` → `foo.sig`（发布脚本采用这种，把 .sig 追加在去掉扩展名的名字后）；
//   - `foo.zip` → `foo.zip.sig`。
func findSignatureAsset(assets []githubAsset, assetName string) string {
	lowerAsset := strings.ToLower(assetName)
	base := strings.TrimSuffix(lowerAsset, filepath.Ext(lowerAsset))

	wanted := map[string]bool{
		lowerAsset + ".sig":       true,
		lowerAsset + ".signature": true,
		base + ".sig":             true,
		base + ".signature":       true,
	}
	for _, a := range assets {
		if wanted[strings.ToLower(a.Name)] {
			return a.BrowserDownloadURL
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
