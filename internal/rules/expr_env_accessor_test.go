package rules

import (
	"strings"
	"testing"

	"janus/internal/store"
)

// TestExprEnvAccessorRejectedAtCompileTime 是 C1($env 绕过 refs 闸门)的回归测试。
//
// 旧实现里 fieldCasePatcher 只认 13 个小写字段名的 IdentifierNode,而 `$env.Country`
// 在 AST 上是 MemberNode{Node: IdentifierNode("$env"), Property: StringNode("Country")},
// 属性名是字符串字面量不是标识符 —— 补丁器看不见,checker 也放行。
// 后果链:exprAudit 不拦 → expr.Compile 通过 → refs 为空 → 求值前"任一字段取不到即不命中"
// 的闸门空转 → 无 GeoIP 数据时 `$env.Country != "US"` 恒为 true。
//
// 断言的是"编译期失败":本引擎的 env 是零方法扁平结构体,$env 没有任何正当用途,
// 能让它编译成功就等于给下一个新增字段/方法重新开洞。
func TestExprEnvAccessorRejectedAtCompileTime(t *testing.T) {
	payloads := []struct {
		name string
		code string
	}{
		{"成员取反", `$env.Country != "US"`},
		{"成员判空", `$env.ASN != ""`},
		{"in 运算符探测字段", `"ASN" in $env`},
		{"下标访问", `$env["Country"] == "US"`},
		{"裸 $env", `$env == nil`},
		{"成员参与合取", `$env.Country == "US" && ua contains "bot"`},
	}
	for _, p := range payloads {
		t.Run(p.name, func(t *testing.T) {
			err := ValidateExpression(p.code)
			if err == nil {
				t.Fatalf("表达式 %q 必须编译失败:它绕过 refs 闸门,能写出无条件命中的规则", p.code)
			}
			if !strings.Contains(err.Error(), "$env") {
				t.Fatalf("报错应说明是 $env 访问器问题, got: %v", err)
			}
			// API 准入检查走的是 ExpressionFieldRefs,非法表达式必须返回 error
			// (空 refs 是合法状态,不能拿它当"没引用任何字段"放行)。
			if _, refsErr := ExpressionFieldRefs(p.code); refsErr == nil {
				t.Fatalf("ExpressionFieldRefs(%q) 应返回 error,而不是一个空列表", p.code)
			}
			// 整条规则不该参与求值
			if _, ok := compileRule(exprRule(1, p.code), discardLog); ok {
				t.Fatal("整条规则不该参与求值")
			}
			// 走完整快照 + 求值链路:空访客不得命中
			snap := NewSnapshot([]store.Rule{exprRule(1, p.code)}, discardLog)
			if _, matched := snap.Evaluate(&Fact{}, 1); matched {
				t.Fatalf("空访客命中了 %q —— 正是要堵的无条件命中", p.code)
			}
		})
	}
}

// TestExprEnvRejectionKeepsLegitimateFormsWorking 正向对照:拒绝 $env 不能误伤
// 既有合法写法,尤其是原本已依赖 `$` 前缀/`in` 运算符的表达式。
func TestExprEnvRejectionKeepsLegitimateFormsWorking(t *testing.T) {
	allowed := []string{
		`country != "US"`,
		`Country != "US"`,
		`in_cidr(ip, "10.0.0.0/8")`,
		`Country in ["US", "CA"]`,
		`"US" in ["US", "CA"]`,
		`ua contains "bot" && path startsWith "/x"`,
		`len(UA) > 3`,
	}
	for _, code := range allowed {
		if err := ValidateExpression(code); err != nil {
			t.Errorf("表达式 %q 应仍然合法: %v", code, err)
		}
	}
}
