package updater

import (
	"fmt"
	"strings"
)

// versionNewer 判断 tag 是否严格新于 current。两者都是不带 "v" 前缀的
// 语义化版本字符串；current 为空时一律视为更旧。
//
// compareSegments 会忽略预发布/构建后缀，所以 "1.0.0-rc1" 与 "1.0.0" 判定为相等。
// 这是刻意为之：当正式版发布时，启动模板把已安装的预发布版视为同一个版本。
func versionNewer(tag, current string) bool {
	if tag == "" {
		return false
	}
	if current == "" {
		return true
	}
	return compareSegments(tag, current) > 0
}

// compareSegments 按点号分段比较两个数字版本号，缺失的段按 0 处理，
// 返回 -1、0 或 1。
func compareSegments(a, b string) int {
	sa := strings.Split(a, ".")
	sb := strings.Split(b, ".")
	n := len(sa)
	if len(sb) > n {
		n = len(sb)
	}
	for i := 0; i < n; i++ {
		va, vb := 0, 0
		if i < len(sa) {
			va = parseSegment(sa[i])
		}
		if i < len(sb) {
			vb = parseSegment(sb[i])
		}
		if va > vb {
			return 1
		}
		if va < vb {
			return -1
		}
	}
	return 0
}

// parseSegment 取版本段开头的整数，丢弃预发布（"-rc1"）与构建（"+meta"）后缀。
// 无法解析时返回 0。
func parseSegment(s string) int {
	if i := strings.IndexAny(s, "-+"); i >= 0 {
		s = s[:i]
	}
	var n int
	fmt.Sscanf(s, "%d", &n)
	return n
}
