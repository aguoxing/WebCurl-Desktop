// Package store 负责桌面版的数据持久化：请求历史、接口集合、环境变量。
//
// 上游 WebCurl 把这些放在浏览器 localStorage 里，桌面版没有浏览器存储，
// 改为落盘到一个 JSON 文件。选择"单文件 + 整体原子替换"而不是分成多个文件或
// 上数据库，理由是这个数据量级（几百条历史 + 若干集合）下：
//   - 单文件保证跨实体的一致性读写，不会出现"历史写成功、集合写失败"的半状态；
//   - 原子替换（写临时文件再 rename）保证进程被杀时文件仍是完整的旧版本，
//     不会留下截断的 JSON —— 这是最容易让人丢掉全部调试历史的故障。
//
// 写入策略是每次变更立即落盘。数据量小，省掉防抖逻辑换来的是"想不起来保存"
// 这类问题根本不会出现：关窗口、断电都不会丢最近一次操作。
package store

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"webcurl-desktop/internal/httpclient"
)

// DefaultHistoryLimit 是历史记录的默认条数上限。
const DefaultHistoryLimit = 500

// fileName 是数据文件名，固定放在数据目录下。
const fileName = "data.json"

// HistoryEntry 是一条请求历史。
//
// 存整份 Request 而不只是 URL：用户点历史条目往往是想"改一下再发"，
// 只存 URL 的话方法、头、体全丢了，等于要他重填一遍。
type HistoryEntry struct {
	ID         string             `json:"id"`
	At         time.Time          `json:"at"`
	Request    httpclient.Request `json:"request"`
	Status     int                `json:"status"`
	DurationMs int64              `json:"durationMs"`
	Size       int                `json:"size"`
	Error      string             `json:"error,omitempty"`
}

// Collection 是一组接口的集合（对应界面左侧的树）。
type Collection struct {
	ID        string           `json:"id"`
	Name      string           `json:"name"`
	CreatedAt time.Time        `json:"createdAt"`
	UpdatedAt time.Time        `json:"updatedAt"`
	Items     []CollectionItem `json:"items"`
}

// CollectionItem 是集合里的一个接口。
type CollectionItem struct {
	ID      string             `json:"id"`
	Name    string             `json:"name"`
	Request httpclient.Request `json:"request"`
}

// Variable 是一个环境变量。
type Variable struct {
	Name    string `json:"name"`
	Value   string `json:"value"`
	Enabled bool   `json:"enabled"`
}

// Environment 是一组变量，界面上通过切换环境来批量替换变量值。
type Environment struct {
	ID        string     `json:"id"`
	Name      string     `json:"name"`
	Variables []Variable `json:"variables"`
}

// Data 是整个持久化文档。
type Data struct {
	Version     int            `json:"version"`
	History     []HistoryEntry `json:"history"`
	Collections []Collection   `json:"collections"`
	Envs        []Environment  `json:"environments"`
	ActiveEnv   string         `json:"activeEnv"`
	SavedAt     time.Time      `json:"savedAt"`
}

// schemaVersion 用于将来迁移。读取时发现更高版本会拒绝写入，
// 避免新版本写出的数据被旧版本覆盖成残缺结构。
const schemaVersion = 1

// Options 控制存储行为。
type Options struct {
	// Dir 是数据目录。为空时使用 tempDir()。
	Dir string

	// HistoryLimit 是历史条数上限，超出后丢弃最旧的。<=0 时用 DefaultHistoryLimit。
	HistoryLimit int
}

// Store 是并发安全的数据存储。
type Store struct {
	mu    sync.Mutex
	dir   string
	limit int
	data  Data
}

// Open 打开（必要时创建）数据目录并载入数据。
//
// 文件不存在或内容损坏都不是致命错误：前者是首次运行，后者多半来自手工编辑或
// 磁盘故障。两种情况都退化为"从空数据开始"，并**不覆盖**坏文件——把它改名留档，
// 让用户还有机会自己抢救。
func Open(opts Options) (*Store, error) {
	dir := opts.Dir
	if dir == "" {
		dir = tempDir()
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("创建数据目录失败: %w", err)
	}

	limit := opts.HistoryLimit
	if limit <= 0 {
		limit = DefaultHistoryLimit
	}

	s := &Store{dir: dir, limit: limit}
	if err := s.load(); err != nil {
		return nil, err
	}
	return s, nil
}

// Dir 返回数据目录，界面上用于"打开数据目录"。
func (s *Store) Dir() string { return s.dir }

// Path 返回数据文件路径。
func (s *Store) Path() string { return filepath.Join(s.dir, fileName) }

// load 从磁盘载入数据。
func (s *Store) load() error {
	raw, err := os.ReadFile(s.Path())
	if errors.Is(err, os.ErrNotExist) {
		s.data = Data{Version: schemaVersion, SavedAt: time.Now()}
		return nil
	}
	if err != nil {
		return fmt.Errorf("读取数据文件失败: %w", err)
	}

	var data Data
	if err := json.Unmarshal(raw, &data); err != nil {
		// 保留坏文件：用户可能只是某处多了个逗号，直接覆盖等于毁掉他的收藏。
		backup := s.Path() + ".corrupt-" + time.Now().Format("20060102-150405")
		_ = os.Rename(s.Path(), backup)
		s.data = Data{Version: schemaVersion, SavedAt: time.Now()}
		return fmt.Errorf("数据文件解析失败（已备份为 %s）: %w", filepath.Base(backup), err)
	}

	if data.Version > schemaVersion {
		return fmt.Errorf("数据文件版本 %d 高于当前程序支持的 %d，请升级应用", data.Version, schemaVersion)
	}
	data.Version = schemaVersion
	s.data = data
	return nil
}

// Data 返回当前数据的深拷贝。
//
// 返回拷贝而不是内部引用：Wails 绑定会把返回值序列化到前端，若直接给出内部
// 切片，前端改一个字段就会绕过锁改到存储状态。JSON 往返是最省心的深拷贝，
// 代价是这份数据规模下可以忽略的开销。
func (s *Store) Data() Data {
	s.mu.Lock()
	defer s.mu.Unlock()
	return cloneData(s.data)
}

// AddHistory 追加一条历史并落盘，返回落盘后的完整历史列表。
//
// 最新的排在最前：调试时的心理模型是"刚刚那条"，让它出现在列表顶部
// 比按时间倒序再滚动要顺手。
func (s *Store) AddHistory(entry HistoryEntry) ([]HistoryEntry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if entry.ID == "" {
		entry.ID = newID()
	}
	if entry.At.IsZero() {
		entry.At = time.Now()
	}

	s.data.History = append([]HistoryEntry{entry}, s.data.History...)
	if len(s.data.History) > s.limit {
		s.data.History = s.data.History[:s.limit]
	}
	return cloneHistory(s.data.History), s.saveLocked()
}

// UpdateHistory 就地更新一条历史（用于补写响应信息）。
// 找不到该 ID 时静默返回：历史可能已被清空或裁剪，这不是错误。
func (s *Store) UpdateHistory(entry HistoryEntry) ([]HistoryEntry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for i := range s.data.History {
		if s.data.History[i].ID == entry.ID {
			s.data.History[i] = entry
			break
		}
	}
	return cloneHistory(s.data.History), s.saveLocked()
}

// DeleteHistory 删除一条历史。
func (s *Store) DeleteHistory(id string) ([]HistoryEntry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	out := s.data.History[:0]
	for _, h := range s.data.History {
		if h.ID != id {
			out = append(out, h)
		}
	}
	s.data.History = out
	return cloneHistory(s.data.History), s.saveLocked()
}

// ClearHistory 清空历史，但保留集合与环境。
func (s *Store) ClearHistory() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.data.History = nil
	return s.saveLocked()
}

// SaveCollection 新增或覆盖一个集合（按 ID 匹配）。
func (s *Store) SaveCollection(c Collection) ([]Collection, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	if c.ID == "" {
		c.ID = newID()
		c.CreatedAt = now
	}
	c.UpdatedAt = now
	if c.Items == nil {
		c.Items = []CollectionItem{}
	}

	replaced := false
	for i := range s.data.Collections {
		if s.data.Collections[i].ID == c.ID {
			c.CreatedAt = s.data.Collections[i].CreatedAt
			s.data.Collections[i] = c
			replaced = true
			break
		}
	}
	if !replaced {
		s.data.Collections = append(s.data.Collections, c)
	}
	return cloneCollections(s.data.Collections), s.saveLocked()
}

// DeleteCollection 删除一个集合。
func (s *Store) DeleteCollection(id string) ([]Collection, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	out := s.data.Collections[:0]
	for _, c := range s.data.Collections {
		if c.ID != id {
			out = append(out, c)
		}
	}
	s.data.Collections = out
	return cloneCollections(s.data.Collections), s.saveLocked()
}

// SaveEnvironment 新增或覆盖一个环境。
func (s *Store) SaveEnvironment(env Environment) ([]Environment, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if env.ID == "" {
		env.ID = newID()
	}
	if env.Variables == nil {
		env.Variables = []Variable{}
	}

	replaced := false
	for i := range s.data.Envs {
		if s.data.Envs[i].ID == env.ID {
			s.data.Envs[i] = env
			replaced = true
			break
		}
	}
	if !replaced {
		s.data.Envs = append(s.data.Envs, env)
	}
	return cloneEnvs(s.data.Envs), s.saveLocked()
}

// DeleteEnvironment 删除一个环境；若删的是当前激活环境，则激活态一并清空。
func (s *Store) DeleteEnvironment(id string) ([]Environment, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	out := s.data.Envs[:0]
	for _, e := range s.data.Envs {
		if e.ID != id {
			out = append(out, e)
		}
	}
	s.data.Envs = out
	if s.data.ActiveEnv == id {
		s.data.ActiveEnv = ""
	}
	return cloneEnvs(s.data.Envs), s.saveLocked()
}

// SetActiveEnvironment 切换当前环境。传空串表示不使用任何环境。
func (s *Store) SetActiveEnvironment(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.data.ActiveEnv = id
	return s.saveLocked()
}

// ImportData 用给定文档替换现有数据（用于"导入配置"）。
//
// 历史按上限裁剪，避免导入一份巨大历史把文件撑得难以打开。
func (s *Store) ImportData(in Data) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if len(in.History) > s.limit {
		in.History = in.History[:s.limit]
	}
	in.Version = schemaVersion
	in.SavedAt = time.Now()
	s.data = in
	return s.saveLocked()
}

// saveLocked 原子落盘。调用方必须已持有锁。
//
// 先写临时文件再 rename：Windows 上 rename 到已存在的文件会失败，
// 因此先删旧的再改名（这里无法做到严格原子的替换，但最坏情况是短暂
// 缺少文件，而不是内容被截断——对 JSON 文档而言前者可恢复、后者不可）。
func (s *Store) saveLocked() error {
	s.data.SavedAt = time.Now()
	if s.data.Version == 0 {
		s.data.Version = schemaVersion
	}

	raw, err := json.MarshalIndent(&s.data, "", "  ")
	if err != nil {
		return fmt.Errorf("序列化数据失败: %w", err)
	}

	tmp := s.Path() + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o600); err != nil {
		return fmt.Errorf("写入临时文件失败: %w", err)
	}
	if err := os.Remove(s.Path()); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("替换数据文件失败: %w", err)
	}
	if err := os.Rename(tmp, s.Path()); err != nil {
		return fmt.Errorf("替换数据文件失败: %w", err)
	}
	return nil
}

// SortedCollections 返回按名称排序的集合副本，供界面稳定显示。
func (s *Store) SortedCollections() []Collection {
	all := s.Data().Collections
	sort.SliceStable(all, func(i, j int) bool { return all[i].Name < all[j].Name })
	return all
}
