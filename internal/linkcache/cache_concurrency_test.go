package linkcache

// 惊群保护（single-flight）的测试规格。
//
// 事故形态：热门短链在缓存冷启动的一瞬间被 N 个并发跳转同时打到 DB，
// 每次要付 GetDomainByFQDN + LookupLinkForVisit(JOIN) + link_targets 查询的代价。
// 冷启动不是罕见路径 —— 进程重启、TTL 过期、按域名失效之后，
// 第一批并发流量永远落在"无人缓存"的窗口里。
//
// 这组用例不靠 sleep 制造时序，而用 barrier：所以它在慢机器/CI 上结果一致。

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// TestCacheConcurrentColdGetCollapsesToSingleLoad 固化"同一冷 key 的并发 Get 只回源一次"。
func TestCacheConcurrentColdGetCollapsesToSingleLoad(t *testing.T) {
	const goroutines = 64
	ctx := context.Background()

	entered := make(chan struct{}) // loader 已进入第一次加载
	release := make(chan struct{}) // 放第一次加载继续
	var enteredOnce sync.Once
	var loads atomic.Int64

	loader := func(_ context.Context, domainID int64, _ string) (*Record, error) {
		loads.Add(1)
		// 把第一次加载卡住：其余 goroutine 只有两个去处 ——
		// 正确地等待同一次加载，或者错误地各查一次库（后者由 loads==1 抓出来）。
		enteredOnce.Do(func() { close(entered) })
		<-release
		return testRecord(100, 7, domainID), nil
	}
	c := New(loader)

	results := make([]*Record, goroutines)
	errs := make([]error, goroutines)

	start := make(chan struct{})
	var ready sync.WaitGroup // 每个 goroutine 越过 start 后自报一次
	var done sync.WaitGroup
	ready.Add(goroutines)
	done.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func(i int) {
			defer done.Done()
			<-start
			ready.Done()
			results[i], errs[i] = c.Get(ctx, 10, "hot")
		}(i)
	}
	close(start)
	ready.Wait() // 64 个 goroutine 全部就位，正在进入 Get
	<-entered    // 其中一个已经进到 loader 里
	close(release)

	doneCh := make(chan struct{})
	go func() { done.Wait(); close(doneCh) }()
	select {
	case <-doneCh:
	case <-time.After(10 * time.Second):
		// 死锁也算这条契约的失败：等待者必须由那次加载唤醒，不能互相等。
		t.Fatal("并发 Get 在冷 key 上卡住 10s：等待者没有被那次加载唤醒")
	}

	t.Run("Given 64 个 goroutine 同时 Get 同一冷 key When 加载完成 Then loader 只被调用一次", func(t *testing.T) {
		if got := loads.Load(); got != 1 {
			t.Fatalf("loader 调用 %d 次, want 1(惊群保护失效:每个并发跳转各查一次库)", got)
		}
		if s := c.Stats(); s.Loads != 1 {
			t.Fatalf("Stats.Loads = %d, want 1", s.Loads)
		}
	})

	t.Run("Given 64 个 goroutine 同时 Get 同一冷 key When 加载完成 Then 全部拿到同一个对象", func(t *testing.T) {
		for i := range results {
			if errs[i] != nil {
				t.Fatalf("第 %d 个 goroutine 拿到错误: %v", i, errs[i])
			}
			if results[i] == nil {
				t.Fatalf("第 %d 个 goroutine 拿到 nil Record", i)
			}
			// 同一指针是"大家共享一次加载结果"的直接证据；
			// 各拿一份内容相同的副本意味着 TTL 到期判断会出现 N 个不同的时间点。
			if results[i] != results[0] {
				t.Fatalf("第 %d 个 goroutine 拿到不同的 *Record(等待者被各自回源了)", i)
			}
		}
	})

	t.Run("Given 64 次并发 Get When 读 Stats Then 每次 Get 恰好记一次 hit 或 miss", func(t *testing.T) {
		s := c.Stats()
		if s.Hits+s.Misses != goroutines {
			t.Fatalf("Hits+Misses = %d, want %d(每次 Get 都必须被统计,且只能记一次)", s.Hits+s.Misses, goroutines)
		}
		if n := c.Len(); n != 1 {
			t.Fatalf("Len = %d, want 1", n)
		}
	})

	t.Run("Given 冷 key 已被这次惊群填满 When 再来一波并发 Get Then 不再产生任何回源", func(t *testing.T) {
		var wg sync.WaitGroup
		for i := 0; i < goroutines; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				if got, err := c.Get(ctx, 10, "hot"); err != nil || got != results[0] {
					t.Errorf("热 key 上 Get 拿到的结果不一致: got=%p err=%v", got, err)
				}
			}()
		}
		wg.Wait()
		if got := loads.Load(); got != 1 {
			t.Fatalf("热 key 上又回源了: loader 调用 %d 次, want 1", got)
		}
	})
}
