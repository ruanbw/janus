package linkcache

// 跳转热路径"短链快照缓存"的测试规格（TDD 红阶段：本包还没有实现文件）。
//
// 每条断言都对应一次可观测的线上事故：
//   - 惊群：热门短链冷启动瞬间，N 个并发跳转各回一次库（含 JOIN 与两次 link_targets）；
//   - 负缓存：刚创建的短码被 ErrNotFound 的缓存挡住，访问者要等 TTL 才打得开；
//   - 失效粒度错：改一条短链顺带清掉整个域名的缓存（跳转全表回源），
//     或者反过来漏清某条（停用/删除的短链继续跳转）；
//   - 过期值复用：TTL 到期后仍把旧对象当有效值返回，改了目标也不生效。
//
// 缓存键必须含 domainID：同一租户的多个域名下允许存在同名短码，
// 只按 code 做键会把 A 域名下的目标错误地送到 B 域名。

import (
	"context"
	"errors"
	"reflect"
	"sync"
	"testing"
	"time"
)

type loadKey struct {
	domainID int64
	code     string
}

// countingLoader 包一层 Loader，按 key 统计回源次数。
//
// 为什么不只看 Stats().Loads：那是被测实现自己报的数，而这里要断言的恰恰是
// "它到底有没有回源"。两者都查，才能区分"缓存住了但计数器写错"和
// "没缓存但计数器看起来对"。
type countingLoader struct {
	mu     sync.Mutex
	perKey map[loadKey]int
	total  int
	fn     func(ctx context.Context, domainID int64, code string) (*Record, error)
}

func (l *countingLoader) load(ctx context.Context, domainID int64, code string) (*Record, error) {
	l.mu.Lock()
	if l.perKey == nil {
		l.perKey = make(map[loadKey]int)
	}
	l.perKey[loadKey{domainID: domainID, code: code}]++
	l.total++
	l.mu.Unlock()
	return l.fn(ctx, domainID, code)
}

func (l *countingLoader) calls(domainID int64, code string) int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.perKey[loadKey{domainID: domainID, code: code}]
}

func (l *countingLoader) totalCalls() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.total
}

// testRecord 构造一条字段齐全的 Record。
//
// 缓存层是纯粹的搬运工：任何字段被抹平或改名都会让跳转链路按错误的状态做决策
// （Status 丢了 → 停用短链仍可访问；Targets 顺序变了 → 轮询目标错位；
// DomainID 丢了 → 访问明细记到别的域名下）。
func testRecord(linkID, tenantID, domainID int64) *Record {
	return &Record{
		LinkID:         linkID,
		TenantID:       tenantID,
		DomainID:       domainID,
		Status:         "enabled",
		Deleted:        false,
		LinkType:       "redirect",
		LandingSource:  "url",
		LandingURL:     "https://landing.example.com/p/1",
		RedirectStatus: 302,
		RulesEnabled:   true,
		Targets:        []string{"https://a.example.com", "https://b.example.com"},
	}
}

// TestCacheGetColdStartLoadsOnce 固化"冷启动一次回源、之后就吃缓存"。
func TestCacheGetColdStartLoadsOnce(t *testing.T) {
	ctx := context.Background()
	want := testRecord(100, 7, 10)
	rec := &countingLoader{fn: func(_ context.Context, domainID int64, _ string) (*Record, error) {
		return testRecord(100, 7, domainID), nil
	}}
	c := New(rec.load)

	first, err := c.Get(ctx, 10, "abc123")
	if err != nil {
		t.Fatalf("冷启动 Get 返回错误: %v", err)
	}
	if first == nil {
		t.Fatal("冷启动 Get 返回 nil, want loader 的 Record")
	}

	t.Run("Given 冷启动 When 首次 Get Then 回源一次并计为一次 miss", func(t *testing.T) {
		if n := rec.calls(10, "abc123"); n != 1 {
			t.Fatalf("loader 调用 %d 次, want 1", n)
		}
		if s := c.Stats(); s.Loads != 1 || s.Misses != 1 || s.Hits != 0 || s.LoadErrors != 0 {
			t.Fatalf("Stats = %+v, want Loads=1 Misses=1 Hits=0 LoadErrors=0", s)
		}
		if n := c.Len(); n != 1 {
			t.Fatalf("Len = %d, want 1", n)
		}
	})

	t.Run("Given 已缓存 When 再次 Get 同 key Then 不再回源且返回同一个对象", func(t *testing.T) {
		second, err := c.Get(ctx, 10, "abc123")
		if err != nil {
			t.Fatalf("命中缓存时 Get 返回错误: %v", err)
		}
		if second != first {
			t.Fatal("命中缓存必须复用同一个 *Record:快照只读,拷贝会让热路径多一次分配")
		}
		if n := rec.totalCalls(); n != 1 {
			t.Fatalf("第二次 Get 之后 loader 总调用 %d 次, want 1(缓存没生效)", n)
		}
		if s := c.Stats(); s.Hits != 1 || s.Misses != 1 || s.Loads != 1 {
			t.Fatalf("Stats = %+v, want Hits=1 Misses=1 Loads=1", s)
		}
	})

	t.Run("Given 冷启动 When 首次 Get Then loader 的 Record 字段原样返回", func(t *testing.T) {
		if !reflect.DeepEqual(first, want) {
			t.Fatalf("Get 返回的 Record 被改写过:\n got %+v\nwant %+v", first, want)
		}
	})
}

// TestCacheGetLoaderErrorReturnedAndNotCached 固化"加载失败不入缓存"。
//
// 若把失败结果缓存住，一次瞬时 DB 抖动会让整条短链在最热的路径上持续 404/500，
// 且和"短链确实被删除"无法区分；调用方也没机会用自己的重试/兜底策略。
func TestCacheGetLoaderErrorReturnedAndNotCached(t *testing.T) {
	ctx := context.Background()
	boom := errors.New("db: connection refused")
	rec := &countingLoader{fn: func(_ context.Context, _ int64, _ string) (*Record, error) {
		return nil, boom
	}}
	c := New(rec.load)

	t.Run("Given loader 返回普通错误 When Get Then 原样返回该错误且不写入缓存", func(t *testing.T) {
		got, err := c.Get(ctx, 10, "abc123")
		if !errors.Is(err, boom) {
			t.Fatalf("Get 返回 err = %v, want %v(必须原样透出,不能被换成 ErrNotFound)", err, boom)
		}
		if got != nil {
			t.Fatalf("失败时返回了非 nil Record: %+v", got)
		}
		if n := c.Len(); n != 0 {
			t.Fatalf("加载失败后 Len = %d, want 0(失败不入缓存)", n)
		}
		if s := c.Stats(); s.LoadErrors != 1 {
			t.Fatalf("LoadErrors = %d, want 1", s.LoadErrors)
		}
	})

	t.Run("Given 上一次加载失败 When 再次 Get 同 key Then 重新回源而不是复用失败结果", func(t *testing.T) {
		if _, err := c.Get(ctx, 10, "abc123"); !errors.Is(err, boom) {
			t.Fatalf("第二次 Get err = %v, want %v", err, boom)
		}
		if n := rec.calls(10, "abc123"); n != 2 {
			t.Fatalf("loader 调用 %d 次, want 2", n)
		}
		if s := c.Stats(); s.LoadErrors != 2 {
			t.Fatalf("LoadErrors = %d, want 2(累计语义)", s.LoadErrors)
		}
	})
}

// TestCacheGetErrNotFoundNotCached 固化"未命中不做负缓存"。
//
// 短码刚创建就必须立刻可用：负缓存会让"我在后台建好短链、刷新页面还是 404"
// 变成一个必须等 TTL 才恢复的谜。ErrNotFound 是"现在没有"，不是"永远没有"。
func TestCacheGetErrNotFoundNotCached(t *testing.T) {
	ctx := context.Background()
	rec := &countingLoader{fn: func(_ context.Context, _ int64, _ string) (*Record, error) {
		return nil, ErrNotFound
	}}
	c := New(rec.load)

	t.Run("Given loader 返回 ErrNotFound When Get Then 原样返回且不缓存", func(t *testing.T) {
		got, err := c.Get(ctx, 10, "missing")
		if !errors.Is(err, ErrNotFound) {
			t.Fatalf("Get 返回 err = %v, want ErrNotFound", err)
		}
		if got != nil {
			t.Fatalf("未命中时返回了非 nil Record: %+v", got)
		}
		if n := c.Len(); n != 0 {
			t.Fatalf("未命中后 Len = %d, want 0(不能负缓存)", n)
		}
	})

	t.Run("Given 上一次未命中 When 再次 Get 同 key Then 重新回源", func(t *testing.T) {
		if _, err := c.Get(ctx, 10, "missing"); !errors.Is(err, ErrNotFound) {
			t.Fatalf("第二次 Get err = %v, want ErrNotFound", err)
		}
		if n := rec.calls(10, "missing"); n != 2 {
			t.Fatalf("loader 调用 %d 次, want 2(负缓存挡住了刚创建的短码)", n)
		}
	})
}

// TestCacheInvalidateCode 固化"单条失效只影响这一条"。
func TestCacheInvalidateCode(t *testing.T) {
	ctx := context.Background()
	// 每次回源返回一个新对象(LinkID 递增)，用来区分"读的是旧值还是新值"
	var seq int64
	rec := &countingLoader{fn: func(_ context.Context, domainID int64, _ string) (*Record, error) {
		seq++
		return testRecord(seq, 7, domainID), nil
	}}
	c := New(rec.load)

	before, err := c.Get(ctx, 10, "abc")
	if err != nil {
		t.Fatalf("预热 abc 失败: %v", err)
	}
	if _, err := c.Get(ctx, 10, "abd"); err != nil {
		t.Fatalf("预热 abd 失败: %v", err)
	}

	t.Run("Given 同域名两条短链都已缓存 When 失效其中一条 Then 该条重新回源", func(t *testing.T) {
		c.InvalidateCode(10, "abc")
		got, err := c.Get(ctx, 10, "abc")
		if err != nil {
			t.Fatalf("失效后 Get 返回错误: %v", err)
		}
		if n := rec.calls(10, "abc"); n != 2 {
			t.Fatalf("abc 回源 %d 次, want 2", n)
		}
		if got == before {
			t.Fatal("失效后仍返回旧对象:改了目标/状态不会生效")
		}
	})

	t.Run("Given 相邻短链 When 失效 abc Then abd 仍吃缓存", func(t *testing.T) {
		if _, err := c.Get(ctx, 10, "abd"); err != nil {
			t.Fatalf("Get abd 返回错误: %v", err)
		}
		if n := rec.calls(10, "abd"); n != 1 {
			t.Fatalf("abd 回源 %d 次, want 1(按 code 失效不该误伤相邻短链)", n)
		}
		if n := c.Len(); n != 2 {
			t.Fatalf("Len = %d, want 2", n)
		}
	})
}

// TestCacheInvalidateDomain 固化"域名级失效清掉该域名全部 code，且不跨域名误伤"。
//
// 缓存键必须含 domainID：不同域名允许同名短码，若只按 code 做键，
// 失效一个域名会连带清掉另一个域名（热路径全回源），更糟的是可能读到别的域名的目标。
func TestCacheInvalidateDomain(t *testing.T) {
	ctx := context.Background()
	rec := &countingLoader{fn: func(_ context.Context, domainID int64, _ string) (*Record, error) {
		return testRecord(100, 7, domainID), nil
	}}
	c := New(rec.load)

	for _, key := range []loadKey{{10, "abc"}, {10, "abd"}, {11, "abc"}} {
		if _, err := c.Get(ctx, key.domainID, key.code); err != nil {
			t.Fatalf("预热 %+v 失败: %v", key, err)
		}
	}
	if n := c.Len(); n != 3 {
		t.Fatalf("预热后 Len = %d, want 3", n)
	}

	c.InvalidateDomain(10)

	t.Run("Given 域名 10 下有两条短链 When InvalidateDomain(10) Then 两条都要重新回源", func(t *testing.T) {
		for _, code := range []string{"abc", "abd"} {
			if _, err := c.Get(ctx, 10, code); err != nil {
				t.Fatalf("Get(10, %s) 返回错误: %v", code, err)
			}
			if n := rec.calls(10, code); n != 2 {
				t.Fatalf("domain=10 code=%s 回源 %d 次, want 2", code, n)
			}
		}
		if n := c.Len(); n != 3 {
			t.Fatalf("重新加载后 Len = %d, want 3", n)
		}
	})

	t.Run("Given 域名 11 下有同名短码 abc When 失效域名 10 Then 域名 11 的 abc 仍吃缓存", func(t *testing.T) {
		if _, err := c.Get(ctx, 11, "abc"); err != nil {
			t.Fatalf("Get(11, abc) 返回错误: %v", err)
		}
		if n := rec.calls(11, "abc"); n != 1 {
			t.Fatalf("domain=11 code=abc 回源 %d 次, want 1(缓存键必须含 domainID)", n)
		}
	})
}

// TestCacheInvalidateTenant 固化"租户级失效跨域名生效"。
//
// 租户维度改配置（比如整租户停用、配额/规则批量变更）时一次清干净；
// 其他租户的缓存不能被顺手清掉，否则一个租户的操作会打到全平台的库。
func TestCacheInvalidateTenant(t *testing.T) {
	ctx := context.Background()
	// 域名 → 租户的映射:10/11 属租户 7,20 属租户 8
	tenantOf := map[int64]int64{10: 7, 11: 7, 20: 8}
	rec := &countingLoader{fn: func(_ context.Context, domainID int64, _ string) (*Record, error) {
		return testRecord(100, tenantOf[domainID], domainID), nil
	}}
	c := New(rec.load)

	for _, key := range []loadKey{{10, "abc"}, {10, "abd"}, {11, "abc"}, {20, "abc"}} {
		if _, err := c.Get(ctx, key.domainID, key.code); err != nil {
			t.Fatalf("预热 %+v 失败: %v", key, err)
		}
	}
	if n := c.Len(); n != 4 {
		t.Fatalf("预热后 Len = %d, want 4", n)
	}

	c.InvalidateTenant(7)

	t.Run("Given 租户 7 的条目分布在两个域名 When InvalidateTenant(7) Then Len 只剩其他租户的条目", func(t *testing.T) {
		if n := c.Len(); n != 1 {
			t.Fatalf("失效后 Len = %d, want 1(只剩 domain=20 code=abc)", n)
		}
	})

	t.Run("Given 租户 7 的条目已失效 When 逐个 Get Then 跨域名的条目都重新回源", func(t *testing.T) {
		for _, key := range []loadKey{{10, "abc"}, {10, "abd"}, {11, "abc"}} {
			if _, err := c.Get(ctx, key.domainID, key.code); err != nil {
				t.Fatalf("Get(%+v) 返回错误: %v", key, err)
			}
			if n := rec.calls(key.domainID, key.code); n != 2 {
				t.Fatalf("%+v 回源 %d 次, want 2", key, n)
			}
		}
	})

	t.Run("Given 租户 8 的条目 When 失效租户 7 Then 租户 8 仍吃缓存", func(t *testing.T) {
		if _, err := c.Get(ctx, 20, "abc"); err != nil {
			t.Fatalf("Get(20, abc) 返回错误: %v", err)
		}
		if n := rec.calls(20, "abc"); n != 1 {
			t.Fatalf("租户 8 的回源次数 = %d, want 1(失效不能跨租户误伤)", n)
		}
	})
}

// TestCacheTTLExpiryReloads 固化"TTL 到期必须重新回源，且返回新对象"。
//
// 用假时钟而不是 sleep：跳转链路的正确性不该由墙钟决定，测试也不该因此变慢/变脆。
// 指针不同这条断言是关键 —— 只比较内容的话，"把过期对象继续当有效值返回"
// 与"重新加载到相同内容"无法区分，而前者意味着改了目标要等重启才生效。
func TestCacheTTLExpiryReloads(t *testing.T) {
	ctx := context.Background()
	var loads int
	rec := &countingLoader{fn: func(_ context.Context, domainID int64, _ string) (*Record, error) {
		loads++
		// 每次回源都是新对象，且带一个新的 LinkID 便于辨识
		return testRecord(int64(loads), 7, domainID), nil
	}}
	now := time.Date(2026, 10, 20, 10, 0, 0, 0, time.UTC)
	c := New(rec.load)
	c.SetClock(func() time.Time { return now })
	c.SetTTL(10 * time.Second)

	first, err := c.Get(ctx, 10, "abc123")
	if err != nil {
		t.Fatalf("冷启动 Get 返回错误: %v", err)
	}
	if got := rec.calls(10, "abc123"); got != 1 {
		t.Fatalf("冷启动回源 %d 次, want 1", got)
	}

	t.Run("Given 距加载已过 5s(TTL=10s) When Get Then 复用缓存对象", func(t *testing.T) {
		now = now.Add(5 * time.Second)
		got, err := c.Get(ctx, 10, "abc123")
		if err != nil {
			t.Fatalf("未过期时 Get 返回错误: %v", err)
		}
		if got != first {
			t.Fatal("未过期却换了对象,TTL 判定把未过期当成过期了")
		}
		if n := rec.calls(10, "abc123"); n != 1 {
			t.Fatalf("未过期时回源 %d 次, want 1", n)
		}
	})

	t.Run("Given 距加载已过 15s(超过 TTL) When Get Then 重新回源并返回新对象", func(t *testing.T) {
		now = now.Add(10 * time.Second)
		got, err := c.Get(ctx, 10, "abc123")
		if err != nil {
			t.Fatalf("过期后 Get 返回错误: %v", err)
		}
		if n := rec.calls(10, "abc123"); n != 2 {
			t.Fatalf("过期后回源 %d 次, want 2", n)
		}
		if got == first {
			t.Fatal("过期后仍返回旧对象:TTL 形同虚设")
		}
		if got.LinkID == first.LinkID {
			t.Fatalf("过期后 LinkID 仍是 %d,没有读到新数据", got.LinkID)
		}
	})
}

// TestCacheLenAndStatsAreCumulative 固化"Len 跟随条目数、Stats 是累计值"。
//
// Stats 在失效后清零会让监控把"缓存大量失效"看成"缓存刚开始工作"，
// 事故复盘时读到的掉底曲线是假的。
func TestCacheLenAndStatsAreCumulative(t *testing.T) {
	ctx := context.Background()
	rec := &countingLoader{fn: func(_ context.Context, domainID int64, _ string) (*Record, error) {
		return testRecord(100, 7, domainID), nil
	}}
	c := New(rec.load)

	if _, err := c.Get(ctx, 10, "abc123"); err != nil { // miss
		t.Fatalf("Get 返回错误: %v", err)
	}
	if _, err := c.Get(ctx, 10, "abc123"); err != nil { // hit
		t.Fatalf("Get 返回错误: %v", err)
	}
	if n := c.Len(); n != 1 {
		t.Fatalf("Len = %d, want 1", n)
	}
	before := c.Stats()

	c.InvalidateCode(10, "abc123")

	t.Run("Given 失效后 When 读 Len Then 条目数下降", func(t *testing.T) {
		if n := c.Len(); n != 0 {
			t.Fatalf("失效后 Len = %d, want 0", n)
		}
	})

	t.Run("Given 失效后 When 读 Stats Then 计数不清零(累计语义)", func(t *testing.T) {
		after := c.Stats()
		if after != before {
			t.Fatalf("失效改写了累计统计: before=%+v after=%+v", before, after)
		}
		if after.Hits != 1 || after.Misses != 1 || after.Loads != 1 || after.LoadErrors != 0 {
			t.Fatalf("Stats = %+v, want Hits=1 Misses=1 Loads=1 LoadErrors=0", after)
		}
	})

	t.Run("Given 失效后重新加载 When 读 Stats Then 计数继续累加", func(t *testing.T) {
		if _, err := c.Get(ctx, 10, "abc123"); err != nil {
			t.Fatalf("Get 返回错误: %v", err)
		}
		after := c.Stats()
		if after.Hits != 1 || after.Misses != 2 || after.Loads != 2 {
			t.Fatalf("Stats = %+v, want Hits=1 Misses=2 Loads=2", after)
		}
		if n := c.Len(); n != 1 {
			t.Fatalf("Len = %d, want 1", n)
		}
	})
}
