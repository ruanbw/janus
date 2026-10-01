package rules

// 快照缓存的单元测试:用注入的 Loader,不依赖数据库。

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/netip"
	"slices"
	"strings"
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
		r.Conditions = store.Conditions(store.RuleCondition{Field: FieldUA, Operator: OpContains, Values: []string{ua}})
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
//
// 注意:与 Invalidate 并发时,正在加载的那次调用**会**拿到空快照 ——
// 它手里的数据是失效之前提交的,写回去就是"刚保存的规则整个 TTL 不生效"。
// 丢弃并按未命中继续(fail-open)是刻意的,见 Cache.load 的注释。
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
				// 与"规则变更"并发:Invalidate + 重新加载
				if j%7 == 0 {
					c.Invalidate(1)
				}
				// 拿到手的快照要么是完整的 20 条,要么是空(撞上了失效),
				// 不该出现"半份"。
				switch len(snap.Rules) {
				case 20:
					if _, ok := snap.Evaluate(Fact{UA: "bot"}, 1); !ok {
						t.Errorf("完整快照应命中")
						return
					}
				case 0:
				default:
					t.Errorf("规则数 = %d,只该是 20 或 0", len(snap.Rules))
					return
				}
			}
		}()
	}
	wg.Wait()
	if calls.Load() == 0 {
		t.Fatal("从未加载")
	}
	// 风暴过去之后,缓存必须收敛到一份完整快照(没有被丢弃的结果永久卡住)
	final := c.Get(ctx, 1)
	if len(final.Rules) != 20 {
		t.Fatalf("风暴后缓存未收敛:规则数 = %d", len(final.Rules))
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
	return store.Conditions(
		store.RuleCondition{Field: FieldUA, Operator: OpContains, Values: []string{"bot"}},
		store.RuleCondition{Field: FieldDevType, Operator: OpIn, Values: []string{"bot", "mobile", "desktop"}},
		store.RuleCondition{Field: FieldIP, Operator: OpIn, Values: []string{"10.0.0.0/8", "203.0.113.0/24", "192.168.0.0/16"}},
		store.RuleCondition{Field: FieldLang, Operator: OpRegex, Values: []string{"^(zh|en|pt|de|ja)-"}},
		store.RuleCondition{Field: FieldPath, Operator: OpNotContains, Values: []string{"/static/", "/assets/"}})
}

// benchMissConds 一条"必然不命中"的条件(country 恒空,空值恒不命中):
// 用来模拟"绝大多数规则不命中"时遍历成本有多低。
func benchMissConds() store.RuleConditions {
	return store.Conditions(store.RuleCondition{Field: FieldCountry, Operator: OpEq, Values: []string{"ZZ"}})
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
		if _, ok := snap.Evaluate(&fact, 1); !ok {
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
		if _, ok := snap.Evaluate(&fact, 1); !ok {
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
		if _, ok := snap.Evaluate(&fact, 1); ok {
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

// simFact 造一个可预期的访客画像:中国 IP、爬虫设备、/promo 路径。
func simFact() Fact {
	return Fact{
		IP: "203.0.113.7", IPAttr: "public", Country: "CN", Lang: "zh-cn",
		Ref: "news.example.com", UTM: "wechat", DevType: "bot",
		Path: "/promo", Domain: "shop.example.com",
	}
}

// simCtx 造一个可求值的画像指针(Simulate/Evaluate 都吃 VisitorContext)。
func simCtx() *Fact {
	f := simFact()
	return &f
}

// simRules 一条"美国访客 404"与一条"/promo 放行"的全局规则。
// 优先级 10 的规则在前,首条命中即裁决(First-Match-Wins)。
func simRules() []store.Rule {
	return []store.Rule{
		{
			ID: 1, Name: "美国访客拦截", Priority: 10, Enabled: true,
			Scope: store.RuleScopeGlobal, Logic: store.RuleLogicAll,
			Action: store.RuleActionNotfound,
			Conditions: store.Conditions(
				store.RuleCondition{Field: FieldCountry, Operator: OpIn, Values: []string{"US", "CA"}}),
		},
		{
			ID: 2, Name: "活动页放行", Priority: 20, Enabled: true,
			Scope: store.RuleScopeGlobal, Logic: store.RuleLogicAll,
			Action: store.RuleActionPass,
			Conditions: store.Conditions(
				store.RuleCondition{Field: FieldPath, Operator: OpContains, Values: []string{"/promo"}}),
		},
	}
}

// TestSnapshotSimulateTracesConditions 逐条件记录"实际值 / 期望值 / 是否成立",
// 并按真实求值顺序给出首条命中。
func TestSnapshotSimulateTracesConditions(t *testing.T) {
	lg, _ := logBuf()
	snap := NewSnapshot(simRules(), lg)

	res := snap.Simulate(simCtx(), 0, nil, nil)

	if res.Error != "" {
		t.Fatalf("不该有求值错误, got %q", res.Error)
	}
	if !res.Matched || res.Verdict.RuleID != 2 {
		t.Fatalf("赢家 = %+v matched=%v, want rule 2", res.Verdict, res.Matched)
	}
	if len(res.Steps) != 2 {
		t.Fatalf("steps = %d 条, want 2(每条规则一步)", len(res.Steps))
	}

	first := res.Steps[0]
	if first.RuleID != 1 || first.Status != StepStatusSkip || first.Reason == "" {
		t.Fatalf("step1 = %+v, want rule1/skip + 说明", first)
	}
	if len(first.Conditions) != 1 {
		t.Fatalf("step1 条件明细 = %d 条, want 1", len(first.Conditions))
	}
	c := first.Conditions[0]
	if c.Field != FieldCountry || c.Operator != OpIn {
		t.Fatalf("条件 = %+v, want country in", c)
	}
	if c.Actual != "CN" || !c.Available {
		t.Fatalf("条件实际值 = %q available=%v, want CN/true", c.Actual, c.Available)
	}
	if c.Matched {
		t.Fatal("CN 不在 [US,CA] 内,条件不应成立")
	}
	// 期望值回显租户写下的原始字面量(不是求值时小写化的内部形态)。
	if len(c.Expected) != 2 || c.Expected[0] != "US" || c.Expected[1] != "CA" {
		t.Fatalf("expected = %v, want [US CA]", c.Expected)
	}
	if c.Description == "" {
		t.Fatal("每个条件都要有一句人能读的解释")
	}

	second := res.Steps[1]
	if second.RuleID != 2 || second.Status != StepStatusHit || !second.Conditions[0].Matched {
		t.Fatalf("step2 = %+v, want rule2/hit", second)
	}
}

// TestSnapshotSimulateEmptyFieldNeverMatches 取不到的字段恒不命中,
// 仿真必须把它标成 unavailable 而不是"条件不成立"。
func TestSnapshotSimulateEmptyFieldNeverMatches(t *testing.T) {
	lg, _ := logBuf()
	snap := NewSnapshot([]store.Rule{{
		ID: 7, Name: "美国访客拦截", Priority: 10, Enabled: true,
		Scope: store.RuleScopeGlobal, Logic: store.RuleLogicAll,
		Action: store.RuleActionNotfound,
		Conditions: store.Conditions(
			store.RuleCondition{Field: FieldCountry, Operator: OpNotIn, Values: []string{"CN"}}),
	}}, lg)

	// 没有 GeoIP 值:country 恒为空 ⇒ not_in 也恒不成立(关键不变式)。
	res := snap.Simulate(&Fact{IP: "203.0.113.7"}, 0, nil, nil)
	if res.Matched {
		t.Fatalf("country 取不到时不应命中, got %+v", res.Verdict)
	}
	c := res.Steps[0].Conditions[0]
	if c.Available || c.Matched || c.Actual != "" {
		t.Fatalf("条件 = %+v, want unavailable/未成立/空值", c)
	}
	if c.Description == "" {
		t.Fatal("空字段也要有解释")
	}
}

// TestSnapshotSimulateDisabledDraft 草稿规则没启用时,链路里应显示 disabled
// 且不参与裁决(否则租户会以为停用的规则还在拦人)。
func TestSnapshotSimulateDisabledDraft(t *testing.T) {
	lg, _ := logBuf()
	snap := NewSnapshot(simRules(), lg)

	draft, ok := compileRule(store.Rule{
		ID: 3, Name: "未启用的草稿", Priority: 5, Enabled: false,
		Scope: store.RuleScopeGlobal, Logic: store.RuleLogicAll,
		Action: store.RuleActionNotfound,
		Conditions: store.Conditions(
			store.RuleCondition{Field: FieldPath, Operator: OpEq, Values: []string{"/promo"}}),
	}, lg)
	if !ok {
		t.Fatal("草稿编译失败")
	}

	res := snap.Simulate(simCtx(), 0, nil, &draft)
	if !res.Matched || res.Verdict.RuleID != 2 {
		t.Fatalf("停用草稿不该改变裁决, got %+v", res.Verdict)
	}
	if len(res.Steps) != 3 {
		t.Fatalf("steps = %d, want 3(草稿也要占一步)", len(res.Steps))
	}
	if res.Steps[0].Status != StepStatusDisabled || !res.Steps[0].Draft {
		t.Fatalf("草稿步骤 = %+v, want disabled 且标记 draft", res.Steps[0])
	}
}

// TestSnapshotSimulateAgreesWithEvaluate 核心不变式:同一份快照 + 同一份画像,
// Simulate 的裁决必须与 Evaluate 完全一致(赢家、动作、命中与否)。
func TestSnapshotSimulateAgreesWithEvaluate(t *testing.T) {
	lg, _ := logBuf()

	scoped := func(id int64, priority int, linkIDs ...int64) store.Rule {
		return store.Rule{
			ID: id, Name: fmt.Sprintf("scoped-%d", id), Priority: priority, Enabled: true,
			Scope: store.RuleScopeLinks, Logic: store.RuleLogicAll,
			Action: store.RuleActionThrottle,
			Conditions: store.Conditions(
				store.RuleCondition{Field: FieldDomain, Operator: OpEq, Values: []string{"shop.example.com"}}),
			LinkIDs: linkIDs,
		}
	}

	sets := map[string][]store.Rule{
		"全局两条":    simRules(),
		"零关联不命中":  append(simRules(), scoped(3, 5), scoped(4, 1)),
		"同优先级按id": {withPriority(simRules()[0], 10), withPriority(simRules()[1], 10)},
		"逻辑any": {{
			ID: 1, Name: "任一成立即命中", Priority: 10, Enabled: true,
			Scope: store.RuleScopeGlobal, Logic: store.RuleLogicAny,
			Action: store.RuleActionRedirect, Destination: "https://blocked.example.com",
			Conditions: store.Conditions(
				store.RuleCondition{Field: FieldCountry, Operator: OpIn, Values: []string{"US"}},
				store.RuleCondition{Field: FieldDevType, Operator: OpEq, Values: []string{"bot"}}),
		}},
		"无���件恒命中": {{
			ID: 1, Name: "无条件", Priority: 10, Enabled: true,
			Scope: store.RuleScopeGlobal, Logic: store.RuleLogicAll,
			Action: store.RuleActionNotfound,
		}},
		"只有对某链生效": {scoped(1, 10, 7)},
	}

	facts := map[string]Fact{
		"爬虫CN": simFact(),
		"访客US": {IP: "198.51.100.9", Country: "US", DevType: "desktop", Path: "/x", Domain: "shop.example.com"},
		"无画像":  {},
		"纯IP":  {IP: "203.0.113.7"},
	}
	linkIDs := []int64{0, 7, 8}

	for setName, rs := range sets {
		snap := NewSnapshot(rs, lg)
		for factName, fact := range facts {
			for _, linkID := range linkIDs {
				f := fact
				dec, matched := snap.Evaluate(&f, linkID)
				res := snap.Simulate(&f, linkID, nil, nil)
				if res.Error != "" {
					t.Fatalf("[%s/%s/link=%d] 仿真报错: %s", setName, factName, linkID, res.Error)
				}
				if res.Matched != matched || res.Verdict != dec {
					t.Fatalf("[%s/%s/link=%d] 仿真 %+v(%v) != 线上 %+v(%v)",
						setName, factName, linkID, res.Verdict, res.Matched, dec, matched)
				}
			}
		}
	}
}

// withPriority 复制一条规则并改优先级(测试构造用)。
func withPriority(r store.Rule, priority int) store.Rule {
	r.Priority = priority
	return r
}

// TestSnapshotSimulateOnlyRule 只看一条规则时,链路上就只有它,
// 它的命中与否直接就是结论(诊断"某条规则单独生效吗")。
func TestSnapshotSimulateOnlyRule(t *testing.T) {
	lg, _ := logBuf()
	snap := NewSnapshot(simRules(), lg)

	only := int64(1)
	res := snap.Simulate(simCtx(), 0, &only, nil)
	if len(res.Steps) != 1 || res.Steps[0].RuleID != 1 {
		t.Fatalf("steps = %+v, want 只剩 rule 1", res.Steps)
	}
	if res.Matched {
		t.Fatalf("CN 访客不该命中美国规则, got %+v", res.Verdict)
	}
	// 与 Evaluate 单独看这条规则的结论一致(线上不存在 only,但语义相同)。
	onlySnap := NewSnapshot([]store.Rule{simRules()[0]}, lg)
	dec, matched := onlySnap.Evaluate(simCtx(), 0)
	if res.Matched != matched || res.Verdict != dec {
		t.Fatalf("onlyRuleId 仿真 %+v != 单独求值 %+v", res.Verdict, dec)
	}
}

// TestSnapshotSimulateDraftReplacesRule 草稿与同 id 的存量规则二选一,
// 且仿真不得改写快照(诊断请求绝不能影响线上跳转)。
func TestSnapshotSimulateDraftReplacesRule(t *testing.T) {
	lg, _ := logBuf()
	snap := NewSnapshot(simRules(), lg)
	before := len(snap.Rules)

	// 草稿把规则 1 改成"中国访客 404":改了之后赢家应当是规则 1。
	draft, ok := compileRule(store.Rule{
		ID: 1, Name: "中国访客拦截", Priority: 10, Enabled: true,
		Scope: store.RuleScopeGlobal, Logic: store.RuleLogicAll,
		Action: store.RuleActionNotfound,
		Conditions: store.Conditions(
			store.RuleCondition{Field: FieldCountry, Operator: OpIn, Values: []string{"CN"}}),
	}, lg)
	if !ok {
		t.Fatal("草稿编译失败")
	}

	res := snap.Simulate(simCtx(), 0, nil, &draft)
	if !res.Matched || res.Verdict.RuleID != 1 || res.Verdict.Name != "中国访客拦截" {
		t.Fatalf("草稿裁决 = %+v, want 草稿版 rule 1", res.Verdict)
	}
	if len(res.Steps) != 2 {
		t.Fatalf("steps = %d, want 2(草稿顶替存量,不新增一条)", len(res.Steps))
	}
	if !res.Steps[0].Draft {
		t.Fatalf("step1 = %+v, want 标记为草稿", res.Steps[0])
	}
	// 草稿不写回快照:线上 Evaluate 的结论必须还是原来的规则 2。
	if len(snap.Rules) != before {
		t.Fatalf("仿真改写了快照: %d -> %d 条", before, len(snap.Rules))
	}
	dec, matched := snap.Evaluate(simCtx(), 0)
	if !matched || dec.RuleID != 2 || dec.Name != "活动页放行" {
		t.Fatalf("仿真污染了快照,线上现在是 %+v(matched=%v)", dec, matched)
	}
}

// TestSnapshotSimulateDraftNewRule 草稿是新规则(存量没有同 id)时,
// 按优先级插进链路;id 为 0 的未保存草稿排在同优先级末尾。
func TestSnapshotSimulateDraftNewRule(t *testing.T) {
	lg, _ := logBuf()
	snap := NewSnapshot(simRules(), lg)

	draft, ok := compileRule(store.Rule{
		ID: 0, Name: "新草稿", Priority: 20, Enabled: true,
		Scope: store.RuleScopeGlobal, Logic: store.RuleLogicAll,
		Action: store.RuleActionThrottle,
		Conditions: store.Conditions(
			store.RuleCondition{Field: FieldDevType, Operator: OpEq, Values: []string{"bot"}}),
	}, lg)
	if !ok {
		t.Fatal("草稿编译失败")
	}

	res := snap.Simulate(simCtx(), 0, nil, &draft)
	if len(res.Steps) != 3 {
		t.Fatalf("steps = %d, want 3", len(res.Steps))
	}
	// priority 同为 20,存量 id=2 在前(新草稿排在同优先级末尾)。
	if res.Steps[1].RuleID != 2 || res.Steps[2].RuleID != 0 {
		t.Fatalf("排序 = %d,%d, want 2,0(同优先级新草稿在后)", res.Steps[1].RuleID, res.Steps[2].RuleID)
	}
	if !res.Matched || res.Verdict.RuleID != 2 {
		t.Fatalf("裁决 = %+v, want 规则 2 先命中", res.Verdict)
	}
	if res.Steps[2].Status != StepStatusSkip || !strings.Contains(res.Steps[2].Reason, "命中") {
		t.Fatalf("首条命中后的草稿 = %+v, want skip 并说明已被首条命中盖过", res.Steps[2])
	}
}

// TestSnapshotSimulateNeverPanics 诊断接口不能把 panic 抛给调用方:
// 取字段炸了也要把错误如实报出来,并且不给出命中结论。
func TestSnapshotSimulateNeverPanics(t *testing.T) {
	lg, _ := logBuf()
	snap := NewSnapshot(simRules(), lg)

	res := snap.Simulate(boomCtx{}, 0, nil, nil)
	if res.Matched {
		t.Fatal("求值炸了不得给出命中结论")
	}
	if res.Error == "" {
		t.Fatal("panic 应当记进 Error 供诊断")
	}
	// nil 画像 / nil 快照都是"没有规则适用",不是崩溃。
	if res := snap.Simulate(nil, 0, nil, nil); res.Matched || res.Error != "" {
		t.Fatalf("nil 画像 = %+v", res)
	}
	var nilSnap *Snapshot
	if res := nilSnap.Simulate(simCtx(), 0, nil, nil); res.Matched || len(res.Steps) != 0 {
		t.Fatalf("nil 快照 = %+v", res)
	}
}

// TestSnapshotSimulateConcurrent 诊断与跳转热路径并发跑:
// 仿真只读快照,不能与 Evaluate 抢写(go test -race 会抓)。
func TestSnapshotSimulateConcurrent(t *testing.T) {
	lg, _ := logBuf()
	rules := make([]store.Rule, 0, 20)
	for i := 1; i <= 20; i++ {
		rules = append(rules, store.Rule{
			ID: int64(i), Name: fmt.Sprintf("r%d", i), Priority: i, Enabled: true,
			Scope: store.RuleScopeGlobal, Logic: store.RuleLogicAll,
			Action: store.RuleActionNotfound,
			Conditions: store.Conditions(
				store.RuleCondition{Field: FieldUA, Operator: OpContains, Values: []string{"bot"}}),
		})
	}
	snap := NewSnapshot(rules, lg)
	bot := simFact()
	bot.UA = "Googlebot/2.1 (+http://www.google.com/bot.html)"
	human := simFact()
	human.UA = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36"

	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				f, want := bot, true
				if j%2 == 1 {
					f, want = human, false
				}
				res := snap.Simulate(&f, int64(j), nil, nil)
				if res.Matched != want {
					t.Errorf("仿真结论漂移: matched=%v want=%v %+v", res.Matched, want, res.Verdict)
					return
				}
				if want && res.Verdict.RuleID != 1 {
					t.Errorf("命中的不是优先级最高的规则: %+v", res.Verdict)
					return
				}
				dec, ok := snap.Evaluate(&f, int64(j))
				if ok != res.Matched || (ok && dec != res.Verdict) {
					t.Errorf("仿真与线上求值结论不一致: sim=%+v/%v eval=%+v/%v",
						res.Verdict, res.Matched, dec, ok)
					return
				}
			}
		}()
	}
	wg.Wait()
	if len(snap.Rules) != 20 {
		t.Fatalf("快照被改写: %d 条", len(snap.Rules))
	}
}

// boomCtx 取字段时炸掉的画像(模拟第三方 VisitorContext 实现出问题)。
type boomCtx struct{}

func (boomCtx) ClientIP() netip.Addr { return netip.Addr{} }

func (boomCtx) Field(string) (string, bool) { panic("boom") }
