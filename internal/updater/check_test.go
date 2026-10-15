package updater

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/hex"
	"strings"
	"testing"
)

// validSignatureText 返回一个长度合法（64 字节）的签名文本，供解析用例使用。
func validSignatureText(encoding string) string {
	raw := make([]byte, ed25519.SignatureSize)
	for i := range raw {
		raw[i] = byte(i + 1)
	}
	if encoding == "hex" {
		return hex.EncodeToString(raw)
	}
	return base64.StdEncoding.EncodeToString(raw)
}

// TestToUpdateInfoSignatureFlat 验证扁平结构里的 signature 会被读到。
func TestToUpdateInfoSignatureFlat(t *testing.T) {
	sig := validSignatureText("base64")

	info, err := apiResponse{
		Version:   "1.2.3",
		URL:       "https://example.test/app.zip",
		Signature: sig,
	}.toUpdateInfo()
	if err != nil {
		t.Fatalf("toUpdateInfo() error: %v", err)
	}
	if info.Signature != sig {
		t.Fatalf("Signature = %q, want the flat value", info.Signature)
	}
}

// TestToUpdateInfoSignatureNested 验证嵌套 verification 结构里的 signature 会被读到。
func TestToUpdateInfoSignatureNested(t *testing.T) {
	sig := validSignatureText("hex")

	var raw apiResponse
	raw.Version = "1.2.3"
	raw.URL = "https://example.test/app.zip"
	raw.Verification.Signature = sig

	info, err := raw.toUpdateInfo()
	if err != nil {
		t.Fatalf("toUpdateInfo() error: %v", err)
	}
	if info.Signature != sig {
		t.Fatalf("Signature = %q, want the nested value", info.Signature)
	}
}

// TestToUpdateInfoSignatureFlatWins 验证两种写法同时存在时扁平字段优先，
// 与 checksum 的处理保持一致。
func TestToUpdateInfoSignatureFlatWins(t *testing.T) {
	flat := validSignatureText("base64")
	nested := validSignatureText("hex")

	var raw apiResponse
	raw.Version = "1.2.3"
	raw.URL = "https://example.test/app.zip"
	raw.Signature = flat
	raw.Verification.Signature = nested

	info, err := raw.toUpdateInfo()
	if err != nil {
		t.Fatalf("toUpdateInfo() error: %v", err)
	}
	if info.Signature != flat {
		t.Fatalf("Signature = %q, want the flat value to win", info.Signature)
	}
}

// TestToUpdateInfoRejectsInvalidSignature 验证格式非法的签名会被视为硬错误：
// 静默丢弃签名会让"钉住公钥"变成拒绝服务，明确报错更容易定位。
func TestToUpdateInfoRejectsInvalidSignature(t *testing.T) {
	_, err := apiResponse{
		Version:   "1.2.3",
		URL:       "https://example.test/app.zip",
		Signature: "obviously-not-a-signature",
	}.toUpdateInfo()
	if err == nil {
		t.Fatal("toUpdateInfo() = nil, want an error for a malformed signature")
	}
	if !strings.Contains(err.Error(), "signature") {
		t.Fatalf("error = %q, want it to mention the signature", err)
	}
}

// TestToUpdateInfoWithoutSignature 验证响应不带签名时字段保持为空，
// 是否因此拒绝安装由 Apply 决定。
func TestToUpdateInfoWithoutSignature(t *testing.T) {
	info, err := apiResponse{
		Version: "1.2.3",
		URL:     "https://example.test/app.zip",
	}.toUpdateInfo()
	if err != nil {
		t.Fatalf("toUpdateInfo() error: %v", err)
	}
	if info.Signature != "" {
		t.Fatalf("Signature = %q, want empty", info.Signature)
	}
}
