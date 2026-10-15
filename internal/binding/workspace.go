package binding

import (
	"webcurl-desktop/internal/service"
	"webcurl-desktop/internal/store"
)

// WorkspaceServiceBinding 是 service.WorkspaceService 面向前端的包装。
//
// 历史、集合、环境变量这三份数据共用同一份落盘文档，因此由同一个服务暴露：
// 界面上的"导入配置""清空数据"这类操作天然是跨实体的。
type WorkspaceServiceBinding struct {
	inner *service.WorkspaceService

	// pickSavePath / pickOpenPath 由 app 层注入，走系统对话框。可为 nil。
	pickSavePath func(suggested string) (string, error)
	pickOpenPath func() (string, error)
}

// NewWorkspaceServiceBinding 构造工作区服务绑定。
func NewWorkspaceServiceBinding(
	inner *service.WorkspaceService,
	pickSavePath func(suggested string) (string, error),
	pickOpenPath func() (string, error),
) *WorkspaceServiceBinding {
	return &WorkspaceServiceBinding{inner: inner, pickSavePath: pickSavePath, pickOpenPath: pickOpenPath}
}

// Snapshot 返回历史 + 集合 + 环境的完整快照。
func (b *WorkspaceServiceBinding) Snapshot() store.Data { return b.inner.Snapshot() }

// Path 返回数据文件所在目录与文件路径。
func (b *WorkspaceServiceBinding) Path() service.DataPath { return b.inner.Path() }

// AddHistory 追加历史，返回新列表。
func (b *WorkspaceServiceBinding) AddHistory(e store.HistoryEntry) ([]store.HistoryEntry, error) {
	return b.inner.AddHistory(e)
}

// UpdateHistory 更新一条历史。
func (b *WorkspaceServiceBinding) UpdateHistory(e store.HistoryEntry) ([]store.HistoryEntry, error) {
	return b.inner.UpdateHistory(e)
}

// DeleteHistory 删除一条历史。
func (b *WorkspaceServiceBinding) DeleteHistory(id string) ([]store.HistoryEntry, error) {
	return b.inner.DeleteHistory(id)
}

// ClearHistory 清空历史。
func (b *WorkspaceServiceBinding) ClearHistory() error { return b.inner.ClearHistory() }

// SaveCollection 保存集合。
func (b *WorkspaceServiceBinding) SaveCollection(c store.Collection) ([]store.Collection, error) {
	return b.inner.SaveCollection(c)
}

// DeleteCollection 删除集合。
func (b *WorkspaceServiceBinding) DeleteCollection(id string) ([]store.Collection, error) {
	return b.inner.DeleteCollection(id)
}

// SaveEnvironment 保存环境。
func (b *WorkspaceServiceBinding) SaveEnvironment(e store.Environment) ([]store.Environment, error) {
	return b.inner.SaveEnvironment(e)
}

// DeleteEnvironment 删除环境。
func (b *WorkspaceServiceBinding) DeleteEnvironment(id string) ([]store.Environment, error) {
	return b.inner.DeleteEnvironment(id)
}

// SetActiveEnvironment 切换当前环境。
func (b *WorkspaceServiceBinding) SetActiveEnvironment(id string) error {
	return b.inner.SetActiveEnvironment(id)
}

// ImportData 用导入的文档替换全部数据。
func (b *WorkspaceServiceBinding) ImportData(in store.Data) error { return b.inner.ImportData(in) }

// ExportData 返回可写盘的完整数据。
func (b *WorkspaceServiceBinding) ExportData() store.Data { return b.inner.ExportData() }

// OpenDataDir 用系统文件管理器打开数据目录。
func (b *WorkspaceServiceBinding) OpenDataDir() error { return b.inner.OpenDataDir() }

// ExportToFile 让用户选一个路径，把内容写进去（导出集合 / curl 命令）。
// 返回实际写入的路径；用户取消时返回空串且不报错。
func (b *WorkspaceServiceBinding) ExportToFile(suggestedName, content string) (string, error) {
	if b.pickSavePath == nil {
		return "", errNoDialog
	}
	path, err := b.pickSavePath(suggestedName)
	if err != nil {
		return "", err
	}
	if path == "" {
		return "", nil
	}
	if err := b.inner.SaveRequestToFile(path, content); err != nil {
		return "", err
	}
	return path, nil
}

// ImportFromFile 让用户选一个文件，返回其文本内容（导入集合 / curl 命令）。
// 用户取消时返回空串。
func (b *WorkspaceServiceBinding) ImportFromFile() (string, error) {
	if b.pickOpenPath == nil {
		return "", errNoDialog
	}
	path, err := b.pickOpenPath()
	if err != nil {
		return "", err
	}
	if path == "" {
		return "", nil
	}
	return b.inner.ReadTextFile(path)
}
