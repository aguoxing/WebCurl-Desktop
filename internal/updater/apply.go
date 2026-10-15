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

const (
	// workDirPrefix 是更新工作目录的临时目录前缀，也是 cleanupStaleWorkDirs
	// 用来辨认「上一次更新留下的残迹」的依据。
	workDirPrefix = "gows-update-"

	// helperDirName 是工作目录中存放助手副本的子目录。
	helperDirName = "helper"

	// cleanupEnvVar 由主程序传给助手，指明「安装完成后可以删除」的工作目录。
	//
	// 用环境变量而不是新增命令行参数，是为了向前兼容：升级包里的助手可能与
	// 正在运行的那份不同版本，而旧版助手的 flag 集会因为陌生参数直接以退出码 2
	// 拒绝启动，未知的环境变量则只会被安静地忽略。
	cleanupEnvVar = "GOWS_UPDATE_CLEANUP"

	// staleWorkDirAge 是清理残留工作目录的年龄下限。取得足够大，是为了绝无可能
	// 误删另一次正在进行中的更新所使用的目录。
	staleWorkDirAge = time.Hour
)

// Apply 依次完成下载、校验、解压，然后把控制权交给一个独立的助手程序。
//
// 成功时返回 ErrRestartRequired，调用方必须退出进程，助手程序才能替换正在运行的二进制。
// 这里不会就地安装任何文件：真正的替换由助手程序完成。
//
// 助手优先取发布包自带的那一份，其次才回退到可执行文件同目录的旧副本
// （见 locateHelper）。无论取自哪里，都会先复制到临时目录再运行 —— 这样安装目录
// 里的 updater 在替换阶段不是「运行中的映像」，可以被升级包里附带的新版本覆盖。
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

	// 上一次更新留下的助手副本删不掉自己（Windows 不允许删除运行中的映像），
	// 顺手在这里清掉：此刻它早已退出，删除一定成功。
	cleanupStaleWorkDirs()

	workDir, err := os.MkdirTemp("", workDirPrefix+"*")
	if err != nil {
		return fmt.Errorf("create temp dir: %w", err)
	}
	// 失败路径由本函数收尾；一旦把控制权交给了助手，清理由它接手。
	//
	// 这里曾是无条件的 defer 清理，那样会在助手读到 --source 之前就删掉解压目录：
	// 助手要先等主进程退出才开始安装，而主进程正是在本函数返回之后才退出 ——
	// 目录没了，安装必然失败。
	handedOff := false
	defer func() {
		if !handedOff {
			_ = os.RemoveAll(workDir)
		}
	}()

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

	// 5. 定位助手程序。发布包自带的那份优先：它与主程序同批发布，版本天然一致；
	//    可执行文件同目录的那份只作旧发布包的兜底（当时的 zip 里没有它）。
	exePath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("locate current executable: %w", err)
	}
	exeDir := filepath.Dir(exePath)

	helperSrc := locateHelper(
		filepath.Join(sourceDir, helperBinaryName()),
		filepath.Join(exeDir, helperBinaryName()),
	)
	if helperSrc == "" {
		return fmt.Errorf("update helper %s not found in the release package or in %s",
			helperBinaryName(), exeDir)
	}

	// 6. 复制到临时目录再运行。正在运行的映像无法在 Windows 上被覆盖，而升级包里
	//    通常也带着新一版助手，需要能落到安装目录里。
	helperPath := filepath.Join(workDir, helperDirName, helperBinaryName())
	if err := os.MkdirAll(filepath.Dir(helperPath), 0o755); err != nil {
		return fmt.Errorf("create helper dir: %w", err)
	}
	if err := copyExecutable(helperSrc, helperPath); err != nil {
		return fmt.Errorf("stage update helper: %w", err)
	}

	// 7. 以脱离 stdio 的方式启动助手程序；调用方随后退出，二进制才能被替换。
	cmd := exec.Command(helperPath,
		"--source", sourceDir,
		"--target", exeDir,
		"--exe", filepath.Base(exePath),
		"--pid", fmt.Sprintf("%d", os.Getpid()),
	)
	cmd.Stdout = nil
	cmd.Stderr = nil
	cmd.Env = append(os.Environ(), cleanupEnvVar+"="+workDir)

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start update helper: %w", err)
	}
	handedOff = true

	// 留一点时间让助手程序接管，调用方随后关闭。
	time.Sleep(500 * time.Millisecond)
	return ErrRestartRequired
}

// locateHelper 返回 candidates 中第一个存在的助手程序路径，都不存在时返回空串。
func locateHelper(candidates ...string) string {
	for _, path := range candidates {
		if st, err := os.Stat(path); err == nil && !st.IsDir() {
			return path
		}
	}
	return ""
}

// copyExecutable 把 src 复制为 dst，并显式赋予可执行权限。
//
// 权限不能指望继承：源文件可能来自解压出来的 zip 条目，而 zip 记录的权限位
// 并不可靠，umask 也可能把它抹掉。助手必须处于可执行状态。
func copyExecutable(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		return err
	}
	if err := out.Close(); err != nil {
		return err
	}
	return os.Chmod(dst, 0o755)
}

// cleanupStaleWorkDirs 清理此前更新遗留在系统临时目录里的工作目录。
//
// 助手副本就放在工作目录里，而 Windows 不允许删除正在运行的映像，因此助手自己
// 收尾时总会留下自己那一份。与其为它引入平台相关的自删除技巧，不如让下一次更新
// 顺手清掉 —— 此时那个助手早已退出，删除一定成功。
//
// 只清理超过 staleWorkDirAge 的目录，绝不触碰可能仍在进行的更新。
func cleanupStaleWorkDirs() {
	entries, err := os.ReadDir(os.TempDir())
	if err != nil {
		return
	}
	cutoff := time.Now().Add(-staleWorkDirAge)
	for _, e := range entries {
		if !e.IsDir() || !strings.HasPrefix(e.Name(), workDirPrefix) {
			continue
		}
		info, err := e.Info()
		if err != nil || info.ModTime().After(cutoff) {
			continue
		}
		_ = os.RemoveAll(filepath.Join(os.TempDir(), e.Name()))
	}
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
