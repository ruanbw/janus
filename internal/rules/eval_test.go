package rules

// 条件求值的单元测试是纯函数测试,不需要数据库。

import (
	"bytes"
	"log/slog"
	"regexp"
	"strings"
	"testing"

	"cloak/internal/store"
)

// discardLog 静默日志器(编译期丢弃的告警不该污染测试输出)。
var discardLog = slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil))

// oneRule 造一条只含给定条件的规则。
func oneRule(id int64, conds ...store.RuleCondition) store.Rule {
	return store.Rule{
		ID: id, Name: "r", Enabled: true, Scope: store.RuleScopeGlobal,
		Logic: store.RuleLogicAll, Action: store.RuleActionNotfound,
		Priority: int(id), Conditions: conds,
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
		conds: []compiledCond{{field: FieldUA, op: OpRegex, res: []*regexp.Regexp{nil}}},
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
