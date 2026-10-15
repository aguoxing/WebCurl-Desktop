package version

import (
	"strings"
	"testing"
)

// stub 临时改写构建注入的变量，并在用例结束时恢复。
// 这些变量本应由 -ldflags -X 写入，测试里直接赋值是最贴近的模拟方式。
func stub(t *testing.T, v, commit, date, modified string) {
	t.Helper()
	oldV, oldC, oldD, oldM := Version, Commit, Date, Modified
	Version, Commit, Date, Modified = v, commit, date, modified
	t.Cleanup(func() {
		Version, Commit, Date, Modified = oldV, oldC, oldD, oldM
	})
}

func TestIsInjected(t *testing.T) {
	stub(t, "", "", "", "")
	if IsInjected() {
		t.Fatal("空版本不应被视为已注入")
	}

	// 只含空白同样视为未注入：它多半来自空的 -X 或环境变量。
	stub(t, "   ", "", "", "")
	if IsInjected() {
		t.Fatal("纯空白版本不应被视为已注入")
	}

	stub(t, "1.2.3", "", "", "")
	if !IsInjected() {
		t.Fatal("非空版本应被视为已注入")
	}
}

func TestResolve(t *testing.T) {
	t.Run("未注入时回退到配置文件的值", func(t *testing.T) {
		stub(t, "", "", "", "")
		if got := Resolve("0.0.1"); got != "0.0.1" {
			t.Fatalf("Resolve = %q, want %q", got, "0.0.1")
		}
	})

	t.Run("未注入且无回退值时给出 dev", func(t *testing.T) {
		stub(t, "", "", "", "")
		if got := Resolve("   "); got != DevVersion {
			t.Fatalf("Resolve = %q, want %q", got, DevVersion)
		}
	})

	t.Run("注入值优先于配置文件", func(t *testing.T) {
		// 这是本包存在的理由：用户改 YAML 不能改变二进制的版本身份。
		stub(t, "1.2.3", "", "", "")
		if got := Resolve("9.9.9"); got != "1.2.3" {
			t.Fatalf("Resolve = %q, want %q", got, "1.2.3")
		}
	})

	t.Run("注入值周围的空白被裁掉", func(t *testing.T) {
		stub(t, "  1.2.3  ", "", "", "")
		if got := Resolve("9.9.9"); got != "1.2.3" {
			t.Fatalf("Resolve = %q, want %q", got, "1.2.3")
		}
	})
}

func TestString(t *testing.T) {
	cases := []struct {
		name                      string
		version, commit, modified string
		want                      string
	}{
		{"未注入", "", "", "", DevVersion},
		{"仅版本号", "1.2.3", "", "", "1.2.3"},
		{"版本 + 提交", "1.2.3", "a1b2c3d", "", "1.2.3 (a1b2c3d)"},
		{"未提交改动", "1.2.3", "a1b2c3d", "true", "1.2.3 (a1b2c3d, dirty)"},
		{"脏标记大小写不敏感", "1.2.3", "", "TRUE", "1.2.3 (dirty)"},
		{"只有脏标记", "1.2.3", "", "false", "1.2.3"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			stub(t, tc.version, tc.commit, "", tc.modified)
			if got := String(); got != tc.want {
				t.Fatalf("String = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestFull(t *testing.T) {
	t.Run("未注入时等同 dev", func(t *testing.T) {
		stub(t, "", "a1b2c3d", "2026-10-11T00:00:00Z", "")
		if got := Full(); got != DevVersion {
			t.Fatalf("Full = %q, want %q", got, DevVersion)
		}
	})

	t.Run("注入后带上构建时间", func(t *testing.T) {
		stub(t, "1.2.3", "a1b2c3d", "2026-10-11T00:00:00Z", "")
		want := "1.2.3 (a1b2c3d) built 2026-10-11T00:00:00Z"
		if got := Full(); got != want {
			t.Fatalf("Full = %q, want %q", got, want)
		}
	})
}

// TestInjectedBuild 只在真正注入了版本时执行，用于对发布产物做自检：
//
//	go test -ldflags "$(go run ./cmd/versionldflags)" ./internal/version
//
// 常规 `go test ./...` 会跳过它，因为那时版本本来就没注入。
func TestInjectedBuild(t *testing.T) {
	if !IsInjected() {
		t.Skip("未注入版本；用 -ldflags 注入后此用例才有意义")
	}
	if got := Resolve("bogus"); got != Version {
		t.Fatalf("注入后 Resolve 应返回注入值，得到 %q（注入值为 %q）", got, Version)
	}
	if !strings.Contains(String(), strings.TrimSpace(Version)) {
		t.Fatalf("String() = %q 应包含注入的版本号 %q", String(), Version)
	}
}
