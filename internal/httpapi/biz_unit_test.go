package httpapi

// httpapi 业务接口层审查项(BIZ*)的包内单测:针对纯函数/内部结构的确定性断言。

import (
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"

	"janus/internal/config"
)

// BIZ10: resolveLandingFilePath 必须解析符号链接后再比对路径前缀。
func TestResolveLandingFilePathRejectsSymlinkEscape(t *testing.T) {
	base := t.TempDir()
	uploadDir := filepath.Join(base, "uploads")
	root := filepath.Join(uploadDir, "7")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "index.html"), []byte("ok"), 0o644); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(base, "outside")
	if err := os.MkdirAll(outside, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(outside, "secret.txt"), []byte("secret"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "evil")); err != nil {
		t.Skipf("symlink unsupported: %v", err)
	}

	a := &API{cfg: config.Config{LandingUploadDir: uploadDir}}

	if _, _, ok := a.resolveLandingFilePath(7, "index.html"); !ok {
		t.Fatal("正常文件被误拒")
	}
	if _, _, ok := a.resolveLandingFilePath(7, "evil/secret.txt"); ok {
		t.Fatal("目录内符号链接指向外部未被拒绝")
	}
}

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
