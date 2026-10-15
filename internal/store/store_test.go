package store

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"webcurl-desktop/internal/httpclient"
)

// TestHistoryLimitDropsOldest 验证历史超上限时丢最旧的，而不是最新的。
func TestHistoryLimitDropsOldest(t *testing.T) {
	s, err := Open(Options{Dir: t.TempDir(), HistoryLimit: 3})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	for _, name := range []string{"a", "b", "c", "d"} {
		if _, err := s.AddHistory(HistoryEntry{
			Request: httpclient.Request{Method: "GET", URL: "http://x/" + name},
		}); err != nil {
			t.Fatalf("AddHistory: %v", err)
		}
	}

	got := s.Data().History
	if len(got) != 3 {
		t.Fatalf("历史条数 = %d, 期望 3", len(got))
	}
	// 最新的排最前，最旧的 a 应已被丢弃。
	if !strings.HasSuffix(got[0].Request.URL, "/d") {
		t.Errorf("首条 = %s, 期望最新的 /d", got[0].Request.URL)
	}
	for _, h := range got {
		if strings.HasSuffix(h.Request.URL, "/a") {
			t.Error("最旧的 /a 应被裁剪掉")
		}
	}
}

// TestPersistAcrossReopen 验证写入后重新打开仍能读回——这是"数据真的落盘了"。
func TestPersistAcrossReopen(t *testing.T) {
	dir := t.TempDir()

	s, err := Open(Options{Dir: dir})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if _, err := s.SaveCollection(Collection{
		Name: "用户中心",
		Items: []CollectionItem{{
			Name:    "登录",
			Request: httpclient.Request{Method: "POST", URL: "http://x/login"},
		}},
	}); err != nil {
		t.Fatalf("SaveCollection: %v", err)
	}
	if _, err := s.SaveEnvironment(Environment{
		Name:      "测试环境",
		Variables: []Variable{{Name: "host", Value: "http://dev", Enabled: true}},
	}); err != nil {
		t.Fatalf("SaveEnvironment: %v", err)
	}

	reopened, err := Open(Options{Dir: dir})
	if err != nil {
		t.Fatalf("重新 Open: %v", err)
	}
	data := reopened.Data()
	if len(data.Collections) != 1 || data.Collections[0].Name != "用户中心" {
		t.Errorf("集合未持久化: %+v", data.Collections)
	}
	if len(data.Envs) != 1 || data.Envs[0].Variables[0].Value != "http://dev" {
		t.Errorf("环境未持久化: %+v", data.Envs)
	}
}

// TestSaveCollectionUpdatesInPlace 验证同 ID 保存是覆盖而不是追加。
func TestSaveCollectionUpdatesInPlace(t *testing.T) {
	s, _ := Open(Options{Dir: t.TempDir()})

	list, err := s.SaveCollection(Collection{Name: "原名"})
	if err != nil {
		t.Fatalf("SaveCollection: %v", err)
	}
	id := list[0].ID

	list, err = s.SaveCollection(Collection{ID: id, Name: "新名"})
	if err != nil {
		t.Fatalf("SaveCollection: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("集合数量 = %d, 期望 1（同 ID 应覆盖）", len(list))
	}
	if list[0].Name != "新名" {
		t.Errorf("名称 = %q, 期望 新名", list[0].Name)
	}
	if list[0].CreatedAt.IsZero() {
		t.Error("覆盖保存不应丢掉创建时间")
	}
}

// TestDeleteEnvironmentClearsActive 验证删除激活环境时激活态一并清空，
// 否则界面会指向一个不存在的环境，变量全部解析失败。
func TestDeleteEnvironmentClearsActive(t *testing.T) {
	s, _ := Open(Options{Dir: t.TempDir()})

	envs, _ := s.SaveEnvironment(Environment{Name: "dev"})
	id := envs[0].ID
	if err := s.SetActiveEnvironment(id); err != nil {
		t.Fatalf("SetActiveEnvironment: %v", err)
	}

	if _, err := s.DeleteEnvironment(id); err != nil {
		t.Fatalf("DeleteEnvironment: %v", err)
	}
	if got := s.Data().ActiveEnv; got != "" {
		t.Errorf("ActiveEnv = %q, 期望空", got)
	}
}

// TestClearHistoryKeepsCollections 验证清空历史不会误伤集合与环境。
func TestClearHistoryKeepsCollections(t *testing.T) {
	s, _ := Open(Options{Dir: t.TempDir()})

	if _, err := s.AddHistory(HistoryEntry{Request: httpclient.Request{URL: "http://x"}}); err != nil {
		t.Fatalf("AddHistory: %v", err)
	}
	if _, err := s.SaveCollection(Collection{Name: "keep"}); err != nil {
		t.Fatalf("SaveCollection: %v", err)
	}
	if err := s.ClearHistory(); err != nil {
		t.Fatalf("ClearHistory: %v", err)
	}

	data := s.Data()
	if len(data.History) != 0 {
		t.Errorf("历史应被清空, 实际 %d 条", len(data.History))
	}
	if len(data.Collections) != 1 {
		t.Errorf("集合应保留, 实际 %d 个", len(data.Collections))
	}
}

// TestCorruptFileIsBackedUpNotLost 验证坏文件被改名留档而不是直接覆盖。
func TestCorruptFileIsBackedUpNotLost(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, fileName)
	if err := os.WriteFile(path, []byte("{ 这不是合法 JSON"), 0o600); err != nil {
		t.Fatalf("准备坏文件: %v", err)
	}

	s, err := Open(Options{Dir: dir})
	if err == nil {
		t.Fatal("坏文件应报错，让用户知道数据出了问题")
	}
	if s != nil {
		t.Error("坏文件时不应返回可用的 Store")
	}

	// 原始内容必须还在（改名保留），不能被静默丢弃。
	entries, readErr := os.ReadDir(dir)
	if readErr != nil {
		t.Fatalf("读目录: %v", readErr)
	}
	found := false
	for _, e := range entries {
		if strings.Contains(e.Name(), ".corrupt-") {
			found = true
			raw, _ := os.ReadFile(filepath.Join(dir, e.Name()))
			if !strings.Contains(string(raw), "这不是合法 JSON") {
				t.Error("备份文件内容与原始不符")
			}
		}
	}
	if !found {
		t.Errorf("未找到备份文件, 目录内容: %v", entries)
	}
}

// TestRejectsNewerSchema 验证高版本数据文件被拒绝，避免被旧版本写坏。
func TestRejectsNewerSchema(t *testing.T) {
	dir := t.TempDir()
	raw := `{"version": 99, "history": []}`
	if err := os.WriteFile(filepath.Join(dir, fileName), []byte(raw), 0o600); err != nil {
		t.Fatalf("准备文件: %v", err)
	}

	if _, err := Open(Options{Dir: dir}); err == nil {
		t.Fatal("更高版本的数据文件应被拒绝")
	}
}

// TestDataReturnsCopy 验证外部改动返回值不会污染存储。
func TestDataReturnsCopy(t *testing.T) {
	s, _ := Open(Options{Dir: t.TempDir()})
	if _, err := s.SaveCollection(Collection{
		Name:  "c1",
		Items: []CollectionItem{{Name: "i1", Request: httpclient.Request{URL: "http://a"}}},
	}); err != nil {
		t.Fatalf("SaveCollection: %v", err)
	}

	snapshot := s.Data()
	snapshot.Collections[0].Items[0].Request.URL = "http://被改坏了"

	if got := s.Data().Collections[0].Items[0].Request.URL; got != "http://a" {
		t.Errorf("内部状态被外部改动污染: %q", got)
	}
}

// TestDeleteAndUpdateHistory 覆盖删除与更新两条历史操作。
func TestDeleteAndUpdateHistory(t *testing.T) {
	s, _ := Open(Options{Dir: t.TempDir()})

	list, _ := s.AddHistory(HistoryEntry{Request: httpclient.Request{URL: "http://a"}})
	list, _ = s.AddHistory(HistoryEntry{Request: httpclient.Request{URL: "http://b"}})
	if len(list) != 2 {
		t.Fatalf("历史条数 = %d, 期望 2", len(list))
	}

	target := list[1]
	target.Status = 404
	target.DurationMs = 12
	list, _ = s.UpdateHistory(target)
	if list[1].Status != 404 || list[1].DurationMs != 12 {
		t.Errorf("更新未生效: %+v", list[1])
	}

	list, _ = s.DeleteHistory(target.ID)
	// 历史按"最新在前"排列，target 是后加入的 a，删掉后只剩 b。
	if len(list) != 1 || list[0].Request.URL != "http://b" {
		t.Errorf("删除后剩余 = %+v", list)
	}

	// 更新已不存在的 ID 不应报错（历史可能已被裁剪）。
	if _, err := s.UpdateHistory(HistoryEntry{ID: "不存在", Status: 500}); err != nil {
		t.Errorf("更新不存在的历史不应报错: %v", err)
	}
}

// TestImportDataTrimsHistory 验证导入超大历史时按上限裁剪。
func TestImportDataTrimsHistory(t *testing.T) {
	s, _ := Open(Options{Dir: t.TempDir(), HistoryLimit: 2})

	in := Data{}
	for i := 0; i < 10; i++ {
		in.History = append(in.History, HistoryEntry{
			ID:      newID(),
			At:      time.Now(),
			Request: httpclient.Request{URL: "http://x"},
		})
	}
	if err := s.ImportData(in); err != nil {
		t.Fatalf("ImportData: %v", err)
	}
	if got := len(s.Data().History); got != 2 {
		t.Errorf("导入后历史条数 = %d, 期望 2", got)
	}
}
