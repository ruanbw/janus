package rules

import (
	"context"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ruanbw/janus/internal/store"
)

// TestCacheLoaderPanicRecordsBackoff loader panic 也必须进退避窗口。
//
// 旧实现只把 loader **返回 error** 记成加载失败,panic 路径刻意不记 failedAt。
// 代价是:一个每次调用都 panic 的 loader 会在每个请求上重跑一遍 —— 每跑一次
// 就要展开一份 debug.Stack 并写一条 ERROR 日志,故障期间把 CPU 与日志一起打满,
// 这正是 loadFailureBackoff 存在的理由(见其注释)。
func TestCacheLoaderPanicRecordsBackoff(t *testing.T) {
	var calls atomic.Int32
	loader := func(context.Context, int64) ([]store.Rule, error) {
		calls.Add(1)
		panic("loader boom")
	}
	log, buf := logBuf()
	c := NewCache(loader, WithLogger(log), WithTTL(time.Minute))
	ctx := context.Background()

	// When: 第一个请求触发一次 panic 的加载
	if snap := c.Get(ctx, 1); len(snap.Rules) != 0 {
		t.Fatalf("panic 的加载应 fail-open(无规则), got %d 条", len(snap.Rules))
	}
	if !strings.Contains(buf.String(), "panic") {
		t.Fatalf("panic 没有被记录:\n%s", buf.String())
	}

	// Then 1: failedAt 被写下,该租户进入退避窗口
	c.mu.RLock()
	e := c.items[1]
	c.mu.RUnlock()
	if e == nil || e.failedAt.IsZero() {
		t.Fatal("loader panic 没有记 failedAt:退避窗口不会生效")
	}

	// Then 2: 窗口内的后续请求不再重跑 loader
	for i := 0; i < 3; i++ {
		if snap := c.Get(ctx, 1); len(snap.Rules) != 0 {
			t.Fatalf("退避窗口内应返回空快照(fail-open), got %d 条", len(snap.Rules))
		}
	}
	if got := calls.Load(); got != 1 {
		t.Fatalf("loader 被调用 %d 次, want 1(退避窗口内不该重跑)", got)
	}
}
