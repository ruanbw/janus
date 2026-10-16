package rules

import (
	"testing"

	"janus/internal/store"
)

// TestIPRadixPredicateNilContext 钉住 IP 谓词的 nil 上下文防御,与其余谓词对齐。
//
// StringPredicate / 数值 / 正则 / duplicated 三个谓词的 Match 都在第一行判 ctx == nil,
// 而 IPRadixPredicate.Match 直接 ctx.ClientIP() —— nil 接口调用会 panic。
// 更重要的是"没有数据恒不命中"这条不变式:否定运算符(not_in_cidr / neq)
// 绝不能在"拿不到上下文"时被取反成命中。
func TestIPRadixPredicateNilContext(t *testing.T) {
	for _, op := range []string{OpInCIDR, OpNotInCIDR, OpNeq, OpNotIn} {
		t.Run(op, func(t *testing.T) {
			pred, ok := compileIPRadix(store.ConditionLeafNode{
				Field: FieldIP, Operator: op, Values: []string{"10.0.0.0/8"},
			}, discardLog)
			if !ok {
				t.Fatalf("前置条件不成立:%s 条件应编译成 IP 谓词", op)
			}
			func() {
				defer func() {
					if rec := recover(); rec != nil {
						t.Fatalf("%s 谓词对 nil 上下文 panic: %v", op, rec)
					}
				}()
				if pred.Match(nil) {
					t.Fatalf("%s 谓词对 nil 上下文不应命中(取不到数据恒不命中)", op)
				}
			}()
		})
	}

	// 正向对照:有上下文且地址落在网段内时行为不变
	pred, ok := compileIPRadix(store.ConditionLeafNode{
		Field: FieldIP, Operator: OpInCIDR, Values: []string{"10.0.0.0/8"},
	}, discardLog)
	if !ok {
		t.Fatal("IP 条件应编译成功")
	}
	if !pred.Match(Fact{IP: "10.1.2.3"}) {
		t.Fatal("10.1.2.3 应落在 10.0.0.0/8 内")
	}
}
