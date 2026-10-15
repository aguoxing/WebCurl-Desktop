package updater

import (
	"archive/zip"
	"os"
	"path/filepath"
	"testing"
)

func TestUnzipExtractsPayload(t *testing.T) {
	dir := t.TempDir()
	zipPath := filepath.Join(dir, "ok.zip")
	writeTestZip(t, zipPath, "webcurl-desktop.exe", []byte("binary"))

	out := filepath.Join(dir, "out")
	if err := os.MkdirAll(out, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := unzip(zipPath, out); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	got, err := os.ReadFile(filepath.Join(out, "webcurl-desktop.exe"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "binary" {
		t.Fatalf("got %q, want %q", got, "binary")
	}
}

func TestUnzipRejectsPathTraversal(t *testing.T) {
	dir := t.TempDir()
	zipPath := filepath.Join(dir, "evil.zip")
	writeTestZip(t, zipPath, "../escaped.exe", []byte("pwned"))

	out := filepath.Join(dir, "out")
	if err := os.MkdirAll(out, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := unzip(zipPath, out); err == nil {
		t.Fatal("expected a zip entry escaping the destination to be rejected")
	}
	if _, err := os.Stat(filepath.Join(dir, "escaped.exe")); err == nil {
		t.Fatal("a file was written outside the destination directory")
	}
}

// writeTestZip 为解压相关测试生成一个只含单个条目的 zip 包。
func writeTestZip(t *testing.T, path, name string, content []byte) {
	t.Helper()

	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	zw := zip.NewWriter(f)
	w, err := zw.Create(name)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write(content); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
}
