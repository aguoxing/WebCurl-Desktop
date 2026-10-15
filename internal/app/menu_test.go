package app

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/wailsapp/wails/v3/pkg/application"
)

// TestMenuTextsCoverDefaultLocale 保证回退目标确实存在。
// 少了这一项，menuTextFor 的兜底会返回零值，菜单整片变成空白。
func TestMenuTextsCoverDefaultLocale(t *testing.T) {
	if _, ok := menuTexts[defaultMenuLocale]; !ok {
		t.Fatalf("default locale %q is missing from menuTexts", defaultMenuLocale)
	}
}

// TestMenuTextsHaveNoEmptyFields 逐字段检查每种语言都写全了。
//
// 漏翻一项的后果是菜单里出现一块空白，而这种问题只会在对应平台上
// 被肉眼发现，因此这里用反射一次性覆盖结构体的全部字段。
func TestMenuTextsHaveNoEmptyFields(t *testing.T) {
	for locale, text := range menuTexts {
		value := reflect.ValueOf(text)
		typ := value.Type()

		for i := 0; i < value.NumField(); i++ {
			if value.Field(i).String() == "" {
				t.Errorf("locale %q: field %s is empty", locale, typ.Field(i).Name)
			}
		}
	}
}

// TestMenuTextForKnownLocales 确认已知语言取到的是它自己那一份，
// 而不是被 normalize 之类的逻辑改写成了默认语言。
func TestMenuTextForKnownLocales(t *testing.T) {
	for locale := range menuTexts {
		if got := menuTextFor(locale); got != menuTexts[locale] {
			t.Errorf("menuTextFor(%q) did not return its own text", locale)
		}
	}
}

// TestMenuTextForFallsBackToDefault 覆盖未知与空语言的回退。
// "zh_CN" 与 "ZH-CN" 也放在这里：语言标识只做精确匹配，
// 大小写或分隔符不同即视为不支持，宁可回退也不要猜。
func TestMenuTextForFallsBackToDefault(t *testing.T) {
	want := menuTexts[defaultMenuLocale]

	for _, locale := range []string{"", "fr", "zh_CN", "ZH-CN"} {
		if got := menuTextFor(locale); got != want {
			t.Errorf("menuTextFor(%q) = %+v, want the default locale", locale, got)
		}
	}
}

// TestLocaleFromEvent 覆盖事件负载的取值。
// 只接受字符串：其它类型意味着前后端契约被破坏，
// 此时应当视为"没带语言"，而不是猜一个默认值。
func TestLocaleFromEvent(t *testing.T) {
	tests := []struct {
		name string
		data any
		want string
	}{
		{"字符串", "zh-CN", "zh-CN"},
		{"空字符串", "", ""},
		{"数字", 42, ""},
		{"对象", map[string]any{"lang": "zh-CN"}, ""},
		{"nil", nil, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			event := &application.CustomEvent{Name: eventLanguage, Data: tt.data}
			if got := localeFromEvent(event); got != tt.want {
				t.Errorf("localeFromEvent() = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestEventNamesMatchFrontend 检查事件名与前端常量表一致。
//
// 这两处是跨语言契约：改名时只动一边不会有任何编译错误，
// 只会在运行时静默失效（菜单点了没反应）。用测试把它变成可见的失败。
func TestEventNamesMatchFrontend(t *testing.T) {
	path := filepath.Join("..", "..", "frontend", "src", "hooks", "event", "useNativeEvent.ts")

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read frontend event table: %v", err)
	}
	content := string(data)

	for _, name := range []string{eventNavigate, eventLanguage} {
		if !strings.Contains(content, `"`+name+`"`) && !strings.Contains(content, `'`+name+`'`) {
			t.Errorf("event %q is not declared in %s", name, path)
		}
	}
}
