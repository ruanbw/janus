package rules

import (
	"strings"
	"testing"

	"cloak/internal/store"
)

func TestExprCompileAndEvaluate(t *testing.T) {
	cases := []struct {
		name       string
		expression string
		fact       Fact
		wantMatch  bool
	}{
		{
			name:       "大写字段与 in 运算符",
			expression: `Country in ["US", "CA"] && DevType == "bot"`,
			fact:       Fact{Country: "US", DevType: "bot"},
			wantMatch:  true,
		},
		{
			name:       "大写字段条件不匹配",
			expression: `Country in ["US", "CA"] && DevType == "bot"`,
			fact:       Fact{Country: "GB", DevType: "bot"},
			wantMatch:  false,
		},
		{
			name:       "小写字段别名与 startsWith / endsWith",
			expression: `path startsWith "/api" && path endsWith ".json"`,
			fact:       Fact{Path: "/api/v1/users.json"},
			wantMatch:  true,
		},
		{
			name:       "UA 包含与逻辑非",
			expression: `ua contains "Googlebot" && !(country == "CN")`,
			fact:       Fact{UA: "Mozilla/5.0 (compatible; Googlebot/2.1)", Country: "US"},
			wantMatch:  true,
		},
		{
			name:       "内置 in_cidr 运算符匹配",
			expression: `in_cidr(ip, "10.0.0.0/8") || in_cidr(ip, "192.168.0.0/16")`,
			fact:       Fact{IP: "192.168.1.100"},
			wantMatch:  true,
		},
		{
			name:       "内置 in_cidr 运算符未匹配",
			expression: `in_cidr(ip, "10.0.0.0/8")`,
			fact:       Fact{IP: "172.16.0.1"},
			wantMatch:  false,
		},
		// 以下三条锁定与 Tier-1 相同的不变式:字段取不到时恒不命中。
		// 取反写法(Country != "US")在 country 为空时若参与求值会变成"拦截所有人";
		// asn 当前无数据源(fields.go:恒空),`asn == ""` 恒真等于无条件拦截。
		{
			name:       "取不到 country 时取反写法不得命中",
			expression: `Country != "US"`,
			fact:       Fact{UA: "Mozilla/5.0"},
			wantMatch:  false,
		},
		{
			name:       "asn 无数据源时等空判定不得命中",
			expression: `asn == ""`,
			fact:       Fact{UA: "Mozilla/5.0"},
			wantMatch:  false,
		},
		{
			name:       "引用的任一字段取不到则整体不命中(fail-open)",
			expression: `ua contains "Googlebot" && !(country == "CN")`,
			fact:       Fact{UA: "Mozilla/5.0 (compatible; Googlebot/2.1)"},
			wantMatch:  false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := store.Rule{
				ID: 1, Name: tc.name, Scope: store.RuleScopeGlobal, Action: store.RuleActionPass,
				RuleType: store.RuleTypeExpression, Expression: tc.expression, Enabled: true,
			}
			snap := NewSnapshot([]store.Rule{r}, discardLog)
			_, matched := snap.Evaluate(&tc.fact, 1)
			if matched != tc.wantMatch {
				t.Fatalf("Evaluate matched = %v, want %v", matched, tc.wantMatch)
			}
		})
	}
}

func TestExprValidation(t *testing.T) {
	// 合法表达式
	if err := ValidateExpression(`Country == "US"`); err != nil {
		t.Fatalf("合法表达式校验报错: %v", err)
	}
	if err := ValidateExpression(`in_cidr(ip, "10.0.0.0/8") && devtype != "bot"`); err != nil {
		t.Fatalf("合法表达式校验报错: %v", err)
	}

	// 语法错误
	if err := ValidateExpression(`Country == `); err == nil {
		t.Fatal("语法错误应校验失败")
	}

	// 返回值非 bool
	if err := ValidateExpression(`Country + "suffix"`); err == nil {
		t.Fatal("返回值非 bool 应校验失败")
	}

	// 未知字段名
	if err := ValidateExpression(`unknown_field == "test"`); err == nil {
		t.Fatal("未知字段应校验失败")
	}
}

func TestExprFailOpenOnRuntimeError(t *testing.T) {
	// 在表达式内制造异常除零或者类型异常
	prog, err := CompileExpression(`1 / (Country == "US" ? 1 : 0) == 1`)
	if err != nil {
		t.Fatalf("编译失败: %v", err)
	}
	eval := &exprEvaluator{program: prog, log: discardLog}

	// 触发除零异常:Country != "US" => 1/0
	fact := Fact{Country: "CN"}
	matched := eval.Match(&fact)
	if matched {
		t.Fatal("运行时除零异常应按 fail-open(未命中)处理,不应命中")
	}
}

func TestSnapshotSimulateExprRule(t *testing.T) {
	r := store.Rule{
		ID: 10, Name: "Expr 仿真测试", Scope: store.RuleScopeGlobal, Action: store.RuleActionRedirect,
		Destination: "https://expr.example.com", RuleType: store.RuleTypeExpression,
		Expression: `Country in ["US", "CA"] && path startsWith "/docs"`, Enabled: true,
	}
	snap := NewSnapshot([]store.Rule{r}, discardLog)

	// 命中仿真
	factHit := Fact{Country: "US", Path: "/docs/readme"}
	resHit := snap.Simulate(&factHit, 1, nil, nil)
	if !resHit.Matched {
		t.Fatalf("仿真应命中: %+v", resHit.Verdict)
	}
	if len(resHit.Steps) != 1 || resHit.Steps[0].Status != StepStatusHit {
		t.Fatalf("仿真步骤异常: %+v", resHit.Steps)
	}
	if !strings.Contains(resHit.Steps[0].Reason, "表达式求值结果为 true") {
		t.Errorf("仿真理由未说明表达式: %s", resHit.Steps[0].Reason)
	}

	// 未命中仿真
	factMiss := Fact{Country: "CN", Path: "/docs/readme"}
	resMiss := snap.Simulate(&factMiss, 1, nil, nil)
	if resMiss.Matched {
		t.Fatal("仿真不应命中")
	}
	if len(resMiss.Steps) != 1 || resMiss.Steps[0].Status != StepStatusSkip {
		t.Fatalf("仿真步骤异常: %+v", resMiss.Steps)
	}
	if !strings.Contains(resMiss.Steps[0].Reason, "表达式求值结果为 false") {
		t.Errorf("仿真理由未说明表达式: %s", resMiss.Steps[0].Reason)
	}
}

func BenchmarkEvaluateExprRule(b *testing.B) {
	r := store.Rule{
		ID: 1, Scope: store.RuleScopeGlobal, Action: store.RuleActionPass,
		RuleType: store.RuleTypeExpression, Expression: `Country in ["US", "CA"] && DevType == "bot"`,
		Enabled: true,
	}
	snap := NewSnapshot([]store.Rule{r}, discardLog)
	fact := Fact{Country: "US", DevType: "bot"}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, ok := snap.Evaluate(&fact, 1)
		if !ok {
			b.Fatal("expected match")
		}
	}
}
