package service

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"runtime"

	"webcurl-desktop/internal/httpclient"
	"webcurl-desktop/internal/store"
)

// WorkspaceService 是"工作区"数据（历史、集合、环境变量）的统一入口。
//
// 之所以把这些放在一个服务里而不是拆成三个：界面上的导入/导出、清空、
// 快照读取都是跨实体的操作，拆开后同一个事务要跨多个绑定调用，
// 反而更容易出现"历史读到了、集合还是旧的"这种不一致。
type WorkspaceService struct {
	store *store.Store
}

// NewWorkspaceService 构造工作区服务。
func NewWorkspaceService(st *store.Store) *WorkspaceService {
	return &WorkspaceService{store: st}
}

// Snapshot 返回历史 + 集合 + 环境的完整快照。
func (s *WorkspaceService) Snapshot() store.Data { return s.store.Data() }

// DataPath 返回数据文件路径与所在目录，界面上用于"打开数据目录"与显示位置。
type DataPath struct {
	Dir  string `json:"dir"`
	File string `json:"file"`
}

// Path 返回数据文件位置。
func (s *WorkspaceService) Path() DataPath {
	return DataPath{Dir: s.store.Dir(), File: s.store.Path()}
}

// --- 历史 ---

// AddHistory 追加一条历史并返回新列表。
func (s *WorkspaceService) AddHistory(entry store.HistoryEntry) ([]store.HistoryEntry, error) {
	return s.store.AddHistory(entry)
}

// UpdateHistory 更新一条历史（补写状态码、耗时等）。
func (s *WorkspaceService) UpdateHistory(entry store.HistoryEntry) ([]store.HistoryEntry, error) {
	return s.store.UpdateHistory(entry)
}

// DeleteHistory 删除一条历史。
func (s *WorkspaceService) DeleteHistory(id string) ([]store.HistoryEntry, error) {
	return s.store.DeleteHistory(id)
}

// ClearHistory 清空历史。
func (s *WorkspaceService) ClearHistory() error { return s.store.ClearHistory() }

// --- 集合 ---

// SaveCollection 保存集合（ID 为空时新建）。
func (s *WorkspaceService) SaveCollection(c store.Collection) ([]store.Collection, error) {
	return s.store.SaveCollection(c)
}

// DeleteCollection 删除集合。
func (s *WorkspaceService) DeleteCollection(id string) ([]store.Collection, error) {
	return s.store.DeleteCollection(id)
}

// --- 环境 ---

// SaveEnvironment 保存环境。
func (s *WorkspaceService) SaveEnvironment(e store.Environment) ([]store.Environment, error) {
	return s.store.SaveEnvironment(e)
}

// DeleteEnvironment 删除环境。
func (s *WorkspaceService) DeleteEnvironment(id string) ([]store.Environment, error) {
	return s.store.DeleteEnvironment(id)
}

// SetActiveEnvironment 切换当前环境。
func (s *WorkspaceService) SetActiveEnvironment(id string) error {
	return s.store.SetActiveEnvironment(id)
}

// --- 导入导出 ---

// ImportData 用给定文档替换全部数据（界面的"导入配置"）。
func (s *WorkspaceService) ImportData(in store.Data) error { return s.store.ImportData(in) }

// ExportData 返回可直接写盘为 JSON 的完整数据。
//
// 与 Snapshot 的区别只在于用途：导入导出的文档需要版本号与时间戳，
// 而界面上的快照不需要。
func (s *WorkspaceService) ExportData() store.Data { return s.store.Data() }

// --- 文件系统 ---

// OpenDataDir 用系统文件管理器打开数据目录。
//
// 让用户能直接看到自己的集合与历史存成了什么样子：JSON 可读、可手工编辑、
// 可丢进 git，这是本地优先的工具该有的透明度。
func (s *WorkspaceService) OpenDataDir() error {
	dir := s.store.Dir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("创建数据目录失败: %w", err)
	}

	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("explorer", dir)
	case "darwin":
		cmd = exec.Command("open", dir)
	default:
		cmd = exec.Command("xdg-open", dir)
	}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("打开目录失败: %w", err)
	}
	return nil
}

// SaveRequestToFile 把一段文本（如导出的 curl 命令或配置 JSON）写到指定路径。
//
// 由后端写而不是前端触发浏览器下载：桌面应用没有浏览器下载那套机制，
// 而且路径来自系统对话框，后端直接写最直接。
func (s *WorkspaceService) SaveRequestToFile(path, content string) error {
	if path == "" {
		return errors.New("未指定保存路径")
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return fmt.Errorf("写入文件失败: %w", err)
	}
	return nil
}

// ReadTextFile 读取文本文件内容（导入 curl 命令或配置 JSON）。
func (s *WorkspaceService) ReadTextFile(path string) (string, error) {
	if path == "" {
		return "", errors.New("未指定文件路径")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("读取文件失败: %w", err)
	}
	return string(raw), nil
}

// NewEmptyRequest 返回按当前默认参数构造的空请求。
//
// 放这里而不是前端硬编码默认值：默认超时/重试/SSL 策略来自配置，
// 前端每处新建请求都自己拼一遍，改默认值时就必然漏掉几个入口。
func NewEmptyRequest(defaults RequestDefaults) httpclient.Request {
	req := httpclient.Request{
		Method:           "GET",
		BodyKind:         httpclient.BodyNone,
		TimeoutSec:       defaults.TimeoutSec,
		RetryCount:       defaults.RetryCount,
		RetryDelaySec:    defaults.RetryDelaySec,
		SkipSSLVerify:    !defaults.VerifySSL,
		NoFollowRedirect: !defaults.FollowRedirect,
	}
	return req
}

// RequestDefaults 是从配置里抽出来的新建请求默认值，避免服务层依赖 config 包。
type RequestDefaults struct {
	TimeoutSec     int
	RetryCount     int
	RetryDelaySec  int
	VerifySSL      bool
	FollowRedirect bool
}
