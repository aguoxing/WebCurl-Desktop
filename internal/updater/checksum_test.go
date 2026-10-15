package updater

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseChecksumFile(t *testing.T) {
	const (
		sum  = "3b1f7c9d4e2a6088f0c5b1a7d9e3f2c4a6b8d0e2f4a6c8b0d2e4f6a8c0b2d4e6"
		sum2 = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	)
	manifest := sum + "  webcurl-desktop-v1.2.3-windows-amd64.zip\n" +
		sum2 + " *webcurl-desktop-v1.2.3-linux-amd64.zip\n"

	got, err := parseChecksumFile(strings.NewReader(manifest), "webcurl-desktop-v1.2.3-windows-amd64.zip")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if want := "sha256:" + sum; got != want {
		t.Fatalf("got %q, want %q", got, want)
	}

	// sha256sum -b 使用的二进制模式 "*" 前缀必须被容忍。
	got, err = parseChecksumFile(strings.NewReader(manifest), "webcurl-desktop-v1.2.3-linux-amd64.zip")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if want := "sha256:" + sum2; got != want {
		t.Fatalf("got %q, want %q", got, want)
	}

	if _, err := parseChecksumFile(strings.NewReader(manifest), "missing.zip"); err == nil {
		t.Fatal("expected an error for an asset missing from the manifest")
	}
}

func TestNormalizeChecksum(t *testing.T) {
	raw := sha256.Sum256([]byte("payload"))
	hexSum := hex.EncodeToString(raw[:])

	cases := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{"bare hex", hexSum, "sha256:" + hexSum, false},
		{"prefixed hex", "sha256:" + hexSum, "sha256:" + hexSum, false},
		{"uppercase hex", strings.ToUpper(hexSum), "sha256:" + hexSum, false},
		{"base64 digest", base64.StdEncoding.EncodeToString(raw[:]), "sha256:" + hexSum, false},
		{"unsupported algo", "sha512:" + hexSum, "", true},
		{"too short", "abcd", "", true},
		{"empty", "", "", true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := normalizeChecksum(tc.input)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected an error, got %q", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestNormalizeDigest(t *testing.T) {
	raw := sha256.Sum256([]byte("payload"))
	hexSum := hex.EncodeToString(raw[:])

	got, err := normalizeDigest("", base64.StdEncoding.EncodeToString(raw[:]))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if want := "sha256:" + hexSum; got != want {
		t.Fatalf("got %q, want %q", got, want)
	}

	if _, err := normalizeDigest("sha512", hexSum); err == nil {
		t.Fatal("expected an error for an unsupported digest algorithm")
	}
}

func TestVerifyChecksum(t *testing.T) {
	path := filepath.Join(t.TempDir(), "artifact.bin")
	payload := []byte("release payload")
	if err := os.WriteFile(path, payload, 0o644); err != nil {
		t.Fatal(err)
	}
	raw := sha256.Sum256(payload)

	if err := verifyChecksum(path, "sha256:"+hex.EncodeToString(raw[:])); err != nil {
		t.Fatalf("expected the checksum to match: %v", err)
	}
	if err := verifyChecksum(path, "sha256:"+strings.Repeat("0", 64)); err == nil {
		t.Fatal("expected a checksum mismatch")
	}
}
