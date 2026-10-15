package updater

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"runtime"
	"strings"
	"time"
)

// apiResponse 同时兼容自定义更新服务可能采用的两种响应结构：
// 扁平结构（checksum/notes 在顶层）与嵌套结构（artifact.filename + verification.digest）。
// 两者都在 docs/updating.md 中有文档。
// HasUpdate 用指针，是为了区分"字段缺失"与"服务端明确说没有新版本"。
type apiResponse struct {
	Version      string `json:"version"`
	HasUpdate    *bool  `json:"has_update"`
	URL          string `json:"url"`
	Checksum     string `json:"checksum"`
	Signature    string `json:"signature"`
	ReleaseNotes string `json:"release_notes"`
	Notes        string `json:"notes"`
	Size         int64  `json:"size"`
	Artifact     struct {
		Filename string `json:"filename"`
		Size     int64  `json:"size"`
	} `json:"artifact"`
	Verification struct {
		DigestAlgo string `json:"digestAlgo"`
		Digest     string `json:"digest"`
		Signature  string `json:"signature"`
	} `json:"verification"`
}

// Check 向自定义 HTTP API 查询是否有新版本。
//
//	GET {apiURL}?version={currentVersion}&platform={os}&arch={arch}
//
// 服务端报告无更新时返回 (nil, nil)，最常见的形式是 HTTP 204/304。
// 服务端可以忽略 platform/arch 参数，客户端只把它们当作附带信息。
func Check(apiURL string, currentVersion string) (*UpdateInfo, error) {
	if apiURL == "" {
		return nil, fmt.Errorf("update API URL not configured")
	}

	endpoint, err := url.Parse(apiURL)
	if err != nil {
		return nil, fmt.Errorf("parse update API URL: %w", err)
	}
	q := endpoint.Query()
	q.Set("version", currentVersion)
	q.Set("platform", runtime.GOOS)
	q.Set("arch", runtime.GOARCH)
	endpoint.RawQuery = q.Encode()

	// 设置超时上限：更新服务无响应时不能让应用一直挂住。
	client := &http.Client{Timeout: 15 * time.Second}
	req, err := http.NewRequest(http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("build update request: %w", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotModified || resp.StatusCode == http.StatusNoContent {
		return nil, nil
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API returned HTTP %d", resp.StatusCode)
	}

	var raw apiResponse
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxAPIResponseSize)).Decode(&raw); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	return raw.toUpdateInfo()
}

// toUpdateInfo 把两种响应结构折叠成统一的 UpdateInfo。
// 缺少 version 字段，或明确带 has_update=false，都表示"已是最新"。
func (r apiResponse) toUpdateInfo() (*UpdateInfo, error) {
	if r.Version == "" {
		return nil, nil
	}
	if r.HasUpdate != nil && !*r.HasUpdate {
		return nil, nil
	}

	info := &UpdateInfo{
		HasUpdate:    true,
		Version:      strings.TrimPrefix(strings.TrimSpace(r.Version), "v"),
		URL:          firstNonEmpty(r.URL, r.Artifact.Filename),
		ReleaseNotes: firstNonEmpty(r.ReleaseNotes, r.Notes),
		Size:         r.Size,
	}
	if info.URL == "" {
		return nil, fmt.Errorf("update response carries no artifact URL")
	}
	if info.Size == 0 {
		info.Size = r.Artifact.Size
	}

	// 两者同时存在时，扁平的 checksum 优先于嵌套的 verification。
	// 摘要格式非法一律视为硬错误：静默地不校验就安装，比拒绝这次更新更糟。
	switch {
	case r.Checksum != "":
		checksum, err := normalizeChecksum(r.Checksum)
		if err != nil {
			return nil, fmt.Errorf("invalid checksum in update response: %w", err)
		}
		info.Checksum = checksum
	case r.Verification.Digest != "":
		checksum, err := normalizeDigest(r.Verification.DigestAlgo, r.Verification.Digest)
		if err != nil {
			return nil, fmt.Errorf("invalid verification block in update response: %w", err)
		}
		info.Checksum = checksum
	}

	// 签名同样先看扁平字段，再退回嵌套结构。这里只做格式校验 ——
	// 是否"必需"由 Apply 按有没有钉住公钥决定。
	signature := firstNonEmpty(r.Signature, r.Verification.Signature)
	if signature != "" {
		if _, err := decodeSignature(signature); err != nil {
			return nil, fmt.Errorf("invalid signature in update response: %w", err)
		}
		info.Signature = signature
	}

	return info, nil
}

// firstNonEmpty 返回第一个非空白字符串。
func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}
