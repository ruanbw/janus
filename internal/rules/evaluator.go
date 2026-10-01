package rules

import (
	"fmt"
	"log/slog"
	"net/netip"
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
	log     *slog.Logger
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
type fieldCasePatcher struct{}

func (fieldCasePatcher) Visit(node *ast.Node) {
	if id, ok := (*node).(*ast.IdentifierNode); ok {
		switch strings.ToLower(id.Value) {
		case "ip":
			id.Value = "IP"
		case "ipattr":
			id.Value = "IPAttr"
		case "country":
			id.Value = "Country"
		case "asn":
			id.Value = "ASN"
		case "lang":
			id.Value = "Lang"
		case "ref":
			id.Value = "Ref"
		case "utm":
			id.Value = "UTM"
		case "ua":
			id.Value = "UA"
		case "devtype":
			id.Value = "DevType"
		case "os":
			id.Value = "OS"
		case "browser":
			id.Value = "Browser"
		case "path":
			id.Value = "Path"
		case "domain":
			id.Value = "Domain"
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
	return expr.Compile(code,
		expr.Env(&Fact{}),
		expr.AsBool(),
		expr.Patch(fieldCasePatcher{}),
		inCIDRFunction,
	)
}

// ValidateExpression 校验一条规则表达式语法与类型(必须返回 bool)。
func ValidateExpression(code string) error {
	_, err := CompileExpression(code)
	if err != nil {
		return fmt.Errorf("compile expression: %w", err)
	}
	return nil
}
