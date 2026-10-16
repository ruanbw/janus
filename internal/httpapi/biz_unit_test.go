package httpapi

// httpapi 业务接口层审查项(BIZ*)的包内单测:针对纯函数/内部结构的确定性断言。

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
	"unicode/utf8"

	"janus/internal/config"
	"janus/internal/store"
)

// bizZip 构造一个最小合法 zip(仅用于安装错误分类的单测)。
func bizZip(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, body := range files {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte(body)); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// BIZ4: installLandingZip 的 zip 校验错误应分类为 400 用户可读,文件系统等内部
// 错误应分类为 500 统一文案(不回显绝对路径)。
func TestLandingInstallErrorClassification(t *testing.T) {
	// 压缩包本身的问题 → 400 + 可读原因
	if st, code, msg := landingInstallError(zipErr{"压缩包为空"}); st != http.StatusBadRequest ||
		code != errValidation || msg != "压缩包为空" {
		t.Fatalf("zipErr 分类 = (%d,%s,%q)", st, code, msg)
	}

	// 非 zipErr(如 os.PathError)→ 500 + 统一文案,且不含内部路径
	secret := &os.PathError{Op: "mkdir", Path: "/var/lib/janus/secret/uploads", Err: errors.New("permission denied")}
	st, code, msg := landingInstallError(secret)
	if st != http.StatusInternalServerError || code != errInternal || msg != "internal error" {
		t.Fatalf("内部错误分类 = (%d,%s,%q)", st, code, msg)
	}
	if strings.Contains(msg, "secret") || strings.Contains(msg, "/var") {
		t.Fatalf("内部错误文案泄露路径: %q", msg)
	}

	// 端到端:上传目录的父路径是一个普通文件 → MkdirAll 失败 → 归为内部错误
	base := t.TempDir()
	blocker := filepath.Join(base, "blocker")
	if err := os.WriteFile(blocker, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	a := &API{cfg: config.Config{
		LandingUploadDir:     filepath.Join(blocker, "uploads"),
		LandingMaxZipBytes:   1 << 20,
		LandingMaxTotalBytes: 1 << 20,
		LandingMaxFiles:      10,
	}}
	_, _, err := a.installLandingZip(1, bizZip(t, map[string]string{"index.html": "<html>ok</html>"}))
	if err == nil {
		t.Fatal("父路径是文件时 MkdirAll 应失败")
	}
	if st, _, msg := landingInstallError(err); st != http.StatusInternalServerError {
		t.Fatalf("文件系统错误应归为 500,得到 %d(文案 %q,err %v)", st, msg, err)
	}
}

// BIZ14: 后置访问钩子的 goroutine 数量有上限,满则丢弃且不阻塞调用方。
func TestTriggerPostVisitHooksBoundedAndNonBlocking(t *testing.T) {
	ResetPostVisitHooks()
	t.Cleanup(ResetPostVisitHooks)

	release := make(chan struct{})
	var started atomic.Int32
	RegisterPostVisitHook(func(context.Context, *http.Request, store.VisitRecord) {
		started.Add(1)
		<-release
	})

	req := httptest.NewRequest("GET", "/", nil)
	// 占满全部槽位
	for i := 0; i < maxConcurrentPostHooks; i++ {
		TriggerPostVisitHooks(context.Background(), req, store.VisitRecord{})
	}
	deadline := time.Now().Add(3 * time.Second)
	for started.Load() < int32(maxConcurrentPostHooks) && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if got := started.Load(); got != int32(maxConcurrentPostHooks) {
		t.Fatalf("已启动钩子 = %d, want %d", got, maxConcurrentPostHooks)
	}

	// 饱和后再触发必须立即返回(丢弃),不得阻塞响应路径
	done := make(chan struct{})
	go func() {
		TriggerPostVisitHooks(context.Background(), req, store.VisitRecord{})
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("并发饱和时 TriggerPostVisitHooks 阻塞了调用方")
	}

	time.Sleep(50 * time.Millisecond)
	if got := started.Load(); got != int32(maxConcurrentPostHooks) {
		t.Fatalf("丢弃后仍启动了新钩子: %d", got)
	}
	close(release)
	// 等槽位全部归还,避免影响同包后续用例(它们可能真的依赖钩子被执行)。
	deadline = time.Now().Add(3 * time.Second)
	for len(postHookSlots) > 0 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
}

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
