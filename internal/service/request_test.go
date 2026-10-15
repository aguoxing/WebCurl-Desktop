package service

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"webcurl-desktop/internal/httpclient"
)

// TestSendShapesTextResponse 验证文本响应被放进 Text 而不是 Base64，
// 并且状态、头、耗时这些展示字段都被填好。
func TestSendShapesTextResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Trace", "t-1")
		_, _ = w.Write([]byte(`{"hello":"世界"}`))
	}))
	defer srv.Close()

	svc := NewRequestService(0, nil)
	res := svc.Send("req-1", httpclient.Request{Method: "GET", URL: srv.URL})

	if res.Error != "" {
		t.Fatalf("Error = %q", res.Error)
	}
	if res.Status != 200 || res.StatusText != "OK" {
		t.Errorf("状态 = %d %q", res.Status, res.StatusText)
	}
	if !res.IsText || res.Text != `{"hello":"世界"}` {
		t.Errorf("文本响应处理错误: isText=%v text=%q", res.IsText, res.Text)
	}
	if res.Base64 != "" {
		t.Error("文本响应不应产生 Base64 预览")
	}
	if res.Size != len(`{"hello":"世界"}`) {
		t.Errorf("Size = %d", res.Size)
	}
	if res.Attempts != 1 {
		t.Errorf("Attempts = %d, 期望 1", res.Attempts)
	}
	if !hasKV(res.Headers, "X-Trace", "t-1") {
		t.Errorf("响应头缺失: %+v", res.Headers)
	}
}

// TestSendShapesBinaryResponseAsBase64 验证二进制响应走 Base64 预览。
func TestSendShapesBinaryResponseAsBase64(t *testing.T) {
	png := []byte{0x89, 0x50, 0x4e, 0x47, 0x00, 0x0d, 0x0a}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(png)
	}))
	defer srv.Close()

	svc := NewRequestService(0, nil)
	res := svc.Send("", httpclient.Request{Method: "GET", URL: srv.URL})

	if res.IsText {
		t.Fatal("image/png 应为二进制")
	}
	if res.Base64 == "" {
		t.Error("二进制响应应给出 Base64 预览")
	}
	if res.SuggestedFileName != "response.png" {
		t.Errorf("建议文件名 = %q, 期望 response.png", res.SuggestedFileName)
	}
}

// TestSendReportsFriendlyError 验证连不上时给的是人话，而不是 url.Error 的包装链。
func TestSendReportsFriendlyError(t *testing.T) {
	// 用一个必然连不上的地址（保留端口上的空闲端口）。
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	url := srv.URL
	srv.Close()

	svc := NewRequestService(0, nil)
	res := svc.Send("", httpclient.Request{Method: "GET", URL: url})

	if res.Error == "" {
		t.Fatal("连不上时应返回错误信息")
	}
	if res.Status != 0 {
		t.Errorf("失败时 Status 应为 0, 实际 %d", res.Status)
	}
}

// TestSendValidationError 验证参数错误也走 Error 字段而不是 panic。
func TestSendValidationError(t *testing.T) {
	svc := NewRequestService(0, nil)
	res := svc.Send("", httpclient.Request{Method: "GET", URL: ""})
	if !strings.Contains(res.Error, "地址不能为空") {
		t.Errorf("Error = %q", res.Error)
	}
}

// TestSaveBodyWritesCompleteContent 验证"另存为"写出的是完整响应体，
// 而不是发给界面的预览片段。
func TestSaveBodyWritesCompleteContent(t *testing.T) {
	// 内容必须是真正的二进制：全是可打印字符的响应会被判定为文本，
	// 走的是另一条分支（整份内容直接进 Text），测不到预览截断。
	big := bytes.Repeat([]byte{0x89, 0x50, 0x4e, 0x47}, (previewBytes+4096)/4)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/octet-stream")
		_, _ = w.Write(big)
	}))
	defer srv.Close()

	svc := NewRequestService(0, nil)
	res := svc.Send("big-1", httpclient.Request{Method: "GET", URL: srv.URL})

	if res.IsText {
		t.Fatal("含 NUL/非 UTF-8 字节的内容应判定为二进制")
	}
	if !res.PreviewTruncated {
		t.Fatalf("超过预览上限（%d 字节）的响应应标记 PreviewTruncated", previewBytes)
	}
	if len(res.Base64) > previewBytes*2 {
		t.Errorf("Base64 预览过大: %d 字符", len(res.Base64))
	}

	path := filepath.Join(t.TempDir(), "out.bin")
	if err := svc.SaveBody("big-1", path); err != nil {
		t.Fatalf("SaveBody: %v", err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("读回文件: %v", err)
	}
	if len(raw) != len(big) {
		t.Errorf("保存的字节数 = %d, 期望 %d（应是完整内容）", len(raw), len(big))
	}
}

// TestSaveBodyUnknownID 验证缓存过期时给出可理解的提示。
func TestSaveBodyUnknownID(t *testing.T) {
	svc := NewRequestService(0, nil)
	err := svc.SaveBody("不存在的ID", filepath.Join(t.TempDir(), "x.bin"))
	if err == nil || !strings.Contains(err.Error(), "已过期") {
		t.Errorf("err = %v, 期望提示内容已过期", err)
	}
}

// TestSendResultIsJSONFriendly 确认结果结构能被序列化给前端
// （Wails 绑定会把返回值编成 JSON，字段标签写错会在此暴露）。
func TestSendResultIsJSONFriendly(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.SetCookie(w, &http.Cookie{Name: "sid", Value: "1"})
		_, _ = w.Write([]byte("ok"))
	}))
	defer srv.Close()

	svc := NewRequestService(0, nil)
	res := svc.Send("j-1", httpclient.Request{Method: "GET", URL: srv.URL})

	raw, err := json.Marshal(res)
	if err != nil {
		t.Fatalf("序列化: %v", err)
	}
	var back map[string]any
	if err := json.Unmarshal(raw, &back); err != nil {
		t.Fatalf("反序列化: %v", err)
	}
	if back["status"].(float64) != 200 {
		t.Errorf("status = %v", back["status"])
	}
	if _, ok := back["isText"]; !ok {
		t.Error("缺少 isText 字段")
	}
}

// TestSuggestFileNameFromDisposition 验证优先采用服务端给的文件名。
func TestSuggestFileNameFromDisposition(t *testing.T) {
	resp := &httpclient.Response{
		Headers: []httpclient.KV{
			{Name: "Content-Disposition", Value: `attachment; filename="报表.xlsx"`},
			{Name: "Content-Type", Value: "application/octet-stream"},
		},
		FinalURL: "http://example.com/download",
	}
	if got := suggestFileName(resp); got != "报表.xlsx" {
		t.Errorf("文件名 = %q, 期望 报表.xlsx", got)
	}
}

// TestSuggestFileNameFallsBackToURL 验证没有 Content-Disposition 时按 URL 与类型推断。
func TestSuggestFileNameFallsBackToURL(t *testing.T) {
	resp := &httpclient.Response{
		Headers:  []httpclient.KV{{Name: "Content-Type", Value: "application/json; charset=utf-8"}},
		FinalURL: "http://example.com/api/users",
	}
	if got := suggestFileName(resp); got != "users.json" {
		t.Errorf("文件名 = %q, 期望 users.json", got)
	}
}

// hasKV 判断键值列表里是否存在指定条目。
func hasKV(list []httpclient.KV, name, value string) bool {
	for _, kv := range list {
		if strings.EqualFold(kv.Name, name) && kv.Value == value {
			return true
		}
	}
	return false
}
