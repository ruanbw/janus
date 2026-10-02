package rules

// 条件求值的单元测试是纯函数测试,不需要数据库。

import (
	"bytes"
	"log/slog"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"janus/internal/store"
)

// discardLog 静默日志器(编译期丢弃的告警不该污染测试输出)。
var discardLog = slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil))

// oneRule 造一条只含给定条件的规则。
func oneRule(id int64, conds ...store.RuleCondition) store.Rule {
	return store.Rule{
		ID: id, Name: "r", Enabled: true, Scope: store.RuleScopeGlobal,
		Logic: store.RuleLogicAll, Action: store.RuleActionNotfound,
		Priority: int(id), Conditions: store.Conditions(conds...),
	}
}

// oneTreeRule 造一条以条件树为条件的规则(嵌套形态,组内 logic 自带)。
func oneTreeRule(id int64, root store.ConditionNode) store.Rule {
	return store.Rule{
		ID: id, Name: "tree", Enabled: true, Scope: store.RuleScopeGlobal,
		Logic: store.RuleLogicAll, Action: store.RuleActionNotfound,
		Priority: int(id), Conditions: store.Tree(root),
	}
}

// evalCond 对单条条件求值,返回是否命中。
func evalCond(t *testing.T, fact Fact, cond store.RuleCondition) bool {
	t.Helper()
	c, ok := compileCond(cond, 1, discardLog)
	if !ok {
		t.Fatalf("条件 %+v 编译失败", cond)
	}
	return c.match(&fact)
}

func TestOperators(t *testing.T) {
	ua := "Mozilla/5.0 (iPhone; CPU iPhone OS 17_0 like Mac OS X) AppleWebKit/605 Version/17.0 Mobile/15E148 Safari/604.1"
	cases := []struct {
		name string
		cond store.RuleCondition
		want bool
	}{
		{"in 命中", store.RuleCondition{Field: FieldDevType, Operator: OpIn, Values: []string{"bot", "mobile"}}, true},
		{"in 不命中", store.RuleCondition{Field: FieldDevType, Operator: OpIn, Values: []string{"bot", "desktop"}}, false},
		{"not_in 命中", store.RuleCondition{Field: FieldDevType, Operator: OpNotIn, Values: []string{"bot"}}, true},
		{"not_in 不命中", store.RuleCondition{Field: FieldDevType, Operator: OpNotIn, Values: []string{"mobile"}}, false},
		{"eq 命中", store.RuleCondition{Field: FieldOS, Operator: OpEq, Values: []string{"iOS"}}, true},
		{"eq 不命中", store.RuleCondition{Field: FieldOS, Operator: OpEq, Values: []string{"Android"}}, false},
		{"neq 命中", store.RuleCondition{Field: FieldOS, Operator: OpNeq, Values: []string{"Android"}}, true},
		{"neq 不命中", store.RuleCondition{Field: FieldOS, Operator: OpNeq, Values: []string{"ios"}}, false},
		{"contains 不命中", store.RuleCondition{Field: FieldUA, Operator: OpContains, Values: []string{"bot"}}, false},
		{"contains 命中且大小写不敏感", store.RuleCondition{Field: FieldBrowser, Operator: OpContains, Values: []string{"safari"}}, true},
		{"not_contains 命中", store.RuleCondition{Field: FieldUA, Operator: OpNotContains, Values: []string{"bot"}}, true},
		{"not_contains 不命中", store.RuleCondition{Field: FieldUA, Operator: OpNotContains, Values: []string{"safari"}}, false},
		{"regex 命中", store.RuleCondition{Field: FieldUA, Operator: OpRegex, Values: []string{"^Mozilla.*iPhone"}}, true},
		{"regex 不命中", store.RuleCondition{Field: FieldUA, Operator: OpRegex, Values: []string{"^Opera"}}, false},
		{"regex 带 (?i)", store.RuleCondition{Field: FieldRef, Operator: OpRegex, Values: []string{"(?i)^www\\.google\\.com$"}}, true},
		{"in 多值任一命中", store.RuleCondition{Field: FieldLang, Operator: OpIn, Values: []string{"en", "pt-BR"}}, false},
		{"duplicated 无计数不命中", store.RuleCondition{Field: FieldIP, Operator: OpDuplicated, Values: []string{"2"}}, false},
	}
	fact := FromRequest(mustRequest(t, "https://go.example.com/abc?utm_source=news", "https://www.google.com/", ua))
	fact = fact.WithIP("1.2.3.4")
	// 逐个 case 覆盖 10 个运算符
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := fact
			if got := evalCond(t, f, tc.cond); got != tc.want {
				t.Fatalf("命中 = %v, want %v(条件 %+v, 画像 %+v)", got, tc.want, tc.cond, f)
			}
		})
	}
}

// duplicated:计数来源是调用方给的 Fact.Seen,没有计数恒不命中。
func TestDuplicatedUsesSeenCounter(t *testing.T) {
	cond := store.RuleCondition{Field: FieldIP, Operator: OpDuplicated, Values: []string{"2"}}
	fact := Fact{IP: "9.9.9.9"}
	if evalCond(t, fact, cond) {
		t.Fatal("无计数时 duplicated 不该命中")
	}
	fact.Seen = map[string]int{SeenKey(FieldIP, "9.9.9.9"): 1}
	if evalCond(t, fact, cond) {
		t.Fatal("计数 1 < 阈值 2 时不该命中")
	}
	fact.Seen[SeenKey(FieldIP, "9.9.9.9")] = 2
	if !evalCond(t, fact, cond) {
		t.Fatal("计数达到阈值时该命中")
	}
}

// gt / lt 按数值比较;非数值字段恒不命中。
func TestNumericOperators(t *testing.T) {
	fact := Fact{Path: "/2024/abc"}
	if !evalCond(t, fact, store.RuleCondition{Field: FieldPath, Operator: OpContains, Values: []string{"2024"}}) {
		t.Fatal("path 含 2024")
	}
	// 阈值侧不是数字 → 加载期丢弃(见 TestBadConditionDroppedAtLoad)
	// 字段值不是数字 → 恒不命中,不 panic
	if evalCond(t, fact, store.RuleCondition{Field: FieldOS, Operator: OpGT, Values: []string{"1"}}) {
		t.Fatal("非数值字段不该满足 gt")
	}
	ua := "Mozilla/5.0 (Windows NT 10.0) AppleWebKit/537 Chrome/120 Safari/537"
	f := FromRequest(mustRequest(t, "http://x.test/p", "", ua))
	if !evalCond(t, f, store.RuleCondition{Field: FieldDevType, Operator: OpEq, Values: []string{"Desktop"}}) {
		t.Fatal("大小写归一后 desktop 应命中 Desktop")
	}
	// 用一个数值型字段做大小比较:path 里没有数字型字段,这里直接构造
	num := Fact{UTM: "42"}
	if !evalCond(t, num, store.RuleCondition{Field: FieldUTM, Operator: OpGT, Values: []string{"41"}}) {
		t.Fatal("42 > 41")
	}
	if evalCond(t, num, store.RuleCondition{Field: FieldUTM, Operator: OpLT, Values: []string{"41"}}) {
		t.Fatal("42 < 41 不成立")
	}
}

func TestCIDRMatching(t *testing.T) {
	cond := store.RuleCondition{Field: FieldIP, Operator: OpIn, Values: []string{
		"10.0.0.0/8", "192.168.1.0/24", "2001:db8::/32", "203.0.113.7",
	}}
	cases := []struct {
		ip   string
		want bool
	}{
		{"10.1.2.3", true}, {"192.168.1.99", true}, {"192.168.2.1", false},
		{"2001:db8::1", true}, {"2001:db9::1", false}, {"203.0.113.7", true},
		{"203.0.113.8", false}, {"8.8.8.8", false},
	}
	for _, tc := range cases {
		f := Fact{IP: tc.ip}
		if got := evalCond(t, f, cond); got != tc.want {
			t.Fatalf("ip %s in 网段 = %v, want %v", tc.ip, got, tc.want)
		}
	}
	// not_in:网段外才算命中;网段内即便整个网段没命中单个 IP 也不命中
	notin := cond
	notin.Operator = OpNotIn
	if !evalCond(t, Fact{IP: "8.8.8.8"}, notin) {
		t.Fatal("8.8.8.8 not_in 内网网段")
	}
	if evalCond(t, Fact{IP: "10.1.2.3"}, notin) {
		t.Fatal("10.1.2.3 not_in 内网网段")
	}
}

// 关键不变式:字段值取不到时恒不命中——包括 not_in / neq / not_contains。
// 反例后果:GeoIP 未接入时 country 恒空,"country not_in ['US']" 会对全部流量命中。
func TestEmptyValueNeverMatches(t *testing.T) {
	fact := Fact{} // 全部字段取不到
	conds := []store.RuleCondition{
		{Field: FieldCountry, Operator: OpIn, Values: []string{"US"}},
		{Field: FieldCountry, Operator: OpNotIn, Values: []string{"US"}},
		{Field: FieldASN, Operator: OpEq, Values: []string{"AS15169"}},
		{Field: FieldASN, Operator: OpNeq, Values: []string{"AS15169"}},
		{Field: FieldUA, Operator: OpNotContains, Values: []string{"bot"}},
		{Field: FieldRef, Operator: OpNotIn, Values: []string{"spam.test"}},
		{Field: FieldIP, Operator: OpNotIn, Values: []string{"10.0.0.0/8"}},
	}
	for _, c := range conds {
		if evalCond(t, fact, c) {
			t.Fatalf("空值条件 %+v 不该命中", c)
		}
	}
	// geo 注入后才有值(接入 GeoIP 前的占位通道)
	withGeo := fact
	withGeo.Country = "US"
	if !evalCond(t, withGeo, store.RuleCondition{Field: FieldCountry, Operator: OpIn, Values: []string{"us"}}) {
		t.Fatal("注入 country 后 in 应命中")
	}
}

func TestLogicAllAny(t *testing.T) {
	ua := "Googlebot/2.1 (+http://www.google.com/bot.html)"
	fact := FromRequest(mustRequest(t, "https://s.test/promo", "", ua))
	countryUS := store.RuleCondition{Field: FieldCountry, Operator: OpIn, Values: []string{"US"}}
	bot := store.RuleCondition{Field: FieldDevType, Operator: OpEq, Values: []string{"bot"}}
	cn := store.RuleCondition{Field: FieldIPAttr, Operator: OpIn, Values: []string{"private"}}

	all := oneRule(1, countryUS, bot, cn)
	all.Logic = store.RuleLogicAll
	if _, ok := NewSnapshot([]store.Rule{all}, discardLog).Evaluate(fact, 1); ok {
		t.Fatal("all:缺 country/ipattr 条件时不该命中")
	}
	any := oneRule(2, countryUS, bot, cn)
	any.Logic = store.RuleLogicAny
	dec, ok := NewSnapshot([]store.Rule{any}, discardLog).Evaluate(fact, 1)
	if !ok {
		t.Fatal("any:bot 条件命中时该命中")
	}
	if dec.Action != store.RuleActionNotfound || dec.RuleID != 2 {
		t.Fatalf("裁决 = %+v", dec)
	}
	// 空条件组 = 无条件即命中(可以配一条兜底规则)
	empty := oneRule(3)
	if _, ok := NewSnapshot([]store.Rule{empty}, discardLog).Evaluate(fact, 1); !ok {
		t.Fatal("空条件组应恒命中")
	}
}

// 坏条件在加载期被丢弃,不影响其他规则与其他条件。
func TestBadConditionDroppedAtLoad(t *testing.T) {
	var buf bytes.Buffer
	log := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
	fact := Fact{UA: "Googlebot/2.1", DevType: "bot", Domain: "s.test", Path: "/p"}

	rules := []store.Rule{
		// 整条规则的条件全坏 → 整条规则不参与求值
		oneRule(1,
			store.RuleCondition{Field: FieldUA, Operator: OpRegex, Values: []string{"(unclosed"}},
			store.RuleCondition{Field: "region", Operator: OpEq, Values: []string{"北京"}},
			store.RuleCondition{Field: FieldUA, Operator: "全等于", Values: []string{"x"}},
		),
		// 一个正则坏、另一个好 → 只丢坏的那个
		oneRule(2,
			store.RuleCondition{Field: FieldUA, Operator: OpRegex, Values: []string{"(unclosed", "Googlebot"}},
		),
		// 非法 CIDR → 整条条件丢弃
		oneRule(3, store.RuleCondition{Field: FieldIP, Operator: OpIn, Values: []string{"10.0.0.0/33", "不是IP"}}),
		// ip 条件丢弃后剩下的空条件会退化成恒命中,所以整条规则不该保留
		oneRule(4, store.RuleCondition{Field: FieldIP, Operator: OpIn, Values: []string{"10.0.0.0/33"}}),
	}
	snap := NewSnapshot(rules, log)
	if len(snap.Rules) != 1 || snap.Rules[0].Rule.ID != 2 {
		ids := make([]int64, 0, len(snap.Rules))
		for _, r := range snap.Rules {
			ids = append(ids, r.Rule.ID)
		}
		t.Fatalf("快照里的规则 = %v, want 只剩 2", ids)
	}
	if _, ok := snap.Evaluate(fact, 1); !ok {
		t.Fatal("规则 2 里那个好正则应该命中")
	}
	if !strings.Contains(buf.String(), "被丢弃") || !strings.Contains(buf.String(), "无法编译") {
		t.Fatalf("缺少丢弃告警日志:\n%s", buf.String())
	}
}

// fail-open:求值内部 panic 时按"未命中"处理并记日志,绝不让规则把短链打成 500。
func TestEvaluateFailsOpenOnPanic(t *testing.T) {
	var buf bytes.Buffer
	log := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
	snap := NewSnapshot(nil, log)
	// 手工塞一条"损坏的预编译产物":正则指针为 nil,匹配时会 panic
	snap.Rules = append(snap.Rules, Compiled{
		Rule:       store.Rule{ID: 99, Enabled: true, Action: store.RuleActionNotfound},
		appliesAll: true, logicAll: true,
		root: compiledNode{op: nodeOpAll, conds: []compiledCond{
			{field: FieldUA, op: OpRegex, res: []*regexp.Regexp{nil}},
		}},
	})
	dec, ok := snap.Evaluate(Fact{UA: "anything"}, 1)
	if ok {
		t.Fatalf("panic 后不该判为命中: %+v", dec)
	}
	if dec != (Decision{}) {
		t.Fatalf("panic 后应返回零值裁决, got %+v", dec)
	}
	if !strings.Contains(buf.String(), "fail-open") {
		t.Fatalf("缺少 fail-open 日志:\n%s", buf.String())
	}
	// 之后再求值仍能正常工作(recover 只兜当次)
	if _, ok := snap.Evaluate(Fact{}, 1); ok {
		t.Fatal("无画像时不该命中")
	}
}

// 空快照与 nil 快照都直接返回未命中,不做任何遍历。
func TestEmptySnapshotNeverMatches(t *testing.T) {
	var nilSnap *Snapshot
	if _, ok := nilSnap.Evaluate(Fact{}, 1); ok {
		t.Fatal("nil 快照不该命中")
	}
	empty := NewSnapshot(nil, discardLog)
	if _, ok := empty.Evaluate(Fact{UA: "x"}, 1); ok {
		t.Fatal("空快照不该命中")
	}
	// 停用规则不进快照
	disabled := oneRule(1, store.RuleCondition{Field: FieldUA, Operator: OpContains, Values: []string{"bot"}})
	disabled.Enabled = false
	if len(NewSnapshot([]store.Rule{disabled}, discardLog).Rules) != 0 {
		t.Fatal("停用规则不该进快照")
	}
}

// Fact 手工构造时(不经 FromRequest)也要能正确比对 IP 集合:解析在求值期按需补一次。
func TestManualFactIPParsing(t *testing.T) {
	f := Fact{IP: "10.1.2.3"}
	if !evalCond(t, f, store.RuleCondition{Field: FieldIP, Operator: OpIn, Values: []string{"10.0.0.0/8"}}) {
		t.Fatal("手工构造的 Fact 也该命中 CIDR")
	}
	if evalCond(t, f, store.RuleCondition{Field: FieldIP, Operator: OpIn, Values: []string{"192.168.0.0/16"}}) {
		t.Fatal("不该命中其他网段")
	}
	// 条件值不是 IP/CIDR:加载期就该被丢弃(不会退化成恒真)
	if _, ok := compileCond(store.RuleCondition{
		Field: FieldIP, Operator: OpEq, Values: []string{"垃圾"}}, 1, discardLog); ok {
		t.Fatal("非 IP 的 ip 条件不该编译通过")
	}
}

// ip 集合比较的四个运算符真值表:in / eq 在集合内命中,neq / not_in 在集合外命中。
// 回归的 bug:ip 分支只看 op==not_in,导致 neq 的两个方向都反了
// ("ip neq 203.0.113.7" 会命中落在集合内的访客、放过集合外的访客)。
func TestIPSetOperatorsTruthTable(t *testing.T) {
	// 每个 case 只声明语义:访客 IP 落不落在配置集合里,期望由运算符决定。
	cases := []struct {
		name   string   // 用例名
		values []string // 条件里配的 IP 集合
		ip     string   // 访客 IP
		inSet  bool     // 该访客 IP 是否落在上面这个集合内
	}{
		{"IPv4 字面量-集合内", []string{"203.0.113.7"}, "203.0.113.7", true},
		{"IPv4 字面量-集合外", []string{"203.0.113.7"}, "203.0.113.8", false},
		{"IPv4 网段-集合内", []string{"10.0.0.0/8", "192.168.1.0/24"}, "192.168.1.99", true},
		{"IPv4 网段-集合外", []string{"10.0.0.0/8", "192.168.1.0/24"}, "192.168.2.1", false},
		{"IPv6 字面量-集合内", []string{"2001:db8::1"}, "2001:0db8:0000::1", true},
		{"IPv6 字面量-集合外", []string{"2001:db8::1"}, "2001:db8::2", false},
		{"IPv6 网段-集合内", []string{"2001:db8::/32"}, "2001:db8:1::9", true},
		{"IPv6 网段-集合外", []string{"2001:db8::/32"}, "2001:db9::1", false},
	}
	// 四个集合运算符的真值表:in / eq = 落在集合内;neq / not_in = 落在集合外。
	ops := []struct {
		op   string
		want func(inSet bool) bool
	}{
		{OpIn, func(inSet bool) bool { return inSet }},
		{OpEq, func(inSet bool) bool { return inSet }},
		{OpNeq, func(inSet bool) bool { return !inSet }},
		{OpNotIn, func(inSet bool) bool { return !inSet }},
	}
	for _, tc := range cases {
		for _, o := range ops {
			t.Run(tc.name+"/"+o.op, func(t *testing.T) {
				f := Fact{IP: tc.ip}
				got := evalCond(t, f, store.RuleCondition{
					Field: FieldIP, Operator: o.op, Values: tc.values,
				})
				if want := o.want(tc.inSet); got != want {
					t.Fatalf("访客 %s %s %v(落在集合内=%v)= %v, want %v",
						tc.ip, o.op, tc.values, tc.inSet, got, want)
				}
			})
		}
	}
}

// 取不到访客 IP 时恒不命中——包括 neq / not_in(否则空 ipattr 会被当成"不在集合里"而全部命中)。
func TestIPSetEmptyValueNeverMatches(t *testing.T) {
	values := []string{"10.0.0.0/8", "203.0.113.7"}
	// IP 为空,以及 IP 非法(ipattr 随之为空)两种画像
	facts := []Fact{{}, {IP: "不是IP"}}
	for _, f := range facts {
		for _, op := range []string{OpIn, OpEq, OpNeq, OpNotIn} {
			if evalCond(t, f, store.RuleCondition{
				Field: FieldIP, Operator: op, Values: values,
			}) {
				t.Fatalf("画像 %+v 的 ip %s 不该命中", f, op)
			}
		}
	}
	// ipattr 字段本身同理:恒不命中
	if evalCond(t, Fact{IP: "10.1.2.3"}, store.RuleCondition{
		Field: FieldIPAttr, Operator: OpNeq, Values: []string{"private"},
	}) {
		t.Fatal("ipattr 为空时 neq 不该命中")
	}
	if evalCond(t, Fact{IP: "8.8.8.8"}, store.RuleCondition{
		Field: FieldIPAttr, Operator: OpNotIn, Values: []string{"private"},
	}) {
		t.Fatal("ipattr 为空时 not_in 不该命中")
	}
}

func TestDecisionCarriesCustomHTML(t *testing.T) {
	rule := store.Rule{
		ID:         1,
		Name:       "custom-404",
		Enabled:    true,
		Scope:      store.RuleScopeGlobal,
		Action:     store.RuleActionNotfound,
		PageMode:   "custom",
		CustomHTML: "<h1>Blocked</h1>",
	}
	snap := NewSnapshot([]store.Rule{rule}, discardLog)
	dec, matched := snap.Evaluate(Fact{}, 123)
	if !matched {
		t.Fatal("expected rule to match")
	}
	if dec.PageMode != "custom" {
		t.Errorf("dec.PageMode = %q, want custom", dec.PageMode)
	}
	if dec.CustomHTML != "<h1>Blocked</h1>" {
		t.Errorf("dec.CustomHTML = %q, want <h1>Blocked</h1>", dec.CustomHTML)
	}
}

// TestPrecompiledLitMap 验证非 IP 字符串集合操作符 (in/not_in) 预编译 hash map 及 O(1) 匹配正确性。
func TestPrecompiledLitMap(t *testing.T) {
	cond := store.RuleCondition{
		Field:    FieldCountry,
		Operator: OpIn,
		Values:   []string{"US", "GB", "CA", "DE", "FR"},
	}
	c, ok := compileCond(cond, 1, discardLog)
	if !ok {
		t.Fatal("compileCond failed")
	}
	if c.litMap == nil {
		t.Fatal("expected litMap to be compiled for OpIn")
	}
	if len(c.litMap) != 5 {
		t.Fatalf("expected litMap len 5, got %d", len(c.litMap))
	}
	for _, expected := range []string{"us", "gb", "ca", "de", "fr"} {
		if _, exists := c.litMap[expected]; !exists {
			t.Fatalf("expected %q in litMap", expected)
		}
	}

	// 匹配大小写不敏感
	if !c.match(&Fact{Country: "US"}) {
		t.Fatal("US should match")
	}
	if !c.match(&Fact{Country: "us"}) {
		t.Fatal("us should match")
	}
	if !c.match(&Fact{Country: "Us"}) {
		t.Fatal("Us should match")
	}
	if c.match(&Fact{Country: "JP"}) {
		t.Fatal("JP should not match")
	}

	// not_in 也是预编译 litMap
	notInCond := store.RuleCondition{
		Field:    FieldCountry,
		Operator: OpNotIn,
		Values:   []string{"CN", "RU"},
	}
	cNotIn, ok := compileCond(notInCond, 2, discardLog)
	if !ok {
		t.Fatal("compileCond failed for not_in")
	}
	if cNotIn.litMap == nil {
		t.Fatal("expected litMap to be compiled for OpNotIn")
	}
	if !cNotIn.match(&Fact{Country: "US"}) {
		t.Fatal("US should match not_in [CN, RU]")
	}
	if cNotIn.match(&Fact{Country: "cn"}) {
		t.Fatal("cn should not match not_in [CN, RU]")
	}
}

// TestInFallbackNoAlloc 固定 in/not_in 的非 ASCII / 超长输入回退路径:
// 缓冲区快速路径只覆盖 <=64 字节的纯 ASCII,长串与非 ASCII 一律走线性扫描,
// 该路径必须与 map 路径同样零分配(旧实现用 strings.ToLower,每次请求都堆分配)。
func TestInFallbackNoAlloc(t *testing.T) {
	// 超过 64 字节,走出栈缓冲区快速路径;另含一个非 ASCII 字面量。
	longPath := "/Promo/Summer-Campaign/2026?utm_source=wechat&utm_medium=social-promo"
	if len(longPath) <= 64 {
		t.Fatalf("测试字面量需长于 64 字节,实际 %d", len(longPath))
	}
	nonASCII := "/活动/夏季促销"
	cond := store.RuleCondition{
		Field:    FieldPath,
		Operator: OpIn,
		Values:   []string{longPath, nonASCII},
	}
	c, ok := compileCond(cond, 1, discardLog)
	if !ok {
		t.Fatal("compileCond failed")
	}
	if c.litMap == nil {
		t.Fatal("expected litMap to be compiled for OpIn")
	}

	cases := []struct {
		name string
		path string
		want bool
	}{
		{"长串原样", longPath, true},
		{"长串大小写不同", strings.ToUpper(longPath), true},
		{"长串未命中", "/Promo/Summer-Campaign/2025", false},
		{"非 ASCII 命中", nonASCII, true},
		{"非 ASCII 未命中", "/活动/冬季促销", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := c.match(&Fact{Path: tc.path})
			if got != tc.want {
				t.Fatalf("match(%q) = %v, want %v", tc.path, got, tc.want)
			}
			// Fact 提到循环外:传进 VisitorContext 接口时若在闭包里新建会逃逸到堆,
			// 测出来的那次分配与回退路径无关。
			fact := Fact{Path: tc.path}
			allocs := testing.AllocsPerRun(100, func() {
				_ = c.match(&fact)
			})
			if allocs > 0 {
				t.Errorf("in 回退路径分配了 %v allocs/op, want 0", allocs)
			}
		})
	}

	// not_in 的极性不能被回退路径重复取反:命中集合的输入不命中 not_in,未命中的命中。
	notIn, ok := compileCond(store.RuleCondition{
		Field:    FieldPath,
		Operator: OpNotIn,
		Values:   []string{longPath, nonASCII},
	}, 2, discardLog)
	if !ok {
		t.Fatal("compileCond failed for not_in")
	}
	for _, tc := range cases {
		got := notIn.match(&Fact{Path: tc.path})
		if got == tc.want {
			t.Errorf("not_in: match(%q) = %v, want %v", tc.path, got, !tc.want)
		}
	}
}

// BenchmarkLazyEvaluate 性能基准测试:对比惰性求值 (LazyVisitorContext) 与贪婪求值 (Fact/FromRequest)
// 分别在纯国家规则(跳过 UA 解析)与设备规则(按需解析 UA)下的耗时与堆分配。
func BenchmarkLazyEvaluate(b *testing.B) {
	ua := "Mozilla/5.0 (iPhone; CPU iPhone OS 17_0 like Mac OS X) AppleWebKit/605 Version/17.0 Mobile/15E148 Safari/604.1"
	req := mustRequest(b, "https://shop.example.com/promo?utm_source=wechat", "https://www.google.com/", ua)
	req.RemoteAddr = "203.0.113.5:44321"

	// 场景 1: 仅含地理位置条件的规则(最常见风控场景:拦截特定国家/放行特定国家)
	countryRule := oneRule(1, store.RuleCondition{
		Field:    FieldCountry,
		Operator: OpIn,
		Values:   []string{"US", "CA", "GB", "DE", "FR"},
	})
	snapCountry := NewSnapshot([]store.Rule{countryRule}, discardLog)

	// 场景 2: 包含设备条件的规则(需要解析 UA)
	deviceRule := oneRule(2, store.RuleCondition{
		Field:    FieldDevType,
		Operator: OpIn,
		Values:   []string{"mobile", "tablet"},
	})
	snapDevice := NewSnapshot([]store.Rule{deviceRule}, discardLog)

	b.Run("Lazy_CountryOnly_EvalOnly", func(b *testing.B) {
		vCtx := AcquireVisitorContext(req, "US", "").WithIP("203.0.113.5")
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			if _, ok := snapCountry.Evaluate(vCtx, 1); !ok {
				b.Fatal("expected match")
			}
		}
		if vCtx.UAParsed() {
			b.Fatal("UA parsing should have been skipped")
		}
		ReleaseVisitorContext(vCtx)
	})

	b.Run("Lazy_CountryOnly_FullCycle", func(b *testing.B) {
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			vCtx := AcquireVisitorContext(req, "US", "").WithIP("203.0.113.5")
			if _, ok := snapCountry.Evaluate(vCtx, 1); !ok {
				b.Fatal("expected match")
			}
			if vCtx.UAParsed() {
				b.Fatal("UA parsing should have been skipped")
			}
			ReleaseVisitorContext(vCtx)
		}
	})

	b.Run("Eager_CountryOnly_FullCycle", func(b *testing.B) {
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			fact := FromRequest(req)
			fact.Country = "US"
			if _, ok := snapCountry.Evaluate(&fact, 1); !ok {
				b.Fatal("expected match")
			}
		}
	})

	b.Run("Lazy_DeviceRule_FullCycle", func(b *testing.B) {
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			vCtx := AcquireVisitorContext(req, "US", "").WithIP("203.0.113.5")
			if _, ok := snapDevice.Evaluate(vCtx, 1); !ok {
				b.Fatal("expected match")
			}
			ReleaseVisitorContext(vCtx)
		}
	})

	b.Run("Eager_DeviceRule_FullCycle", func(b *testing.B) {
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			fact := FromRequest(req)
			if _, ok := snapDevice.Evaluate(&fact, 1); !ok {
				b.Fatal("expected match")
			}
		}
	})
}

// BenchmarkSetOperators 性能基准测试:验证预编译 hash map 在大规模枚举值集合下的 O(1) 查找性能
func BenchmarkSetOperators(b *testing.B) {
	countries := []string{
		"US", "GB", "CA", "DE", "FR", "JP", "KR", "AU", "NZ", "SG",
		"MY", "TH", "VN", "PH", "ID", "IN", "BR", "MX", "AR", "CL",
		"CO", "PE", "ZA", "EG", "NG", "KE", "SA", "AE", "TR", "IL",
		"IT", "ES", "NL", "BE", "SE", "NO", "DK", "FI", "CH", "AT",
		"IE", "PT", "GR", "PL", "CZ", "HU", "RO", "BG", "UA", "RU",
	}
	cond := store.RuleCondition{
		Field:    FieldCountry,
		Operator: OpIn,
		Values:   countries,
	}
	rule := oneRule(1, cond)
	snap := NewSnapshot([]store.Rule{rule}, discardLog)
	vCtxHit := AcquireVisitorContext(nil, "RU", "") // 集合末尾元素
	defer ReleaseVisitorContext(vCtxHit)
	vCtxMiss := AcquireVisitorContext(nil, "CN", "") // 未收录元素
	defer ReleaseVisitorContext(vCtxMiss)

	b.Run("MapLookup_HitLast", func(b *testing.B) {
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			if _, ok := snap.Evaluate(vCtxHit, 1); !ok {
				b.Fatal("expected match")
			}
		}
	})

	b.Run("MapLookup_Miss", func(b *testing.B) {
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			if _, ok := snap.Evaluate(vCtxMiss, 1); ok {
				b.Fatal("expected miss")
			}
		}
	})
}

// ---------- 条件树 ----------

// nestedTree 造一棵 "any[ 中国, all[ /promo, 爬虫 ] ]"。
func nestedTree() store.ConditionNode {
	return store.Group("any",
		store.Leaf(store.RuleCondition{Field: FieldCountry, Operator: OpEq, Values: []string{"CN"}}),
		store.Group("all",
			store.Leaf(store.RuleCondition{Field: FieldPath, Operator: OpContains, Values: []string{"/promo"}}),
			store.Leaf(store.RuleCondition{Field: FieldDevType, Operator: OpEq, Values: []string{"bot"}}),
		),
	)
}

// 树形态求值:any 与嵌套 all 各自生效,且组内 logic 优先于 rules.logic。
func TestNestedConditionTree(t *testing.T) {
	promo := Fact{Path: "/promo", DevType: "bot", Domain: "s.test"}
	human := Fact{Path: "/promo", DevType: "mobile", Domain: "s.test"}
	cn := Fact{Country: "CN", Path: "/other", DevType: "mobile", Domain: "s.test"}
	none := Fact{Path: "/other", DevType: "mobile", Domain: "s.test"}

	cases := []struct {
		name  string
		fact  Fact
		want  bool
		whyIt string
	}{
		{"嵌套 all 两条都成立", promo, true, "路径 + 爬虫同时成立"},
		{"嵌套 all 只成立一条", human, false, "只是 /promo 不够"},
		{"外层 any 的叶子成立", cn, true, "中国访客直接命中"},
		{"全不成立", none, false, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// rules.logic 故意写 all:树形态下必须以组内的 any 为准
			rule := oneTreeRule(1, nestedTree())
			rule.Logic = store.RuleLogicAll
			_, got := NewSnapshot([]store.Rule{rule}, discardLog).Evaluate(&tc.fact, 1)
			if got != tc.want {
				t.Fatalf("命中 = %v, want %v(%s)", got, tc.want, tc.whyIt)
			}
		})
	}
}

// 树形态下坏叶子不能只丢自己:在 and 组里丢一条等于把规则放宽(fail-open),
// 在 any 组里丢一条等于把规则收紧。与扁平形态的"丢单个"策略故意不同。
func TestNestedTreeBadLeafDropsWholeRule(t *testing.T) {
	badField := store.RuleCondition{Field: "region", Operator: OpEq, Values: []string{"北京"}}
	good := store.RuleCondition{Field: FieldDevType, Operator: OpEq, Values: []string{"bot"}}
	fact := Fact{DevType: "bot", Domain: "s.test"}

	// 同一棵树:bad 叶子挂在 and 组里 / 挂在 any 组里,结果都该是整条规则不参与求值
	for _, tc := range []struct {
		name string
		root store.ConditionNode
	}{
		{"and 组里的坏叶子", store.Group("all", store.Leaf(good), store.Leaf(badField))},
		{"any 组里的坏叶子", store.Group("any", store.Leaf(good), store.Leaf(badField))},
		{"坏叶子自己成一个组", store.Group("all", store.Leaf(good), store.Group("all", store.Leaf(badField)))},
	} {
		t.Run(tc.name, func(t *testing.T) {
			snap := NewSnapshot([]store.Rule{oneTreeRule(7, tc.root)}, discardLog)
			if len(snap.Rules) != 0 {
				t.Fatalf("整条规则都该被丢弃,却留下了 %d 条", len(snap.Rules))
			}
			if _, ok := snap.Evaluate(&fact, 1); ok {
				t.Fatal("不参与求值的规则不该命中")
			}
		})
	}

	// 对照:同样的坏叶子放在扁平条件里,只丢它自己,规则照常生效(既有行为不变)
	flat := oneRule(8, good, badField)
	snap := NewSnapshot([]store.Rule{flat}, discardLog)
	if len(snap.Rules) != 1 {
		t.Fatalf("扁平形态不该整条丢弃: %d 条", len(snap.Rules))
	}
	if _, ok := snap.Evaluate(&fact, 1); !ok {
		t.Fatal("扁平形态下剩下的好条件应该命中")
	}
}

// 结构性空组(一个子节点都没有)直接跳过;空组不是"写了坏叶子",不该毁掉整条规则。
func TestNestedTreeStructuralEmptyGroup(t *testing.T) {
	good := store.Leaf(store.RuleCondition{Field: FieldDevType, Operator: OpEq, Values: []string{"bot"}})
	fact := Fact{DevType: "bot", Domain: "s.test"}

	for _, tc := range []struct {
		name string
		root store.ConditionNode
		want bool
	}{
		{"空 any 组被跳过", store.Group("all", good, store.Group("any")), true},
		{"空 all 组被跳过", store.Group("any", good, store.Group("all")), true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			snap := NewSnapshot([]store.Rule{oneTreeRule(9, tc.root)}, discardLog)
			if len(snap.Rules) != 1 {
				t.Fatalf("规则不该被丢弃,剩 %d 条", len(snap.Rules))
			}
			if _, got := snap.Evaluate(&fact, 1); got != tc.want {
				t.Fatalf("命中 = %v, want %v", got, tc.want)
			}
		})
	}
	// 反过来:整棵树每个分支都只剩空组 = 用户写的条件一条都不存在。
	// 这种规则不能求值(它既不是"恒成立"也不是"恒不成立",而是被写坏了),
	// 整条丢弃并在日志里留下告警,而不是猜一个语义。API 侧本来就拒收空组,
	// 只有手写 JSONB 的脏数据才会走到这里。
	var buf bytes.Buffer
	lg := slog.New(slog.NewTextHandler(&buf, nil))
	snap := NewSnapshot([]store.Rule{oneTreeRule(9, store.Group("any", store.Group("all")))}, lg)
	if len(snap.Rules) != 0 {
		t.Fatalf("只剩空组的树该整条丢弃,剩 %d 条", len(snap.Rules))
	}
	if !strings.Contains(buf.String(), "条件树不可求值") {
		t.Fatalf("缺少丢弃告警:\n%s", buf.String())
	}
}

// 根组逻辑缺失时按 all(和扁平形态、库里 CHECK 的约定一致)。
func TestNestedTreeMissingLogicIsAll(t *testing.T) {
	root := store.Group("",
		store.Leaf(store.RuleCondition{Field: FieldCountry, Operator: OpEq, Values: []string{"CN"}}),
		store.Leaf(store.RuleCondition{Field: FieldDevType, Operator: OpEq, Values: []string{"bot"}}),
	)
	snap := NewSnapshot([]store.Rule{oneTreeRule(10, root)}, discardLog)
	if got := snap.Rules[0].root.op; got != nodeOpAll {
		t.Fatalf("根组 op = %d, want nodeOpAll", got)
	}
	onlyBot := Fact{DevType: "bot", Domain: "s.test"}
	if _, ok := snap.Evaluate(&onlyBot, 1); ok {
		t.Fatal("缺 country 的叶子不该命中(all 语义)")
	}
}

// 仿真留痕必须覆盖整棵树的每个叶子,包括被短路掉的那一支——
// "为什么另一条没生效"才是决策链要回答的问题。
func TestNestedTreeSimulateTracesEveryLeaf(t *testing.T) {
	// 三个叶子都不成立(path 不带 /promo、devtype 不是 bot、country 不是 CN)
	fact := Fact{Path: "/pricing", DevType: "mobile", Country: "US", Domain: "s.test"}
	snap := NewSnapshot([]store.Rule{oneTreeRule(11, nestedTree())}, discardLog)
	res := snap.Simulate(&fact, 1, nil, nil)
	if res.Matched {
		t.Fatalf("不该命中: %+v", res.Verdict)
	}
	step := res.Steps[0]
	if len(step.Conditions) != 3 {
		t.Fatalf("留痕条数 = %d, want 3(全树叶子)", len(step.Conditions))
	}
	fields := []string{step.Conditions[0].Field, step.Conditions[1].Field, step.Conditions[2].Field}
	want := []string{FieldCountry, FieldPath, FieldDevType}
	for i := range want {
		if fields[i] != want[i] {
			t.Fatalf("第 %d 条留痕字段 = %q, want %q", i, fields[i], want[i])
		}
	}
	if !strings.Contains(step.Reason, "条件树求值未通过") || !strings.Contains(step.Reason, "0/3") {
		t.Fatalf("未命中理由 = %q, want 提到条件树与 0/3", step.Reason)
	}
}

// 树形态的求值不分配内存(与扁平形态同一条 0 B/op 基线)。
func BenchmarkEvaluateNestedTree(b *testing.B) {
	snap := NewSnapshot([]store.Rule{oneTreeRule(1, nestedTree())}, discardLog)
	vCtx := AcquireVisitorContext(nil, "US", "")
	defer ReleaseVisitorContext(vCtx)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		snap.Evaluate(vCtx, 1)
	}
}

// TestExtendedOperators 验证 starts_with、ends_with、in_cidr 运算符的求值与仿真留痕。
func TestExtendedOperators(t *testing.T) {
	t.Run("starts_with 命中与大小写不敏感", func(t *testing.T) {
		r := store.Rule{
			ID: 1, Enabled: true, Scope: store.RuleScopeGlobal, Logic: store.RuleLogicAll, Action: store.RuleActionPass,
			Conditions: store.Conditions(store.RuleCondition{Field: FieldPath, Operator: OpStartsWith, Values: []string{"/API", "/Admin"}}),
		}
		snap := NewSnapshot([]store.Rule{r}, discardLog)
		cases := []struct {
			path string
			want bool
		}{
			{"/api/v1/links", true},
			{"/API/v1/links", true},
			{"/admin/settings", true},
			{"/user/api", false},
			{"/ap", false},
			{"", false},
		}
		for _, tc := range cases {
			fact := Fact{Path: tc.path, Domain: "s.test"}
			_, got := snap.Evaluate(&fact, 1)
			if got != tc.want {
				t.Fatalf("path %q: got %v, want %v", tc.path, got, tc.want)
			}
		}
	})

	t.Run("ends_with 命中与大小写不敏感", func(t *testing.T) {
		r := store.Rule{
			ID: 2, Enabled: true, Scope: store.RuleScopeGlobal, Logic: store.RuleLogicAll, Action: store.RuleActionPass,
			Conditions: store.Conditions(store.RuleCondition{Field: FieldPath, Operator: OpEndsWith, Values: []string{".HTML", ".JSON"}}),
		}
		snap := NewSnapshot([]store.Rule{r}, discardLog)
		cases := []struct {
			path string
			want bool
		}{
			{"/docs/index.html", true},
			{"/docs/index.HTML", true},
			{"/api/data.json", true},
			{"/docs/index.htm", false},
			{".htm", false},
			{"", false},
		}
		for _, tc := range cases {
			fact := Fact{Path: tc.path, Domain: "s.test"}
			_, got := snap.Evaluate(&fact, 1)
			if got != tc.want {
				t.Fatalf("path %q: got %v, want %v", tc.path, got, tc.want)
			}
		}
	})

	t.Run("in_cidr 网段与单点匹配", func(t *testing.T) {
		r := store.Rule{
			ID: 3, Enabled: true, Scope: store.RuleScopeGlobal, Logic: store.RuleLogicAll, Action: store.RuleActionPass,
			Conditions: store.Conditions(store.RuleCondition{
				Field:    FieldIP,
				Operator: OpInCIDR,
				Values:   []string{"10.0.0.0/8", "192.168.1.0/24", "203.0.113.5", "2001:db8::/32"},
			}),
		}
		snap := NewSnapshot([]store.Rule{r}, discardLog)
		cases := []struct {
			ip   string
			want bool
		}{
			{"10.5.6.7", true},
			{"192.168.1.200", true},
			{"203.0.113.5", true}, // 单 IP 自动转 /32
			{"2001:db8::99", true},
			{"192.168.2.1", false},
			{"203.0.113.6", false},
			{"2001:db9::1", false},
			{"8.8.8.8", false},
			{"", false},
		}
		for _, tc := range cases {
			fact := Fact{IP: tc.ip, Domain: "s.test"}
			_, got := snap.Evaluate(&fact, 1)
			if got != tc.want {
				t.Fatalf("ip %q: got %v, want %v", tc.ip, got, tc.want)
			}
		}
	})

	t.Run("not_in_cidr IP白名单网段与单点匹配(白名单外命中阻断/白名单内放行)", func(t *testing.T) {
		// Given: 规则配置为 IP not_in_cidr 白名单网段与单点 IP，命中时执行 pass/阻断判定
		r := store.Rule{
			ID: 301, Enabled: true, Scope: store.RuleScopeGlobal, Logic: store.RuleLogicAll, Action: store.RuleActionNotfound,
			Conditions: store.Conditions(store.RuleCondition{
				Field:    FieldIP,
				Operator: OpNotInCIDR,
				Values:   []string{"10.0.0.0/8", "192.168.1.0/24", "203.0.113.5", "2001:db8::/32"},
			}),
		}
		snap := NewSnapshot([]store.Rule{r}, discardLog)

		// When & Then: 白名单内的 IP 不应命中规则(not_in_cidr 为 false)，白名单外的 IP 应该命中规则(not_in_cidr 为 true)
		cases := []struct {
			name string
			ip   string
			want bool // 是否命中规则
		}{
			{"白名单内 10/8 子网 IP 不命中", "10.5.6.7", false},
			{"白名单内 192.168.1/24 子网 IP 不命中", "192.168.1.200", false},
			{"白名单内 203.0.113.5 单点 IP 不命中", "203.0.113.5", false},
			{"白名单内 2001:db8::/32 v6 IP 不命中", "2001:db8::99", false},
			{"白名单外 192.168.2.1 命中拦截", "192.168.2.1", true},
			{"白名单外 203.0.113.6 命中拦截", "203.0.113.6", true},
			{"白名单外 2001:db9::1 命中拦截", "2001:db9::1", true},
			{"白名单外 公网 IP 8.8.8.8 命中拦截", "8.8.8.8", true},
			{"空 IP 画像恒不命中(避免无 IP 请求被误拦)", "", false},
		}
		for _, tc := range cases {
			fact := Fact{IP: tc.ip, Domain: "s.test"}
			_, got := snap.Evaluate(&fact, 1)
			if got != tc.want {
				t.Fatalf("case %s (ip %q): got %v, want %v", tc.name, tc.ip, got, tc.want)
			}
		}
	})

	t.Run("not_in_cidr 非 ip 字段在加载期丢弃", func(t *testing.T) {
		var buf bytes.Buffer
		lg := slog.New(slog.NewTextHandler(&buf, nil))
		r := store.Rule{
			ID: 302, Enabled: true, Scope: store.RuleScopeGlobal, Logic: store.RuleLogicAll, Action: store.RuleActionPass,
			Conditions: store.Conditions(store.RuleCondition{Field: FieldPath, Operator: OpNotInCIDR, Values: []string{"10.0.0.0/8"}}),
		}
		snap := NewSnapshot([]store.Rule{r}, lg)
		if len(snap.Rules) != 0 {
			t.Fatalf("非法字段的 not_in_cidr 规则该整条丢弃,剩 %d 条", len(snap.Rules))
		}
		if !strings.Contains(buf.String(), "not_in_cidr 运算符仅支持 ip 字段") {
			t.Fatalf("缺少丢弃日志: %s", buf.String())
		}
	})

	t.Run("in_cidr 非 ip 字段在加载期丢弃", func(t *testing.T) {
		var buf bytes.Buffer
		lg := slog.New(slog.NewTextHandler(&buf, nil))
		r := store.Rule{
			ID: 4, Enabled: true, Scope: store.RuleScopeGlobal, Logic: store.RuleLogicAll, Action: store.RuleActionPass,
			Conditions: store.Conditions(store.RuleCondition{Field: FieldPath, Operator: OpInCIDR, Values: []string{"10.0.0.0/8"}}),
		}
		snap := NewSnapshot([]store.Rule{r}, lg)
		if len(snap.Rules) != 0 {
			t.Fatalf("非法字段的 in_cidr 规则该整条丢弃,剩 %d 条", len(snap.Rules))
		}
		if !strings.Contains(buf.String(), "in_cidr 运算符仅支持 ip 字段") {
			t.Fatalf("缺少丢弃日志:\n%s", buf.String())
		}
	})

	t.Run("新运算符的仿真留痕文案准确", func(t *testing.T) {
		r := store.Rule{
			ID: 5, Enabled: true, Name: "扩展运算符规则", Scope: store.RuleScopeGlobal, Logic: store.RuleLogicAll, Action: store.RuleActionPass,
			Conditions: store.Conditions(
				store.RuleCondition{Field: FieldPath, Operator: OpStartsWith, Values: []string{"/api"}},
				store.RuleCondition{Field: FieldPath, Operator: OpEndsWith, Values: []string{".json"}},
				store.RuleCondition{Field: FieldIP, Operator: OpInCIDR, Values: []string{"10.0.0.0/8"}},
				store.RuleCondition{Field: FieldIP, Operator: OpNotInCIDR, Values: []string{"172.16.0.0/12"}},
			),
		}
		snap := NewSnapshot([]store.Rule{r}, discardLog)
		fact := Fact{Path: "/api/v1/users.json", IP: "10.1.2.3", Domain: "s.test"}
		res := snap.Simulate(&fact, 1, nil, nil)
		if !res.Matched {
			t.Fatalf("仿真应该命中: %+v", res.Verdict)
		}
		step := res.Steps[0]
		if len(step.Conditions) != 4 {
			t.Fatalf("条件留痕数 = %d, want 4", len(step.Conditions))
		}
		wantPhrases := []string{"以 /api 开头", "以 .json 结尾", "在网段 10.0.0.0/8 内", "不在网段 172.16.0.0/12 内"}
		for i, p := range wantPhrases {
			if !strings.Contains(step.Conditions[i].Description, p) {
				t.Errorf("条件 %d 描述 %q 未包含 %q", i, step.Conditions[i].Description, p)
			}
		}
	})
}

// BenchmarkEvaluateRadixCIDR 性能基准测试:验证 IPRadixTree 挂载到 Snapshot 求值链路后的大规模网段 0 分配性能
func BenchmarkEvaluateRadixCIDR(b *testing.B) {
	var cidrs []string
	for i := 0; i < 256; i++ {
		cidrs = append(cidrs, "10."+strconv.Itoa(i)+".0.0/16")
	}
	r := store.Rule{
		ID: 1, Enabled: true, Scope: store.RuleScopeGlobal, Logic: store.RuleLogicAll, Action: store.RuleActionPass,
		Conditions: store.Conditions(store.RuleCondition{Field: FieldIP, Operator: OpInCIDR, Values: cidrs}),
	}
	snap := NewSnapshot([]store.Rule{r}, discardLog)
	fact := Fact{IP: "10.42.1.2", Domain: "s.test"}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, ok := snap.Evaluate(&fact, 1)
		if !ok {
			b.Fatal("expected match")
		}
	}
}
