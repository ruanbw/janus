package rules

import (
	"strings"
	"testing"

	"janus/internal/store"
)

// TestEngineRejectsOversizedRegexCondition 是引擎层正则规模上限的回归测试。
//
// RE2 免疫 ReDoS,不代表正则条件没有成本:编译成本随 pattern 长度增长,
// 单次匹配成本随 pattern 条数线性增长,而条件会进常驻快照、在跳转热路径上
// 对每个访客求值。API 层已有同名上限(100 条 / 1024 字节),这里补的是
// **直写库 / 导入**通道的第二道防线。
func TestEngineRejectsOversizedRegexCondition(t *testing.T) {
	short := strings.Repeat("a", 10)
	okPattern := strings.Repeat("a", maxRegexPatternLen)

	t.Run("pattern 条数超限整条条件丢弃", func(t *testing.T) {
		values := make([]string, 0, maxRegexPatterns+1)
		for i := 0; i <= maxRegexPatterns; i++ {
			values = append(values, short)
		}
		log, buf := logBuf()
		snap := NewSnapshot([]store.Rule{oneRule(1, store.RuleCondition{
			Field: FieldUA, Operator: OpRegex, Values: values,
		})}, log)
		if len(snap.Rules) != 0 {
			t.Fatalf("%d 条 pattern 的 regex 条件不该进快照", maxRegexPatterns+1)
		}
		if !strings.Contains(buf.String(), "超过引擎上限") {
			t.Fatalf("超限丢弃没有留痕:\n%s", buf.String())
		}
	})

	t.Run("pattern 长度超限整条条件丢弃", func(t *testing.T) {
		log, buf := logBuf()
		snap := NewSnapshot([]store.Rule{oneRule(1, store.RuleCondition{
			Field: FieldUA, Operator: OpRegex, Values: []string{strings.Repeat("a", maxRegexPatternLen+1)},
		})}, log)
		if len(snap.Rules) != 0 {
			t.Fatalf("%d 字节的 pattern 不该进快照", maxRegexPatternLen+1)
		}
		if !strings.Contains(buf.String(), "超过引擎上限") {
			t.Fatalf("超限丢弃没有留痕:\n%s", buf.String())
		}
	})

	t.Run("恰好在上限内仍可用", func(t *testing.T) {
		values := make([]string, 0, maxRegexPatterns)
		for i := 0; i < maxRegexPatterns; i++ {
			values = append(values, short)
		}
		snap := NewSnapshot([]store.Rule{oneRule(1, store.RuleCondition{
			Field: FieldUA, Operator: OpRegex, Values: values,
		})}, discardLog)
		if len(snap.Rules) != 1 {
			t.Fatalf("%d 条 pattern 的 regex 条件应正常进快照, got %d 条", maxRegexPatterns, len(snap.Rules))
		}
		snap2 := NewSnapshot([]store.Rule{oneRule(2, store.RuleCondition{
			Field: FieldUA, Operator: OpRegex, Values: []string{okPattern},
		})}, discardLog)
		if len(snap2.Rules) != 1 {
			t.Fatalf("%d 字节的 pattern 应正常进快照, got %d 条", maxRegexPatternLen, len(snap2.Rules))
		}
	})
}
