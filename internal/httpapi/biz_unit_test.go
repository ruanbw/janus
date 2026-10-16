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
	"sync"
	"sync/atomic"
	"testing"
	"time"
	"unicode/utf8"

	"janus/internal/config"
	"janus/internal/domain"
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

// BIZ9: 同一短链的并发上传必须串行化,不得出现 rename ENOENT 或残留临时/备份目录。
func TestInstallLandingZipGuardedSerializesConcurrentUploads(t *testing.T) {
	dir := t.TempDir()
	a := &API{cfg: config.Config{
		LandingUploadDir:     dir,
		LandingMaxZipBytes:   1 << 20,
		LandingMaxTotalBytes: 1 << 20,
		LandingMaxFiles:      10,
	}}
	zipA := bizZip(t, map[string]string{"index.html": "<html>A</html>"})
	zipB := bizZip(t, map[string]string{"index.html": "<html>B</html>", "a.css": "body{}"})

	// 先装一份,让并发上传走"替换已有目录"这条最容易踩的路径。
	commit, _, err := a.installLandingZipGuarded(7, zipA)
	if err != nil {
		t.Fatal(err)
	}
	commit()

	var wg sync.WaitGroup
	errCh := make(chan error, 32)
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			z := zipA
			if i%2 == 0 {
				z = zipB
			}
			c, _, err := a.installLandingZipGuarded(7, z)
			if err != nil {
				errCh <- err
				return
			}
			c()
		}(i)
	}
	wg.Wait()
	close(errCh)
	for err := range errCh {
		t.Fatalf("并发安装失败: %v", err)
	}

	if _, err := os.Stat(filepath.Join(dir, "7", "index.html")); err != nil {
		t.Fatalf("并发安装后 index.html 缺失: %v", err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".landing-") || strings.Contains(e.Name(), ".old-") {
			t.Fatalf("残留临时/备份目录: %s", e.Name())
		}
	}
}

// bizSlowOwnership 模拟慢 DNS 的归属校验器:直到 ctx 超时才返回。
type bizSlowOwnership struct{ delay time.Duration }

func (bizSlowOwnership) Tasks() *domain.TaskQueue { return nil }

func (s bizSlowOwnership) Verify(ctx context.Context, _, _ string) (domain.VerifyResult, error) {
	select {
	case <-time.After(s.delay):
		return domain.VerifyResult{Status: domain.VerifyVerified}, nil
	case <-ctx.Done():
		return domain.VerifyResult{}, ctx.Err()
	}
}

// BIZ2: 同步域名校验的子 ctx 超时生效(不会无限等待慢 DNS)。
func TestSyncDomainVerifyTimeout(t *testing.T) {
	old := domainVerifySyncTimeout
	domainVerifySyncTimeout = 20 * time.Millisecond
	t.Cleanup(func() { domainVerifySyncTimeout = old })

	a := &API{dns: bizSlowOwnership{delay: 5 * time.Second}}
	start := time.Now()
	if _, err := a.syncDomainVerify(context.Background(), "example.com", ""); err == nil {
		t.Fatal("超过子 ctx 超时仍返回 nil 错误")
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Fatalf("校验耗时 %v,超时上限未生效", elapsed)
	}
}

// BIZ2: 域名校验按租户限流,超限拒绝,不同租户互不影响。
func TestDomainVerifyLimiterPerTenant(t *testing.T) {
	a := &API{}
	for i := 0; i < domainVerifyPerMinute; i++ {
		if !a.domainVerifyLimiter().Allow("1") {
			t.Fatalf("第 %d 次域名校验被限流", i+1)
		}
	}
	if a.domainVerifyLimiter().Allow("1") {
		t.Fatal("超过每分钟上限仍放行")
	}
	if !a.domainVerifyLimiter().Allow("2") {
		t.Fatal("限流桶应按租户隔离,另一个租户被误伤")
	}
}

// BIZ1: 错误页缓存按租户串行化,不同租户的慢路径互不阻塞。
func TestErrorPagesCachePerTenantLoadsDoNotBlock(t *testing.T) {
	entered := make(chan struct{})
	release := make(chan struct{})
	var once sync.Once
	load := func(_ context.Context, tenantID int64) (string, string, error) {
		if tenantID == 1 {
			once.Do(func() { close(entered) })
			<-release // 模拟租户 1 的慢回源
			return "<p>t1</p>", "", nil
		}
		return "<p>t2</p>", "", nil
	}
	c := newErrorPagesCache(load)

	go c.Get(context.Background(), 1)
	select {
	case <-entered:
	case <-time.After(2 * time.Second):
		close(release)
		t.Fatal("租户 1 未进入加载")
	}

	res2 := make(chan *errorPagesSnapshot, 1)
	go func() { res2 <- c.Get(context.Background(), 2) }()
	select {
	case snap := <-res2:
		if snap.notFound != "<p>t2</p>" {
			close(release)
			t.Fatalf("租户 2 快照 = %+v", snap)
		}
	case <-time.After(2 * time.Second):
		close(release)
		t.Fatal("租户 1 回源期间租户 2 的加载被阻塞(全局锁未拆)")
	}
	close(release)
}

// BIZ1: 同一租户的并发未命中只触发一次加载(双检 + 按租户锁)。
func TestErrorPagesCacheSameTenantCoalesced(t *testing.T) {
	var loads atomic.Int32
	c := newErrorPagesCache(func(context.Context, int64) (string, string, error) {
		loads.Add(1)
		time.Sleep(50 * time.Millisecond)
		return "x", "", nil
	})
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			c.Get(context.Background(), 1)
		}()
	}
	wg.Wait()
	if got := loads.Load(); got != 1 {
		t.Fatalf("同一租户并发加载 %d 次, want 1", got)
	}
}

// BIZ1: 回源使用独立超时,超时空快照 fail-open,且不写缓存。
func TestErrorPagesCacheLoadTimeoutFailsOpen(t *testing.T) {
	c := newErrorPagesCache(func(ctx context.Context, _ int64) (string, string, error) {
		<-ctx.Done()
		return "", "", ctx.Err()
	})
	c.loadTimeout = 30 * time.Millisecond

	start := time.Now()
	snap := c.Get(context.Background(), 1) // 请求 ctx 无 deadline
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Fatalf("回源耗时 %v,独立超时未生效", elapsed)
	}
	if snap.notFound != "" || snap.tooManyRequests != "" {
		t.Fatalf("超时应返回空快照(fail-open),得到 %+v", snap)
	}
	if c.cached(1) != nil {
		t.Fatal("超时结果被写进缓存")
	}
}

// BIZ1: 回源不受访客请求 ctx 取消影响(用 context.WithoutCancel)。
func TestErrorPagesCacheIgnoresRequestCtxCancellation(t *testing.T) {
	called := make(chan struct{}, 1)
	c := newErrorPagesCache(func(ctx context.Context, _ int64) (string, string, error) {
		if ctx.Err() != nil {
			return "", "", ctx.Err()
		}
		called <- struct{}{}
		return "<p>ok</p>", "", nil
	})
	reqCtx, cancel := context.WithCancel(context.Background())
	cancel() // 访客已断连

	snap := c.Get(reqCtx, 1)
	select {
	case <-called:
	default:
		t.Fatal("请求 ctx 已取消时 loader 未被调用")
	}
	if snap.notFound != "<p>ok</p>" {
		t.Fatalf("请求 ctx 取消不该影响回源: %+v", snap)
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
