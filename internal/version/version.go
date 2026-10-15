// Package version 承载构建时注入的版本信息。
//
// 构建流程通过 `-ldflags -X` 把版本号、提交哈希与构建时间写进下面的变量，
// 于是二进制**自身**携带版本，不再依赖任何随包分发的配置文件。
// 未注入时（`go run`、`go test`、本地 `task verify`）各变量保持空值，
// 程序照常运行，只是版本显示为 dev。
//
// 注入示例（见 Taskfile.yml 的 VERSION_LDFLAGS）：
//
//	go build -ldflags "-X webcurl-desktop/internal/version.Version=1.2.3 \
//	                   -X webcurl-desktop/internal/version.Commit=a1b2c3d"
package version

import "strings"

// 以下变量只在构建时由 -ldflags -X 赋值。
// 注意它们不能是常量，否则链接器无法改写。
var (
	// Version 是语义化版本号，例如 "1.2.3"，不含前导 v。
	Version string
	// Commit 是构建时的 git 提交短哈希。
	Commit string
	// Date 是构建时间，UTC 的 RFC3339 格式。
	Date string
	// Modified 为 "true" 时表示构建时工作区含有未提交改动。
	Modified string
)

// DevVersion 是未注入版本时的占位值。刻意选用一个明显不是版本号的字面量，
// 以便在日志和界面上都能一眼看出"这是个本地构建"。
const DevVersion = "dev"

// value 返回去掉首尾空白后的注入版本号。注入值本不该含空白，
// 但版本也可能来自环境变量或 CI，这里统一收敛一次。
func value() string {
	return strings.TrimSpace(Version)
}

// IsInjected 报告版本是否由构建流程注入。
func IsInjected() bool {
	return value() != ""
}

// Resolve 返回应当对外报告的版本号：优先构建注入值，其次 fallback
// （通常来自配置文件），两者都没有时返回 DevVersion。
//
// 这个优先级是刻意的——版本号属于二进制本身，把它交给用户可编辑的 YAML
// 会让"当前版本"随配置漂移，进而让更新检查判断错误。
func Resolve(fallback string) string {
	if v := value(); v != "" {
		return v
	}
	if v := strings.TrimSpace(fallback); v != "" {
		return v
	}
	return DevVersion
}

// String 返回适合写入日志的单行描述，例如 "1.2.3 (a1b2c3d, dirty)"。
// 未注入时返回 DevVersion。
func String() string {
	v := value()
	if v == "" {
		return DevVersion
	}

	extra := make([]string, 0, 2)
	if c := strings.TrimSpace(Commit); c != "" {
		extra = append(extra, c)
	}
	if strings.EqualFold(strings.TrimSpace(Modified), "true") {
		extra = append(extra, "dirty")
	}
	if len(extra) == 0 {
		return v
	}
	return v + " (" + strings.Join(extra, ", ") + ")"
}

// Full 返回包含构建时间的完整描述，供 --version 输出与问题排障使用。
func Full() string {
	out := String()
	if IsInjected() {
		if d := strings.TrimSpace(Date); d != "" {
			out += " built " + d
		}
	}
	return out
}
