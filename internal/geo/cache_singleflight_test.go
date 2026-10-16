package geo

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// TestCachedSingleflightCoalescesConcurrentMisses 钉住「同 IP 并发未命中只回源一次」:
// 否则高并发下同一个冷 IP 会把底层查询打满(缓存击穿)。
func TestCachedSingleflightCoalescesConcurrentMisses(t *testing.T) {
	var calls int32
	release := make(chan struct{})
	slow := LookupFunc(func(string) Info {
		atomic.AddInt32(&calls, 1)
		<-release
		return Info{Country: "US"}
	})
	c := Cached(slow, 4096)

	const n = 50
	results := make([]Info, n)
	start := make(chan struct{})
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			results[i] = c.Lookup("203.0.113.77")
		}(i)
	}
	close(start)
	// 给所有 goroutine 时间进入 LoadOrStore 等待首个回源。
	time.Sleep(200 * time.Millisecond)
	close(release)
	wg.Wait()

	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Fatalf("并发同 IP 应只回源一次,实际 %d 次", got)
	}
	for i, r := range results {
		if r.Country != "US" {
			t.Fatalf("results[%d].Country = %q, want US", i, r.Country)
		}
	}
}
