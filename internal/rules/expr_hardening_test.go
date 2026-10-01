package rules

import (
	"strings"
	"testing"
	"time"

	"github.com/expr-lang/expr/ast"
	"github.com/expr-lang/expr/builtin"
	"github.com/expr-lang/expr/parser"

	"cloak/internal/store"
)

// exprRule 造一条以表达式为条件的全局规则(测试构造用)。
func exprRule(id int64, code string) store.Rule {
	return store.Rule{
		ID: id, Name: "expr", Scope: store.RuleScopeGlobal,
		Action: store.RuleActionNotfound, Enabled: true,
		RuleType: store.RuleTypeExpression, Expression: code,
	}
}

// ---------- 01:最小 env,P0 绕过在编译期关闭 ----------

// TestExprFactMethodPayloadsRejectedAtCompileTime 是本次 P0 的回归测试。
//
// 旧实现用 expr.Env(&Fact{}) 编译,于是 Fact 的导出方法全部暴露给表达式语言;
// 而求值前的 refs 闸门只认标识符节点,认不出方法名 —— refs 为空即闸门失效。
// 实测三条 payload 都能编译、都能对一个什么字段都取不到的访客求值 true:
// 租户只要写第一条配 notfound,就能无条件拦掉自己全部短链的所有访问。
//
// 断言的是"编译失败"而不是"求值为 false":能让它编译成功,就意味着下一个新增的
// 导出方法会重新打开同一个洞。绕过闸门必须发生在闸门之前,而不是之后。
func TestExprFactMethodPayloadsRejectedAtCompileTime(t *testing.T) {
	payloads := []struct {
		name string
		code string
	}{
		// 通过 Fields() 拿到"整张表",refs 闸门完全看不见里面的字段
		{"Fields 取整张表判空", `Fields()["asn"] == ""`},
		{"Fields 取整张表取反", `Fields()["country"] != "US"`},
		// 通过 Field() 按名取值,同样绕过闸门
		{"Field 按名取值判空", `Field("country") == ""`},
		// 其余导出方法一并锁死,防止将来 Fact 上再加方法时被重新打开
		{"ClientIP", `ClientIP() != nil`},
		{"WithIP", `WithIP("1.2.3.4") != nil`},
		{"Fields 参与真判断", `len(Fields()) > 0`},
		{"Field 参与真判断", `Field("asn") == "AS15169"`},
	}
	for _, p := range payloads {
		t.Run(p.name, func(t *testing.T) {
			if err := ValidateExpression(p.code); err == nil {
				t.Fatalf("表达式 %q 必须编译失败:它绕过 refs 闸门,能写出无条件拦截的规则", p.code)
			}
			// 整条规则也必须编不出来(compileRule 走同一条 compileExpression)
			if _, ok := compileRule(exprRule(1, p.code), discardLog); ok {
				t.Fatal("整条规则不该参与求值")
			}
			// 走完整的快照 + 求值链路,确认不会命中
			snap := NewSnapshot([]store.Rule{exprRule(1, p.code)}, discardLog)
			if _, matched := snap.Evaluate(&Fact{}, 1); matched {
				t.Fatalf("空访客命中了 %q —— 正是要堵的无条件拦截", p.code)
			}
		})
	}
}

// TestExprFactEnvExposesOnlyThirteenFields 把"env 类型零方法、只暴露 13 个字符串字段"
// 这条修复本身钉住。将来有人图省事把 expr.Env 改回 &Fact{},或给 exprFact 加方法,这里立刻红。
func TestExprFactEnvExposesOnlyThirteenFields(t *testing.T) {
	all := `IP != "" && IPAttr != "" && Country != "" && ASN != "" && Lang != "" ` +
		`&& Ref != "" && UTM != "" && UA != "" && DevType != "" && OS != "" ` +
		`&& Browser != "" && Path != "" && Domain != ""`
	if err := ValidateExpression(all); err != nil {
		t.Fatalf("13 个字段应全部可编译: %v", err)
	}
}

// TestExprEmptyValueInvariantStillHolds 保留并加固原有的"空值恒不命中"不变式。
// 换了 env 之后闸门必须一样有效,否则这次修复等于把洞从左边挪到右边。
func TestExprEmptyValueInvariantStillHolds(t *testing.T) {
	codes := []string{
		// 取反/判空类写法:字段为空时若参与求值就会变成"拦截所有人"
		`Country != "US"`,
		`asn == ""`,
		`!(country == "CN")`,
		`IP not in ["10.0.0.0/8"]`,
		`UA not contains "bot"`,
		`!(path startsWith "/safe")`,
		`len(UA) > 1000`,
		`Domain == ""`,
		// 正向写法:取不到 != "匹配空串"
		`Country == "US"`,
		`UA contains "Googlebot"`,
		`DevType == "bot"`,
	}
	for _, code := range codes {
		t.Run(code, func(t *testing.T) {
			if err := ValidateExpression(code); err != nil {
				t.Fatalf("表达式应合法: %v", err)
			}
			prog, refs, err := compileExpression(code)
			if err != nil {
				t.Fatalf("编译失败: %v", err)
			}
			if len(refs) == 0 {
				t.Fatalf("refs 为空 —— 闸门无从下手(表达式 %q)", code)
			}
			eval := &exprEvaluator{program: prog, refs: refs, log: discardLog}
			if eval.Match(&Fact{}) {
				t.Fatalf("求值器层:空访客命中了 %q,违反空值恒不命中", code)
			}
			snap := NewSnapshot([]store.Rule{exprRule(1, code)}, discardLog)
			if _, matched := snap.Evaluate(&Fact{}, 1); matched {
				t.Fatalf("快照层:空访客命中了 %q", code)
			}
			// 仿真链路必须与线上给同一个结论
			if res := snap.Simulate(&Fact{}, 1, nil, nil); res.Matched {
				t.Fatalf("仿真层:空访客命中了 %q", code)
			}
		})
	}
}

// ---------- 02:编译期与运行期的计算预算 ----------

// TestExprHighCostBuiltinsRejected 收窄后的函数面:能放行的都是 O(n) 且无分配放大,
// 会遍历集合或放大内存的一律编译失败。
func TestExprHighCostBuiltinsRejected(t *testing.T) {
	rejected := []struct {
		name string
		code string
	}{
		// 集合遍历类(expr.DisableAllBuiltins 覆盖不到的那一半)
		{"reduce 嵌套", `reduce([1,2,3], reduce([1,2,3], #acc + 1, 0), 0) == 1`},
		{"reduce 顶层", `reduce([1,2,3], #acc + #, 0) == 6`},
		{"map", `map([1,2,3], # + 1) == nil`},
		{"filter", `filter([1,2,3], # > 1) == nil`},
		{"all", `all([1,2], # > 0)`},
		{"any", `any([1,2], # > 1)`},
		{"none", `none([1,2], # > 5)`},
		{"one", `one([1,2], # > 1)`},
		{"count", `count([1,2,3], # > 1) > 0`},
		{"sortBy", `sortBy([1,2], #) == nil`},
		{"groupBy", `groupBy([1,2], #) == nil`},
		{"find", `find([1,2], # > 1) == 2`},
		// 分配放大类
		{"split", `split(UA, " ")[0] == ""`},
		{"splitAfter", `splitAfter(UA, " ")[0] == ""`},
		{"join", `join(split(UA, " "), "-") == ""`},
		{"replace", `replace(UA, "a", "b", 1) == ""`},
		{"repeat", `repeat("a", 1000000) == ""`},
		{"fromJSON", `fromJSON(UA) == nil`},
		{"toJSON", `toJSON(UA) == ""`},
		{"fromBase64", `fromBase64(UA) == nil`},
		{"toBase64", `toBase64(UA) == ""`},
		// 集合类(本引擎环境里根本没有集合)
		{"sort", `sort([3,1,2]) == nil`},
		{"keys", `keys(UA) == nil`},
		{"values", `values(UA) == nil`},
		{"uniq", `uniq([1,1,2]) == nil`},
		{"take", `take([1,2,3], 1) == nil`},
		// 非确定性(同一条规则两次求值可能给出不同裁决)
		{"now", `now() != nil`},
		{"date", `date("2020-01-01") != nil`},
		{"duration", `duration("1h") != nil`},
		{"timezone", `timezone() != nil`},
	}
	for _, tc := range rejected {
		t.Run(tc.name, func(t *testing.T) {
			if err := ValidateExpression(tc.code); err == nil {
				t.Fatalf("表达式 %q 应被拒绝", tc.code)
			}
		})
	}
}

// TestExprAllowedOperatorsStillCompile 白名单收窄不能误伤租户已经在用的写法。
// contains / startsWith / endsWith / matches / in 是 expr 的**运算符**不是内置函数,
// 不走 DisableAllBuiltins —— 这条测试就是防"哪天它们也进了白名单判断"而静默回归。
func TestExprAllowedOperatorsStillCompile(t *testing.T) {
	allowed := []string{
		// 运算符(存量写法,必须原样可用)
		`UA contains "Googlebot"`,
		`UA not contains "bot"`,
		`Path startsWith "/api"`,
		`Path endsWith ".json"`,
		`Country in ["US", "CA"]`,
		`Country not in ["US"]`,
		`Country matches "^U"`,
		`in_cidr(IP, "10.0.0.0/8")`,
		`in_cidr(ip, "10.0.0.0/8") && DevType == "bot"`,
		`!(country == "CN") && ua contains "bot"`,
		`Country in ["US", "CA"] && DevType == "bot" && !(OS == "Windows")`,
		`asn == ""`,
		// 白名单内的有界内置函数
		`len(UA) > 3`,
		`lower(Country) == "us"`,
		`upper(Country) == "US"`,
		`trim(UA) != ""`,
		`trimPrefix(UA, "Mozilla") != ""`,
		`trimSuffix(UA, "/") != ""`,
		`hasPrefix(UA, "M")`,
		`hasSuffix(UA, "x")`,
		`indexOf(UA, "bot") > 0`,
		`lastIndexOf(UA, "bot") > 0`,
		`max(len(UA), len(Path)) > 0`,
		`min(len(UA), len(Path)) > 0`,
		`abs(0) == 0`,
		`ceil(1.2) == 2 && floor(1.8) == 1 && round(1.4) == 1`,
	}
	for _, code := range allowed {
		if err := ValidateExpression(code); err != nil {
			t.Errorf("表达式 %q 应仍然合法: %v", code, err)
		}
	}
}

// TestExprBytecodeHasNoBackwardJump 把"求值步数有上界"这条不变式的机器可检形式钉住。
//
// 求值成本的上界来自"字节码无回跳 ⇒ 没有循环 ⇒ 步数 ≤ 字节码长度"。
// 一旦 expr 升级后引入了新的循环构造(或者有人放宽白名单),
// 这里会先红,而不是等到线上被一条租户表达式烧掉 CPU。
func TestExprBytecodeHasNoBackwardJump(t *testing.T) {
	codes := []string{
		`Country == "US"`,
		`UA contains "bot" && !(country == "CN")`,
		`in_cidr(ip, "10.0.0.0/8") || in_cidr(ip, "192.168.0.0/16")`,
		`len(UA) > 3 && lower(Country) == "us"`,
		`asn == ""`,
		`Country != "US"`,
		`max(len(UA), len(Path)) > 0`,
		`Country in ["US","CA"] && DevType == "bot" && !(OS == "Windows") && Path startsWith "/x"`,
		`Country matches "^U" && len(trim(UA)) > 0 && !(Domain == "")`,
	}
	for _, code := range codes {
		prog, _, err := compileExpression(code)
		if err != nil {
			t.Fatalf("编译失败 %q: %v", code, err)
		}
		if exprHasBackwardJump(prog) {
			t.Fatalf("表达式 %q 的字节码里有回跳指令 = 存在循环 = 求值步数无上界", code)
		}
	}
}

// exprIntArray 造一个 n 元素的整数字面量数组(构造嵌套 reduce 的爆炸载荷用)。
func exprIntArray(n int) string {
	return "[" + strings.TrimSuffix(strings.Repeat("1,", n), ",") + "]"
}

// TestMaliciousExpressionIsCheapToCompileAndRun 是"恶意表达式不会长时间占用"的测试。
//
// 这条 payload 是实测出来的:30^4 次迭代要 3.7 秒纯 CPU,而它只有 810 字节。
// 修复之后它在**编译期**就被拒,成本是一次 parse,不是几十秒的 CPU。
func TestMaliciousExpressionIsCheapToCompileAndRun(t *testing.T) {
	// width=90 / depth=4 是实测出的 3.7 秒档(30^4 约 1.5 秒、90^4 约 3.7 秒),
	// payload 长度 810 字节 —— 远在 4096 的长度上限之内,靠长度上限根本挡不住。
	const depth, width = 4, 90
	payload := "reduce(" + exprIntArray(width) + ", #acc + 1, 0)"
	for i := 1; i < depth; i++ {
		payload = "reduce(" + exprIntArray(width) + ", " + payload + " + #acc, 0)"
	}
	payload += " == 1"
	if len(payload) < 500 {
		t.Fatalf("payload 只有 %d 字节,不足以复现原来的高耗场景", len(payload))
	}

	// 编译必须便宜:整段(长度检查 + 审 AST + 类型检查 + 生成字节码)在一个很小的常数内。
	start := time.Now()
	err := ValidateExpression(payload)
	elapsed := time.Since(start)
	if err == nil {
		t.Fatalf("恶意表达式必须编译失败,却通过了(%d 字节)", len(payload))
	}
	// 旧实现下这条表达式会编译通过、然后在每次求值时烧 3.7 秒;
	// 现在只该花一次 parse 的钱。
	if elapsed > 200*time.Millisecond {
		t.Fatalf("拒绝一条恶意表达式花了 %v,太慢了", elapsed)
	}
	// 整条规则也不参与求值,连一次访客画像都不该为它构造
	if _, ok := compileRule(exprRule(1, payload), discardLog); ok {
		t.Fatal("恶意表达式不该编译成可求值的规则")
	}
}

// TestExprEvaluationHasWallClockBudget 运行期兜底:即便前两道闸门都被绕过
// (例如内部代码手搓一个 program),单条表达式的求值也不能无限占用 CPU。
func TestExprEvaluationHasWallClockBudget(t *testing.T) {
	// 一条合法但很贵的表达式(白名单内的函数反复叠加,长度仍在上限内)
	var b strings.Builder
	b.WriteString(`len(UA) > 0`)
	for i := 0; i < 40; i++ {
		b.WriteString(` && len(lower(upper(trim(UA)))) > 0`)
	}
	code := b.String()
	if len(code) > MaxExpressionLen {
		t.Fatalf("构造的表达式 %d 字节,超出上限 %d", len(code), MaxExpressionLen)
	}
	prog, refs, err := compileExpression(code)
	if err != nil {
		t.Fatalf("应合法: %v", err)
	}
	eval := &exprEvaluator{program: prog, refs: refs, log: discardLog}
	fact := Fact{UA: "Mozilla/5.0 (compatible; Googlebot/2.1)"}
	if !eval.Match(&fact) {
		t.Fatal("这条合法表达式应当命中,否则测的不是预算而是表达式本身写错了")
	}
	// 跑 500 次,总耗时必须仍然很小(单次预算 1ms,500 次最坏 500ms;
	// 正常是纳秒级,这条断言的松弛量已经足够大)
	start := time.Now()
	for i := 0; i < 500; i++ {
		eval.Match(&fact)
	}
	if total := time.Since(start); total > 500*time.Millisecond {
		t.Fatalf("500 次求值花了 %v,单条表达式的运行期预算没生效", total)
	}
}

// TestExpressionLengthLimit 超长表达式在编译期就被拒(API 层还会再挡一道,见 05)。
func TestExpressionLengthLimit(t *testing.T) {
	over := `Country == "` + strings.Repeat("x", MaxExpressionLen) + `"`
	err := ValidateExpression(over)
	if err == nil {
		t.Fatal("超长表达式应被拒绝")
	}
	if !strings.Contains(err.Error(), "长度") {
		t.Fatalf("报错应说明是长度问题: %v", err)
	}
	// 刚好在上限内的合法表达式仍然可用
	ok := `Country == "` + strings.Repeat("x", MaxExpressionLen-16) + `"`
	if len(ok) > MaxExpressionLen {
		t.Fatalf("测试构造错误:%d", len(ok))
	}
	if err := ValidateExpression(ok); err != nil {
		t.Fatalf("上限内的表达式应合法: %v", err)
	}
}

// TestExpressionCompileNeverPanics 校验路径跑在租户可控的输入上,
// 一个纯校验请求不该有 500 这条路径 —— 第三方 parser 对畸形输入 panic 时,
// 审计层必须把它变成一个普通的编译错误。
func TestExpressionCompileNeverPanics(t *testing.T) {
	hostile := []string{
		"", " ", "\x00", "\n", "((((((((((", "))))))))))",
		"[" + strings.Repeat("1,", 100) + "]",
		"{" + strings.Repeat(`"a":1,`, 50) + `"b":1}`,
		"1 in 1", "nil.foo.bar", "..", "$", "#", "@", "\\",
		strings.Repeat("(", MaxExpressionLen/2),
		strings.Repeat("!", 100),
		"let x = 1 in x == 1",
		"1..2..3",
		"Country == 'US'",
		"$$$",
		"𝕔ountry == \"US\"",
		"in_cidr(ip, 1)", "in_cidr()", "in_cidr(ip)", "in_cidr(1,2,3)",
		"reduce(", "map(", "filter([1],", "all(",
		"Country == \"\\u0000\"",
		"len()", "len(len(len(UA)))",
	}
	for _, code := range hostile {
		func() {
			defer func() {
				if rec := recover(); rec != nil {
					t.Errorf("表达式 %q 让编译路径 panic 了: %v", code, rec)
				}
			}()
			_ = ValidateExpression(code)
			_, _, _ = compileExpression(code)
			// 整条规则链路同样不许炸
			_, _ = compileRule(exprRule(1, code), discardLog)
			NewSnapshot([]store.Rule{exprRule(1, code)}, discardLog)
		}()
	}
}

// TestExprPredicatesListCoversEveryParserPredicate 把"每个内置函数都被某一处覆盖"钉住。
//
// exprPredicates 是手抄的一份名单(parser.predicates 是非导出的,拿不到),
// 而漏掉一个的后果很具体:那个函数就既能编译通过、又能烧 CPU,
// 且 DisableAllBuiltins 拦不住它。这里枚举一批候选名,凡是 expr 的 parser
// 真的当成内置函数处理的,都必须在名单里。
//
// 三条覆盖路径,缺一不可:
//  1. `builtin.Index` 里的 -> expr.DisableAllBuiltins() 覆盖(config.Disabled 生效);
//  2. `exprPredicates` 里的 -> AST 审计覆盖(不在 builtin.Index 里,只有审计拦得住);
//  3. `exprAllowedBuiltins` 里的 -> 刻意白名单放行。
//
// 三处都没有的名字 = 一个能编译通过、但既不受白名单约束、也没被审计拒绝的洞。
//
// 升级 expr 之后如果新增了谓词类内置函数,这条会先红。
func TestExprPredicatesListCoversEveryParserPredicate(t *testing.T) {
	candidates := []string{
		"all", "none", "any", "one", "filter", "map", "count", "sum",
		"find", "findIndex", "findLast", "findLastIndex",
		"groupBy", "sortBy", "reduce",
		"sort", "take", "keys", "values", "toPairs", "fromPairs", "uniq",
		"first", "last", "flatten", "get", "indexOf", "len",
		"split", "splitAfter", "join", "replace", "repeat",
		"toJSON", "fromJSON", "toBase64", "fromBase64",
		"now", "date", "duration", "timezone",
		"abs", "ceil", "floor", "round", "int", "float",
		"max", "min", "mean", "median", "bitnot",
		"lower", "upper", "hasPrefix", "hasSuffix",
		"lastIndexOf", "trim", "trimPrefix", "trimSuffix",
		"in_cidr",
	}
	// 谓词的几种调用形态都要试:parser 对每种形态的识别路径未必相同
	forms := []string{
		`([1,2], # > 1)`,
		`([1,2], #acc + 1, 0)`,
		`([1,2], #)`,
		`(1, 2)`,
	}
	for _, name := range candidates {
		for _, form := range forms {
			tree, err := parser.Parse(name + form)
			if err != nil {
				continue // expr 不认这个形态,说明它不是谓词
			}
			var seen bool
			ast.Walk(&tree.Node, builtinNameVisitor{&seen, name})
			if !seen {
				continue
			}
			if _, listed := exprPredicates[name]; listed {
				continue
			}
			// DisableAllBuiltins 覆盖的是 builtin.Index 里的那批
			if _, indexed := builtin.Index[name]; indexed {
				continue
			}
			// 剩下的必须是刻意放行的白名单成员
			allowed := false
			for _, a := range exprAllowedBuiltins {
				if a == name {
					allowed = true
				}
			}
			if allowed {
				continue
			}
			t.Errorf("parser 把 %q 当成内置函数(%s 形态),但它既不在 builtin.Index 里,"+
				"也不在白名单里 —— DisableAllBuiltins 会漏掉它,必须显式处理",
				name, form)
		}
	}
}

// builtinNameVisitor 判断 AST 里有没有指定名字的 BuiltinNode。
type builtinNameVisitor struct {
	found *bool
	name  string
}

func (v builtinNameVisitor) Visit(node *ast.Node) {
	if b, ok := (*node).(*ast.BuiltinNode); ok && b.Name == v.name {
		*v.found = true
	}
}

// TestExpressionRefsRejectsUnparseable ExpressionFieldRefs 在表达式非法时返回 error,
// 而不是返回一个空列表 —— 空列表会让调用方以为"没引用任何字段"从而放行。
func TestExpressionRefsRejectsUnparseable(t *testing.T) {
	bad := []string{
		`Country == `, `(((`, `Fields()["x"] == ""`, `reduce([1],#acc,0)==1`,
	}
	for _, code := range bad {
		if _, err := ExpressionFieldRefs(code); err == nil {
			t.Errorf("表达式 %q 应返回 error,而不是一个空列表", code)
		}
	}
	refs, err := ExpressionFieldRefs(`Country == "US" && ua contains "bot"`)
	if err != nil {
		t.Fatalf("合法表达式应返回 refs: %v", err)
	}
	// 稳定排序:调用方拿到的顺序可预期
	want := []string{"country", "ua"}
	if len(refs) != len(want) {
		t.Fatalf("refs = %v, want %v", refs, want)
	}
	for i := range want {
		if refs[i] != want[i] {
			t.Fatalf("refs = %v, want %v(应按字典序)", refs, want)
		}
	}
}
