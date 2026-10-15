// Package buildinfo 推导构建期的版本信息，是版本号的**唯一来源**。
//
// 构建流程有两个出口需要同一个版本号，它们曾经各算各的：
//
//   - cmd/versionldflags 把版本经 `-ldflags -X` 注入二进制，这是应用内
//     「关于」页显示的版本；
//   - cmd/wininfo 把它写进 Windows 版本资源，这是文件属性里显示的版本。
//
// 两份实现一旦漂移，就会出现「应用内 1.0.2、文件属性 1.0.1」这种自相矛盾的产物。
// 因此版本只在 Resolve 里推导一次，两个命令都只是把结果渲染成各自需要的格式。
//
// 取值优先级：$GOWS_VERSION > git describe > 空（不在 git 仓库中）。
package buildinfo

import (
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// FallbackWindowsVersion 是推导不出数字版本时写入 PE 版本资源的值。
// PE 的版本字段只接受四段数字，"没有版本"在那里只能这样表达。
const FallbackWindowsVersion = "0.0.0.0"

// Info 是一次构建的版本信息。零值表示推导不出任何版本。
type Info struct {
	// Version 是完整版本号，形如 "1.2.3" 或 "1.2.3-4-gabc1234"，不含前导 v。
	Version string
	// Commit 是 git 提交短哈希；不在 git 仓库中时为空。
	Commit string
	// Modified 表示构建时工作区含未提交改动。
	Modified bool
	// Date 是构建时间，UTC 的 RFC3339 格式。
	Date string
}

// Resolve 读取环境与 git 推导本次构建的版本信息。
//
// 环境变量与 git 都是「尽力而为」：任何一步失败都只是让对应字段留空，
// 不会中断构建——源码压缩包里没有 git，也应当能构建出东西来。
func Resolve() Info {
	inGit := isGitRepo()
	version := strings.TrimPrefix(strings.TrimSpace(os.Getenv("GOWS_VERSION")), "v")

	// 脏标记与版本号来源无关：即便显式指定了版本，也要如实反映工作区状态，
	// 避免把未提交的改动当作正式版本发布出去。
	info := Info{Modified: inGit && isDirty(), Date: buildTime()}

	if version == "" && inGit {
		version = normalizeDescribe(gitDescribe())
	}
	info.Version = version

	if inGit {
		info.Commit = gitShortHash()
	}
	return info
}

// WindowsVersion 把版本号压成 PE 版本资源要求的四段数字形式。
//
// VS_FIXEDFILEINFO 只能存四个 16 位整数，而注入的版本号可能带 git describe
// 后缀，因此这里只取第一个连字符之前的部分，不足四段补 0：
//
//	1.2.3-4-gabc1234 → 1.2.3.0
//	1.2              → 1.2.0.0
//	1.2.3.4.5        → 1.2.3.4（超出的段被丢弃）
//
// 任何一段不是 0..65535 的整数（含纯非数字的版本号）都退回 0.0.0.0 ——
// PE 字段塞不进非数字，硬凑只会写出一个看起来有意义的假版本。
func WindowsVersion(version string) string {
	base := version
	if i := strings.IndexByte(base, '-'); i >= 0 {
		base = base[:i]
	}

	segments := strings.Split(base, ".")
	if len(segments) > 4 {
		segments = segments[:4]
	}

	out := make([]string, 4)
	for i := range out {
		out[i] = "0"
	}
	for i, segment := range segments {
		n, err := strconv.Atoi(segment)
		if err != nil || n < 0 || n > 65535 {
			return FallbackWindowsVersion
		}
		out[i] = strconv.Itoa(n)
	}
	return strings.Join(out, ".")
}

// normalizeDescribe 把 git describe 的原始输出转成用于展示的版本号：
// 形如 1.2.3 的 tag（或 1.2.3-3-gabc1234）直接采用（去掉 v 前缀）；
// 没有 tag 时 --always 会回退成短哈希，拼成 0.0.0-<hash>，保证低于任何正式版本。
func normalizeDescribe(raw string) string {
	raw = strings.TrimPrefix(raw, "v")
	switch {
	case raw == "":
		return ""
	case strings.Contains(raw, "."):
		return raw
	default:
		return "0.0.0-" + raw
	}
}

// runGit 执行 git 子命令并返回裁剪后的 stdout。git 不存在或命令失败都返回
// ok=false，调用方据此跳过对应字段——与旧 shell 脚本用 `command -v git`
// 先探测的效果一致。
func runGit(args ...string) (string, bool) {
	out, err := exec.Command("git", args...).Output()
	if err != nil {
		return "", false
	}
	return strings.TrimSpace(string(out)), true
}

func isGitRepo() bool {
	_, ok := runGit("rev-parse", "--git-dir")
	return ok
}

func isDirty() bool {
	out, ok := runGit("status", "--porcelain")
	return ok && out != ""
}

func gitDescribe() string {
	out, _ := runGit("describe", "--tags", "--always") // 失败即视为无版本，返回空串
	return out
}

func gitShortHash() string {
	out, _ := runGit("rev-parse", "--short", "HEAD") // 失败时该字段省略，不影响其余注入
	return out
}

// buildTime 输出与 `date -u +%Y-%m-%dT%H:%M:%SZ` 一致的 UTC 时间戳。
func buildTime() string {
	return time.Now().UTC().Format("2006-01-02T15:04:05") + "Z"
}
