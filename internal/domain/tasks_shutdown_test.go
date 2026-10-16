package domain

// DNSChecker.Shutdown:队列持有者的关机手柄。
//
// 队列是懒创建的,所以 Shutdown 必须区分"还没建过"(什么都不做)与"已建过"
// (关闭它);顺手创建一份再关掉等于凭空制造出要清理的 goroutine。

import (
	"context"
	"testing"
	"time"
)

// DNSChecker.Shutdown 关闭它持有的队列(未创建过队列时什么也不做,不能顺手创建一份)。
func TestDNSCheckerShutdownClosesQueue(t *testing.T) {
	c := &DNSChecker{}
	c.Shutdown() // 没有队列时不得 panic,也不得凭空创建一个
	if c.tasks != nil {
		t.Fatal("Shutdown 在未创建队列时创建了队列:那正好制造出要清理的 goroutine")
	}
	q := c.Tasks()
	c.Shutdown()
	select {
	case <-q.ctx.Done():
	case <-time.After(2 * time.Second):
		t.Fatal("Shutdown 未取消队列上下文")
	}
	if q.Submit("k", func(context.Context) {}) {
		t.Fatal("Shutdown 之后 Submit = true, want false")
	}
}
