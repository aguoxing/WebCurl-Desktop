package httpclient

import (
	"net/http"
	"net/textproto"
	"sort"
	"strings"
)

// sortedHeaderNames 返回按字典序排列的头名。
//
// Go 的 http.Header 是 map，直接遍历顺序随机，界面上每次刷新都换一个顺序会让人
// 以为内容变了。排序是这里唯一稳定的做法（保留服务端原始顺序需要自己解析报文，
// 不值得为展示效果付出这个代价）。
func sortedHeaderNames(h http.Header) []string {
	names := make([]string, 0, len(h))
	for name := range h {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// filePartHeader 构造 multipart 文件段的头部。
//
// 必须用 textproto.MIMEHeader 而不是手拼字符串：文件名可能含中文、引号或换行，
// 手拼会直接产生畸形的 multipart，而 CreateFormFile 又无法指定 Content-Type。
func filePartHeader(fieldName, fileName, contentType string) textproto.MIMEHeader {
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	h := make(textproto.MIMEHeader)
	h.Set("Content-Disposition", `form-data; name="`+escapeQuotes(fieldName)+`"; filename="`+escapeQuotes(fileName)+`"`)
	h.Set("Content-Type", contentType)
	return h
}

// escapeQuotes 按 RFC 7578 转义引号与反斜杠，并剔除换行。
func escapeQuotes(s string) string {
	s = strings.ReplaceAll(s, "\r", "")
	s = strings.ReplaceAll(s, "\n", "")
	s = strings.ReplaceAll(s, `\`, `\\`)
	return strings.ReplaceAll(s, `"`, `\"`)
}
