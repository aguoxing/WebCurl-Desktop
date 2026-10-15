package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// templateJSON 是 wails3 生成的 info.json 的等价样本：注意它**没有** FileVersion，
// 这正是修复前那份模板的样子。
const templateJSON = `{
	"fixed": {
		"file_version": "1.0.1.0"
	},
	"info": {
		"0000": {
			"ProductVersion": "1.0.1.0",
			"CompanyName": "My Company",
			"FileDescription": "A webcurl-desktop application",
			"LegalCopyright": "© 2026, My Company",
			"ProductName": "My Product",
			"Comments": "This is a comment"
		}
	}
}`

func decode(t *testing.T, raw []byte) map[string]any {
	t.Helper()
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("解析 JSON 失败：%v", err)
	}
	return doc
}

// infoFields 取回 info 下第一个语言块，顺带校验它确实是对象。
func infoFields(t *testing.T, doc map[string]any) map[string]any {
	t.Helper()
	blocks, ok := doc["info"].(map[string]any)
	if !ok {
		t.Fatalf("info 不是对象：%#v", doc["info"])
	}
	fields, ok := blocks["0000"].(map[string]any)
	if !ok {
		t.Fatalf("info.0000 不是对象：%#v", blocks["0000"])
	}
	return fields
}

func TestPatchFillsBothVersionForms(t *testing.T) {
	doc := decode(t, []byte(templateJSON))
	if err := patch(doc, "1.0.2.0", "1.0.2-2-g93ed8b0"); err != nil {
		t.Fatalf("patch 失败：%v", err)
	}

	fixed, _ := doc["fixed"].(map[string]any)
	// 两个 fixed 字段都必须是四段数字：缺 product_version 会让
	// ProductVersionRaw 显示 0.0.0.0。
	for _, key := range []string{"file_version", "product_version"} {
		if got := fixed[key]; got != "1.0.2.0" {
			t.Fatalf("fixed.%s = %v, want 1.0.2.0", key, got)
		}
	}

	fields := infoFields(t, doc)
	// FileVersion 必须存在：缺了它 wails3 会丢弃整个 StringFileInfo 字符串块。
	if got := fields[fieldFileVersion]; got != "1.0.2.0" {
		t.Fatalf("info.0000.FileVersion = %v, want 1.0.2.0", got)
	}
	if got := fields[fieldProductVersion]; got != "1.0.2-2-g93ed8b0" {
		t.Fatalf("info.0000.ProductVersion = %v, want 1.0.2-2-g93ed8b0", got)
	}
}

func TestPatchKeepsUnrelatedFields(t *testing.T) {
	doc := decode(t, []byte(templateJSON))
	if err := patch(doc, "1.0.2.0", "1.0.2"); err != nil {
		t.Fatalf("patch 失败：%v", err)
	}

	// 公司、产品名、描述、版权来自 build/config.yml，不能被这里覆盖掉。
	want := map[string]string{
		"CompanyName":     "My Company",
		"ProductName":     "My Product",
		"FileDescription": "A webcurl-desktop application",
		"LegalCopyright":  "© 2026, My Company",
		"Comments":        "This is a comment",
	}
	fields := infoFields(t, doc)
	for key, expected := range want {
		if got := fields[key]; got != expected {
			t.Fatalf("info.0000.%s = %v, want %v", key, got, expected)
		}
	}
}

func TestPatchCreatesMissingSections(t *testing.T) {
	t.Run("模板为空对象", func(t *testing.T) {
		doc := map[string]any{}
		if err := patch(doc, "1.0.2.0", "1.0.2"); err != nil {
			t.Fatalf("patch 失败：%v", err)
		}
		if got := infoFields(t, doc)[fieldProductVersion]; got != "1.0.2" {
			t.Fatalf("ProductVersion = %v, want 1.0.2", got)
		}
	})

	t.Run("info 为空对象时补出默认语言块", func(t *testing.T) {
		doc := map[string]any{"fixed": map[string]any{}, "info": map[string]any{}}
		if err := patch(doc, "1.0.2.0", "1.0.2"); err != nil {
			t.Fatalf("patch 失败：%v", err)
		}
		if got := infoFields(t, doc)[fieldFileVersion]; got != "1.0.2.0" {
			t.Fatalf("FileVersion = %v, want 1.0.2.0", got)
		}
	})

	t.Run("多个语言块都被改写", func(t *testing.T) {
		doc := map[string]any{
			"info": map[string]any{
				"0000": map[string]any{},
				"0409": map[string]any{"ProductVersion": "旧值"},
			},
		}
		if err := patch(doc, "2.0.0.0", "2.0.0"); err != nil {
			t.Fatalf("patch 失败：%v", err)
		}
		blocks := doc["info"].(map[string]any)
		for language, block := range blocks {
			fields := block.(map[string]any)
			if fields[fieldProductVersion] != "2.0.0" {
				t.Fatalf("info.%s.ProductVersion = %v, want 2.0.0", language, fields[fieldProductVersion])
			}
		}
	})
}

// TestPatchRejectsMalformedInput 覆盖「结构变了就当哑巴」的失败模式：
// 与其悄悄产出一个版本字段为空的 exe，不如让构建直接失败。
func TestPatchRejectsMalformedInput(t *testing.T) {
	cases := []struct {
		name string
		doc  map[string]any
	}{
		{"fixed 不是对象", map[string]any{"fixed": "1.0.0.0"}},
		{"info 不是对象", map[string]any{"info": []any{}}},
		{"语言块不是对象", map[string]any{"info": map[string]any{"0000": "1.0.0.0"}}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := patch(tc.doc, "1.0.2.0", "1.0.2"); err == nil {
				t.Fatal("结构不符时应当报错")
			}
		})
	}
}

func TestProductVersionFallsBackToDev(t *testing.T) {
	// 与 internal/version 的约定一致：拿不到版本时两边都说 dev。
	if got := productVersion(""); got != "dev" {
		t.Fatalf("productVersion(\"\") = %q, want %q", got, "dev")
	}
	if got := productVersion("1.2.3"); got != "1.2.3" {
		t.Fatalf("productVersion(\"1.2.3\") = %q, want %q", got, "1.2.3")
	}
}

func TestRunWritesGeneratedFile(t *testing.T) {
	dir := t.TempDir()
	templatePath := filepath.Join(dir, "info.json")
	outPath := filepath.Join(dir, "info.generated.json")
	if err := os.WriteFile(templatePath, []byte(templateJSON), 0o644); err != nil {
		t.Fatalf("写模板失败：%v", err)
	}

	// 固定版本，让断言与 git 状态无关。
	t.Setenv("GOWS_VERSION", "1.0.2-2-g93ed8b0")

	if err := run(templatePath, outPath); err != nil {
		t.Fatalf("run 失败：%v", err)
	}

	raw, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("读产物失败：%v", err)
	}
	doc := decode(t, raw)

	fixed, _ := doc["fixed"].(map[string]any)
	if fixed["file_version"] != "1.0.2.0" || fixed["product_version"] != "1.0.2.0" {
		t.Fatalf("fixed = %v, want file/product 均为 1.0.2.0", fixed)
	}

	fields := infoFields(t, doc)
	if fields[fieldFileVersion] != "1.0.2.0" {
		t.Fatalf("FileVersion = %v, want 1.0.2.0", fields[fieldFileVersion])
	}
	// 产品版本与应用内「关于」页显示的是同一个串。
	if fields[fieldProductVersion] != "1.0.2-2-g93ed8b0" {
		t.Fatalf("ProductVersion = %v, want 1.0.2-2-g93ed8b0", fields[fieldProductVersion])
	}

	// 模板本身不应被改动，否则工作区会被构建弄脏。
	original, err := os.ReadFile(templatePath)
	if err != nil {
		t.Fatalf("读模板失败：%v", err)
	}
	if string(original) != templateJSON {
		t.Fatal("run 不应该改写模板文件")
	}
}

func TestRunRejectsBadArguments(t *testing.T) {
	dir := t.TempDir()

	t.Run("缺少 -out", func(t *testing.T) {
		if err := run(filepath.Join(dir, "info.json"), ""); err == nil {
			t.Fatal("-out 为空时应当报错")
		}
	})

	t.Run("模板不存在", func(t *testing.T) {
		if err := run(filepath.Join(dir, "nope.json"), filepath.Join(dir, "out.json")); err == nil {
			t.Fatal("模板缺失时应当报错")
		}
	})

	t.Run("模板不是合法 JSON", func(t *testing.T) {
		bad := filepath.Join(dir, "bad.json")
		if err := os.WriteFile(bad, []byte("{oops"), 0o644); err != nil {
			t.Fatalf("写文件失败：%v", err)
		}
		if err := run(bad, filepath.Join(dir, "out.json")); err == nil {
			t.Fatal("JSON 非法时应当报错")
		}
	})
}
