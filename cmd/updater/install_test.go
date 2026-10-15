package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// newTestLogger 返回一个写入临时文件的日志器。
// 直接写 stderr 会把用例输出淹没在安装日志里，写文件既安静又能顺带验证落盘。
// 注册 Cleanup 关闭文件：Windows 上未关闭的句柄会让 t.TempDir 的清理失败。
func newTestLogger(t *testing.T) *logger {
	t.Helper()
	l := newLogger(filepath.Join(t.TempDir(), "updater.log"))
	t.Cleanup(l.Close)
	return l
}

// writeTestFile 在 dir 下按相对路径创建文件（自动建父目录）。
func writeTestFile(t *testing.T, dir, rel, content string) {
	t.Helper()
	path := filepath.Join(dir, rel)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// TestCollectPayloadEmpty 验证空解压目录会被判为错误，
// 而不是"什么都不做然后报告成功"——那会让用户以为更新成功了。
func TestCollectPayloadEmpty(t *testing.T) {
	if _, err := collectPayload(t.TempDir(), t.TempDir()); err == nil {
		t.Fatal("collectPayload() = nil, want an error for an empty source")
	}
}

// TestCollectPayloadNested 验证多文件负载保留相对路径：
// 包内的目录结构必须原样落到目标目录，否则子目录里的文件会散落一地。
func TestCollectPayloadNested(t *testing.T) {
	src := t.TempDir()
	writeTestFile(t, src, "app", "APP")
	writeTestFile(t, src, filepath.Join("sub", "helper"), "HELPER")

	target := t.TempDir()
	files, err := collectPayload(src, target)
	if err != nil {
		t.Fatalf("collectPayload() error: %v", err)
	}
	if len(files) != 2 {
		t.Fatalf("got %d files, want 2", len(files))
	}

	got := make(map[string]string, len(files))
	for _, f := range files {
		got[f.rel] = f.dest
	}
	for _, rel := range []string{"app", filepath.Join("sub", "helper")} {
		want := filepath.Join(target, rel)
		if got[rel] != want {
			t.Fatalf("dest for %q = %q, want %q", rel, got[rel], want)
		}
	}
}

// TestCollectPayloadSkipsNonRegular 验证符号链接不会被当作负载。
// 跟随链接复制等于把安装目录之外的任意文件搬进来，因此必须跳过。
//
// 该用例依赖平台能如实报告链接类型：Windows 上 os.Symlink 创建的对象有时会被
// Lstat 报告为普通文件（无符号链接权限或被文件系统归一化），此时断言无从成立，
// 直接跳过而不是让 CI 变红。
func TestCollectPayloadSkipsNonRegular(t *testing.T) {
	src := t.TempDir()
	writeTestFile(t, src, "app", "APP")

	outside := filepath.Join(t.TempDir(), "secret")
	if err := os.WriteFile(outside, []byte("SECRET"), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(src, "link")
	if err := os.Symlink(outside, link); err != nil {
		t.Skipf("本环境无法创建符号链接，跳过：%v", err)
	}
	info, err := os.Lstat(link)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Skipf("本平台的 Lstat 未把链接报告为符号链接（mode=%v），跳过", info.Mode())
	}

	files, err := collectPayload(src, t.TempDir())
	if err != nil {
		t.Fatalf("collectPayload() error: %v", err)
	}
	if len(files) != 1 || files[0].rel != "app" {
		t.Fatalf("got %+v, want only the regular file %q", files, "app")
	}
}

// TestWaitForReleaseImmediate 验证目标一开始就空闲时不会进入等待。
func TestWaitForReleaseImmediate(t *testing.T) {
	sleeps := 0
	released := waitForRelease(5*pollInterval,
		func() bool { return false },
		func(time.Duration) { sleeps++ },
		newTestLogger(t),
	)

	if !released {
		t.Fatal("waitForRelease() = false, want true when the target is already free")
	}
	if sleeps != 0 {
		t.Fatalf("slept %d times, want 0", sleeps)
	}
}

// TestWaitForReleaseAfterPolls 验证进程在第 N 次探测时才退出：
// 轮询必须继续到观察到释放为止，不能提前放弃。
func TestWaitForReleaseAfterPolls(t *testing.T) {
	const busyPolls = 2
	calls := 0
	sleeps := 0

	released := waitForRelease(5*pollInterval,
		func() bool {
			calls++
			return calls <= busyPolls
		},
		func(time.Duration) { sleeps++ },
		newTestLogger(t),
	)

	if !released {
		t.Fatal("waitForRelease() = false, want true once the process is gone")
	}
	if sleeps != busyPolls {
		t.Fatalf("slept %d times, want %d", sleeps, busyPolls)
	}
}

// TestWaitForReleaseTimeout 验证一直不释放时会按上限退出并返回 false
// ——上层据此记超时日志，但仍然继续尝试替换。
func TestWaitForReleaseTimeout(t *testing.T) {
	attempts := 3
	sleeps := 0

	released := waitForRelease(time.Duration(attempts)*pollInterval,
		func() bool { return true },
		func(time.Duration) { sleeps++ },
		newTestLogger(t),
	)

	if released {
		t.Fatal("waitForRelease() = true, want false when the target is never released")
	}
	if sleeps != attempts {
		t.Fatalf("slept %d times, want %d", sleeps, attempts)
	}
}

// TestWaitForReleaseZeroTimeout 验证上限短于一个轮询周期时立即放弃等待，
// 不会因为整数除法把 attempts 算成 0 而陷入"先睡一觉"。
func TestWaitForReleaseZeroTimeout(t *testing.T) {
	sleeps := 0
	released := waitForRelease(0,
		func() bool { return true },
		func(time.Duration) { sleeps++ },
		newTestLogger(t),
	)

	if released {
		t.Fatal("waitForRelease() = true, want false with a zero timeout")
	}
	if sleeps != 0 {
		t.Fatalf("slept %d times, want 0", sleeps)
	}
}

// TestInstallWritesAtomicallyAndLeavesNoTemp 验证安装结果正确落地，
// 且不留下中间文件（否则它会在下次更新时被当成负载）。
func TestInstallWritesAtomicallyAndLeavesNoTemp(t *testing.T) {
	src := t.TempDir()
	writeTestFile(t, src, "app", "NEW-BINARY")

	dest := filepath.Join(t.TempDir(), "app")
	if err := install(payloadFile{rel: "app", src: filepath.Join(src, "app"), dest: dest}, newTestLogger(t)); err != nil {
		t.Fatalf("install() error: %v", err)
	}

	data, err := os.ReadFile(dest)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "NEW-BINARY" {
		t.Fatalf("installed content = %q, want %q", data, "NEW-BINARY")
	}

	if _, err := os.Stat(dest + ".new"); !os.IsNotExist(err) {
		t.Fatalf("临时文件 .new 未被清理：err = %v", err)
	}
}

// TestInstallCreatesMissingParentDirs 验证目标目录不存在时会被创建，
// 这是"多文件负载带子目录"能落地的前提。
func TestInstallCreatesMissingParentDirs(t *testing.T) {
	src := t.TempDir()
	writeTestFile(t, src, filepath.Join("sub", "helper"), "HELPER")

	dest := filepath.Join(t.TempDir(), "sub", "helper")
	if err := install(payloadFile{rel: filepath.Join("sub", "helper"), src: filepath.Join(src, "sub", "helper"), dest: dest}, newTestLogger(t)); err != nil {
		t.Fatalf("install() error: %v", err)
	}
	if _, err := os.Stat(dest); err != nil {
		t.Fatalf("目标文件未创建：%v", err)
	}
}

// TestRunSingleFileForcesExeName 验证单文件负载会被强制改名为 --exe。
// 压缩包里的文件名不可信（发布流程可能改过名），只认调用方指定的目标名。
func TestRunSingleFileForcesExeName(t *testing.T) {
	src := t.TempDir()
	writeTestFile(t, src, "webcurl-desktop-1.2.3-linux-amd64", "NEW-BINARY")

	target := t.TempDir()
	opts := options{
		source:    src,
		target:    target,
		exe:       "app",
		timeout:   time.Second,
		noRestart: true, // 不真的拉起进程
	}

	if err := run(opts, newTestLogger(t)); err != nil {
		t.Fatalf("run() error: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(target, "app"))
	if err != nil {
		t.Fatalf("目标文件名未被强制为 --exe：%v", err)
	}
	if string(data) != "NEW-BINARY" {
		t.Fatalf("installed content = %q, want %q", data, "NEW-BINARY")
	}
}

// TestRunMultiFileKeepsLayout 验证多文件负载按原相对路径安装。
func TestRunMultiFileKeepsLayout(t *testing.T) {
	src := t.TempDir()
	writeTestFile(t, src, "app", "APP")
	writeTestFile(t, src, filepath.Join("sub", "helper"), "HELPER")

	target := t.TempDir()
	opts := options{
		source:    src,
		target:    target,
		exe:       "app",
		timeout:   time.Second,
		noRestart: true,
	}

	if err := run(opts, newTestLogger(t)); err != nil {
		t.Fatalf("run() error: %v", err)
	}

	for rel, want := range map[string]string{"app": "APP", filepath.Join("sub", "helper"): "HELPER"} {
		data, err := os.ReadFile(filepath.Join(target, rel))
		if err != nil {
			t.Fatalf("读取 %q: %v", rel, err)
		}
		if string(data) != want {
			t.Fatalf("%q 内容 = %q, want %q", rel, data, want)
		}
	}
}

// TestRunRejectsEmptySource 验证空负载会让 run 失败，而不是静默返回成功。
func TestRunRejectsEmptySource(t *testing.T) {
	opts := options{
		source:    t.TempDir(),
		target:    t.TempDir(),
		exe:       "app",
		timeout:   time.Second,
		noRestart: true,
	}

	if err := run(opts, newTestLogger(t)); err == nil {
		t.Fatal("run() = nil, want an error for an empty payload")
	}
}
