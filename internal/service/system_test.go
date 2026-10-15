package service

import "testing"

// TestWebviewFromPlatform 覆盖各平台上报的键名差异与 server/移动端的空结果。
func TestWebviewFromPlatform(t *testing.T) {
	cases := []struct {
		name        string
		platform    map[string]any
		goos        string
		wantName    string
		wantVersion string
	}{
		{
			name:        "Windows 上报 WebView2",
			platform:    map[string]any{"WebView2": "131.0.2903.86"},
			goos:        "windows",
			wantName:    "WebView2",
			wantVersion: "131.0.2903.86",
		},
		{
			name:        "Linux GTK4 上报 webkitgtk6",
			platform:    map[string]any{"webkitgtk6-runtime": "2.46.5"},
			goos:        "linux",
			wantName:    "WebKitGTK 6",
			wantVersion: "2.46.5",
		},
		{
			name:     "Linux GTK3 上报 webkit2gtk",
			platform: map[string]any{"webkit2gtk-runtime": "2.44.0"},
			goos:     "linux",
			// GTK3 那条键名不含主版本，统一显示为 WebKitGTK。
			wantName:    "WebKitGTK",
			wantVersion: "2.44.0",
		},
		{
			name:     "Windows 探测失败时版本为空",
			platform: map[string]any{"WebView2": ""},
			goos:     "windows",
		},
		{
			name:     "macOS 不报 webview 版本",
			platform: map[string]any{},
			goos:     "darwin",
			// 实现名可推断，版本只能留空，界面据此只显示名字。
			wantName: "WKWebView",
		},
		{
			name:     "macOS 平台信息为 nil 同样返回实现名",
			platform: nil,
			goos:     "darwin",
			wantName: "WKWebView",
		},
		{
			name:     "server 构建没有 webview",
			platform: map[string]any{"mode": "server"},
			goos:     "windows",
			// 必须优先识别 server，否则会被 GOOS 误判成 WebView2。
		},
		{
			name:     "移动端不报 webview",
			platform: map[string]any{"platform": "ios"},
			goos:     "ios",
		},
		{
			name:     "空平台信息",
			platform: map[string]any{},
			goos:     "windows",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			name, version := WebviewFromPlatform(tc.platform, tc.goos)
			if name != tc.wantName || version != tc.wantVersion {
				t.Fatalf("got (%q, %q), want (%q, %q)", name, version, tc.wantName, tc.wantVersion)
			}
		})
	}
}

// TestSystemServiceWithoutSnapshot 验证回调缺失时返回零值，
// 而不是 panic 或编造版本号。
func TestSystemServiceWithoutSnapshot(t *testing.T) {
	svc := NewSystemService(nil)

	if got := svc.GetRuntimeInfo(); got != (RuntimeInfo{}) {
		t.Fatalf("got %+v, want zero value", got)
	}
}

// TestSystemServiceReturnsSnapshot 验证注入的回调每个字段都被原样带出。
func TestSystemServiceReturnsSnapshot(t *testing.T) {
	want := RuntimeInfo{
		WailsVersion:   "v3.0.0-beta.28",
		WebviewName:    "WebView2",
		WebviewVersion: "131.0.2903.86",
		OS:             "windows",
		Arch:           "amd64",
	}
	svc := NewSystemService(func() RuntimeInfo { return want })

	if got := svc.GetRuntimeInfo(); got != want {
		t.Fatalf("got %+v, want %+v", got, want)
	}
}
