package plugin_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/ruanbw/janus/pkg/plugin"
)

// TestPostVisitHookAsyncContext 钉住异步后置钩子的上下文契约:
// 基座传入的请求级 ctx 在交付完成后会被取消,异步钩子不得因此被打断,
// 但必须带一个固定超时护栏。
func TestPostVisitHookAsyncContext(t *testing.T) {
	t.Run("Given 父请求 ctx 在钩子执行前被取消 When 触发钩子 Then 钩子仍以未取消的 ctx 执行", func(t *testing.T) {
		plugin.ResetPostVisitHooks()
		defer plugin.ResetPostVisitHooks()

		type result struct {
			err      error
			hasLimit bool
		}
		got := make(chan result, 1)

		plugin.RegisterPostVisitHook(func(hookCtx context.Context, _ *http.Request, _ plugin.VisitRecord) {
			// 等一小段,确保父 ctx 已被取消。
			time.Sleep(100 * time.Millisecond)
			_, hasDeadline := hookCtx.Deadline()
			got <- result{err: hookCtx.Err(), hasLimit: hasDeadline}
		})

		req := httptest.NewRequest(http.MethodGet, "https://example.com/x", nil)
		parent, cancel := context.WithCancel(context.Background())

		plugin.TriggerPostVisitHooks(parent, req, plugin.VisitRecord{LinkID: 1})
		cancel() // 请求结束:立即取消父 ctx

		select {
		case r := <-got:
			if r.err != nil {
				t.Fatalf("异步钩子的 ctx 不应随父请求 ctx 取消,got %v", r.err)
			}
			if !r.hasLimit {
				t.Fatal("异步钩子的 ctx 应带固定超时护栏(Deadline)")
			}
		case <-time.After(3 * time.Second):
			t.Fatal("钩子未在超时内执行")
		}
	})

	t.Run("Given 未注册任何钩子 When 触发 Then 直接返回不启动 goroutine", func(t *testing.T) {
		plugin.ResetPostVisitHooks()
		defer plugin.ResetPostVisitHooks()

		req := httptest.NewRequest(http.MethodGet, "https://example.com/x", nil)
		plugin.TriggerPostVisitHooks(context.Background(), req, plugin.VisitRecord{LinkID: 2})
	})
}
