package rules

import (
	"bytes"
	"context"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ruanbw/janus/internal/store"
)

// TestCacheClientDisconnectDoesNotTriggerLoadFailureBackoff 是"客户端断连不该被记成
// 租户加载失败"的回归测试。
//
// 旧实现把**发起跳转的请求 ctx** 直接透给 loader。真实 loader 是数据库查询,
// ctx 一取消(pgx 之类)就立刻返回 context.Canceled;于是:
// 客户端断开 → loader 返错 → 写 failedAt → 该租户进入 loadFailureBackoff(1s)窗口,
// 期间所有请求都拿上一份(或空)快照。一次批量断连(或客户端提前断开)就能把
// 整个租户的规则"降级"整整一秒,而真正的原因只是"没人等这次响应了"。
func TestCacheClientDisconnectDoesNotTriggerLoadFailureBackoff(t *testing.T) {
	// Given: 一个会尊重 ctx 的 loader(与真实 DB 驱动同形)
	var calls atomic.Int32
	loader := func(ctx context.Context, _ int64) ([]store.Rule, error) {
		calls.Add(1)
		if err := ctx.Err(); err != nil {
			return nil, fmt.Errorf("load rules: %w", err)
		}
		return []store.Rule{globalRule(1, 10)}, nil
	}
	log, buf := logBuf()
	c := NewCache(loader, WithLogger(log), WithTTL(time.Minute))

	// And: 客户端在加载开始前就断开了(请求 ctx 已取消)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	// When: 跳转热路径用这个已取消的 ctx 取快照
	snap := c.Get(ctx, 1)

	// Then 1: 加载照常完成 —— 规则拿得到,loader 也真的被调用了一次
	if len(snap.Rules) != 1 {
		t.Fatalf("客户端断连时快照规则数 = %d, want 1:加载不该被当成失败", len(snap.Rules))
	}
	if got := calls.Load(); got != 1 {
		t.Fatalf("loader 调用次数 = %d, want 1", got)
	}

	// Then 2: 没有写 failedAt,该租户不进入退避窗口
	c.mu.RLock()
	e := c.items[1]
	c.mu.RUnlock()
	if e == nil {
		t.Fatal("加载后该租户的缓存记录应存在")
	}
	if !e.failedAt.IsZero() {
		t.Fatalf("客户端断连被记成加载失败:failedAt=%v,该租户会进入 %v 退避窗口",
			e.failedAt, loadFailureBackoff)
	}
	if bytes.Contains(buf.Bytes(), []byte("加载租户规则失败")) {
		t.Fatalf("不该出现加载失败日志:\n%s", buf.String())
	}

	// Then 3: 紧接着的正常请求仍能照常加载,不被退避窗口挡住
	newCtx, cancel2 := context.WithTimeout(context.Background(), time.Second)
	defer cancel2()
	c.Invalidate(1) // 模拟规则变更
	if snap2 := c.Get(newCtx, 1); len(snap2.Rules) != 1 {
		t.Fatalf("退避窗口内的正常请求被挡住了:规则数 = %d, want 1", len(snap2.Rules))
	}
	if got := calls.Load(); got != 2 {
		t.Fatalf("loader 调用次数 = %d, want 2:退避不该生效", got)
	}
}
