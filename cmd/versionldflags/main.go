// Command versionldflags 打印构建时注入用的 -ldflags 片段。
//
// 它取代了原先的 scripts/version-ldflags.sh。原因很具体：Taskfile 的
// VERSION_LDFLAGS 变量在**任何 task 执行之前**就会被求值，而 Task 求值 `sh:`
// 时是直接 exec `sh`。Windows 上的 sh 通常只随 Git Bash 提供、并不保证在 PATH 里，
// 于是连不需要版本注入的 `wails3 dev` 也会在变量求值阶段失败
// （"sh": executable file not found in $PATH）。改用 Go 实现后，Taskfile 只需
// `go run`，不依赖任何 POSIX shell。
//
// 输出是一行以空格分隔的 `-X 包.变量=值` 片段，调用方会把它原样拼进
// `go build -ldflags "..."`，所以这里**不做任何引用转义**（各值都不含空格）。
//
// 注入两类内容：
//  1. 版本信息（internal/version），取值由 internal/buildinfo 统一推导：
//     显式指定的 $GOWS_VERSION 优先，否则取 git describe，不在 git 仓库中则整组跳过
//     （程序显示 dev）。提交哈希与构建时间尽力而为地采集，取不到就省略对应字段。
//  2. 更新验签公钥（internal/updater.EmbeddedPublicKey），来自 $GOWS_UPDATE_PUBLIC_KEY，
//     由 scripts/build.sh 依据 --sign-key 推导后导出。
//
// 拿不到任何可注入项时输出为空，调用方据此不添加 -ldflags。
package main

import (
	"fmt"
	"os"
	"strings"

	"webcurl-desktop/internal/buildinfo"
)

const (
	versionModule = "webcurl-desktop/internal/version"
	updaterModule = "webcurl-desktop/internal/updater"
)

func main() {
	fragment := ldflagsFragment()
	if fragment != "" {
		fmt.Println(fragment)
	}
}

// ldflagsFragment 拼出注入片段；没有任何可注入项时返回空串。
func ldflagsFragment() string {
	var parts []string
	add := func(flag string) { parts = append(parts, flag) }

	info := buildinfo.Resolve()

	// 有版本才注入版本信息。拿不到版本时跳过这一组，但**不能**直接返回 ——
	// 公钥与版本无关，必须继续处理。
	if info.Version != "" {
		add("-X " + versionModule + ".Version=" + info.Version)

		if info.Commit != "" {
			add("-X " + versionModule + ".Commit=" + info.Commit)
		}
		add("-X " + versionModule + ".Date=" + info.Date)

		if info.Modified {
			add("-X " + versionModule + ".Modified=true")
		}
	}

	if key := os.Getenv("GOWS_UPDATE_PUBLIC_KEY"); key != "" {
		add("-X " + updaterModule + ".EmbeddedPublicKey=" + key)
	}

	return strings.Join(parts, " ")
}
