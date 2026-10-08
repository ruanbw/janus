package rules

import (
	"bytes"
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"janus/internal/store"
)

// ---------- 03:失效与加载的丢失效竞态 ----------

// TestCacheInvalidateDuringLoadDoesNotResurrectStaleSnapshot 是本次竞态的回归测试。
//
// 旧实现的时序(用 barrier 卡住,不靠 sleep 碰运气):
//
//	Get 开始读库 ────────────── 写回 items
//	      └─── 拿到旧数据
//	              PATCH 提交 → Invalidate(delete items[1],此刻是个空操作)
//	                             ────────────── 写回 items[1] = 旧数据 ← 之后整个 TTL 不生效
//
// 修复:缓存 value 带 gen,Invalidate 自增,Get 写回前比对加载开始时记下的 gen,
// 不一致就丢弃。于是"刚保存的规则不生效且无报错"这条静默失效被关掉了。
func TestCacheInvalidateDuringLoadDoesNotResurrectStaleSnapshot(t *testing.T) {
	// 库里的内容用一个原子变量模拟:PATCH 把规则 id 从 1 改成 2
	var version atomic.Int64
	version.Store(1)

	loading := make(chan struct{}) // loader 已进入
	release := make(chan struct{}) // 放 loader 继续
	var once sync.Once
	loader := func(_ context.Context, _ int64) ([]store.Rule, error) {
		// 第一次加载卡住,制造确定的时序窗口
		once.Do(func() {
			close(loading)
			<-release
		})
		return []store.Rule{globalRule(version.Load(), 10)}, nil
	}
	log, buf := logBuf()
	c := NewCache(loader, WithLogger(log), WithTTL(time.Minute))
	ctx := context.Background()

	got := make(chan *Snapshot, 1)
	go func() { got <- c.Get(ctx, 1) }()

	<-loading // 确定 Get 正在读库,且还没写回
	// 这就是那个 PATCH:先提交(规则 id 从 1 变成 2),再失效
	version.Store(2)
	c.Invalidate(1)
	close(release)

	first := <-got
	// 撞上失效的那次调用按未命中继续(fail-open)——关键是不写回旧数据
	if len(first.Rules) != 0 {
		t.Fatalf("撞上失效的加载不该返回规则,却返回了 %d 条", len(first.Rules))
	}
	if !bytes.Contains(buf.Bytes(), []byte("加载期间变更")) {
		t.Fatalf("应当记录一次可观测的丢弃:\n%s", buf.String())
	}

	// 关键断言:下一次 Get 必须读到新数据,而不是被旧数据占住一个 TTL
	second := c.Get(ctx, 1)
	if len(second.Rules) != 1 {
		t.Fatalf("失效后的 Get 拿到 %d 条规则,旧数据被写回了", len(second.Rules))
	}
	if id := second.Rules[0].Rule.ID; id != 2 {
		t.Fatalf("规则 id = %d, want 2(刚保存的规则必须在 TTL 内生效)", id)
	}
	// 之后的 Get 继续命中新快照(不必每次回库)
	third := c.Get(ctx, 1)
	if third != second || third.Rules[0].Rule.ID != 2 {
		t.Fatal("新快照没有稳定缓存住")
	}
}

// TestCacheInvalidateWithNoInFlightLoadStillBumpsGeneration 代号自增的边界:
// 失效发生在没有任何加载在途时,下一次 Get 一定要重新读库。
func TestCacheInvalidateWithNoInFlightLoadStillBumpsGeneration(t *testing.T) {
	var version atomic.Int64
	version.Store(1)
	var calls atomic.Int64
	loader := func(_ context.Context, _ int64) ([]store.Rule, error) {
		calls.Add(1)
		return []store.Rule{globalRule(version.Load(), 10)}, nil
	}
	log, _ := logBuf()
	// TTL 不兜底:只有 Invalidate 能让缓存失效
	c := NewCache(loader, WithLogger(log), WithTTL(time.Hour))
	ctx := context.Background()

	if got := c.Get(ctx, 1).Rules[0].Rule.ID; got != 1 {
		t.Fatalf("首次加载 id = %d", got)
	}
	version.Store(2)
	c.Invalidate(1)
	if got := c.Get(ctx, 1).Rules[0].Rule.ID; got != 2 {
		t.Fatalf("失效后 id = %d, want 2", got)
	}
	if calls.Load() != 2 {
		t.Fatalf("加载次数 = %d, want 2", calls.Load())
	}
	// 反复失效反复加载,代数不能被"折叠"掉
	for i := int64(3); i < 8; i++ {
		version.Store(i)
		c.Invalidate(1)
		if got := c.Get(ctx, 1).Rules[0].Rule.ID; got != i {
			t.Fatalf("第 %d 次 id = %d, want %d", i, got, i)
		}
	}
}

// TestCacheInvalidateAllDuringLoad 平台级失效也要让在途的加载发现"我过期了"。
func TestCacheInvalidateAllDuringLoad(t *testing.T) {
	loading := make(chan struct{})
	release := make(chan struct{})
	var once sync.Once
	loader := func(_ context.Context, _ int64) ([]store.Rule, error) {
		once.Do(func() {
			close(loading)
			<-release
		})
		return []store.Rule{globalRule(1, 10)}, nil
	}
	log, _ := logBuf()
	c := NewCache(loader, WithLogger(log), WithTTL(time.Minute))
	ctx := context.Background()

	got := make(chan *Snapshot, 1)
	go func() { got <- c.Get(ctx, 7) }()
	<-loading
	c.InvalidateAll()
	close(release)

	if snap := <-got; len(snap.Rules) != 0 {
		t.Fatalf("全局失效后不该把旧数据写回,却拿到 %d 条", len(snap.Rules))
	}
}

// ---------- 04:按租户分片,跨租户不互相阻塞 ----------

// TestCacheDifferentTenantsDoNotBlockEachOther 是"loadMu 全局锁"这个瓶颈的回归测试。
//
// 旧实现里一把 loadMu 串行所有租户:租户 1 的 loader 卡住,租户 2 的 Get
// 就一直排队。修复后每租户一把锁,别的租户不受影响。
func TestCacheDifferentTenantsDoNotBlockEachOther(t *testing.T) {
	blocked := make(chan struct{}) // 租户 1 的 loader 卡在这里
	release := make(chan struct{}) // 放租户 1 回来
	var once sync.Once
	loader := func(_ context.Context, tenantID int64) ([]store.Rule, error) {
		if tenantID == 1 {
			once.Do(func() {
				close(blocked)
				<-release
			})
		}
		return []store.Rule{globalRule(tenantID, 10)}, nil
	}
	log, _ := logBuf()
	c := NewCache(loader, WithLogger(log), WithTTL(0))
	ctx := context.Background()

	// 租户 1 卡在加载里
	tenant1 := make(chan *Snapshot, 1)
	go func() { tenant1 <- c.Get(ctx, 1) }()
	<-blocked

	// 租户 2..50 必须立刻拿到自己的快照
	var others sync.WaitGroup
	for id := int64(2); id <= 50; id++ {
		others.Add(1)
		go func(id int64) {
			defer others.Done()
			snap := c.Get(ctx, id)
			if len(snap.Rules) == 1 && snap.Rules[0].Rule.ID == id {
				return
			}
			t.Errorf("租户 %d 拿到 %d 条规则", id, len(snap.Rules))
		}(id)
	}
	othersWait := make(chan struct{})
	go func() { others.Wait(); close(othersWait) }()
	select {
	case <-othersWait:
	case <-time.After(3 * time.Second):
		t.Fatal("租户 1 的加载卡住了其它租户 —— 惊群保护必须是按租户分片的")
	}

	close(release)
	if snap := <-tenant1; len(snap.Rules) != 1 {
		t.Fatal("租户 1 应正常加载完成")
	}
	// 锁表回收干净
	if n := c.loads.size(); n != 0 {
		t.Fatalf("锁表残留 %d 条(所有租户都已离开)", n)
	}
}

// TestCacheSameTenantStillCollapsesConcurrentLoads 按租户分片不能把惊群保护弄丢:
// 同一租户仍然只加载一次。
func TestCacheSameTenantStillCollapsesConcurrentLoads(t *testing.T) {
	var calls atomic.Int64
	loader := func(_ context.Context, _ int64) ([]store.Rule, error) {
		calls.Add(1)
		time.Sleep(10 * time.Millisecond)
		return []store.Rule{globalRule(1, 10)}, nil
	}
	log, _ := logBuf()
	c := NewCache(loader, WithLogger(log), WithTTL(0))

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if snap := c.Get(context.Background(), 1); len(snap.Rules) != 1 {
				t.Errorf("规则数 = %d", len(snap.Rules))
			}
		}()
	}
	wg.Wait()
	if got := calls.Load(); got != 1 {
		t.Fatalf("加载次数 = %d, want 1(惊群保护失效)", got)
	}
	if n := c.loads.size(); n != 0 {
		t.Fatalf("锁表残留 %d 条", n)
	}
}

// TestTenantLockTableRecovers 锁表按引用计数回收:没人持有也没人等就摘掉。
// 长期运行下租户数会涨落,锁表不回收就是按租户数无界增长。
func TestTenantLockTableRecovers(t *testing.T) {
	locks := &tenantLocks{locks: make(map[int64]*tenantLock)}
	var wg sync.WaitGroup
	for round := 0; round < 50; round++ {
		for id := int64(1); id <= 20; id++ {
			wg.Add(1)
			go func(id int64) {
				defer wg.Done()
				unlock := locks.lock(id)
				time.Sleep(time.Millisecond)
				unlock()
			}(id)
		}
		wg.Wait()
		if n := locks.size(); n != 0 {
			t.Fatalf("第 %d 轮后锁表残留 %d 条", round, n)
		}
	}
}

// ---------- 07:加载期 panic 兜住 ----------

// TestCacheLoaderPanicFailsOpen README 承诺"快照加载失败一律按未命中继续",
// 但原来只覆盖 loader **返回 error**。loader **panic** 会一路冒到全局 recover
// 变成 500 —— 风控配置出一次问题就让整站短链挂掉。
func TestCacheLoaderPanicFailsOpen(t *testing.T) {
	log, buf := logBuf()
	c := NewCache(func(_ context.Context, _ int64) ([]store.Rule, error) {
		panic("loader 内部炸了")
	}, WithLogger(log))
	dec, ok := c.Get(context.Background(), 1).Evaluate(Fact{UA: "bot"}, 1)
	if ok || dec != (Decision{}) {
		t.Fatalf("loader panic 时不该命中: %+v", dec)
	}
	if c.Len() != 0 {
		t.Fatal("panic 的加载不该写入缓存")
	}
	if !bytes.Contains(buf.Bytes(), []byte("fail-open")) {
		t.Fatalf("缺少 fail-open 日志:\n%s", buf.String())
	}
}

// TestCacheErrorAndPanicBothLeaveCacheEmpty 加载出错与加载 panic 的可观测行为一致:
// 都按无规则放行、都不写缓存、都会重试(不会被缓存成"这个租户永远没有规则")。
func TestCacheErrorAndPanicBothLeaveCacheEmpty(t *testing.T) {
	loaders := map[string]Loader{
		"error": func(context.Context, int64) ([]store.Rule, error) {
			return nil, errors.New("db down")
		},
		"panic": func(context.Context, int64) ([]store.Rule, error) {
			panic("db down")
		},
	}
	for name, loader := range loaders {
		t.Run(name, func(t *testing.T) {
			log, _ := logBuf()
			c := NewCache(loader, WithLogger(log), WithTTL(0))
			if snap := c.Get(context.Background(), 1); len(snap.Rules) != 0 {
				t.Fatalf("应返回空快照,却拿到 %d 条", len(snap.Rules))
			}
			if c.Len() != 0 {
				t.Fatal("失败的加载不该写缓存")
			}
			// 第二次仍然尝试加载
			_ = c.Get(context.Background(), 1)
			if c.Len() != 0 {
				t.Fatal("第二次也不该写缓存")
			}
		})
	}
}

// ---------- 重新加载失败:沿用上一份好快照 + 短暂退避 ----------

// flakyLoader 可切换成功/失败的 loader,并统计调用次数。
type flakyLoader struct {
	fail  atomic.Bool
	calls atomic.Int64
	id    atomic.Int64
	// gate 非 nil 时,失败分支先等它关闭(制造"加载在途"的时序窗口)
	gate chan struct{}
}

func (f *flakyLoader) load(_ context.Context, _ int64) ([]store.Rule, error) {
	f.calls.Add(1)
	if f.fail.Load() {
		if f.gate != nil {
			<-f.gate
		}
		return nil, errors.New("db down")
	}
	return []store.Rule{globalRule(f.id.Load(), 10)}, nil
}

// TestCacheReloadFailureKeepsStaleSnapshotAfterTTL TTL 到期后重新加载失败,
// 必须继续用上一份快照,而不是把整租户的规则一下子全部丢掉(旧实现:返回空快照)。
func TestCacheReloadFailureKeepsStaleSnapshotAfterTTL(t *testing.T) {
	f := &flakyLoader{}
	f.id.Store(1)
	log, buf := logBuf()
	c := NewCache(f.load, WithLogger(log), WithTTL(10*time.Millisecond))
	ctx := context.Background()
	first := c.Get(ctx, 1)
	if len(first.Rules) != 1 {
		t.Fatalf("首次加载 = %d 条", len(first.Rules))
	}
	time.Sleep(20 * time.Millisecond) // 过期
	f.fail.Store(true)
	got := c.Get(ctx, 1)
	if got != first {
		t.Fatalf("重新加载失败时应沿用上一份快照,却拿到 %d 条的新对象", len(got.Rules))
	}
	if _, ok := got.Evaluate(Fact{UA: "bot"}, 1); !ok {
		t.Fatal("沿用的旧快照应照常命中")
	}
	if !bytes.Contains(buf.Bytes(), []byte("沿用上一份快照")) {
		t.Fatalf("缺少沿用旧快照的日志:\n%s", buf.String())
	}
}

// TestCacheReloadFailureAfterInvalidateKeepsLastGood 规则保存后 Invalidate,
// 紧接着的重新加载撞上数据库抖动:用失效前那份(最近一次加载成功的)快照,不是空快照。
func TestCacheReloadFailureAfterInvalidateKeepsLastGood(t *testing.T) {
	f := &flakyLoader{}
	f.id.Store(1)
	log, _ := logBuf()
	c := NewCache(f.load, WithLogger(log), WithTTL(time.Hour))
	ctx := context.Background()
	first := c.Get(ctx, 1)
	c.Invalidate(1)
	if c.Len() != 0 {
		t.Fatalf("Invalidate 后有效快照数 = %d, want 0(旧快照只作兜底)", c.Len())
	}
	f.fail.Store(true)
	if got := c.Get(ctx, 1); got != first {
		t.Fatal("失效后重新加载失败,应沿用最近一份好快照")
	}
	// 数据库恢复、退避期内又有一次 Invalidate(新的保存)→ 立即重试并拿到新规则
	f.fail.Store(false)
	f.id.Store(2)
	c.Invalidate(1)
	got := c.Get(ctx, 1)
	if len(got.Rules) != 1 || got.Rules[0].Rule.ID != 2 {
		t.Fatalf("Invalidate 应越过退避立即重试,拿到 %+v", got.Rules)
	}
}

// TestCacheFirstLoadFailureFallsBackToEmpty 从没加载成功过、手里没有任何快照:
// 只有这种情况才按"没有规则"放行。
func TestCacheFirstLoadFailureFallsBackToEmpty(t *testing.T) {
	f := &flakyLoader{}
	f.fail.Store(true)
	log, buf := logBuf()
	c := NewCache(f.load, WithLogger(log))
	snap := c.Get(context.Background(), 1)
	if snap == nil || len(snap.Rules) != 0 {
		t.Fatalf("首次加载失败应返回空快照: %+v", snap)
	}
	if !bytes.Contains(buf.Bytes(), []byte("fail-open")) {
		t.Fatalf("缺少 fail-open 日志:\n%s", buf.String())
	}
}

// TestCacheLoadFailureBackoffStopsSerializedRetries 加载失败后短暂记住失败:
// 退避期内同租户的请求不再打数据库(旧实现:每个排队的请求拿到锁后各自重试一次,串行等待)。
func TestCacheLoadFailureBackoffStopsSerializedRetries(t *testing.T) {
	f := &flakyLoader{}
	f.fail.Store(true)
	log, _ := logBuf()
	c := NewCache(f.load, WithLogger(log))
	c.backoff = time.Hour // 测试内退避不会过期
	ctx := context.Background()
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = c.Get(ctx, 1)
		}()
	}
	wg.Wait()
	if n := f.calls.Load(); n != 1 {
		t.Fatalf("退避期内 loader 被调用 %d 次, want 1", n)
	}
	// 退避结束后才重试
	c.mu.Lock()
	c.items[1].failedAt = time.Now().Add(-2 * time.Hour)
	c.mu.Unlock()
	f.fail.Store(false)
	f.id.Store(5)
	if got := c.Get(ctx, 1); len(got.Rules) != 1 || got.Rules[0].Rule.ID != 5 {
		t.Fatalf("退避结束后应重新加载成功: %+v", got.Rules)
	}
	if n := f.calls.Load(); n != 2 {
		t.Fatalf("loader 调用次数 = %d, want 2", n)
	}
}

// TestCacheStaleSnapshotServedWhileReloading 已有旧快照时,别人正在加载就直接用旧的,
// 不在租户锁上排队(加载卡住也不拖慢跳转)。
func TestCacheStaleSnapshotServedWhileReloading(t *testing.T) {
	f := &flakyLoader{gate: make(chan struct{})}
	f.id.Store(1)
	log, _ := logBuf()
	c := NewCache(f.load, WithLogger(log), WithTTL(10*time.Millisecond))
	ctx := context.Background()
	first := c.Get(ctx, 1)
	time.Sleep(20 * time.Millisecond)
	f.fail.Store(true) // 下一次加载会卡在 gate 上

	done := make(chan *Snapshot, 1)
	go func() { done <- c.Get(ctx, 1) }()
	// 等那次加载真正进入 loader
	deadline := time.Now().Add(2 * time.Second)
	for f.calls.Load() < 2 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	start := time.Now()
	for i := 0; i < 20; i++ {
		if got := c.Get(ctx, 1); got != first {
			t.Fatal("加载在途时应直接返回旧快照")
		}
	}
	if d := time.Since(start); d > 500*time.Millisecond {
		t.Fatalf("加载在途时读旧快照花了 %v,说明在排队等锁", d)
	}
	close(f.gate)
	if got := <-done; got != first {
		t.Fatal("在途加载失败后也应返回旧快照")
	}
	if n := f.calls.Load(); n != 2 {
		t.Fatalf("loader 调用次数 = %d, want 2(其余请求不该再触发加载)", n)
	}
}

// TestCacheLoaderPanicKeepsStaleSnapshot loader panic 与返回 error 同一条路径:沿用旧快照。
func TestCacheLoaderPanicKeepsStaleSnapshot(t *testing.T) {
	var boom atomic.Bool
	log, _ := logBuf()
	c := NewCache(func(context.Context, int64) ([]store.Rule, error) {
		if boom.Load() {
			panic("loader 炸了")
		}
		return []store.Rule{globalRule(1, 10)}, nil
	}, WithLogger(log), WithTTL(10*time.Millisecond))
	ctx := context.Background()
	first := c.Get(ctx, 1)
	time.Sleep(20 * time.Millisecond)
	boom.Store(true)
	if got := c.Get(ctx, 1); got != first {
		t.Fatal("loader panic 时应沿用上一份快照")
	}
}
