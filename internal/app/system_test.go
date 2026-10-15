package app

import (
	"runtime/debug"
	"testing"
)

// TestWailsVersionFrom 覆盖构建元数据里找 Wails 版本的几种形态。
//
// 之所以注入 *debug.BuildInfo 而不是直接调用 linkedWailsVersion：
// go test 生成的二进制不记录依赖列表（实测 deps 为 0 条），
// 真实二进制里才有的数据没法在测试进程内取到。
func TestWailsVersionFrom(t *testing.T) {
	localReplace := &debug.Module{Path: "../wails"}
	remoteReplace := &debug.Module{Path: "example.com/wails", Version: "v3.0.0-beta.29"}

	cases := []struct {
		name string
		info *debug.BuildInfo
		want string
	}{
		{
			name: "有构建元数据时取依赖版本",
			info: &debug.BuildInfo{Deps: []*debug.Module{
				{Path: "github.com/other/dep", Version: "v1.0.0"},
				{Path: wailsModulePath, Version: "v3.0.0-beta.28"},
			}},
			want: "v3.0.0-beta.28",
		},
		{
			name: "构建元数据缺失",
			info: nil,
			want: "",
		},
		{
			name: "依赖里没有 Wails",
			info: &debug.BuildInfo{Deps: []*debug.Module{
				{Path: "github.com/other/dep", Version: "v1.0.0"},
			}},
			want: "",
		},
		{
			name: "replace 到已发布版本时报替换目标的版本",
			info: &debug.BuildInfo{Deps: []*debug.Module{
				{Path: wailsModulePath, Version: "v0.0.0-00010101000000-000000000000", Replace: remoteReplace},
			}},
			want: "v3.0.0-beta.29",
		},
		{
			name: "replace 到本地目录时没有可报告的版本",
			info: &debug.BuildInfo{Deps: []*debug.Module{
				{Path: wailsModulePath, Version: "v0.0.0-00010101000000-000000000000", Replace: localReplace},
			}},
			want: "",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := wailsVersionFrom(tc.info); got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}
