package updater

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"strings"
)

// normalizeDigest 把 {digestAlgo, digest} 校验块转成 "sha256:<小写十六进制>"。
// algo 为空时默认 sha256；其它算法直接拒绝，而不是悄悄忽略。
func normalizeDigest(algo, digest string) (string, error) {
	if algo == "" {
		algo = "sha256"
	}
	if !strings.EqualFold(algo, "sha256") {
		return "", fmt.Errorf("unsupported digest algorithm %q", algo)
	}
	sum, err := decodeHash(digest)
	if err != nil {
		return "", err
	}
	return "sha256:" + sum, nil
}

// normalizeChecksum 接受 "sha256:<hex>" 或不带前缀的摘要（十六进制/Base64），
// 统一返回规范形式 "sha256:<小写十六进制>"。
func normalizeChecksum(s string) (string, error) {
	s = strings.TrimSpace(s)
	if algo, rest, ok := strings.Cut(s, ":"); ok {
		if !strings.EqualFold(algo, "sha256") {
			return "", fmt.Errorf("unsupported checksum algorithm %q", algo)
		}
		s = rest
	}
	sum, err := decodeHash(s)
	if err != nil {
		return "", err
	}
	return "sha256:" + sum, nil
}

// decodeHash 接受十六进制或 Base64 的 sha256 摘要，返回小写十六进制。
// 先按十六进制尝试，因为那是 sha256sum 的输出格式；再按 Base64 尝试，
// 因为嵌套的 verification 结构在文档里就是这么描述的。
func decodeHash(s string) (string, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", fmt.Errorf("empty digest")
	}
	if isHex(s) {
		if len(s) != sha256.Size*2 {
			return "", fmt.Errorf("sha256 hex digest must be %d chars, got %d", sha256.Size*2, len(s))
		}
		return strings.ToLower(s), nil
	}
	raw, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		return "", fmt.Errorf("digest is neither hex nor base64: %w", err)
	}
	if len(raw) != sha256.Size {
		return "", fmt.Errorf("decoded digest is %d bytes, want %d", len(raw), sha256.Size)
	}
	return hex.EncodeToString(raw), nil
}

// isHex 判断 s 是否非空且全部由十六进制字符组成。
func isHex(s string) bool {
	for _, r := range s {
		switch {
		case r >= '0' && r <= '9':
		case r >= 'a' && r <= 'f':
		case r >= 'A' && r <= 'F':
		default:
			return false
		}
	}
	return true
}

// verifyChecksum 比较 path 所指文件的 SHA256 与 checksum（后者可以是
// normalizeChecksum 接受的任意形式）。
func verifyChecksum(path, checksum string) error {
	want, err := normalizeChecksum(checksum)
	if err != nil {
		return err
	}
	got, err := fileHash(path)
	if err != nil {
		return err
	}
	if !strings.EqualFold(got, strings.TrimPrefix(want, "sha256:")) {
		return fmt.Errorf("checksum mismatch: got sha256:%s want %s", got, want)
	}
	return nil
}

// fileHash 返回 path 所指文件的小写十六进制 SHA256。
func fileHash(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
