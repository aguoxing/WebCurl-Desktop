package updater

import (
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// mustKeyPair 生成一对 Ed25519 密钥，供各用例复用。
func mustKeyPair(t *testing.T) (ed25519.PublicKey, ed25519.PrivateKey) {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	return pub, priv
}

// pemPublicKey 把公钥编码为 PEM（SubjectPublicKeyInfo），模拟"从 openssl 复制粘贴"。
func pemPublicKey(t *testing.T, pub ed25519.PublicKey) string {
	t.Helper()
	der, err := x509.MarshalPKIXPublicKey(pub)
	if err != nil {
		t.Fatal(err)
	}
	return string(pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: der}))
}

// TestParsePublicKeyEmptyMeansDisabled 验证空公钥返回 (nil, nil)：
// "未配置"是合法状态，不该被当成错误。
func TestParsePublicKeyEmptyMeansDisabled(t *testing.T) {
	for _, in := range []string{"", "   ", "\n\t"} {
		key, err := parsePublicKey(in)
		if err != nil {
			t.Fatalf("parsePublicKey(%q) error: %v", in, err)
		}
		if key != nil {
			t.Fatalf("parsePublicKey(%q) = %v, want nil", in, key)
		}
	}
}

// TestParsePublicKeyFormats 覆盖三种可接受的公钥写法。
func TestParsePublicKeyFormats(t *testing.T) {
	pub, _ := mustKeyPair(t)

	der, err := x509.MarshalPKIXPublicKey(pub)
	if err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name string
		in   string
	}{
		{"base64 原始 32 字节", base64.StdEncoding.EncodeToString(pub)},
		{"base64 无填充", base64.RawStdEncoding.EncodeToString(pub)},
		{"十六进制原始 32 字节", hex.EncodeToString(pub)},
		{"PEM", pemPublicKey(t, pub)},
		{"base64 的 DER", base64.StdEncoding.EncodeToString(der)},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parsePublicKey(tc.in)
			if err != nil {
				t.Fatalf("parsePublicKey() error: %v", err)
			}
			if !got.Equal(pub) {
				t.Fatal("解析出的公钥与原始公钥不一致")
			}
		})
	}
}

// TestParsePublicKeyRejectsWrongLength 验证长度不对的裸密钥会被拒绝，
// 而不是被截断/补齐后悄悄使用。
func TestParsePublicKeyRejectsWrongLength(t *testing.T) {
	short := base64.StdEncoding.EncodeToString([]byte("too-short"))
	if _, err := parsePublicKey(short); err == nil {
		t.Fatal("parsePublicKey() = nil, want an error for a short key")
	}
}

// TestParsePublicKeyRejectsNonEd25519 验证非 Ed25519 公钥会明确报错。
// 若只是"验签不通过"，排查时会误以为是签名算错了。
func TestParsePublicKeyRejectsNonEd25519(t *testing.T) {
	ecdsaKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	der, err := x509.MarshalPKIXPublicKey(&ecdsaKey.PublicKey)
	if err != nil {
		t.Fatal(err)
	}
	pemText := string(pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: der}))

	_, err = parsePublicKey(pemText)
	if err == nil {
		t.Fatal("parsePublicKey() = nil, want an error for a non-Ed25519 key")
	}
	if !strings.Contains(err.Error(), "ed25519") {
		t.Fatalf("error = %q, want it to mention ed25519", err)
	}
}

// TestParsePublicKeyRejectsBrokenPEM 验证带 PEM 头但内容损坏时会报错，
// 而不是静默退化成"没配公钥"（那会让校验整个失效）。
func TestParsePublicKeyRejectsBrokenPEM(t *testing.T) {
	if _, err := parsePublicKey("-----BEGIN PUBLIC KEY-----\nnot base64\n-----END PUBLIC KEY-----"); err == nil {
		t.Fatal("parsePublicKey() = nil, want an error for a corrupted PEM block")
	}
}

// TestTrustedPublicKeyPrefersEmbedded 验证内嵌公钥优先于配置。
// 这是安全属性：能改配置文件的人不能把信任锚换掉。
func TestTrustedPublicKeyPrefersEmbedded(t *testing.T) {
	old := EmbeddedPublicKey
	t.Cleanup(func() { EmbeddedPublicKey = old })

	EmbeddedPublicKey = "embedded-key"
	if got := TrustedPublicKey("configured-key"); got != "embedded-key" {
		t.Fatalf("TrustedPublicKey() = %q, want the embedded key", got)
	}

	EmbeddedPublicKey = "  "
	if got := TrustedPublicKey("configured-key"); got != "configured-key" {
		t.Fatalf("TrustedPublicKey() = %q, want the configured key when nothing is embedded", got)
	}
}

// TestDecodeSignatureFormats 覆盖签名的两种文本写法。
func TestDecodeSignatureFormats(t *testing.T) {
	raw := make([]byte, ed25519.SignatureSize)
	for i := range raw {
		raw[i] = byte(i)
	}

	for name, in := range map[string]string{
		"十六进制":   hex.EncodeToString(raw),
		"base64": base64.StdEncoding.EncodeToString(raw),
	} {
		t.Run(name, func(t *testing.T) {
			got, err := decodeSignature(in)
			if err != nil {
				t.Fatalf("decodeSignature() error: %v", err)
			}
			if string(got) != string(raw) {
				t.Fatal("解码结果与原始签名不一致")
			}
		})
	}
}

// TestDecodeSignatureRejectsBadInput 覆盖空值与长度错误。
func TestDecodeSignatureRejectsBadInput(t *testing.T) {
	for name, in := range map[string]string{
		"空串":    "",
		"空白":    "   ",
		"长度不对":  base64.StdEncoding.EncodeToString([]byte("short")),
		"非编码文本": "not-a-signature",
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := decodeSignature(in); err == nil {
				t.Fatalf("decodeSignature(%q) = nil, want an error", in)
			}
		})
	}
}

// TestVerifyFileSignatureRoundTrip 验证签名校验的正常路径与三种失败路径。
func TestVerifyFileSignatureRoundTrip(t *testing.T) {
	pub, priv := mustKeyPair(t)
	otherPub, _ := mustKeyPair(t)

	payload := []byte("webcurl-desktop release archive")
	path := filepath.Join(t.TempDir(), "update.zip")
	if err := os.WriteFile(path, payload, 0o600); err != nil {
		t.Fatal(err)
	}

	sig := ed25519.Sign(priv, payload)
	encoded := base64.StdEncoding.EncodeToString(sig)

	if err := verifyFileSignature(path, encoded, pub); err != nil {
		t.Fatalf("verifyFileSignature() error: %v", err)
	}

	if err := verifyFileSignature(path, encoded, otherPub); err == nil {
		t.Fatal("用另一把公钥竟然验签通过了")
	}

	// 篡改签名。
	badSig := make([]byte, len(sig))
	copy(badSig, sig)
	badSig[0] ^= 0xff
	if err := verifyFileSignature(path, base64.StdEncoding.EncodeToString(badSig), pub); err == nil {
		t.Fatal("被篡改的签名竟然验签通过了")
	}

	// 篡改文件内容。
	tampered := filepath.Join(t.TempDir(), "tampered.zip")
	if err := os.WriteFile(tampered, append(payload, '!'), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := verifyFileSignature(tampered, encoded, pub); err == nil {
		t.Fatal("被篡改的文件竟然验签通过了")
	}
}

// writeSignedZip 生成一份内容为 content 的"发布包"及其签名文本。
func writeSignedZip(t *testing.T, content string) (path, signature string, pub ed25519.PublicKey) {
	t.Helper()
	var priv ed25519.PrivateKey
	pub, priv = mustKeyPair(t)

	path = filepath.Join(t.TempDir(), "update.zip")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	signature = base64.StdEncoding.EncodeToString(ed25519.Sign(priv, []byte(content)))
	return path, signature, pub
}

// TestCheckSignatureRequiresSignatureWhenKeyPinned 验证钉住公钥后，
// 未签名的发布包会被拒绝 —— 否则攻击者删掉 .sig 就能绕过校验。
func TestCheckSignatureRequiresSignatureWhenKeyPinned(t *testing.T) {
	path, _, pub := writeSignedZip(t, "content")

	err := checkSignature(path, &UpdateInfo{Version: "1.2.3"}, pub)
	if err == nil {
		t.Fatal("checkSignature() = nil, want a refusal for an unsigned release")
	}
	if !strings.Contains(err.Error(), "unsigned") {
		t.Fatalf("error = %q, want it to explain the release is unsigned", err)
	}
}

// TestCheckSignatureAcceptsValidSignature 验证签名正确时放行。
func TestCheckSignatureAcceptsValidSignature(t *testing.T) {
	path, sig, pub := writeSignedZip(t, "content")

	if err := checkSignature(path, &UpdateInfo{Version: "1.2.3", Signature: sig}, pub); err != nil {
		t.Fatalf("checkSignature() error: %v", err)
	}
}

// TestCheckSignatureRejectsTamperedArchive 验证内容被改动后拒绝安装。
func TestCheckSignatureRejectsTamperedArchive(t *testing.T) {
	path, sig, pub := writeSignedZip(t, "content")

	if err := os.WriteFile(path, []byte("tampered"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := checkSignature(path, &UpdateInfo{Version: "1.2.3", Signature: sig}, pub); err == nil {
		t.Fatal("被篡改的发布包竟然通过了签名校验")
	}
}

// TestCheckSignatureWithoutKeyIsSkipped 验证未钉住公钥时不启用校验：
// 这是向后兼容路径，只在带了签名时记一条警告。
func TestCheckSignatureWithoutKeyIsSkipped(t *testing.T) {
	path, sig, _ := writeSignedZip(t, "content")

	if err := checkSignature(path, &UpdateInfo{Version: "1.2.3", Signature: sig}, nil); err != nil {
		t.Fatalf("checkSignature() error: %v", err)
	}
	if err := checkSignature(path, &UpdateInfo{Version: "1.2.3"}, nil); err != nil {
		t.Fatalf("checkSignature() error: %v", err)
	}
}

// TestIsSignatureAsset 固定签名附件的判定规则。
func TestIsSignatureAsset(t *testing.T) {
	cases := map[string]bool{
		"app-1.2.3-windows-amd64.sig":     true,
		"app-1.2.3-windows-amd64.zip.sig": true,
		"app-1.2.3-linux-arm64.signature": true,
		"sha256sums":                      false,
		"app-1.2.3-windows-amd64.zip":     false,
		"signature-guide.pdf":             false,
	}
	for name, want := range cases {
		if got := isSignatureAsset(name); got != want {
			t.Fatalf("isSignatureAsset(%q) = %v, want %v", name, got, want)
		}
	}
}

// TestPickAssetSkipsSignatureAssets 验证发布包里同时存在 .sig 时，
// 平台安装包不会被签名文件顶替。
//
// 这是一个真实的风险：签名文件名同样含 "windows-amd64"，且字典序上 ".sig" < ".zip"，
// 因此若不过滤，GitHub 返回的附件顺序会让 .sig 被选中当作安装包下载。
func TestPickAssetSkipsSignatureAssets(t *testing.T) {
	name := "webcurl-desktop-1.2.3-" + runtime.GOOS + "-" + runtime.GOARCH
	assets := []githubAsset{
		{Name: name + ".sig", Size: 64},
		{Name: name + ".zip", Size: 1024},
	}

	got, ok := pickAsset(assets)
	if !ok {
		t.Fatal("pickAsset() found nothing")
	}
	if !strings.HasSuffix(got.Name, ".zip") {
		t.Fatalf("pickAsset() chose %q, want the .zip archive", got.Name)
	}
}

// TestFindSignatureAsset 覆盖两种 .sig 命名约定。
func TestFindSignatureAsset(t *testing.T) {
	assets := []githubAsset{
		{Name: "app-1.2.3-linux-amd64.sig", BrowserDownloadURL: "https://example.test/a.sig"},
		{Name: "other-1.0.0-linux-amd64.zip", BrowserDownloadURL: "https://example.test/b.zip"},
	}

	if got := findSignatureAsset(assets, "app-1.2.3-linux-amd64.zip"); got != "https://example.test/a.sig" {
		t.Fatalf("去掉扩展名的约定未匹配：got %q", got)
	}
	if got := findSignatureAsset(assets, "app-1.2.3-linux-amd64"); got != "https://example.test/a.sig" {
		t.Fatalf("无扩展名的约定未匹配：got %q", got)
	}
	if got := findSignatureAsset(assets, "missing-9.9.9-linux-amd64.zip"); got != "" {
		t.Fatalf("不应匹配到无关附件：got %q", got)
	}
}

// TestFetchSignatureNormalizes 验证裸二进制 .sig 会被转成 base64，
// 而本来就是文本的签名原样保留。
func TestFetchSignatureNormalizes(t *testing.T) {
	_, priv := mustKeyPair(t)
	raw := ed25519.Sign(priv, []byte("payload"))

	rawBody := base64.StdEncoding.EncodeToString(raw)

	cases := []struct {
		name string
		body []byte
		want string
	}{
		{"裸二进制", raw, rawBody},
		{"base64 文本", []byte(rawBody + "\n"), rawBody},
		{"十六进制文本", []byte(hex.EncodeToString(raw)), hex.EncodeToString(raw)},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				_, _ = w.Write(tc.body)
			}))
			defer srv.Close()

			got, err := fetchSignature(srv.Client(), srv.URL)
			if err != nil {
				t.Fatalf("fetchSignature() error: %v", err)
			}
			if got != tc.want {
				t.Fatalf("fetchSignature() = %q, want %q", got, tc.want)
			}
		})
	}
}

// TestFetchSignatureRejectsHTTPError 验证附件读取失败会报错，
// 从而让上层把"网络故障"与"未签名"区分开。
func TestFetchSignatureRejectsHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	if _, err := fetchSignature(srv.Client(), srv.URL); err == nil {
		t.Fatal("fetchSignature() = nil, want an error for HTTP 404")
	}
}

// TestEmbeddedPublicKeyBuild 只在构建时真正注入了公钥时执行，用于对发布产物做自检：
//
//	go test -ldflags "$(GOWS_UPDATE_PUBLIC_KEY=... go run ./cmd/versionldflags)" ./internal/updater
//
// 它同时覆盖两件事：注入确实生效（不是被链接器静默忽略），
// 以及 build.sh 推导出的格式确实能被客户端解析。
// 常规 `go test ./...` 会跳过它，因为那时公钥本来就没注入。
func TestEmbeddedPublicKeyBuild(t *testing.T) {
	if strings.TrimSpace(EmbeddedPublicKey) == "" {
		t.Skip("未注入公钥；用 -ldflags 注入后此用例才有意义")
	}
	key, err := parsePublicKey(EmbeddedPublicKey)
	if err != nil {
		t.Fatalf("注入的公钥无法解析：%v", err)
	}
	if len(key) != ed25519.PublicKeySize {
		t.Fatalf("注入的公钥长度为 %d，期望 %d", len(key), ed25519.PublicKeySize)
	}
}
