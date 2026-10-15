package updater

import (
	"crypto/ed25519"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/pem"
	"fmt"
	"os"
	"strings"
)

// EmbeddedPublicKey 是构建时通过 `-ldflags -X` 注入的 Ed25519 公钥，
// 用于验证发布包的 `.sig` 签名。取值形如 base64（原始 32 字节）或 PEM。
//
// 把它放在二进制里而不是配置文件里，是因为"信任锚"本身必须是不可远程篡改的：
// 若公钥只存在于随包分发的配置中，能改配置的攻击者就能换成自己的密钥，
// 签名校验也就形同虚设。未注入时为空串，此时签名校验整体不启用。
var EmbeddedPublicKey string

// TrustedPublicKey 决定实际使用的公钥：内嵌值优先于配置值。
//
// 内嵌值存在时配置被忽略，因此无法通过改配置文件把信任锚换掉。
// 两者都为空表示不启用签名校验（向后兼容）。
func TrustedPublicKey(configured string) string {
	if strings.TrimSpace(EmbeddedPublicKey) != "" {
		return EmbeddedPublicKey
	}
	return configured
}

// parsePublicKey 解析公钥。返回 nil 表示没有配置公钥，因此不启用校验
// ——这是一个合法的配置状态，不是错误。
//
// 支持三种写法，覆盖"从 openssl 复制粘贴"与"脚本单行注入"两种常见来源：
//   - base64 编码的原始 32 字节公钥（推荐：单行，适合 -ldflags 注入）；
//   - 十六进制编码的原始 32 字节公钥；
//   - PEM（SubjectPublicKeyInfo）文本，便于肉眼核对。
func parsePublicKey(s string) (ed25519.PublicKey, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, nil
	}

	if strings.HasPrefix(s, "-----BEGIN") {
		return parsePEMPublicKey(s)
	}

	raw, err := decodeKeyBytes(s)
	if err != nil {
		return nil, err
	}
	if len(raw) == ed25519.PublicKeySize {
		return ed25519.PublicKey(raw), nil
	}
	// 也可能给了 base64/hex 编码的 DER（SubjectPublicKeyInfo）。
	if key, err := parseDERPublicKey(raw); err == nil {
		return key, nil
	}
	return nil, fmt.Errorf("public key must be %d raw bytes (base64/hex) or a DER/PEM SubjectPublicKeyInfo, got %d bytes", ed25519.PublicKeySize, len(raw))
}

// parsePEMPublicKey 解析 PEM 编码的 SubjectPublicKeyInfo。
func parsePEMPublicKey(s string) (ed25519.PublicKey, error) {
	block, _ := pem.Decode([]byte(s))
	if block == nil {
		return nil, fmt.Errorf("public key looks like PEM but has no decodable block")
	}
	return parseDERPublicKey(block.Bytes)
}

// parseDERPublicKey 解析 DER 编码的 SubjectPublicKeyInfo，并要求它是 Ed25519。
// 算法不符时明确报错，而不是"校验不通过"——后者会让人误以为是签名坏了。
func parseDERPublicKey(der []byte) (ed25519.PublicKey, error) {
	key, err := x509.ParsePKIXPublicKey(der)
	if err != nil {
		return nil, fmt.Errorf("parse public key: %w", err)
	}
	ed, ok := key.(ed25519.PublicKey)
	if !ok {
		return nil, fmt.Errorf("public key is %T, want ed25519.PublicKey", key)
	}
	return ed, nil
}

// base64Encodings 列出可接受的 base64 变体。
// 标准带填充是常见形式；原始无填充与 URL 变体则出现在 URL 或单行注入场景里。
var base64Encodings = []*base64.Encoding{
	base64.StdEncoding,
	base64.RawStdEncoding,
	base64.URLEncoding,
	base64.RawURLEncoding,
}

// decodeKeyBytes 按十六进制或 base64 解码密钥文本。
//
// 判定顺序刻意先做"恰好 64 个十六进制字符"这一精确匹配：32 字节原始公钥的
// 十六进制正好 64 字符，而它的 base64 是 43/44 字符，两者长度不同、不会混淆。
// 若一上来就做宽松的 isHex 判断，一段"恰好全由 a-f/0-9 组成的 base64"会被
// 误当成十六进制解出错误字节。
func decodeKeyBytes(s string) ([]byte, error) {
	if len(s) == ed25519.PublicKeySize*2 && isHex(s) {
		return hex.DecodeString(s)
	}
	for _, enc := range base64Encodings {
		if raw, err := enc.DecodeString(s); err == nil {
			return raw, nil
		}
	}
	// 其它长度的十六进制（例如十六进制编码的 DER）。
	if isHex(s) && len(s)%2 == 0 {
		return hex.DecodeString(s)
	}
	return nil, fmt.Errorf("public key is neither hex nor base64")
}

// decodeSignature 把签名文本解码为原始字节。
// 发布脚本产出的是裸二进制 `.sig` 文件，经 GitHub 附件回传时被我们转成 base64；
// 自定义更新源则可能直接给十六进制，因此两种都接受。
func decodeSignature(s string) ([]byte, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, fmt.Errorf("empty signature")
	}
	if len(s) == ed25519.SignatureSize*2 && isHex(s) {
		return hex.DecodeString(s)
	}
	for _, enc := range base64Encodings {
		if raw, err := enc.DecodeString(s); err == nil && len(raw) == ed25519.SignatureSize {
			return raw, nil
		}
	}
	return nil, fmt.Errorf("signature is neither %d-byte hex nor base64", ed25519.SignatureSize)
}

// verifyFileSignature 用 pub 校验 path 所指文件的签名。
//
// Ed25519 是"纯"签名算法，OpenSSL 的 `pkeyutl -sign -rawin` 直接对文件内容签名，
// 因此这里必须把整个文件交给 Verify —— 不能先算摘要再验，那样会把语义变成另一种算法。
// 文件大小在此之前已由 Apply 用 maxSignedArchiveSize 兜过底，不会无界读入内存。
func verifyFileSignature(path, signature string, pub ed25519.PublicKey) error {
	sig, err := decodeSignature(signature)
	if err != nil {
		return err
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read %s: %w", path, err)
	}
	if !ed25519.Verify(pub, data, sig) {
		return fmt.Errorf("signature verification failed for %s", path)
	}
	return nil
}

// isSignatureAsset 判断 name（已转小写）是否是签名附件。
//
// 这一点至关重要：签名文件名通常也含平台与架构（`...-windows-amd64.sig`），
// 若不排除，pickAsset 的"名字里包含 GOOS/GOARCH"匹配会先选中它，
// 于是签名文件被当成安装包下载下来。
func isSignatureAsset(lowerName string) bool {
	return strings.HasSuffix(lowerName, ".sig") ||
		strings.HasSuffix(lowerName, ".signature")
}
