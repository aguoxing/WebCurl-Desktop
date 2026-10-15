package updater

import (
	"archive/zip"
	"crypto/ed25519"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"webcurl-desktop/internal/logger"
)

// Apply 依次完成下载、校验、解压，然后把控制权交给与当前可执行文件同目录的外部助手程序。
//
// 成功时返回 ErrRestartRequired，调用方必须退出进程，助手程序才能替换正在运行的二进制。
// 这里不会就地安装任何文件：真正的替换由助手程序完成。
//
// publicKey 是用于校验发布包签名的 Ed25519 公钥（文本形式，见 parsePublicKey）。
// 校验策略按"信任锚是否已钉住"分两种：
//
//   - 公钥已配置：签名**必需**且必须通过，缺失或验签失败一律中止。
//     钉住公钥却不要求签名，等于给了攻击者一条"删掉 .sig 即可绕过"的捷径。
//   - 公钥为空：不启用签名校验，仅在来源给出了签名时记一条警告。
//
// SHA256 摘要的策略与签名不同（能拿到就强制校验、拿不到才告警并继续），
// 这是刻意的：摘要只防传输损坏，签名才防伪造，两者的容忍度不应相同。
func Apply(info *UpdateInfo, publicKey string) error {
	if info == nil || !info.HasUpdate {
		return fmt.Errorf("no update available")
	}
	if info.Checksum == "" {
		logger.Warn().
			Str("version", info.Version).
			Msg("updater: no checksum available, installing without integrity verification")
	}

	pubKey, err := parsePublicKey(publicKey)
	if err != nil {
		return fmt.Errorf("update public key: %w", err)
	}

	workDir, err := os.MkdirTemp("", "gows-update-*")
	if err != nil {
		return fmt.Errorf("create temp dir: %w", err)
	}
	defer os.RemoveAll(workDir)

	// 1. 下载。
	zipPath := filepath.Join(workDir, "update.zip")
	if err := downloadFile(info.URL, zipPath, info.Size); err != nil {
		return fmt.Errorf("download: %w", err)
	}

	// 2. 签名优先于摘要：它是信任锚，其余校验都在此之后才有意义。
	if err := checkSignature(zipPath, info, pubKey); err != nil {
		return err
	}

	// 3. 有摘要就校验。
	if info.Checksum != "" {
		if err := verifyChecksum(zipPath, info.Checksum); err != nil {
			return fmt.Errorf("checksum: %w", err)
		}
	}

	// 4. 解压。
	extractDir := filepath.Join(workDir, "extracted")
	if err := os.MkdirAll(extractDir, 0o755); err != nil {
		return fmt.Errorf("create extract dir: %w", err)
	}
	if err := unzip(zipPath, extractDir); err != nil {
		return fmt.Errorf("extract: %w", err)
	}

	// 剥掉压缩包内唯一的一层顶层目录：发布包可能打成 "<文件>"，
	// 也可能打成 "<名称>/<文件>"，而助手程序期望拿到直接存放负载的那一层。
	sourceDir := extractDir
	entries, err := os.ReadDir(extractDir)
	if err != nil {
		return fmt.Errorf("read extract dir: %w", err)
	}
	if len(entries) == 1 && entries[0].IsDir() {
		sourceDir = filepath.Join(extractDir, entries[0].Name())
	}

	// 5. 在可执行文件同目录定位助手程序。它与主程序一起分发；
	//    新克隆的仓库缺少它时会在这里失败 —— 这正是预期的、可见的失败方式。
	exePath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("locate current executable: %w", err)
	}
	exeDir := filepath.Dir(exePath)
	updaterPath := filepath.Join(exeDir, helperBinaryName())
	if _, err := os.Stat(updaterPath); err != nil {
		return fmt.Errorf("update helper not found at %s: %w", updaterPath, err)
	}

	// 6. 以脱离 stdio 的方式启动助手程序；调用方随后退出，二进制才能被替换。
	cmd := exec.Command(updaterPath,
		"--source", sourceDir,
		"--target", exeDir,
		"--exe", filepath.Base(exePath),
		"--pid", fmt.Sprintf("%d", os.Getpid()),
	)
	cmd.Stdout = nil
	cmd.Stderr = nil

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start update helper: %w", err)
	}

	// 留一点时间让助手程序接管，调用方随后关闭。
	time.Sleep(500 * time.Millisecond)
	return ErrRestartRequired
}

// checkSignature 按"是否钉住了公钥"执行相应的签名策略。
// pubKey 为 nil 表示未配置公钥，此时不启用校验。
func checkSignature(zipPath string, info *UpdateInfo, pubKey ed25519.PublicKey) error {
	if pubKey == nil {
		if info.Signature != "" {
			logger.Warn().
				Str("version", info.Version).
				Msg("updater: release is signed but no public key is pinned, skipping signature verification")
		}
		return nil
	}

	if info.Signature == "" {
		return fmt.Errorf("release %s is unsigned but a public key is pinned: refusing to install", info.Version)
	}

	size, err := fileSize(zipPath)
	if err != nil {
		return fmt.Errorf("signature: %w", err)
	}
	if size > maxSignedArchiveSize {
		return fmt.Errorf("signature: archive is %d bytes, beyond the %d byte verification limit", size, maxSignedArchiveSize)
	}

	if err := verifyFileSignature(zipPath, info.Signature, pubKey); err != nil {
		return fmt.Errorf("signature: %w", err)
	}
	logger.Info().
		Str("version", info.Version).
		Msg("updater: release signature verified")
	return nil
}

// fileSize 返回 path 所指文件的大小。
func fileSize(path string) (int64, error) {
	info, err := os.Stat(path)
	if err != nil {
		return 0, err
	}
	return info.Size(), nil
}

// helperBinaryName 返回当前平台上更新助手程序的文件名。
func helperBinaryName() string {
	if runtime.GOOS == "windows" {
		return "updater.exe"
	}
	return "updater"
}

// downloadFile 把 url 流式写入 destPath。当响应头 Content-Length 或更新源
// 声明的预期大小存在时，两者都会被校验。
func downloadFile(url, destPath string, expectedSize int64) error {
	out, err := os.Create(destPath)
	if err != nil {
		return err
	}
	defer out.Close()

	client := &http.Client{Timeout: 5 * time.Minute}
	resp, err := client.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	written, err := io.Copy(out, resp.Body)
	if err != nil {
		return err
	}
	if resp.ContentLength > 0 && written != resp.ContentLength {
		return fmt.Errorf("size mismatch: Content-Length %d, got %d", resp.ContentLength, written)
	}
	if expectedSize > 0 && written != expectedSize {
		return fmt.Errorf("size mismatch: expected %d, got %d", expectedSize, written)
	}
	return nil
}

// unzip 把 src 解压到 dest，拒绝任何会逃出目标目录的条目（zip-slip），
// 并强制执行 updater.go 中定义的单个文件与总量体积上限。
func unzip(src, dest string) error {
	r, err := zip.OpenReader(src)
	if err != nil {
		return err
	}
	defer r.Close()

	cleanDest := filepath.Clean(dest) + string(os.PathSeparator)
	var total int64

	for _, f := range r.File {
		target := filepath.Join(dest, f.Name)
		if !strings.HasPrefix(filepath.Clean(target), cleanDest) {
			return fmt.Errorf("invalid zip entry: %s", f.Name)
		}

		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(target, f.Mode()); err != nil {
				return fmt.Errorf("create dir %s: %w", f.Name, err)
			}
			continue
		}

		if f.UncompressedSize64 > maxSingleFileExtract {
			return fmt.Errorf("zip entry %s is too large (%d bytes)", f.Name, f.UncompressedSize64)
		}
		total += int64(f.UncompressedSize64)
		if total > maxExtractedTotal {
			return fmt.Errorf("archive expands beyond the %d byte limit", maxExtractedTotal)
		}

		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return fmt.Errorf("create dir for %s: %w", f.Name, err)
		}
		if err := writeZipEntry(f, target); err != nil {
			return err
		}
	}
	return nil
}

// writeZipEntry 把压缩包中的单个条目写入 target。
func writeZipEntry(f *zip.File, target string) error {
	rc, err := f.Open()
	if err != nil {
		return fmt.Errorf("open %s: %w", f.Name, err)
	}
	defer rc.Close()

	out, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, f.Mode())
	if err != nil {
		return fmt.Errorf("create %s: %w", target, err)
	}
	defer out.Close()

	// 复制过程同样限流：包头里声明的大小可能撒谎。
	written, err := io.Copy(out, io.LimitReader(rc, maxSingleFileExtract+1))
	if err != nil {
		return fmt.Errorf("write %s: %w", target, err)
	}
	if written > maxSingleFileExtract {
		return fmt.Errorf("zip entry %s exceeds the %d byte limit", f.Name, maxSingleFileExtract)
	}
	return nil
}
