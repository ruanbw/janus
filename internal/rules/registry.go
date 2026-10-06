package rules

import (
	"log/slog"
	"strings"
	"sync"

	"janus/internal/store"
)

// PredicateCompiler 算子编译函数，负责将持久化的条件配置编译为具体的原子断言算子。
type PredicateCompiler func(cond store.ConditionLeafNode, log *slog.Logger) (ConditionPredicate, bool)

var (
	compilerMu        sync.RWMutex
	exactCompilers    = make(map[string]PredicateCompiler)
	prefixCompilers   = make(map[string]PredicateCompiler)
	fallbackCompilers []PredicateCompiler
)

// RegisterPredicateCompiler 注册精确匹配字段名（或算子标识）的编译器。
func RegisterPredicateCompiler(field string, c PredicateCompiler) {
	if c == nil {
		return
	}
	compilerMu.Lock()
	defer compilerMu.Unlock()
	exactCompilers[field] = c
}

// RegisterPredicateCompilerPrefix 注册匹配字段名前缀的编译器（例如 "header:" 前缀匹配）。
func RegisterPredicateCompilerPrefix(prefix string, c PredicateCompiler) {
	if c == nil {
		return
	}
	compilerMu.Lock()
	defer compilerMu.Unlock()
	prefixCompilers[prefix] = c
}

// UnregisterPredicateCompilerPrefix 移除前缀编译器（主要用于测试隔离与插件卸载）。
func UnregisterPredicateCompilerPrefix(prefix string) {
	compilerMu.Lock()
	defer compilerMu.Unlock()
	delete(prefixCompilers, prefix)
}

// RegisterFallbackCompiler 注册兜底通用编译器。
func RegisterFallbackCompiler(c PredicateCompiler) {
	if c == nil {
		return
	}
	compilerMu.Lock()
	defer compilerMu.Unlock()
	fallbackCompilers = append(fallbackCompilers, c)
}

// CompilePredicate 依据注册表分发并将条件节点编译为对应的 ConditionPredicate。
func CompilePredicate(cond store.ConditionLeafNode, log *slog.Logger) (ConditionPredicate, bool) {
	if log == nil {
		log = slog.Default()
	}

	compilerMu.RLock()
	// 1. 精确匹配字段名
	if c, ok := exactCompilers[cond.Field]; ok {
		compilerMu.RUnlock()
		return c(cond, log)
	}

	// 2. 前缀匹配
	for prefix, c := range prefixCompilers {
		if strings.HasPrefix(cond.Field, prefix) {
			compilerMu.RUnlock()
			return c(cond, log)
		}
	}

	// 3. 兜底编译器
	fallbacks := make([]PredicateCompiler, len(fallbackCompilers))
	copy(fallbacks, fallbackCompilers)
	compilerMu.RUnlock()

	for _, c := range fallbacks {
		if pred, ok := c(cond, log); ok {
			return pred, true
		}
	}

	return nil, false
}
