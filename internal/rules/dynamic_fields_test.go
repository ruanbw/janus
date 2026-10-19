package rules

// 动态字段契约:请求头 `header:<name>` 与 URL 查询参数 `query:<name>`。
//
// 这两个维度不在 D5 的 13 个固定字段里,而是"带名字的字段":字段名由租户填,
// 形如 header:X-Token / query:utm_content。本文件是它们唯一的规格。
//
// 三条必须先钉死、错了不会报错只会静默出事的边界:
//
//  1. 缺失语义:没带这个头/参数(或值为空串)一律 ok=false 且**恒不命中**,
//     不是"不匹配的反面"——一个没带 X-Token 的访客不能命中 `header:X-Token neq vip`。
//     这条不变式与 13 个固定字段完全一致(见 eval.go 包注释)。
//  2. 多值语义:同一个头/参数出现多次时,正向运算符取"任一值命中即命中",
//     否定运算符(neq / not_in / not_contains)取"所有值都不满足正向形式才成立"。
//     两个方向必须分开,否则 `X-Multi: alpha, beta` 上的 `neq alpha` 会错误命中。
//  3. 名字大小写:请求头名大小写不敏感(HTTP 规范),查询参数名大小写敏感(URL 规范)。
//     字段名里的前缀必须小写,`HEADER:X` 不是合法字段——同一件事只允许一种写法。

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ruanbw/janus/internal/store"
)

// dynCtx 一个带名字的访客画像形态。
type dynCtx struct {
	name string
	ctx  VisitorContext
}

// dynContexts 同一份请求的两种画像形态。两者必须给出同样的取值:
// 惰性上下文跑线上跳转热路径,Fact 快照跑富化器 / 前置拦截器 / 规则模拟器。
// 任何一侧漏了动态字段,"线上求值"与"诊断推演"就会分叉。
func dynContexts(t testing.TB, r *http.Request) []dynCtx {
	t.Helper()
	lazy := AcquireVisitorContext(r, "", "")
	t.Cleanup(func() { ReleaseVisitorContext(lazy) })
	return []dynCtx{
		{name: "LazyVisitorContext", ctx: lazy},
		{name: "Fact", ctx: FromRequest(r)},
	}
}

// dynRequest 一次"什么都有"的访问:单值头、多值头、空值头,
// 以及可解码 / 多值 / 大小写可区分的查询参数。
func dynRequest(t testing.TB) *http.Request {
	t.Helper()
	r := httptest.NewRequest(http.MethodGet,
		"https://shop.example.com/abc?tag=a&tag=b&name=%E5%BC%A0%E4%B8%89&empty=", nil)
	r.RemoteAddr = "203.0.113.9:5555"
	r.Header.Set("X-Promo-Code", "VIP2026")
	r.Header.Set("X-Count", "42")
	r.Header.Add("X-Multi", "alpha")
	r.Header.Add("X-Multi", "beta")
	r.Header.Set("X-Empty", "")
	return r
}

// bareDynRequest 一次"什么头都没带"的访问:动态字段全部取不到值。
func bareDynRequest(t testing.TB) *http.Request {
	t.Helper()
	return httptest.NewRequest(http.MethodGet, "https://shop.example.com/abc", nil)
}

// compileDyn 编译一条动态字段条件;编译不出来直接判失败。
func compileDyn(t testing.TB, field, operator string, values ...string) ConditionPredicate {
	t.Helper()
	pred, ok := CompilePredicate(store.ConditionLeafNode{
		Field:    field,
		Operator: operator,
		Values:   values,
	}, slog.Default())
	if !ok || pred == nil {
		t.Fatalf("CompilePredicate(%q %q) 失败,动态字段条件应当可以编译", field, operator)
	}
	return pred
}

// TestDynamicFieldExtraction 取值口径。
func TestDynamicFieldExtraction(t *testing.T) {
	r := dynRequest(t)
	for _, tc := range dynContexts(t, r) {
		// 请求头:名字大小写不敏感,取值原样返回
		// (比较时的大小写归一由字符串算子做,取值侧不改写用户的数据)
		for _, name := range []string{"header:X-Promo-Code", "header:x-promo-code", "header:x-PROMO-CODE"} {
			if v, ok := tc.ctx.Field(name); !ok || v != "VIP2026" {
				t.Errorf("%s: Field(%q) = (%q,%v), want (VIP2026,true)", tc.name, name, v, ok)
			}
		}
		// 前缀必须小写:`HEADER:` 不是合法字段,取值同样取不到
		if v, ok := tc.ctx.Field("HEADER:X-Promo-Code"); ok {
			t.Errorf("%s: Field(HEADER:X-Promo-Code) = (%q,true), 前缀必须小写", tc.name, v)
		}
		// 查询参数取 URL 解码后的值
		if v, ok := tc.ctx.Field("query:name"); !ok || v != "张三" {
			t.Errorf("%s: Field(query:name) = (%q,%v), want (张三,true)", tc.name, v, ok)
		}
		// 查询参数名大小写敏感(URL 规范)
		if v, ok := tc.ctx.Field("query:Tag"); ok {
			t.Errorf("%s: Field(query:Tag) = (%q,true), 查询参数名应当大小写敏感", tc.name, v)
		}
		// 多值字段:Field 给出其中一个非空值(命中语义见 TestDynamicFieldMultiValue)
		if v, ok := tc.ctx.Field("query:tag"); !ok || (v != "a" && v != "b") {
			t.Errorf("%s: Field(query:tag) = (%q,%v), want a 或 b 之一", tc.name, v, ok)
		}
		// 缺失 → ok=false(不是"取到空串")
		for _, name := range []string{"header:X-Missing", "query:missing"} {
			if v, ok := tc.ctx.Field(name); ok {
				t.Errorf("%s: 缺失的 %s = (%q,true), want ok=false", tc.name, name, v)
			}
		}
		// 存在但值为空串 → 同样是"没有数据"
		for _, name := range []string{"header:X-Empty", "query:empty"} {
			if v, ok := tc.ctx.Field(name); ok {
				t.Errorf("%s: 空值的 %s = (%q,true), 空值应当视为没有数据", tc.name, name, v)
			}
		}
	}
}

// TestDynamicFieldMultiValue 多值字段的命中语义。
func TestDynamicFieldMultiValue(t *testing.T) {
	r := dynRequest(t)
	cases := []struct {
		name     string
		field    string
		operator string
		values   []string
		want     bool
	}{
		// 正向运算符:任一值命中即命中
		{"多值 eq 命中其中一个", "header:X-Multi", OpEq, []string{"beta"}, true},
		{"多值 eq 都不命中", "header:X-Multi", OpEq, []string{"gamma"}, false},
		{"多值 in 命中其中一个", "header:X-Multi", OpIn, []string{"gamma", "beta"}, true},
		{"多值 contains 命中其中一个", "header:X-Multi", OpContains, []string{"alph"}, true},
		{"多值 starts_with 命中其中一个", "header:X-Multi", OpStartsWith, []string{"beta"}, true},
		{"多值 ends_with 命中其中一个", "header:X-Multi", OpEndsWith, []string{"alpha"}, true},
		{"多值 regex 命中其中一个", "header:X-Multi", OpRegex, []string{"^beta$"}, true},
		{"多值 query in 命中其中一个", "query:tag", OpIn, []string{"z", "b"}, true},
		{"多值 query eq 都不命中", "query:tag", OpEq, []string{"c"}, false},
		// 否定运算符:所有值都不满足正向形式才成立
		{"多值 neq 有一个值相等即不成立", "header:X-Multi", OpNeq, []string{"alpha"}, false},
		{"多值 not_in 有一个值在集合内即不成立", "header:X-Multi", OpNotIn, []string{"alpha"}, false},
		{"多值 not_contains 有一个值包含即不成立", "header:X-Multi", OpNotContains, []string{"alph"}, false},
		{"多值 neq 所有值都不等才成立", "header:X-Multi", OpNeq, []string{"gamma"}, true},
		{"多值 not_in 所有值都不在集合内才成立", "header:X-Multi", OpNotIn, []string{"gamma", "delta"}, true},
		{"多值 not_contains 所有值都不包含才成立", "header:X-Multi", OpNotContains, []string{"zzz"}, true},
		{"多值 query not_in 有一个值在集合内即不成立", "query:tag", OpNotIn, []string{"a"}, false},
	}
	for _, tc := range cases {
		for _, c := range dynContexts(t, r) {
			pred := compileDyn(t, tc.field, tc.operator, tc.values...)
			if got := pred.Match(c.ctx); got != tc.want {
				t.Errorf("%s/%s: %s %s %v = %v, want %v",
					tc.name, c.name, tc.field, tc.operator, tc.values, got, tc.want)
			}
		}
	}
}

// TestDynamicFieldMissingNeverMatches 没有数据一律恒不命中,与运算符无关。
//
// 这是本引擎最关键的不变式:空值既不是"匹配",也不是"不匹配的反面"。
func TestDynamicFieldMissingNeverMatches(t *testing.T) {
	ops := []struct {
		operator string
		values   []string
	}{
		{OpIn, []string{"x"}},
		{OpNotIn, []string{"x"}},
		{OpEq, []string{"x"}},
		{OpNeq, []string{"x"}},
		{OpContains, []string{"x"}},
		{OpNotContains, []string{"x"}},
		{OpStartsWith, []string{"x"}},
		{OpEndsWith, []string{"x"}},
		{OpRegex, []string{"^x"}},
		{OpGT, []string{"1"}},
		{OpLT, []string{"1"}},
	}
	// 两种"没有数据"的请求:完全没带、带了但值为空串
	requests := []struct {
		name string
		req  *http.Request
	}{
		{"没带这个头", bareDynRequest(t)},
		{"带了但值为空", dynRequest(t)},
	}
	fields := []string{
		"header:X-Missing",
		"query:missing",
		"header:X-Empty",
		"query:empty",
		"query:Tag", // 参数名大小写不符,等同于没有这个参数
	}
	for _, req := range requests {
		for _, c := range dynContexts(t, req.req) {
			for _, field := range fields {
				for _, op := range ops {
					pred := compileDyn(t, field, op.operator, op.values...)
					if pred.Match(c.ctx) {
						t.Errorf("%s/%s: 无数据的 %s %s %v = 命中, want 恒不命中",
							req.name, c.name, field, op.operator, op.values)
					}
					tr := pred.Trace(c.ctx)
					if tr.Available || tr.Matched {
						t.Errorf("%s/%s: %s %s 留痕 = (available=%v,matched=%v), want 都 false",
							req.name, c.name, field, op.operator, tr.Available, tr.Matched)
					}
				}
			}
		}
	}
}

// TestValidDynamicFieldName 动态字段名的语法。
//
// 落库前就把写错的名字拒掉:一个能存进库、却永远取不到值的字段,
// 就是"能配不能跑"的假能力。查询参数名一律按**解码后**的口径写
// (即租户在地址栏里看到的中文,而不是 %E4%B8%AD%E6%96%87),
// 含歧义的 % / + 一律拒绝——宁可报错,也不要静默不命中。
func TestValidDynamicFieldName(t *testing.T) {
	longest := strings.Repeat("a", 256)
	tooLong := strings.Repeat("a", 257)

	valid := []string{
		"header:X-Token",
		"header:x-forwarded-for",
		"header:X_Promo.Code", // RFC 7230 token 字符
		"header:a",
		"header:" + longest,
		"query:utm_content",
		"query:lang",
		"query:名字", // 查询参数名允许 UTF-8
		"query:a.b-c_d~e",
		"query:" + longest,
	}
	for _, field := range valid {
		if !ValidField(field) {
			t.Errorf("ValidField(%q) = false, want true", field)
		}
	}

	invalid := []string{
		"header:",          // 空名字
		"query:",           // 空名字
		"HEADER:X-Token",   // 前缀必须小写
		"Query:lang",       //
		"Header:x",         //
		"header",           // 没有前缀
		"query",            // 没有前缀
		"cookie:session",   // 未支持的前缀
		"header: bad",      // 前导空格
		"header:bad name",  // 名字里有空格
		"header:X-Token\n", // 控制字符
		"header:x\ttoken",  //
		"header:a:b",       // 冒号不是合法的头名字符
		"header:名字",        // 头名必须是 ASCII token
		"header:" + tooLong,
		"query:" + tooLong,
		"query: bad",
		"query:a b",
		"query:a&b", // 解码后不可能出现的参数名
		"query:a=b",
		"query:a#b",
		"query:a?b",
		"query:a%20b", // 名字按解码后写,不接受百分号编码
		"query:a+b",   // + 在查询串里等于空格,写它只会永远取不到值
	}
	for _, field := range invalid {
		if ValidField(field) {
			t.Errorf("ValidField(%q) = true, want false", field)
		}
	}

	// 13 个固定字段不受影响
	for _, field := range []string{FieldIP, FieldPath, FieldDomain, FieldUA} {
		if !ValidField(field) {
			t.Errorf("ValidField(%q) = false, 固定字段不该被动态字段改动波及", field)
		}
	}
}

// TestDynamicFieldCIDROperatorsNotCompilable CIDR 运算符只属于 ip 字段。
//
// 动态字段落上 in_cidr 必须编译失败(规则被丢弃并记日志),
// 而不是退化成恒真/恒假的静默行为。API 层同样会拒(见 httpapi 用例)。
func TestDynamicFieldCIDROperatorsNotCompilable(t *testing.T) {
	for _, field := range []string{"header:X-Forwarded-For", "query:ip"} {
		for _, op := range []string{OpInCIDR, OpNotInCIDR} {
			pred, ok := CompilePredicate(store.ConditionLeafNode{
				Field:    field,
				Operator: op,
				Values:   []string{"10.0.0.0/8"},
			}, slog.Default())
			if ok || pred != nil {
				t.Errorf("CompilePredicate(%s %s) = (%v,%v), want 编译失败", field, op, pred, ok)
			}
		}
	}
}

// TestDynamicFieldFactSnapshot Fact 快照路径(富化器 / 前置拦截器 / 规则模拟器)
// 也必须能取到动态字段,且回显用的 13 字段表不扩大。
func TestDynamicFieldFactSnapshot(t *testing.T) {
	fact := FromRequest(dynRequest(t))
	if v, ok := fact.Field("header:X-Promo-Code"); !ok || v != "VIP2026" {
		t.Fatalf("Fact.Field(header:X-Promo-Code) = (%q,%v), want (VIP2026,true)", v, ok)
	}

	// WithIP 只改来源 IP,不能把请求头 / 查询串的来源丢掉
	swapped := fact.WithIP("198.51.100.7")
	if v, ok := swapped.Field("header:X-Promo-Code"); !ok || v != "VIP2026" {
		t.Errorf("WithIP 之后取不到请求头 = (%q,%v), want (VIP2026,true)", v, ok)
	}
	if v, ok := swapped.Field("query:name"); !ok || v != "张三" {
		t.Errorf("WithIP 之后取不到查询参数 = (%q,%v), want (张三,true)", v, ok)
	}

	// 诊断接口回显的 13 个字段表不扩大:动态字段由决策链留痕呈现,不塞进这张表
	fields := fact.Fields()
	if len(fields) != 13 {
		t.Errorf("Fact.Fields() 长度 = %d, want 13", len(fields))
	}
	for name := range fields {
		if strings.HasPrefix(name, "header:") || strings.HasPrefix(name, "query:") {
			t.Errorf("Fact.Fields() 含动态字段 %q, 13 字段契约不该扩大", name)
		}
	}
}

// TestDynamicFieldCostForPipelineOrdering 动态字段的取值要便宜到能排在正则之前,
// 成本感知的流水线排序才不会被这两个新字段破坏。
func TestDynamicFieldCostForPipelineOrdering(t *testing.T) {
	headerPred := compileDyn(t, "header:X-Token", OpEq, "vip")
	queryPred := compileDyn(t, "query:tag", OpEq, "a")
	regexPred := compileDyn(t, "header:X-Token", OpRegex, "^v")

	if headerPred.Cost() < 1 || headerPred.Cost() > 2 {
		t.Errorf("请求头条件 Cost = %d, want 1~2", headerPred.Cost())
	}
	if queryPred.Cost() < 1 || queryPred.Cost() > 3 {
		t.Errorf("查询参数条件 Cost = %d, want 1~3", queryPred.Cost())
	}
	if headerPred.Cost() >= regexPred.Cost() {
		t.Errorf("请求头条件 Cost = %d 应当小于同一字段上的正则 %d", headerPred.Cost(), regexPred.Cost())
	}
}

// TestDynamicFieldTrace 模拟器留痕:缺数据要如实说"没有数据",有数据要回显实际值,
// 文案里还得带上具体是哪个头 / 哪个参数——只说"某字段没有数据",用户不知道指谁。
func TestDynamicFieldTrace(t *testing.T) {
	// 命中:回显实际值(eq 是精确相等,这里用 contains 才能命中 VIP2026 的前缀 vip)
	pred := compileDyn(t, "header:X-Promo-Code", OpContains, "vip")
	for _, c := range dynContexts(t, dynRequest(t)) {
		tr := pred.Trace(c.ctx)
		if !tr.Available || !tr.Matched {
			t.Errorf("%s: Trace = (available=%v,matched=%v), want 都 true", c.name, tr.Available, tr.Matched)
		}
		if tr.Actual != "VIP2026" {
			t.Errorf("%s: Trace.Actual = %q, want VIP2026", c.name, tr.Actual)
		}
		if !strings.Contains(tr.Description, "X-Promo-Code") {
			t.Errorf("%s: 留痕文案没指出是哪个请求头: %q", c.name, tr.Description)
		}
	}

	// 缺失:available=false,文案说明本次没带
	missing := compileDyn(t, "query:lang", OpEq, "en")
	for _, c := range dynContexts(t, bareDynRequest(t)) {
		tr := missing.Trace(c.ctx)
		if tr.Available || tr.Matched {
			t.Errorf("%s: 缺失时 Trace = (available=%v,matched=%v), want 都 false", c.name, tr.Available, tr.Matched)
		}
		if !strings.Contains(tr.Description, "lang") {
			t.Errorf("%s: 留痕文案没指出是哪个字段: %q", c.name, tr.Description)
		}
	}
}

// TestDynamicFieldNotAvailableInExpressions 表达式形态不支持动态字段。
//
// 表达式走的是固定字段的编译期 env(evaluator.go 的 exprFact),动态字段只属于可视化条件。
// 这里必须直接报错:若它能通过校验、却在加载期编译失败,用户看到的是
// "规则保存成功但从不生效"——一条被静默丢弃的规则比一条被拒绝的规则糟得多。
func TestDynamicFieldNotAvailableInExpressions(t *testing.T) {
	for _, code := range []string{`header:X-Token == "vip"`, `query:lang == "en"`} {
		if err := ValidateExpression(code); err == nil {
			t.Errorf("ValidateExpression(%q) = nil, want 报错(表达式不支持动态字段)", code)
		}
	}
}

// TestSimulateMultiValueTraceShowsEveryValue 多值字段的诊断留痕必须呈现**全部**取值。
//
// 留痕的 Actual 来自 ctx.Field(多值字段只给第一个非空值),而 Matched 来自
// (*compiledCond).match(遍历全部取值)—— 同一个条件会各说各话:
// `header:X-Multi eq beta` 在 X-Multi: alpha, beta 上会输出
// 「实际为「alpha」,满足条件(等于 beta)」,一句自相矛盾的诊断。
// 而同一个条件的谓词留痕(StringPredicate.Trace)用的是 fieldValuesText,拼成
// 「alpha, beta」—— 两条留痕实现各写各的(见 TestDynamicFieldTrace 钉的是后者)。
//
// 契约:Simulate 的 Actual 必须与 fieldValuesText 逐字一致,且与 Matched 自洽。
//
// Given: X-Multi 同时带 alpha 与 beta 的画像 + 一条多值字段条件
// When:  分别以扁平形态(裁决走谓词流水线)与等价树形态(裁决走 compiledCond)推演
// Then:  两种形态的 Actual 都必须是全部取值,Matched 都必须符合 any-of 语义。
func TestSimulateMultiValueTraceShowsEveryValue(t *testing.T) {
	fact := FromRequest(dynRequest(t)) // X-Multi: alpha, beta

	cases := []struct {
		name     string
		operator string
		values   []string
		want     bool
	}{
		{"eq 命中其中一个取值", OpEq, []string{"beta"}, true},
		{"eq 都不命中", OpEq, []string{"gamma"}, false},
		{"neq 有一个取值相等即不成立", OpNeq, []string{"beta"}, false},
		{"not_in 有一个取值在集合内即不成立", OpNotIn, []string{"alpha"}, false},
		{"not_contains 有一个取值被包含即不成立", OpNotContains, []string{"alph"}, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cond := store.RuleCondition{Field: "header:X-Multi", Operator: tc.operator, Values: tc.values}
			shapes := []struct {
				name string
				rule store.Rule
			}{
				{"扁平形态(谓词流水线)", oneRule(1, cond)},
				{"树形态(compiledCond)", oneTreeRule(1, store.Group("all", store.Leaf(cond)))},
			}
			for _, shape := range shapes {
				snap := NewSnapshot([]store.Rule{shape.rule}, discardLog)
				if len(snap.Rules) != 1 {
					t.Fatalf("[%s] 规则被整条丢弃,测试范围与实现不一致", shape.name)
				}
				res := snap.Simulate(&fact, 1, nil, nil)
				if res.Error != "" {
					t.Fatalf("[%s] 仿真报错: %s", shape.name, res.Error)
				}
				if len(res.Steps) != 1 || len(res.Steps[0].Conditions) != 1 {
					t.Fatalf("[%s] 留痕结构 = %+v, want 1 步 1 条件", shape.name, res.Steps)
				}
				tr := res.Steps[0].Conditions[0]

				wantActual, ok := fieldValuesText(&fact, cond.Field)
				if !ok {
					t.Fatalf("前置条件不成立:fieldValuesText 取不到 %s", cond.Field)
				}
				if tr.Actual != wantActual {
					t.Errorf("[%s] 留痕 Actual = %q, want %q(必须呈现全部取值,不能只给第一个)",
						shape.name, tr.Actual, wantActual)
				}
				if tr.Matched != tc.want {
					t.Errorf("[%s] 留痕 Matched = %v, want %v", shape.name, tr.Matched, tc.want)
				}
				// 自洽:报"满足"的留痕不得写出"不满足条件"的文案,反之亦然。
				if tr.Matched && strings.Contains(tr.Description, "不满足条件") {
					t.Errorf("[%s] 留痕自相矛盾(Matched=true 却写着不满足): %q", shape.name, tr.Description)
				}
				if !tr.Matched && strings.Contains(tr.Description, "满足条件") &&
					!strings.Contains(tr.Description, "不满足条件") {
					t.Errorf("[%s] 留痕自相矛盾(Matched=false 却写着满足): %q", shape.name, tr.Description)
				}
			}
		})
	}
}
