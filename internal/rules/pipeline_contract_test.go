package rules

import (
	"fmt"
	"log/slog"
	"net/netip"
	"strings"
	"sync/atomic"
	"testing"

	"janus/internal/store"
)

// mockTestContext 是专门为谓词与流水线测试构造的简易 VisitorContext。
type mockTestContext struct {
	ip     netip.Addr
	fields map[string]string
}

func (m *mockTestContext) ClientIP() netip.Addr {
	return m.ip
}

func (m *mockTestContext) Field(name string) (string, bool) {
	if m.fields == nil {
		return "", false
	}
	val, ok := m.fields[name]
	return val, ok
}

// countingPredicate 是一个可计数的测试谓词算子，用来精确验证流水线的短路（Short-Circuit）行为。
type countingPredicate struct {
	name       string
	cost       int
	result     bool
	matchCalls *int32
}

func (p *countingPredicate) Type() string { return p.name }
func (p *countingPredicate) Cost() int    { return p.cost }
func (p *countingPredicate) Match(_ VisitorContext) bool {
	if p.matchCalls != nil {
		atomic.AddInt32(p.matchCalls, 1)
	}
	return p.result
}
func (p *countingPredicate) Trace(ctx VisitorContext) ConditionTrace {
	return ConditionTrace{
		Field:       p.name,
		Operator:    "test",
		Matched:     p.result,
		Available:   true,
		Description: fmt.Sprintf("%s trace", p.name),
	}
}

// TestPredicatePipelineShortCircuit_And 验证流水线在 AND (logicAll=true) 模式下的短路特性。
// Given: 流水线按顺序包含 [true算子, false算子, true算子]
// When:  执行 Match(ctx)
// Then:  在遇到第一个 false 算子后立即返回 false，后续的算子绝不被触发求值。
func TestPredicatePipelineShortCircuit_And(t *testing.T) {
	var c1, c2, c3 int32
	p1 := &countingPredicate{name: "p1", cost: 1, result: true, matchCalls: &c1}
	p2 := &countingPredicate{name: "p2", cost: 2, result: false, matchCalls: &c2}
	p3 := &countingPredicate{name: "p3", cost: 3, result: true, matchCalls: &c3}

	pipeline := NewConditionPipeline(true, []ConditionPredicate{p1, p2, p3})
	matched := pipeline.Match(&mockTestContext{})

	if matched {
		t.Fatalf("pipeline.Match() = true, want false")
	}
	if atomic.LoadInt32(&c1) != 1 {
		t.Errorf("p1 调用次数 = %d, want 1", atomic.LoadInt32(&c1))
	}
	if atomic.LoadInt32(&c2) != 1 {
		t.Errorf("p2 调用次数 = %d, want 1", atomic.LoadInt32(&c2))
	}
	if atomic.LoadInt32(&c3) != 0 {
		t.Errorf("p3 调用次数 = %d, want 0 (短路应当生效)", atomic.LoadInt32(&c3))
	}
}

// TestPredicatePipelineShortCircuit_Or 验证流水线在 OR (logicAll=false) 模式下的短路特性。
// Given: 流水线按顺序包含 [false算子, true算子, false算子]
// When:  执行 Match(ctx)
// Then:  在遇到第一个 true 算子后立即返回 true，后续的算子绝不被触发求值。
func TestPredicatePipelineShortCircuit_Or(t *testing.T) {
	var c1, c2, c3 int32
	p1 := &countingPredicate{name: "p1", cost: 1, result: false, matchCalls: &c1}
	p2 := &countingPredicate{name: "p2", cost: 2, result: true, matchCalls: &c2}
	p3 := &countingPredicate{name: "p3", cost: 3, result: false, matchCalls: &c3}

	pipeline := NewConditionPipeline(false, []ConditionPredicate{p1, p2, p3})
	matched := pipeline.Match(&mockTestContext{})

	if !matched {
		t.Fatalf("pipeline.Match() = false, want true")
	}
	if atomic.LoadInt32(&c1) != 1 {
		t.Errorf("p1 调用次数 = %d, want 1", atomic.LoadInt32(&c1))
	}
	if atomic.LoadInt32(&c2) != 1 {
		t.Errorf("p2 调用次数 = %d, want 1", atomic.LoadInt32(&c2))
	}
	if atomic.LoadInt32(&c3) != 0 {
		t.Errorf("p3 调用次数 = %d, want 0 (短路应当生效)", atomic.LoadInt32(&c3))
	}
}

// TestPredicatePipelineTraceCollectsAll 验证流水线在规则模拟器留痕模式下的全量收集特性。
// Given: 一条包含 3 个算子的流水线，其中前 2 个已经可以决定匹配结果
// When:  调用 Trace(ctx)
// Then:  所有算子均生成诊断留痕（绝不短路退出），以保证模拟器能完整呈现全流程判定状态。
func TestPredicatePipelineTraceCollectsAll(t *testing.T) {
	p1 := &countingPredicate{name: "ip_check", result: false}
	p2 := &countingPredicate{name: "country_check", result: true}
	p3 := &countingPredicate{name: "device_check", result: true}

	pipeline := NewConditionPipeline(true, []ConditionPredicate{p1, p2, p3})
	traces := pipeline.Trace(&mockTestContext{})

	if len(traces) != 3 {
		t.Fatalf("Trace 结果长度 = %d, want 3", len(traces))
	}
	if traces[0].Field != "ip_check" || traces[0].Matched {
		t.Errorf("trace[0] = %+v, 期望 ip_check 未命中", traces[0])
	}
	if traces[1].Field != "country_check" || !traces[1].Matched {
		t.Errorf("trace[1] = %+v, 期望 country_check 命中", traces[1])
	}
	if traces[2].Field != "device_check" || !traces[2].Matched {
		t.Errorf("trace[2] = %+v, 期望 device_check 命中", traces[2])
	}
}

// TestPredicatePipelineCostOrdering 验证流水线能够依据算子的计算开销（Cost）自动重排以最大化短路收益。
// Given: 两个均为 false 的算子，一个廉价（Cost=1，如常量匹配），一个昂贵（Cost=100，如复杂外部计算）
// When:  构建带优化选项的流水线并执行求值
// Then:  廉价算子先执行，昂贵算子由于短路被跳过。
func TestPredicatePipelineCostOrdering(t *testing.T) {
	var cheapCalls, expensiveCalls int32
	expensive := &countingPredicate{name: "expensive", cost: 100, result: false, matchCalls: &expensiveCalls}
	cheap := &countingPredicate{name: "cheap", cost: 1, result: false, matchCalls: &cheapCalls}

	// 传入时 expensive 在前，cheap 在后
	pipeline := NewOptimizedConditionPipeline(true, []ConditionPredicate{expensive, cheap})
	matched := pipeline.Match(&mockTestContext{})

	if matched {
		t.Fatalf("pipeline.Match() = true, want false")
	}
	if atomic.LoadInt32(&cheapCalls) != 1 {
		t.Errorf("cheapCalls = %d, want 1 (廉价算子应先跑)", atomic.LoadInt32(&cheapCalls))
	}
	if atomic.LoadInt32(&expensiveCalls) != 0 {
		t.Errorf("expensiveCalls = %d, want 0 (昂贵算子应被短路跳过)", atomic.LoadInt32(&expensiveCalls))
	}
}

// customHeaderPredicate 演示自定义外部可插拔算子：检查特定的 HTTP 请求头。
type customHeaderPredicate struct {
	headerName string
	expected   string
}

func (c *customHeaderPredicate) Type() string { return "custom_header" }
func (c *customHeaderPredicate) Cost() int    { return 10 }
func (c *customHeaderPredicate) Match(ctx VisitorContext) bool {
	// 假设外部通过扩展属性访问
	val, ok := ctx.Field("header:" + c.headerName)
	return ok && val == c.expected
}
func (c *customHeaderPredicate) Trace(ctx VisitorContext) ConditionTrace {
	val, ok := ctx.Field("header:" + c.headerName)
	return ConditionTrace{
		Field:       "header:" + c.headerName,
		Operator:    "eq",
		Expected:    []string{c.expected},
		Actual:      val,
		Available:   ok,
		Matched:     ok && val == c.expected,
		Description: fmt.Sprintf("检查请求头 %s == %s", c.headerName, c.expected),
	}
}

// TestCustomPredicateRegistrationAndExecution 验证通过编译注册表（PredicateRegistry）动态插拔新规则算子。
// Given: 系统在运行时注册了一个自定义算子编译器（用于处理 "header" 字段条件）
// When:  编译一条声明了该字段的规则，并对包含对应请求头的画像进行求值
// Then:  编译成功，且能正确完成命中判定与模拟器诊断留痕。
func TestCustomPredicateRegistrationAndExecution(t *testing.T) {
	// 注册自定义算子编译器
	compiler := func(cond store.ConditionLeafNode, _ *slog.Logger) (ConditionPredicate, bool) {
		if len(cond.Values) == 0 {
			return nil, false
		}
		// 约定：使用 Field 作为 header 标识
		parts := strings.SplitN(cond.Field, ":", 2)
		if len(parts) != 2 {
			return nil, false
		}
		return &customHeaderPredicate{
			headerName: parts[1],
			expected:   cond.Values[0],
		}, true
	}

	RegisterPredicateCompilerPrefix("header:", compiler)
	defer UnregisterPredicateCompilerPrefix("header:")

	condNode := store.ConditionLeafNode{
		Field:    "header:X-Promo-Code",
		Operator: "eq",
		Values:   []string{"VIP2026"},
	}

	pred, ok := CompilePredicate(condNode, slog.Default())
	if !ok || pred == nil {
		t.Fatalf("CompilePredicate 失败，期望成功编译自定义算子")
	}

	// 验证匹配命中
	ctxHit := &mockTestContext{
		fields: map[string]string{"header:X-Promo-Code": "VIP2026"},
	}
	if !pred.Match(ctxHit) {
		t.Errorf("pred.Match(ctxHit) = false, want true")
	}

	// 验证未命中
	ctxMiss := &mockTestContext{
		fields: map[string]string{"header:X-Promo-Code": "NORMAL"},
	}
	if pred.Match(ctxMiss) {
		t.Errorf("pred.Match(ctxMiss) = true, want false")
	}

	// 验证诊断追踪
	trace := pred.Trace(ctxHit)
	if !trace.Matched || trace.Actual != "VIP2026" {
		t.Errorf("pred.Trace(ctxHit) = %+v, 期望诊断追踪正确记录", trace)
	}
}
