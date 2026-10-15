package updater

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"
	"time"
)

// 注意测试顺序：需要「助手缺失」的用例必须排在会往测试可执行文件目录里
// 安置助手的用例之前，否则前者会因为环境里存在助手而跳过。

func TestLocateHelperPrefersFirstExisting(t *testing.T) {
	dir := t.TempDir()
	first := filepath.Join(dir, "first")
	second := filepath.Join(dir, "second")
	for _, p := range []string{first, second} {
		if err := os.WriteFile(p, []byte("helper"), 0o755); err != nil {
			t.Fatal(err)
		}
	}

	if got := locateHelper(first, second); got != first {
		t.Fatalf("got %q, want %q", got, first)
	}
	if got := locateHelper(filepath.Join(dir, "missing"), second); got != second {
		t.Fatalf("got %q, want %q", got, second)
	}
	if got := locateHelper(filepath.Join(dir, "missing")); got != "" {
		t.Fatalf("got %q, want an empty string when nothing matches", got)
	}
	if got := locateHelper(dir); got != "" {
		t.Fatal("a directory must not be accepted as the helper")
	}
}

func TestCopyExecutableCopiesContentAndPermissions(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src")
	dst := filepath.Join(dir, "dst")
	// 源文件刻意不带可执行位：这个权限必须由 copyExecutable 自己补上。
	// zip 条目记录的权限不可靠，助手不能指望继承。
	if err := os.WriteFile(src, []byte("helper-binary"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := copyExecutable(src, dst); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	got, err := os.ReadFile(dst)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "helper-binary" {
		t.Fatalf("got %q, want %q", got, "helper-binary")
	}

	if runtime.GOOS != "windows" {
		info, err := os.Stat(dst)
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm()&0o111 == 0 {
			t.Fatalf("copied helper has no execute bit: %v", info.Mode())
		}
	}
}

func TestCleanupStaleWorkDirs(t *testing.T) {
	tempDir := os.TempDir()
	stale := filepath.Join(tempDir, workDirPrefix+"test-stale")
	fresh := filepath.Join(tempDir, workDirPrefix+"test-fresh")
	unrelated := filepath.Join(tempDir, "gows-unrelated-test")

	for _, d := range []string{stale, fresh, unrelated} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = os.RemoveAll(d) })
	}

	old := time.Now().Add(-2 * staleWorkDirAge)
	if err := os.Chtimes(stale, old, old); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(unrelated, old, old); err != nil {
		t.Fatal(err)
	}

	cleanupStaleWorkDirs()

	if _, err := os.Stat(stale); !os.IsNotExist(err) {
		t.Errorf("%s is stale and should have been removed", stale)
	}
	// 新鲜的目录可能是另一次正在进行中的更新，绝不能碰。
	if _, err := os.Stat(fresh); err != nil {
		t.Errorf("%s is fresh and must be kept: %v", fresh, err)
	}
	if _, err := os.Stat(unrelated); err != nil {
		t.Errorf("%s does not look like a work dir and must be kept: %v", unrelated, err)
	}
}

// TestApplyFailsWhenHelperIsMissing 覆盖「绿色版解压出来只有主程序」之外的
// 兜底分支：包内和安装目录都没有助手时，Apply 必须明确报错，并且把已经解压
// 出来的东西收拾干净。
func TestApplyFailsWhenHelperIsMissing(t *testing.T) {
	if _, err := testBinaryDir(); err != nil {
		t.Skipf("cannot locate the test executable: %v", err)
	}
	if helper, err := installedTestHelper(); err == nil {
		t.Skipf("%s already exists next to the test binary, cannot simulate a missing helper", helper)
	}

	zipBytes := buildTestZip(t, map[string][]byte{"webcurl-desktop.exe": []byte("new binary")})
	info := serveUpdate(t, zipBytes)

	before := workDirsInTemp(t)

	err := Apply(info, "")
	if err == nil {
		t.Fatal("expected Apply to fail when no update helper is available")
	}
	if !strings.Contains(err.Error(), "update helper") {
		t.Fatalf("the error should point at the missing helper, got: %v", err)
	}

	if added := diffWorkDirs(workDirsInTemp(t), before); len(added) > 0 {
		t.Fatalf("a failed Apply left work dirs behind: %v", added)
	}
}

// TestApplyLeavesWorkDirToTheHelper 锁定交接语义：一旦助手被启动，工作目录就
// 归它所有，Apply 不得清理。
//
// 这里曾有一个真实缺陷 —— Apply 无条件 defer 删除工作目录，而助手要先等主进程
// 退出才开始安装，于是它醒来时 --source 已经不在了，更新必然失败。
//
// 助手用测试二进制自身充当：它不认识 --source/--target，会立刻退出，但 Apply
// 只关心 cmd.Start() 是否成功，而这正好是要验证的分支。
func TestApplyLeavesWorkDirToTheHelper(t *testing.T) {
	dir, err := testBinaryDir()
	if err != nil {
		t.Skipf("cannot locate the test executable: %v", err)
	}
	helperPath := filepath.Join(dir, helperBinaryName())
	if _, err := os.Stat(helperPath); err == nil {
		t.Skipf("%s already exists next to the test binary, refusing to overwrite it", helperPath)
	}

	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	if err := copyExecutable(self, helperPath); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Remove(helperPath) })

	// 包里只放负载、不放助手，走的是「回退到安装目录那份」的分支 ——
	// 这样测试不必往 zip 里塞一个十几兆的真实二进制。「包内助手优先」
	// 由 TestLocateHelperPrefersFirstExisting 覆盖。
	// 多放一个 configs/ 是为了同时确认：条目不止一个时不会误剥顶层目录。
	zipBytes := buildTestZip(t, map[string][]byte{
		"webcurl-desktop.exe": []byte("new binary"),
		"configs/config.yaml": []byte("mock: {}"),
	})
	info := serveUpdate(t, zipBytes)

	before := workDirsInTemp(t)

	err = Apply(info, "")
	if !errors.Is(err, ErrRestartRequired) {
		t.Fatalf("got %v, want ErrRestartRequired", err)
	}

	added := diffWorkDirs(workDirsInTemp(t), before)
	if len(added) != 1 {
		t.Fatalf("expected exactly one work dir to be kept for the helper, got %v", added)
	}
	workDir := added[0]
	t.Cleanup(func() { _ = os.RemoveAll(workDir) })

	if _, err := os.Stat(filepath.Join(workDir, "extracted")); err != nil {
		t.Fatalf("the extracted payload must survive the hand-off: %v", err)
	}
	if _, err := os.Stat(filepath.Join(workDir, "update.zip")); err != nil {
		t.Fatalf("the downloaded archive must survive the hand-off: %v", err)
	}
	if _, err := os.Stat(filepath.Join(workDir, helperDirName, helperBinaryName())); err != nil {
		t.Fatalf("the staged helper must survive the hand-off: %v", err)
	}
}

// serveUpdate 起一个只提供单个 zip 的更新源，并返回与之匹配的 UpdateInfo。
func serveUpdate(t *testing.T, zipBytes []byte) *UpdateInfo {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(zipBytes)
	}))
	t.Cleanup(server.Close)

	return &UpdateInfo{
		HasUpdate: true,
		Version:   "9.9.9",
		URL:       server.URL + "/package.zip",
		Checksum:  "sha256:" + sha256Hex(zipBytes),
		Size:      int64(len(zipBytes)),
	}
}

// buildTestZip 在内存中生成一个 zip。条目名排序后写入，让产物保持稳定。
func buildTestZip(t *testing.T, entries map[string][]byte) []byte {
	t.Helper()

	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)

	names := make([]string, 0, len(entries))
	for name := range entries {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write(entries[name]); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func sha256Hex(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// testBinaryDir 返回测试可执行文件所在的目录 —— 也就是 Apply 眼中的
// 「主程序目录」，helperBinaryName() 正是要放在那里。
func testBinaryDir() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	return filepath.Dir(exe), nil
}

// installedTestHelper 返回测试可执行文件目录里那份助手（如果存在）。
func installedTestHelper() (string, error) {
	dir, err := testBinaryDir()
	if err != nil {
		return "", err
	}
	path := filepath.Join(dir, helperBinaryName())
	if _, err := os.Stat(path); err != nil {
		return "", err
	}
	return path, nil
}

// workDirsInTemp 列出系统临时目录里所有更新工作目录。
func workDirsInTemp(t *testing.T) []string {
	t.Helper()

	entries, err := os.ReadDir(os.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	var out []string
	for _, e := range entries {
		if e.IsDir() && strings.HasPrefix(e.Name(), workDirPrefix) {
			out = append(out, filepath.Join(os.TempDir(), e.Name()))
		}
	}
	sort.Strings(out)
	return out
}

// diffWorkDirs 返回 after 相对 before 新增的条目。
func diffWorkDirs(after, before []string) []string {
	known := make(map[string]bool, len(before))
	for _, p := range before {
		known[p] = true
	}

	var added []string
	for _, p := range after {
		if !known[p] {
			added = append(added, p)
		}
	}
	return added
}
