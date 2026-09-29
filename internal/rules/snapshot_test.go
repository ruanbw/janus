package rules

// 快照缓存的单元测试:用注入的 Loader,不依赖数据库。

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"cloak/internal/store"
)

func logBuf() (*slog.Logger, *bytes.Buffer) {
	buf := &bytes.Buffer{}
	return slog.New(slog.NewTextHandler(buf, &slog.HandlerOptions{Level: slog.LevelDebug})), buf
}

// scopedRule 造一条 scope=links 的规则。
func scopedRule(id int64, priority int, linkIDs ...int64) store.Rule {
	return store.Rule{
		ID: id, Name: fmt.Sprintf("r%d", id), Enabled: true,
		Scope: store.RuleScopeLinks, Logic: store.RuleLogicAll,
		Action: store.RuleActionNotfound, Priority: priority, LinkIDs: linkIDs,
	}
}

// globalRule 造一条 scope=global 的规则。
func globalRule(id int64, priority int) store.Rule {
	r := scopedRule(id, priority)
	r.Scope = store.RuleScopeGlobal
	return r
}

func TestScopeApplicability(t *testing.T) {
	rules := []store.Rule{
		globalRule(1, 10),      // 全局:对所有短链生效
		scopedRule(2, 20, 100), // 限定短链 100
		scopedRule(3, 30, 100, 200),
		scopedRule(4, 40), // 零关联的 scoped 规则:永不命中(spec D2)
	}
	snap := NewSnapshot(rules, discardLog)
	if len(snap.Rules) != 4 {
		t.Fatalf("快照规则数 = %d, want 4", len(snap.Rules))
	}
	// 逐条看适用性:全局对所有短链适用,scoped 只对关联短链适用
	want := map[int64][]int64{ // 规则 → 适用的短链
		1: {0, 100, 200, 300},
		2: {100},
		3: {100, 200},
		4: {}, // 零关联的 scoped 规则:对任何短链都不适用(spec D2)
	}
	for i := range snap.Rules {
		r := &snap.Rules[i]
		for _, link := range []int64{0, 100, 200, 300} {
			got := r.applies(link)
			exp := slices.Contains(want[r.Rule.ID], link)
			if got != exp {
				t.Fatalf("规则 %d 对短链 %d 适用 = %v, want %v", r.Rule.ID, link, got, exp)
			}
		}
	}
	// 只留零关联规则时,任何短链都不命中
	onlyOrphan := NewSnapshot([]store.Rule{scopedRule(4, 10)}, discardLog)
	for _, link := range []int64{0, 100, 200, 300} {
		if _, ok := onlyOrphan.Evaluate(Fact{}, link); ok {
			t.Fatalf("零关联规则对短链 %d 命中了(不应兜底成全局)", link)
		}
	}
}

// 求值顺序 = priority 升序,id 升序;首命中即裁决,不做多规则叠加。
func TestEvaluateHonorsPriority(t *testing.T) {
	mk := func(id int64, priority int, ua string) store.Rule {
		r := globalRule(id, priority)
		r.Action = store.RuleActionRedirect
		r.Destination = fmt.Sprintf("https://r%d.test/", id)
		r.Conditions = store.RuleConditions{{Field: FieldUA, Operator: OpContains, Values: []string{ua}}}
		return r
	}
	// 故意打乱顺序传入:排序必须由 NewSnapshot 负责,不能依赖数据库 ORDER BY
	snap := NewSnapshot([]store.Rule{
		mk(3, 30, "bot"), mk(1, 10, "bot"), mk(2, 20, "bot"),
	}, discardLog)
	dec, ok := snap.Evaluate(Fact{UA: "Googlebot/2.1"}, 1)
	if !ok {
		t.Fatal("应命中")
	}
	if dec.RuleID != 1 || dec.Action != store.RuleActionRedirect ||
		dec.Destination != "https://r1.test/" || dec.Priority != 10 {
		t.Fatalf("裁决 = %+v, want 规则 1(priority 10)", dec)
	}
	// 同优先级按 id 升序
	same := NewSnapshot([]store.Rule{mk(9, 5, "bot"), mk(7, 5, "bot")}, discardLog)
	if dec, _ := same.Evaluate(Fact{UA: "bot"}, 1); dec.RuleID != 7 {
		t.Fatalf("同优先级应取 id 小的: %+v", dec)
	}
	// 不命中任何一条时返回零值
	if dec, ok := snap.Evaluate(Fact{UA: "Chrome/120"}, 1); ok || dec != (Decision{}) {
		t.Fatalf("未命中应返回零值: %+v %v", dec, ok)
	}
}

func TestCacheGetAndInvalidate(t *testing.T) {
	var calls atomic.Int64
	loader := func(_ context.Context, tenantID int64) ([]store.Rule, error) {
		calls.Add(1)
		if tenantID == 9 {
			return nil, errors.New("boom")
		}
		return []store.Rule{globalRule(tenantID*10, 10)}, nil
	}
	log, _ := logBuf()
	c := NewCache(loader, WithLogger(log))
	ctx := context.Background()

	s1 := c.Get(ctx, 1)
	if s1 == nil || len(s1.Rules) != 1 || s1.Rules[0].Rule.ID != 10 {
		t.Fatalf("首次加载 = %+v", s1)
	}
	if s2 := c.Get(ctx, 1); s2 != s1 {
		t.Fatal("第二次应命中同一份快照")
	}
	if calls.Load() != 1 {
		t.Fatalf("加载次数 = %d, want 1", calls.Load())
	}
	// 其他租户各自一份
	c.Get(ctx, 2)
	if c.Len() != 2 {
		t.Fatalf("缓存租户数 = %d, want 2", c.Len())
	}
	// 失效后重新加载
	c.Invalidate(1)
	s3 := c.Get(ctx, 1)
	if s3 == s1 {
		t.Fatal("Invalidate 后应重新加载")
	}
	if calls.Load() != 3 {
		t.Fatalf("加载次数 = %d, want 3", calls.Load())
	}
	// Invalidate 不存在的租户不报错
	c.Invalidate(12345)
	c.InvalidateAll()
	if c.Len() != 0 {
		t.Fatalf("清空后缓存租户数 = %d", c.Len())
	}
}

// 加载失败 fail-open:按"没有规则"放行,且不把失败写进缓存(下次还会重试)。
func TestCacheLoaderFailureFailsOpen(t *testing.T) {
	log, buf := logBuf()
	c := NewCache(func(_ context.Context, _ int64) ([]store.Rule, error) {
		return nil, errors.New("db down")
	}, WithLogger(log))
	dec, ok := c.Get(context.Background(), 1).Evaluate(Fact{UA: "bot"}, 1)
	if ok || dec != (Decision{}) {
		t.Fatalf("加载失败时不该命中: %+v", dec)
	}
	if c.Len() != 0 {
		t.Fatal("加载失败不应写入缓存")
	}
	if !bytes.Contains(buf.Bytes(), []byte("fail-open")) {
		t.Fatalf("缺少 fail-open 日志:\n%s", buf.String())
	}
	// TTL 兜底:不调 Invalidate 也会在 TTL 后重新加载
	c2 := NewCache(func(_ context.Context, tenantID int64) ([]store.Rule, error) {
		return []store.Rule{globalRule(tenantID, 10)}, nil
	}, WithLogger(log), WithTTL(20*time.Millisecond))
	first := c2.Get(context.Background(), 1)
	time.Sleep(30 * time.Millisecond)
	if c2.Get(context.Background(), 1) == first {
		t.Fatal("TTL 到期后应重新加载")
	}
	// 无 Loader 的缓存等价于"没有规则"
	empty := NewCache(nil, WithLogger(log))
	if _, ok := empty.Get(context.Background(), 1).Evaluate(Fact{UA: "bot"}, 1); ok {
		t.Fatal("无 Loader 时不该命中")
	}
	if len(empty.Get(context.Background(), 1).Rules) != 0 {
		t.Fatal("无 Loader 时应返回空快照")
	}
}

// 并发 Get 只触发一次加载(惊群保护),且求值与整体替换并发安全。
func TestCacheConcurrentGet(t *testing.T) {
	var calls atomic.Int64
	loader := func(_ context.Context, _ int64) ([]store.Rule, error) {
		calls.Add(1)
		time.Sleep(5 * time.Millisecond)
		out := make([]store.Rule, 0, 20)
		for i := 1; i <= 20; i++ {
			out = append(out, globalRule(int64(i), i))
		}
		return out, nil
	}
	c := NewCache(loader, WithLogger(slog.Default()), WithTTL(0))
	ctx := context.Background()
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 20; j++ {
				snap := c.Get(ctx, 1)
				if len(snap.Rules) != 20 {
					t.Errorf("规则数 = %d", len(snap.Rules))
					return
				}
				// 与"规则变更"并发:Invalidate + 重新加载
				if j%7 == 0 {
					c.Invalidate(1)
				}
				if _, ok := snap.Evaluate(Fact{UA: "bot"}, 1); !ok {
					t.Errorf("应命中")
					return
				}
			}
		}()
	}
	wg.Wait()
	if calls.Load() == 0 {
		t.Fatal("从未加载")
	}
}

func TestCacheNilSafe(t *testing.T) {
	var c *Cache
	if c.Len() != 0 {
		t.Fatal("nil Cache.Len")
	}
	c.Invalidate(1)
	c.InvalidateAll()
	if c.Get(context.Background(), 1) == nil {
		t.Fatal("nil Cache.Get 应返回空快照")
	}
}

// ---------- 基准 ----------

// benchConds 一条"会命中"规则的 5 条条件,覆盖五种典型判定:
// contains / in(枚举) / in(CIDR) / regex / not_contains。
func benchConds() store.RuleConditions {
	return store.RuleConditions{
		{Field: FieldUA, Operator: OpContains, Values: []string{"bot"}},
		{Field: FieldDevType, Operator: OpIn, Values: []string{"bot", "mobile", "desktop"}},
		{Field: FieldIP, Operator: OpIn, Values: []string{"10.0.0.0/8", "203.0.113.0/24", "192.168.0.0/16"}},
		{Field: FieldLang, Operator: OpRegex, Values: []string{"^(zh|en|pt|de|ja)-"}},
		{Field: FieldPath, Operator: OpNotContains, Values: []string{"/static/", "/assets/"}},
	}
}

// benchMissConds 一条"必然不命中"的条件(country 恒空,空值恒不命中):
// 用来模拟"绝大多数规则不命中"时遍历成本有多低。
func benchMissConds() store.RuleConditions {
	return store.RuleConditions{{Field: FieldCountry, Operator: OpEq, Values: []string{"ZZ"}}}
}

// benchRules 造 n 条规则:前 n-1 条不命中,最后一条命中。
// 测的是最坏情况——"要遍历到最后一条才裁决",也就是 priority 排得靠后、
// 且前面的规则都没拦住时的一次求值成本。
func benchRules(n int) []store.Rule {
	out := make([]store.Rule, 0, n)
	for i := 1; i <= n; i++ {
		r := globalRule(int64(i), i*10)
		r.Action = store.RuleActionThrottle
		r.Conditions = benchMissConds()
		if i == n {
			r.Conditions = benchConds()
		}
		out = append(out, r)
	}
	return out
}

// benchMissRules 造 n 条全部不命中的规则(要遍历完全部规则才返回未命中)。
func benchMissRules(n int) []store.Rule {
	out := make([]store.Rule, 0, n)
	for i := 1; i <= n; i++ {
		r := globalRule(int64(i), i*10)
		r.Conditions = benchMissConds()
		out = append(out, r)
	}
	return out
}

func benchFact(b testing.TB) Fact {
	r := mustRequest(b, "https://shop.example.com/promo?utm_source=wechat&ref=x",
		"https://www.google.com/search?q=cloak",
		"Mozilla/5.0 (compatible; Googlebot/2.1; +http://www.google.com/bot.html)")
	r.Header.Set("Accept-Language", "zh-CN,zh;q=0.9")
	r.RemoteAddr = "203.0.113.5:44321"
	return FromRequest(r)
}

// BenchmarkEvaluate10Rules 10 条规则 × 一次求值(spec D7 的热路径量级)。
func BenchmarkEvaluate10Rules(b *testing.B) {
	snap := NewSnapshot(benchRules(10), discardLog)
	fact := benchFact(b)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, ok := snap.Evaluate(fact, 1); !ok {
			b.Fatal("期望命中")
		}
	}
}

// BenchmarkEvaluate200Rules 单租户规则数上限(200)× 一次求值:遍历到最后一条才裁决。
func BenchmarkEvaluate200Rules(b *testing.B) {
	snap := NewSnapshot(benchRules(200), discardLog)
	fact := benchFact(b)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, ok := snap.Evaluate(fact, 1); !ok {
			b.Fatal("期望命中")
		}
	}
}

// BenchmarkEvaluate200RulesMiss 全部不命中:要走完 200 条规则才返回。
func BenchmarkEvaluate200RulesMiss(b *testing.B) {
	snap := NewSnapshot(benchMissRules(200), discardLog)
	fact := benchFact(b)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, ok := snap.Evaluate(fact, 1); ok {
			b.Fatal("期望不命中")
		}
	}
}

// BenchmarkCacheGet 缓存命中的取快照成本(热路径上的另一半)。
func BenchmarkCacheGet(b *testing.B) {
	c := NewCache(func(_ context.Context, _ int64) ([]store.Rule, error) {
		return benchRules(200), nil
	}, WithLogger(discardLog), WithTTL(0))
	ctx := context.Background()
	c.Get(ctx, 1)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		c.Get(ctx, 1)
	}
}

// BenchmarkNewSnapshot 加载期的预编译成本(每租户每次失效只做一次)。
func BenchmarkNewSnapshot(b *testing.B) {
	rules := benchRules(200)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		NewSnapshot(rules, discardLog)
	}
}

// BenchmarkFromRequest 请求画像提取(纯字符串判定,无 IO)。
func BenchmarkFromRequest(b *testing.B) {
	r := mustRequest(b, "https://shop.example.com/promo?utm_source=wechat&ref=x",
		"https://www.google.com/search?q=cloak",
		"Mozilla/5.0 (iPhone; CPU iPhone OS 17_0 like Mac OS X) AppleWebKit/605.1.15 "+
			"(KHTML, like Gecko) Version/17.0 Mobile/15E148 Safari/604.1")
	r.RemoteAddr = "203.0.113.5:44321"
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		FromRequest(r)
	}
}
