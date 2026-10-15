package binding

import (
	"webcurl-desktop/internal/httpclient"
	"webcurl-desktop/internal/service"
)

// RequestServiceBinding 是 service.RequestService 面向前端的包装。
//
// 这里的方法就是"发送"这条主路径的全部入口：发起、取消、把完整响应体写到磁盘。
type RequestServiceBinding struct {
	inner *service.RequestService

	// pickFiles / pickSavePath 由 app 层注入，走系统对话框。
	// 允许为 nil：无界面场景（server 构建、单测）下这些方法只报错，不影响发送。
	pickFiles    func(multiple bool) ([]string, error)
	pickSavePath func(suggested string) (string, error)
}

// NewRequestServiceBinding 构造请求服务绑定。
func NewRequestServiceBinding(
	inner *service.RequestService,
	pickFiles func(multiple bool) ([]string, error),
	pickSavePath func(suggested string) (string, error),
) *RequestServiceBinding {
	return &RequestServiceBinding{inner: inner, pickFiles: pickFiles, pickSavePath: pickSavePath}
}

// Send 执行一次请求。requestID 用于后续 Cancel / SaveBody 关联同一次发送。
func (b *RequestServiceBinding) Send(requestID string, req httpclient.Request) service.SendResult {
	return b.inner.Send(requestID, req)
}

// Cancel 取消进行中的请求。
func (b *RequestServiceBinding) Cancel(requestID string) {
	b.inner.Cancel(requestID)
}

// SaveBody 把完整响应体另存为文件，返回实际写入的路径（用户取消时为空串）。
func (b *RequestServiceBinding) SaveBody(requestID, suggestedName string) (string, error) {
	if b.pickSavePath == nil {
		return "", errNoDialog
	}
	path, err := b.pickSavePath(suggestedName)
	if err != nil {
		return "", err
	}
	if path == "" {
		return "", nil // 用户取消了对话框，不算失败
	}
	if err := b.inner.SaveBody(requestID, path); err != nil {
		return "", err
	}
	return path, nil
}

// PickFiles 弹出文件选择对话框，返回选中的路径列表。
//
// 由后端拿路径而不是让前端读文件：桌面版的请求体可以是一个几百 MB 的文件，
// 让 webview 读进来再回传等于把文件内容在 IPC 上搬两遍。
func (b *RequestServiceBinding) PickFiles(multiple bool) ([]string, error) {
	if b.pickFiles == nil {
		return nil, errNoDialog
	}
	return b.pickFiles(multiple)
}

// BodySize 返回缓存中该响应体的字节数，-1 表示已过期。
func (b *RequestServiceBinding) BodySize(requestID string) int {
	return b.inner.BodySize(requestID)
}
