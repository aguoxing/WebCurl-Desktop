package buildinfo

import "testing"

func TestNormalizeDescribe(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want string
	}{
		{"带 v 前缀的 tag", "v1.2.3", "1.2.3"},
		{"不带前缀的 tag", "1.2.3", "1.2.3"},
		{"tag 之后还有提交", "v1.2.3-4-gabc1234", "1.2.3-4-gabc1234"},
		{"没有 tag 时回退成短哈希", "abc1234", "0.0.0-abc1234"},
		{"空输入", "", ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := normalizeDescribe(tc.raw); got != tc.want {
				t.Fatalf("normalizeDescribe(%q) = %q, want %q", tc.raw, got, tc.want)
			}
		})
	}
}

func TestWindowsVersion(t *testing.T) {
	cases := []struct {
		name    string
		version string
		want    string
	}{
		{"恰好四段", "1.2.3.4", "1.2.3.4"},
		{"三段补零", "1.2.3", "1.2.3.0"},
		{"两段补零", "1.2", "1.2.0.0"},
		{"一段补零", "1", "1.0.0.0"},
		// 这是最常见的一种：git describe 在 tag 之后会带 -<n>-g<hash> 后缀。
		{"带 git describe 后缀", "1.0.2-2-g93ed8b0", "1.0.2.0"},
		{"没有 tag 的本地构建", "0.0.0-93ed8b0", "0.0.0.0"},
		{"超过四段时截断", "1.2.3.4.5", "1.2.3.4"},
		{"前导零被规范化", "01.02.03", "1.2.3.0"},
		{"空版本", "", FallbackWindowsVersion},
		{"纯非数字", "dev", FallbackWindowsVersion},
		{"其中一段非数字", "1.2.x", FallbackWindowsVersion},
		{"负数段", "-1.2", FallbackWindowsVersion},
		{"超过 65535", "1.2.70000", FallbackWindowsVersion},
		{"恰好 65535", "1.2.65535", "1.2.65535.0"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := WindowsVersion(tc.version); got != tc.want {
				t.Fatalf("WindowsVersion(%q) = %q, want %q", tc.version, got, tc.want)
			}
		})
	}
}

// TestResolvePrefersEnv 只覆盖环境变量这条确定性分支：其余分支取决于
// 测试机是否在 git 仓库中、以及工作区是否干净，断言它们会把用例绑死在环境上。
func TestResolvePrefersEnv(t *testing.T) {
	t.Run("GOWS_VERSION 优先于 git describe", func(t *testing.T) {
		t.Setenv("GOWS_VERSION", "v9.9.9")
		if got := Resolve().Version; got != "9.9.9" {
			t.Fatalf("Resolve().Version = %q, want %q", got, "9.9.9")
		}
	})

	t.Run("版本号周围的空白被裁掉", func(t *testing.T) {
		t.Setenv("GOWS_VERSION", "  1.2.3  ")
		if got := Resolve().Version; got != "1.2.3" {
			t.Fatalf("Resolve().Version = %q, want %q", got, "1.2.3")
		}
	})

	t.Run("构建时间总是填上", func(t *testing.T) {
		t.Setenv("GOWS_VERSION", "1.2.3")
		if got := Resolve().Date; got == "" {
			t.Fatal("Resolve().Date 不应为空")
		}
	})
}
