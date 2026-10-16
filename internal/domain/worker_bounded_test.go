package domain

// 单轮 pass 的有界并发测试。
//
// 背景:一轮最多 500 条域名,顺序处理时单轮最坏耗时(dns-retry ≈75 分钟、
// cert-probe ≈83 分钟)远超 5 分钟的轮询间隔,新 pending 域名永远排在队尾。
// 这里钉住两个不变量:并发不超过上限(不能无界放大出网压力)、且任务全部跑完
// 才返回(不能因为并发而漏处理)。

import (
	"sync/atomic"
	"testing"
	"time"
)

// 并发上限:任意时刻在跑的 fn 数量不得超过 limit,且确实用满了 limit(证明不是
// 退化回顺序处理)。
func TestRunBoundedRespectsLimitAndUsesIt(t *testing.T) {
	const (
		limit = 4
		total = 40
	)
	items := make([]int, total)
	for i := range items {
		items[i] = i
	}

	var (
		inFlight    int64
		maxInFlight int64
		processed   int64
	)
	runBounded(items, limit, func(int) {
		cur := atomic.AddInt64(&inFlight, 1)
		for {
			old := atomic.LoadInt64(&maxInFlight)
			if cur <= old || atomic.CompareAndSwapInt64(&maxInFlight, old, cur) {
				break
			}
		}
		// 睡一小会儿,让同批的其它 goroutine 有机会同时在场:
		// 没有这段等待,并发度会退化成 1 而断言假通过。
		time.Sleep(2 * time.Millisecond)
		atomic.AddInt64(&inFlight, -1)
		atomic.AddInt64(&processed, 1)
	})

	if got := atomic.LoadInt64(&processed); got != total {
		t.Fatalf("处理了 %d 条, want %d(runBounded 必须等全部处理完才返回)", got, total)
	}
	if got := atomic.LoadInt64(&maxInFlight); got > limit {
		t.Fatalf("并发峰值 %d 超过上限 %d:出网查询会被放大,上游 DNS 与平台自己一起被打满", got, limit)
	} else if got < limit {
		t.Fatalf("并发峰值只有 %d(< %d):单轮耗时没有改善,域名仍会排不上队", got, limit)
	}
}

// limit <= 0 或 1 时退化为顺序执行(不能变成无界并发)。
func TestRunBoundedSerialWhenLimitIsOne(t *testing.T) {
	items := make([]int, 10)
	var (
		inFlight    int64
		maxInFlight int64
	)
	runBounded(items, 1, func(int) {
		cur := atomic.AddInt64(&inFlight, 1)
		if cur > atomic.LoadInt64(&maxInFlight) {
			atomic.StoreInt64(&maxInFlight, cur)
		}
		time.Sleep(time.Millisecond)
		atomic.AddInt64(&inFlight, -1)
	})
	if got := atomic.LoadInt64(&maxInFlight); got != 1 {
		t.Fatalf("limit=1 时并发峰值为 %d, want 1", got)
	}
	// limit <= 0 按 1 处理(构造方传错参数时不能退化成无界并发)
	runBounded(items, 0, func(int) {
		cur := atomic.AddInt64(&inFlight, 1)
		if cur > atomic.LoadInt64(&maxInFlight) {
			atomic.StoreInt64(&maxInFlight, cur)
		}
		atomic.AddInt64(&inFlight, -1)
	})
	if got := atomic.LoadInt64(&maxInFlight); got != 1 {
		t.Fatalf("limit=0 时并发峰值为 %d, want 1", got)
	}
}

// 空输入不得阻塞(不能因为等信号量而挂住)。
func TestRunBoundedEmptyInput(t *testing.T) {
	done := make(chan struct{})
	go func() {
		defer close(done)
		runBounded([]int(nil), 4, func(int) { t.Error("空输入不应执行 fn") })
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("空输入时 runBounded 未返回")
	}
}
