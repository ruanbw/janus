package httpapi

// httpapi 业务接口层审查项(BIZ*)的包内单测:针对纯函数/内部结构的确定性断言。

import (
	"net/http/httptest"
	"strings"
	"testing"
	"unicode/utf8"
)

// BIZ13: clientLang 按字节截断会切坏多字节 UTF-8,必须按 rune 截断。
func TestClientLangTruncatesByRune(t *testing.T) {
	long := strings.Repeat("中", maxLangLen+36) // 100 个 3 字节字符 = 300 字节
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Accept-Language", long)

	got := clientLang(req)
	if !utf8.ValidString(got) {
		t.Fatalf("clientLang 返回非法 UTF-8: %q", got)
	}
	if n := utf8.RuneCountInString(got); n != maxLangLen {
		t.Fatalf("截断后 rune 数 = %d, want %d", n, maxLangLen)
	}
}
