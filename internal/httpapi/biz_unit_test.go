package httpapi

// httpapi 业务接口层审查项(BIZ*)的包内单测:针对纯函数/内部结构的确定性断言。

import (
	"context"
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
