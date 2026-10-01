package rules

import (
	"errors"
	"fmt"
	"log/slog"
	"net/netip"
	"sort"
	"strings"
	"time"

	"github.com/expr-lang/expr"
	"github.com/expr-lang/expr/ast"
	"github.com/expr-lang/expr/parser"
	"github.com/expr-lang/expr/vm"
)

// 表达式的编译期与运行期预算(包内常量,不加配置项)。
//
// 这里的每个数字都是"租户能单方面触发的最坏情况"的上界,不是调优参数:
// 表达式的长度、节点数、可调用的函数、可求值的时长,四项全部在编译期定死。
const (
	// MaxExpressionLen 单条表达式的字符数上限。编译成本与节点数都随长度线性增长,
	// 没有上限就等于把一次编译的时间与内存交给租户自己定。
	MaxExpressionLen = 4096

	// MaxExpressionNodes AST 节点数上限(expr 的 MaxNodes)。
	// 默认 1e4 对"一条规则条件"来说过大:节点数直接决定字节码长度,而字节码长度
	// 是我们下面"求值步数上界"的唯一依据。1024 足够写出任何有意义的条件。
	MaxExpressionNodes = 1024

	// exprBudgetTimeout 单条表达式求值的兜底时限。
	// 正常表达式是纳秒级(见 BenchmarkEvaluateExprRule);1ms 意味着"出了事"才会触发。
	// 这是第二道防线,不是第一道 —— 见 compileExpression 的注释。
	exprBudgetTimeout = time.Millisecond
)

// exprFact 是喂给 expr 的**唯一**环境类型。
//
// 为什么不用 &Fact{}:expr.Env 会把 env 的全部导出字段**与导出方法**放进类型表,
// 于是 Fact.Fields()/Field()/ClientIP()/WithIP 全都能被租户表达式直接调用。
// 而求值前的 refs 闸门靠遍历 *ast.IdentifierNode 收集字段名,方法调用节点不是标识符,
// 闸门看不见 —— refs 为空,闸门被完全绕过。实测三条 payload 编译成功、
// 对什么字段都取不到的访客求值为 true:
//
//	Fields()["asn"] == ""        → refs=[] → true
//	Fields()["country"] != "US"  → refs=[] → true
//	Field("country") == ""       → refs=[] → true
//
// 换成这个**零方法**的独立 struct 之后,那三个名字在编译期就是 `unknown name Fields`,
// 直接编译失败。修复落在编译期而不是"让求值返回 false":能让它编译成功,
// 就意味着下一个新增的导出方法会重新打开同一个洞。
//
// 字段集与 Fact 严格一致(13 个字符串字段),两者的对应关系收敛在 factToExprEnv 一处。
type exprFact struct {
	IP      string
	IPAttr  string
	Country string
	ASN     string
	Lang    string
	Ref     string
	UTM     string
	UA      string
	DevType string
	OS      string
	Browser string
	Path    string
	Domain  string
}

// factToExprEnv 把访客画像折成表达式环境。字段清单只在这里出现一次。
func factToExprEnv(f Fact) exprFact {
	return exprFact{
		IP:      f.IP,
		IPAttr:  f.IPAttr,
		Country: f.Country,
		ASN:     f.ASN,
		Lang:    f.Lang,
		Ref:     f.Ref,
		UTM:     f.UTM,
		UA:      f.UA,
		DevType: f.DevType,
		OS:      f.OS,
		Browser: f.Browser,
		Path:    f.Path,
		Domain:  f.Domain,
	}
}

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

	env := factToExprEnv(FactFromContext(ctx))
	out, err := runProgram(e.program, env, e.log)
	if err != nil {
		logExprRunError(e.log, err)
		return false
	}
	res, ok := out.(bool)
	return ok && res
}

// runProgram 在预算内求值一条已编译的表达式。
//
// 为什么不用 vm.Step() 做步数预算:Step 只在 expr_debug 构建下有意义 ——
// vm.go 里的取指循环是 `if debug && vm.debug { <-vm.step }`,而 debug 是编译期常量。
// 默认构建下 Run 根本不读 vm.step,此时 Step() 会**永久阻塞**(实测)。
// vm.Step 这条路是死的。
//
// 为什么不用 goroutine + select 做超时:vm.Run 不能被 ctx 中断,
// 超时只能"放弃等待",那个 goroutine 仍在烧 CPU —— 攻击者可以用并发把 CPU 打满,
// 而每个泄漏的 goroutine 还占着栈。实测开销 642ns/5 allocs(直跑 150ns/1 alloc),
// 放在跳转热路径上不划算,换来的却只是一个"不再等它"的假象。
//
// 所以预算的**主体在编译期**:长度上限、节点上限、禁用全部内置函数、拒绝谓词类
// 内置函数 —— 四者合起来让字节码里**不可能出现回跳**,即不存在循环,
// 求值步数被字节码长度静态封顶。runProgram 里的两道兜底只负责最后这一层意外:
//
//   - MemoryBudget:expr 自带的分配预算,超了 panic,被 Run 内部 recover 成 error;
//   - 超时:兜住"上界估计错了"的情况(例如将来 expr 升级引入了新的循环构造)。
//     因为字节码无回跳,被放弃的 goroutine 一定会在极短时间内自行结束,不是泄漏。
func runProgram(prog *vm.Program, env exprFact, log *slog.Logger) (any, error) {
	// MemoryBudget 压到远低于 expr 默认值(1MB):表达式环境只有 13 个字符串,
	// 正常求值的分配是个位数,给 256KB 已经宽到不可能误伤,又足以挡住
	// 任何"构造一个巨大数组/字符串"的写法。
	machine := &vm.VM{MemoryBudget: 256 << 10}
	done := make(chan result, 1) // 缓冲 1:超时后发送方也一定发得出去,不会卡死在发送上
	go func() {
		// 内层 recover:vm.Run 自己也 recover 成 error,这里再兜一层是防
		// "panic 发生在 Run 的 defer 之外"这种边角(例如 env 访问反射失败)。
		defer func() {
			if rec := recover(); rec != nil {
				done <- result{err: fmt.Errorf("expr panic: %v", rec)}
			}
		}()
		v, err := machine.Run(prog, env)
		done <- result{val: v, err: err}
	}()
	timer := time.NewTimer(exprBudgetTimeout)
	defer timer.Stop()
	select {
	case r := <-done:
		return r.val, r.err
	case <-timer.C:
		return nil, errExprBudget
	}
}

// result 是 runProgram 里 goroutine 的回传信封。
type result struct {
	val any
	err error
}

// errExprBudget 表示求值超出兜底时限。日志里按"引擎异常"而不是"表达式写错"归类。
var errExprBudget = errors.New("expr 求值超出时间预算")

func logExprRunError(log *slog.Logger, err error) {
	if log == nil {
		return
	}
	if errors.Is(err, errExprBudget) {
		log.Error("expr 表达式求值超出时间预算,按未命中(fail-open)处理", "err", err)
		return
	}
	log.Warn("expr 表达式运行时执行异常,按未命中(fail-open)处理", "err", err)
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

// exprAllowedBuiltins 允许租户使用的内置函数白名单。
//
// 收窄到"对单个字符串/数字做有界 O(n) 计算"的函数:没有分配放大、没有集合遍历、
// 没有 JSON 解析、没有编解码、没有时间依赖。同一个租户 10 万次调用
// 也就是 10 万次字符串比较 —— 这就是热路径上该有的成本。
//
// 刻意**不**放行的,以及原因:
//   - split / splitAfter / join / replace:按输入长度分配结果串,
//     且 expr 没给它们套 Safe 包装(不进 memGrow 预算),replace 还要现编译正则;
//   - fromJSON / toJSON / fromBase64 / toBase64:解析/编码一个攻击者可控的字符串,
//     产物大小不受限,是内存放大器;
//   - repeat:直接按倍数造串(expr 只在 1e6 处粗拦,仍然能一次要 1MB);
//   - sort / sortBy / keys / values / toPairs / fromPairs / uniq / take / first / last:
//     面向集合的函数,本引擎的环境里根本没有集合,留着只是等着被误用;
//   - now / date / duration / timezone:结果依赖求值时刻,同一条规则两次求值
//     可以给出不同结论 —— 对风控裁决来说这是不可接受的非确定性。
//
// 注意 `contains` / `startsWith` / `endsWith` / `matches` / `in` **不是**内置函数,
// 它们是 expr 的运算符,不走这张表 —— 租户现有的 `ua contains "Googlebot"`
// 这类写法不受影响(见 TestExprAllowedOperatorsStillCompile)。
var exprAllowedBuiltins = []string{
	// 字符串检查
	"len", "hasPrefix", "hasSuffix", "indexOf", "lastIndexOf",
	// 字符串归一(大小写/空白)
	"lower", "upper", "trim", "trimPrefix", "trimSuffix",
	// 数值
	"int", "float", "abs", "ceil", "floor", "round", "max", "min",
}

// exprPredicates 是 parser 层的谓词类"内置函数"。
//
// 这类函数**关不掉**:`expr.DisableAllBuiltins()` 只往 config.Disabled 里塞名字,
// 而 config.Disabled 全仓库只在 parser.go:622 被查一次,那里问的是
// `builtin.Index` —— parser.predicates(all/any/none/one/filter/map/count/sum/
// find/findIndex/findLast/findLastIndex/groupBy/sortBy/reduce)不在那张表里。
// 于是只调 DisableAllBuiltins 之后,`reduce` 照样能编译通过并烧 CPU:
// 实测 30^4 次迭代要 3.7 秒,而这条表达式只有 810 字节,再嵌一层就是几分钟。
//
// 所以这里显式列出来,在 AST 上直接拒。它们同时也是"字节码里唯一会出现
// OpJumpBackward(回跳)的地方"—— 全部拒掉之后,字节码就是一个无环的直线程序,
// 求值步数被字节码长度静态封顶(见 runProgram 的注释)。
var exprPredicates = map[string]struct{}{
	"all": {}, "none": {}, "any": {}, "one": {},
	"filter": {}, "map": {}, "count": {}, "sum": {},
	"find": {}, "findIndex": {}, "findLast": {}, "findLastIndex": {},
	"groupBy": {}, "sortBy": {}, "reduce": {},
}

// exprAudit 编译前审一遍 AST:拒绝谓词类内置函数。
//
// 为什么必须单独审:见 exprPredicates 的注释 —— DisableAllBuiltins 覆盖不到它们。
// 为什么放在 expr.Compile 之前单独 parse 一次:谓词在 AST 上是 *ast.BuiltinNode,
// 只有拿到 AST 才认得出;而 expr.Compile 内部的报错绑定的是它自己那棵树。
func exprAudit(code string) error {
	if n := len(code); n > MaxExpressionLen {
		return fmt.Errorf("表达式长度 %d 超过上限 %d 字符", n, MaxExpressionLen)
	}
	tree, err := parseForAudit(code)
	if err != nil {
		// 语法错误交给 expr.Compile 报(那里的报错带源码位置,更有用);
		// parser 自己 panic 掉则就地判失败,否则同样的 panic 会在
		// expr.Compile 里再发生一次,最终还是变成 500。
		var crash *exprParseCrash
		if errors.As(err, &crash) {
			return fmt.Errorf("表达式无法解析: %v", crash.reason)
		}
		return nil
	}
	bad := ""
	ast.Walk(&tree.Node, auditVisitor{reject: &bad})
	if bad != "" {
		return fmt.Errorf("表达式不允许使用 %q:这类函数要遍历集合,无法给出有界的计算成本", bad)
	}
	return nil
}

// exprParseCrash 表示 parser 对某个输入 panic 了(而不是正常返回语法错误)。
type exprParseCrash struct{ reason any }

func (e *exprParseCrash) Error() string { return fmt.Sprintf("parser panic: %v", e.reason) }

// parseForAudit 解析一份待审计的表达式,把 parser 的 panic 也变成 error。
//
// 为什么:审计跑在**租户可控的输入**上,而且 exprAudit 是整条编译链上第一个
// 碰原文的地方(在长度检查之后)。第三方 parser 若对某个畸形输入 panic,
// 这里就会一路冒到 HTTP handler 的 gin recover 变成 500 —— 一个纯校验请求
// 不该有 500 这条路径。转成 error 之后,它和语法错误一样"编译失败",形状正确。
func parseForAudit(code string) (tree *parser.Tree, err error) {
	defer func() {
		if rec := recover(); rec != nil {
			tree, err = nil, &exprParseCrash{reason: rec}
		}
	}()
	return parser.Parse(code)
}

// auditVisitor 找出第一个被禁的谓词类内置函数(找到就停,不用报全部)。
type auditVisitor struct {
	reject *string
}

func (v auditVisitor) Visit(node *ast.Node) {
	if *v.reject != "" {
		return
	}
	if b, ok := (*node).(*ast.BuiltinNode); ok {
		if _, bad := exprPredicates[b.Name]; bad {
			*v.reject = b.Name
		}
	}
}

// CompileExpression 编译一条规则表达式。
func CompileExpression(code string) (*vm.Program, error) {
	prog, _, err := compileExpression(code)
	return prog, err
}

// compileExpression 编译并同时返回表达式引用到的字段集合(小写规范名)。
//
// 编译期的四道闸门,顺序有意义:
//  1. 长度上限(最便宜,先挡掉最大的输入)
//  2. AST 审计:拒谓词类内置函数(DisableAllBuiltins 覆盖不到的那一半)
//  3. expr.Compile:类型检查 —— 这一层把 Fact 的方法名、未知字段名、
//     非 bool 返回值全部变成编译错误(refs 闸门因此不再可能被绕过)
//  4. DisableAllBuiltins + 白名单重开
//
// 前三道合起来保证:字节码里没有回跳,也就没有循环,求值成本与表达式长度成正比。
func compileExpression(code string) (*vm.Program, []string, error) {
	if err := exprAudit(code); err != nil {
		return nil, nil, err
	}
	refs := map[string]struct{}{}
	opts := []expr.Option{
		// 零方法的最小 env:让 Fields/Field/ClientIP/WithIP 在编译期就不存在。
		expr.Env(exprFact{}),
		expr.AsBool(),
		expr.Patch(fieldCasePatcher{refs: refs}),
		inCIDRFunction,
		expr.MaxNodes(MaxExpressionNodes),
		expr.DisableAllBuiltins(),
	}
	for _, name := range exprAllowedBuiltins {
		opts = append(opts, expr.EnableBuiltin(name))
	}
	prog, err := expr.Compile(code, opts...)
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

// exprHasBackwardJump 字节码里是否存在回跳指令。
//
// 它是"求值有界"这条不变式的机器可检形式:回跳 = 循环 = 步数无上界。
// exprPredicates 把所有能生成回跳的构造(谓词类内置函数)都拒了,
// 所以任何编译通过的程序都不该有回跳。测试用它在编译期把这条性质钉住 ——
// 将来升级 expr、或者有人放宽白名单时,测试会先红,而不是等到线上被烧 CPU。
func exprHasBackwardJump(prog *vm.Program) bool {
	if prog == nil {
		return false
	}
	for _, op := range prog.Bytecode {
		if op == vm.OpJumpBackward {
			return true
		}
	}
	return false
}

// ValidateExpression 校验一条规则表达式语法与类型(必须返回 bool)。
func ValidateExpression(code string) error {
	_, err := CompileExpression(code)
	if err != nil {
		return fmt.Errorf("compile expression: %w", err)
	}
	return nil
}

// ExpressionFieldRefs 返回一条表达式引用到的字段(小写规范名,稳定排序)。
//
// 供 API 层做"这个字段当前有没有数据源"的准入判断:asn 恒空,
// 引用它的规则是一条永不命中的规则,应该在落库前就被拒(见 httpapi 的 resolveRule)。
// 表达式非法时返回错误,调用方按"已经校验过"的路径处理即可。
func ExpressionFieldRefs(code string) ([]string, error) {
	_, refs, err := compileExpression(code)
	if err != nil {
		return nil, err
	}
	return refs, nil
}
