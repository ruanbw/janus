package rules

import (
	"fmt"
	"log/slog"
	"net/netip"
	"sort"
	"strings"

	"github.com/expr-lang/expr"
	"github.com/expr-lang/expr/ast"
	"github.com/expr-lang/expr/vm"
)

// ConditionEvaluator 统一的条件求值器接口(双层模型:nativeVisual 与 expr)。
type ConditionEvaluator interface {
	Match(ctx VisitorContext) bool
}

// nativeVisualEvaluator Tier-1 原生条件树求值器(基于 compiledNode)。
type nativeVisualEvaluator struct {
	root compiledNode
}

func (e *nativeVisualEvaluator) Match(ctx VisitorContext) bool {
	return e.root.match(ctx)
}

// exprEvaluator Tier-2 Expr 字节码虚拟机求值器。
type exprEvaluator struct {
	program *vm.Program
	// refs 编译期收集的、表达式真正引用到的访客字段(小写规范名,即 ctx.Field 的入参)。
	// 求值前逐个确认可取,任一取不到即整体不命中——见 Match 的不变式注释。
	refs []string
	log  *slog.Logger
}

func (e *exprEvaluator) Match(ctx VisitorContext) (matched bool) {
	defer func() {
		if rec := recover(); rec != nil {
			matched = false
			if e.log != nil {
				e.log.Error("expr 表达式 panic,按未命中(fail-open)处理", "panic", rec)
			}
		}
	}()
	// 关键不变式(与 Tier-1 compiledCond.match 同源,见 eval.go):
	// 字段值取不到(空)时恒不命中。表达式层若不守住这条,取反写法会把它反过来 ——
	// `country != "US"` 在 country 为空时为真(把"拦截非美国"变成拦截所有人),
	// `asn == ""` 因 ASN 当前无数据源而恒真(等价于一条无条件拦截)。
	// 代价:混合引用时可能放弃一次本可命中的判定(fail-open 方向,与既有约定一致)。
	for _, name := range e.refs {
		if _, ok := ctx.Field(name); !ok {
			return false
		}
	}

	fact := FactFromContext(ctx)
	out, err := vm.Run(e.program, &fact)
	if err != nil {
		if e.log != nil {
			e.log.Warn("expr 表达式运行时执行异常,按未命中(fail-open)处理", "err", err)
		}
		return false
	}
	res, ok := out.(bool)
	return ok && res
}

// FactFromContext 将 VisitorContext 转换为 Fact 结构体实例。
func FactFromContext(ctx VisitorContext) Fact {
	if f, ok := ctx.(*Fact); ok && f != nil {
		return *f
	}
	if ctx == nil {
		return Fact{}
	}
	var ip string
	if a := ctx.ClientIP(); a.IsValid() {
		ip = a.String()
	}
	get := func(name string) string {
		v, _ := ctx.Field(name)
		return v
	}
	return Fact{
		IP:      ip,
		IPAttr:  get(FieldIPAttr),
		Country: get(FieldCountry),
		ASN:     get(FieldASN),
		Lang:    get(FieldLang),
		Ref:     get(FieldRef),
		UTM:     get(FieldUTM),
		UA:      get(FieldUA),
		DevType: get(FieldDevType),
		OS:      get(FieldOS),
		Browser: get(FieldBrowser),
		Path:    get(FieldPath),
		Domain:  get(FieldDomain),
	}
}

// fieldCasePatcher 兼容小写字段标识符(如 country == "US" 自动映射到 Country == "US")。
// 同时收集表达式引用到的字段(存小写规范名,与 ctx.Field 的入参一致),
// 供求值前的"取不到即不命中"闸门使用。
type fieldCasePatcher struct {
	refs map[string]struct{}
}

// exprFieldNames 小写规范名 → expr 环境(Fact)里的 Go 字段名。
var exprFieldNames = map[string]string{
	"ip":      "IP",
	"ipattr":  "IPAttr",
	"country": "Country",
	"asn":     "ASN",
	"lang":    "Lang",
	"ref":     "Ref",
	"utm":     "UTM",
	"ua":      "UA",
	"devtype": "DevType",
	"os":      "OS",
	"browser": "Browser",
	"path":    "Path",
	"domain":  "Domain",
}

func (p fieldCasePatcher) Visit(node *ast.Node) {
	if id, ok := (*node).(*ast.IdentifierNode); ok {
		lower := strings.ToLower(id.Value)
		if goName, known := exprFieldNames[lower]; known {
			id.Value = goName
			if p.refs != nil {
				p.refs[lower] = struct{}{}
			}
		}
	}
}

// inCIDRFunction 提供 in_cidr(ip, cidr) 辅助函数
var inCIDRFunction = expr.Function("in_cidr", func(params ...any) (any, error) {
	if len(params) != 2 {
		return false, nil
	}
	ipStr, ok1 := params[0].(string)
	cidrStr, ok2 := params[1].(string)
	if !ok1 || !ok2 {
		return false, nil
	}
	addr, err := netip.ParseAddr(ipStr)
	if err != nil {
		return false, nil
	}
	prefix, err := netip.ParsePrefix(cidrStr)
	if err != nil {
		return false, nil
	}
	return prefix.Contains(addr), nil
})

// CompileExpression 编译一条规则表达式。
func CompileExpression(code string) (*vm.Program, error) {
	prog, _, err := compileExpression(code)
	return prog, err
}

// compileExpression 编译并同时返回表达式引用到的字段集合(小写规范名)。
func compileExpression(code string) (*vm.Program, []string, error) {
	refs := map[string]struct{}{}
	prog, err := expr.Compile(code,
		expr.Env(&Fact{}),
		expr.AsBool(),
		expr.Patch(fieldCasePatcher{refs: refs}),
		inCIDRFunction,
	)
	if err != nil {
		return nil, nil, err
	}
	names := make([]string, 0, len(refs))
	for name := range refs {
		names = append(names, name)
	}
	sort.Strings(names) // 求值前的闸门按稳定顺序遍历,便于测试与调试复现
	return prog, names, nil
}

// ValidateExpression 校验一条规则表达式语法与类型(必须返回 bool)。
func ValidateExpression(code string) error {
	_, err := CompileExpression(code)
	if err != nil {
		return fmt.Errorf("compile expression: %w", err)
	}
	return nil
}
