package store

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

// cloneJSON 通过 JSON 往返做深拷贝。
//
// 这些结构里嵌着切片（Request 的参数/头/表单/文件都是切片），浅拷贝会让
// 调用方拿到仍然指向内部状态的引用，一次误改就绕过锁改坏了存储。
// 数据规模是"几百条历史"这个量级，JSON 往返的开销完全可以忽略，
// 换来的是不必为每种类型手写一遍深拷贝（也就不用担心将来加字段时漏拷）。
func cloneJSON[T any](in T) T {
	raw, err := json.Marshal(in)
	if err != nil {
		// 序列化失败说明数据本身不可序列化，此时返回原值比返回零值更安全：
		// 至少调用方看到的是当前状态，而不是一份"空数据"。
		return in
	}
	var out T
	if err := json.Unmarshal(raw, &out); err != nil {
		return in
	}
	return out
}

func cloneData(d Data) Data                        { return cloneJSON(d) }
func cloneHistory(h []HistoryEntry) []HistoryEntry { return cloneJSON(h) }
func cloneCollections(c []Collection) []Collection { return cloneJSON(c) }
func cloneEnvs(e []Environment) []Environment      { return cloneJSON(e) }

// newID 生成一个短随机 ID。
//
// 用随机串而不是自增序号或时间戳：集合可以在不同机器间导入导出，
// 自增序号会撞车；时间戳在同一毫秒内连续创建两个接口时会重复。
func newID() string {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return hex.EncodeToString([]byte(time.Now().Format(time.RFC3339Nano)))
	}
	return hex.EncodeToString(b)
}

// tempDir 是未指定数据目录时的兜底位置。
func tempDir() string {
	return filepath.Join(os.TempDir(), "webcurl-desktop")
}
