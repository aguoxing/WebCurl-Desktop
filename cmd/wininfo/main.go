// Command wininfo 生成 Windows 版本资源，供 `wails3 generate syso -info` 使用。
//
// 它解决的是「应用内显示的版本号与文件属性里显示的版本号对不上」：
// 二进制里的版本由 -ldflags 注入，而 exe 的版本资源来自一个静态 JSON，
// 两条来源各写各的，于是出现了应用内 1.0.2、文件属性 1.0.1 这种矛盾。
// 现在两者都从 internal/buildinfo 取同一个版本，只是渲染成不同格式。
//
// 用法：
//
//	go run ./cmd/wininfo -template build/windows/info.json -out build/windows/info.generated.json
//
// # 为什么不直接改写 build/windows/info.json
//
//   - 那是 wails3 的可再生资产：`wails3 task common:update:build-assets` 会依据
//     build/config.yml 重写它，手改的内容随时会被冲掉；
//   - 改写一个已被 git 跟踪的文件会把工作区弄脏，而 buildinfo 正是用
//     `git status` 判断脏标记的——那样每次构建都会把下一次构建标成 dirty，
//     形成自反馈。
//
// 因此模板（公司、产品名、描述、版权等静态信息）仍取自 info.json，
// 只有版本字段由这里覆盖，产物写到单独的文件里（已在 .gitignore 中忽略）。
//
// # 两个由实测确定的硬约束
//
//   - info.<语言>.FileVersion **必须存在**。缺了它，wails3 generate syso 会把整个
//     StringFileInfo 字符串块丢掉——不只是版本号，连同公司名、产品名、描述、版权
//     一起消失，而它并不会报错。
//   - fixed.product_version 决定「产品版本」的数值部分（VS_FIXEDFILEINFO 里那个
//     四段数字，对应 PowerShell 的 ProductVersionRaw）。缺了它那里会显示 0.0.0.0。
//
// # 字段取值
//
//   - FileVersion（含 fixed 里的两个数字）：四段数字，如 1.0.2.0。Windows 与各类
//     构建、安装、更新工具都以它做版本比较，必须是纯数字。
//   - ProductVersion：完整版本号，如 1.0.2-2-g93ed8b0，与应用内「关于」页显示的一致。
//     它是给人看的自由文本，因此可以带 git 后缀。
package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"

	"webcurl-desktop/internal/buildinfo"
	"webcurl-desktop/internal/version"
)

const (
	fieldFileVersion    = "FileVersion"
	fieldProductVersion = "ProductVersion"

	// defaultLanguage 是没有 info 块时新建的语言键。Windows 资源用四位十六进制
	// 表示语言 ID，"0000" 是语言中性——wails3 生成的模板用的就是它。
	defaultLanguage = "0000"
)

func main() {
	templatePath := flag.String("template", "build/windows/info.json", "info.json 模板路径")
	outPath := flag.String("out", "", "输出路径（必填）")
	flag.Parse()

	if err := run(*templatePath, *outPath); err != nil {
		fmt.Fprintln(os.Stderr, "wininfo:", err)
		os.Exit(1)
	}
}

// run 读模板、覆盖版本字段、写出结果。单独拆出来是为了能被用例直接调用。
func run(templatePath, outPath string) error {
	if outPath == "" {
		return errors.New("-out 不能为空")
	}

	raw, err := os.ReadFile(templatePath)
	if err != nil {
		return fmt.Errorf("读取模板：%w", err)
	}

	// 用 map 而不是结构体解码：既保留模板里我们没建模的字段，
	// 也避免 wails3 日后新增字段时被这里无声丢弃。
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		return fmt.Errorf("解析 %s：%w", templatePath, err)
	}

	info := buildinfo.Resolve()
	numeric := buildinfo.WindowsVersion(info.Version)
	product := productVersion(info.Version)

	if err := patch(doc, numeric, product); err != nil {
		return fmt.Errorf("改写 %s：%w", templatePath, err)
	}

	out, err := json.MarshalIndent(doc, "", "\t")
	if err != nil {
		return fmt.Errorf("序列化：%w", err)
	}
	if err := os.WriteFile(outPath, append(out, '\n'), 0o644); err != nil {
		return fmt.Errorf("写出 %s：%w", outPath, err)
	}
	return nil
}

// productVersion 返回应当写入 ProductVersion 的字符串。
//
// 推导不出版本时（源码压缩包里没有 git）二进制会显示 dev，版本资源也如实写成
// dev，而不是伪造一个 0.0.0 —— 两边保持一致才是这个命令存在的意义。
func productVersion(v string) string {
	if v == "" {
		return version.DevVersion
	}
	return v
}

// patch 就地覆盖 doc 里的版本字段。
//
// 对结构不符的输入一律报错而不是跳过：这正是本次修复要防的失败模式——
// 版本字段没写进去时 wails3 不报错，只是让文件属性里的版本悄悄变成空或旧值。
func patch(doc map[string]any, numeric, product string) error {
	fixed, err := objectField(doc, "fixed")
	if err != nil {
		return err
	}
	fixed["file_version"] = numeric
	fixed["product_version"] = numeric

	blocks, err := objectField(doc, "info")
	if err != nil {
		return err
	}
	if len(blocks) == 0 {
		blocks[defaultLanguage] = map[string]any{}
	}

	for language, block := range blocks {
		fields, ok := block.(map[string]any)
		if !ok {
			return fmt.Errorf("info.%s 不是对象", language)
		}
		fields[fieldFileVersion] = numeric
		fields[fieldProductVersion] = product
	}
	return nil
}

// objectField 取出 doc 里的对象字段；字段不存在时就地建一个空对象。
func objectField(doc map[string]any, name string) (map[string]any, error) {
	value, ok := doc[name]
	if !ok {
		object := map[string]any{}
		doc[name] = object
		return object, nil
	}

	object, ok := value.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("%s 不是对象", name)
	}
	return object, nil
}
