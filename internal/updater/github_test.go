package updater

import (
	"net/http"
	"net/http/httptest"
	"runtime"
	"strings"
	"testing"
)

// withLocalGitHub 把两个端点前缀指向同一个本地服务，并在测试结束后复原。
//
// 这里刻意不用 t.Parallel：改的是包级变量。
func withLocalGitHub(t *testing.T, handler http.HandlerFunc) {
	t.Helper()

	server := httptest.NewServer(handler)

	apiBase, webBase := githubAPIBase, githubWebBase
	githubAPIBase, githubWebBase = server.URL, server.URL
	t.Cleanup(func() {
		githubAPIBase, githubWebBase = apiBase, webBase
		server.Close()
	})
}

// TestCheckGitHubFallsBackToWebWhenRateLimited 覆盖这条降级路径：
// 匿名额度用尽（403 且 X-RateLimit-Remaining: 0）不该把错误抛给用户，
// 而应当改走公开页面与下载路径完成检查。
func TestCheckGitHubFallsBackToWebWhenRateLimited(t *testing.T) {
	const repo = "owner/repo"

	assetName := "webcurl-desktop-v9.9.9-" + runtime.GOOS + "-" + runtime.GOARCH + ".zip"
	digest := strings.Repeat("a", 64)

	withLocalGitHub(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasPrefix(r.URL.Path, "/repos/"):
			w.Header().Set("X-RateLimit-Remaining", "0")
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte(`{"message":"API rate limit exceeded for 203.0.113.7."}`))

		case r.URL.Path == "/"+repo+"/releases/latest":
			http.Redirect(w, r, "/"+repo+"/releases/tag/v9.9.9", http.StatusFound)

		case r.URL.Path == "/"+repo+"/releases/download/v9.9.9/SHA256SUMS":
			_, _ = w.Write([]byte(digest + "  " + assetName + "\n"))

		default:
			// 签名文件的 HEAD 探测：这次发布没有签名。
			w.WriteHeader(http.StatusNotFound)
		}
	})

	info, err := CheckGitHub(repo, "", "1.0.0")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if info == nil || !info.HasUpdate {
		t.Fatal("expected an update to be reported")
	}
	if info.Version != "9.9.9" {
		t.Errorf("got version %q, want 9.9.9", info.Version)
	}
	if info.AssetName != assetName {
		t.Errorf("got asset %q, want %q", info.AssetName, assetName)
	}
	if info.Checksum != "sha256:"+digest {
		t.Errorf("got checksum %q, want sha256:%s", info.Checksum, digest)
	}
	if !strings.HasSuffix(info.URL, "/releases/download/v9.9.9/"+assetName) {
		t.Errorf("got url %q", info.URL)
	}
}

// TestCheckGitHubKeepsNonRateLimitErrors 确认降级只针对限流：
// 凭据错误这类真正的失败必须原样报出来，不能被「兜底」掩盖掉。
func TestCheckGitHubKeepsNonRateLimitErrors(t *testing.T) {
	withLocalGitHub(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"message":"Bad credentials"}`))
	})

	_, err := CheckGitHub("owner/repo", "expired-token", "1.0.0")
	if err == nil {
		t.Fatal("expected an error")
	}
	if !strings.Contains(err.Error(), "Bad credentials") {
		t.Fatalf("the API response body should be surfaced, got: %v", err)
	}
}

// TestCheckGitHubWebPathRespectsVersionGate 公开页面路径也要遵守版本比较：
// tag 不比当前版本新时返回 nil，而且不该再去抓清单。
func TestCheckGitHubWebPathRespectsVersionGate(t *testing.T) {
	const repo = "owner/repo"

	withLocalGitHub(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasPrefix(r.URL.Path, "/repos/"):
			w.Header().Set("X-RateLimit-Remaining", "0")
			w.WriteHeader(http.StatusForbidden)

		case r.URL.Path == "/"+repo+"/releases/latest":
			http.Redirect(w, r, "/"+repo+"/releases/tag/v1.0.0", http.StatusFound)

		default:
			t.Errorf("unexpected request to %s: nothing else is needed when the tag is not newer", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	})

	info, err := CheckGitHub(repo, "", "1.0.0")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if info != nil {
		t.Fatalf("got %+v, want nil when the release is not newer", info)
	}
}

func TestIsRateLimited(t *testing.T) {
	cases := []struct {
		name   string
		status int
		remain string
		body   string
		want   bool
	}{
		{name: "header reports zero", status: http.StatusForbidden, remain: "0", want: true},
		{name: "body mentions rate limit", status: http.StatusForbidden, remain: "57",
			body: `{"message":"API rate limit exceeded"}`, want: true},
		{name: "too many requests", status: http.StatusTooManyRequests, want: true},
		{name: "forbidden for another reason", status: http.StatusForbidden, remain: "57",
			body: `{"message":"Bad credentials"}`, want: false},
		{name: "success", status: http.StatusOK, want: false},
		{name: "not found", status: http.StatusNotFound, want: false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resp := &http.Response{StatusCode: tc.status, Header: http.Header{}}
			if tc.remain != "" {
				resp.Header.Set("X-RateLimit-Remaining", tc.remain)
			}
			if got := isRateLimited(resp, tc.body); got != tc.want {
				t.Errorf("got %v, want %v", got, tc.want)
			}
		})
	}
}

func TestPickPlatformAsset(t *testing.T) {
	digest := "sha256:" + strings.Repeat("a", 64)
	wanted := "app-v1.0.0-" + runtime.GOOS + "-" + runtime.GOARCH + ".zip"

	entries := map[string]string{
		"SHA256SUMS":                          "sha256:" + strings.Repeat("0", 64),
		wanted:                                digest,
		"app-v1.0.0-" + runtime.GOOS + ".txt": "sha256:" + strings.Repeat("b", 64),
	}

	name, sum := pickPlatformAsset(entries)
	if name != wanted {
		t.Fatalf("got %q, want %q", name, wanted)
	}
	if sum != digest {
		t.Fatalf("got %q, want %q", sum, digest)
	}

	if name, _ := pickPlatformAsset(map[string]string{"SHA256SUMS": digest}); name != "" {
		t.Fatalf("got %q, want an empty name when nothing matches this platform", name)
	}
}

func TestParseChecksumEntries(t *testing.T) {
	digest := strings.Repeat("a", 64)
	input := digest + "  app.zip\n" +
		digest + " *app2.zip\n" +
		"\n" +
		digest + "  sub/dir/app3.zip\n"

	entries, err := parseChecksumEntries(strings.NewReader(input))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(entries) != 3 {
		t.Fatalf("got %d entries, want 3: %v", len(entries), entries)
	}
	// 词典里的名字要去掉路径与 sha256sum 二进制模式的 "*"，摘要统一归一化。
	for _, want := range []string{"app.zip", "app2.zip", "app3.zip"} {
		if entries[want] != "sha256:"+digest {
			t.Errorf("entry %q: got %q, want sha256:%s", want, entries[want], digest)
		}
	}
}
