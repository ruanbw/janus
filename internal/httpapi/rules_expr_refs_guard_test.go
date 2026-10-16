package httpapi_test

// 表达式字段准入闸门(审查项 COND3)的防御性测试。
//
// resolveRule 用 ExpressionFieldRefs 做"引用无数据源字段即拒"的准入校验;
// 旧实现在该函数报错时静默跳过整个校验(fail-open)。修复后是 fail-closed(400 + 日志)。
//
// 该错误在当前实现里不可达:ValidateExpression 与 ExpressionFieldRefs 走的是
// 同一套 compileExpression,前者通过则后者必不报错。本测试固化这条前提——
// 一旦编译路径分叉(前提被打破),它会先红,提醒 COND3 的 fail-closed 分支开始真正生效。

import (
	"testing"

	"janus/internal/rules"
)

// TestExpressionRefsAndValidationAgree 前提:ValidateExpression 通过 ⇒
// ExpressionFieldRefs 必不报错(否则准入闸门会对合法表达式误判 400)。
func TestExpressionRefsAndValidationAgree(t *testing.T) {
	cases := []string{
		`Country in ["US","CA"]`,
		`DevType == "bot" && Path contains "/x"`,
		`ip in_cidr "10.0.0.0/8"`,
		`true`,
		`Fields()["country"] == "US"`, // 方法调用:应被审计拒绝
		`$env.Country != "US"`,        // $env 访问器
		`foo bar`,                     // 语法错误
	}
	for _, expr := range cases {
		if err := rules.ValidateExpression(expr); err != nil {
			continue // 校验已失败:根本走不到 refs 闸门
		}
		if _, err := rules.ExpressionFieldRefs(expr); err != nil {
			t.Fatalf("表达式 %q 通过 ValidateExpression 但 refs 收集失败,"+
				"准入闸门会 fail-closed: %v", expr, err)
		}
	}
}
