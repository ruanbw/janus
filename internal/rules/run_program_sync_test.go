package rules

import (
	"runtime"
	"strconv"
	"strings"
	"testing"

	"github.com/expr-lang/expr"
)

// currentGoid 返回当前 goroutine 的编号(仅测试用)。
// 解析 runtime.Stack 的首行 "goroutine <id> [running]:"。
func currentGoid() int64 {
	var buf [64]byte
	n := runtime.Stack(buf[:], false)
	s := strings.TrimPrefix(string(buf[:n]), "goroutine ")
	i := strings.IndexByte(s, ' ')
	if i < 0 {
		return -1
	}
	id, err := strconv.ParseInt(s[:i], 10, 64)
	if err != nil {
		return -1
	}
	return id
}

// TestRunProgramRunsSynchronously 是"表达式求值不再为每条规则起 goroutine+timer"的回归测试。
//
// 旧实现在 goroutine 里跑 vm.Run,再 select 一个 1ms 的 timer:注释本身论证过不划算
// (实测 642ns/5 allocs vs 直跑 150ns/1 alloc),实现却恰恰是它。这条测试用一个自带的
// probe 函数在求值**期间**取当前 goroutine id,与调用方比对 —— 只要求值被挪进
// 另一个 goroutine,断言立刻失败(而不是靠测耗时这种不稳定信号)。
func TestRunProgramRunsSynchronously(t *testing.T) {
	caller := currentGoid()
	if caller <= 0 {
		t.Skip("无法取得当前 goroutine 编号")
	}
	var inner int64
	probe := expr.Function("probe", func(params ...any) (any, error) {
		inner = currentGoid()
		return true, nil
	})
	prog, err := expr.Compile(`probe()`,
		expr.Env(exprFact{}),
		expr.AsBool(),
		probe,
		expr.DisableAllBuiltins(),
	)
	if err != nil {
		t.Fatalf("构造探针程序失败: %v", err)
	}
	out, err := runProgram(prog, exprFact{})
	if err != nil {
		t.Fatalf("求值失败: %v", err)
	}
	res, ok := out.(bool)
	if !ok || !res {
		t.Fatalf("探针程序应返回 true, got %#v", out)
	}
	if inner != caller {
		t.Fatalf("runProgram 在 goroutine %d 上执行,调用方是 %d:求值必须同步直跑", inner, caller)
	}
}
