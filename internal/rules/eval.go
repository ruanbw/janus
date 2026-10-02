// Package rules 是规则求值引擎:把"一条规则 + 一次访问"判定成"命中/未命中",
// 并按优先级给出裁决。求值发生在跳转热路径上(spec D7),因此:
//   - 求值期是纯内存的:条件组遍历 + 已预编译的字面量/CIDR/正则,零 parse、零 IO;
//   - 任何异常都 fail-open(放行并记日志):风控规则不该把线上短链打成 500;
//   - 取不到数据的字段(未收录网段的 country、当前无数据源的 asn)恒不命中,
//     绝不"默认放行"或"默认拦截"。
package rules

import (
	"log/slog"
	"net/netip"
	"regexp"
	"strconv"
	"strings"

	"janus/internal/store"
)

// 条件运算符白名单(spec D5)。落库时只接受白名单内的运算符,
// 其余一律在加载期被丢弃并记日志——不认识的运算符绝不能退化成"恒真"。
const (
	OpIn          = "in"           // 字段值 ∈ values
	OpNotIn       = "not_in"       // 字段值 ∉ values
	OpEq          = "eq"           // 字段值 = values[0]
	OpNeq         = "neq"          // 字段值 ≠ values[0]
	OpContains    = "contains"     // 字段值包含任一 values(子串,忽略大小写)
	OpNotContains = "not_contains" // 字段值不包含任一 values
	OpStartsWith  = "starts_with"  // 字段值以任一 values 开头(前缀匹配,忽略大小写)
	OpEndsWith    = "ends_with"    // 字段值以任一 values 结尾(后缀匹配,忽略大小写)
	OpInCIDR      = "in_cidr"      // 访客 IP 落在任一 values CIDR 网段内
	OpNotInCIDR   = "not_in_cidr"  // 访客 IP 未落在任一 values CIDR 网段内
	OpGT          = "gt"           // 数值大于 values[0]
	OpLT          = "lt"           // 数值小于 values[0]
	OpRegex       = "regex"        // 字段值匹配任一 values(加载期已编译)
	OpDuplicated  = "duplicated"   // 该字段值的出现次数 ≥ values[0](计数来自 Fact.Seen)
)

// ValidOperator 判断运算符是否在白名单内(API 层入参校验复用)。
func ValidOperator(op string) bool {
	switch op {
	case OpIn, OpNotIn, OpEq, OpNeq, OpContains, OpNotContains,
		OpStartsWith, OpEndsWith, OpInCIDR, OpNotInCIDR,
		OpGT, OpLT, OpRegex, OpDuplicated:
		return true
	}
	return false
}

// ValidField 判断字段是否在 13 个可求值字段内(API 层入参校验复用)。
func ValidField(field string) bool {
	switch field {
	case FieldIP, FieldIPAttr, FieldCountry, FieldASN, FieldLang, FieldRef, FieldUTM,
		FieldUA, FieldDevType, FieldOS, FieldBrowser, FieldPath, FieldDomain:
		return true
	}
	return false
}

// Decision 一次求值的裁决结果(值拷贝,调用方可以安全持有)。
type Decision struct {
	RuleID      int64
	Name        string
	Action      string // pass / redirect / notfound / throttle
	Destination string // action=redirect 时的改写目标
	Priority    int
	PageMode    string // default / custom
	CustomHTML  string // action=notfound/throttle 且 page_mode=custom 时的专属 HTML
}

// compiledCond 一条预编译后的条件。字面量按比较语义归一(小写),
// CIDR/正则/阈值在加载期解析完毕,求值期只做比较。
type compiledCond struct {
	field  string
	op     string
	raw    []string            // 租户写下的原始字面量(去空白、未小写),仿真回显"期望值"用
	lits   []string            // 小写归一后的字面量(in/eq/contains/starts_with/ends_with/...)
	litMap map[string]struct{} // in / not_in 的 O(1) 预编译集合
	radix  *IPRadixTree        // 预编译的前缀基数树(仅 ip 字段的 in/not_in/eq/neq/in_cidr)
	res    []*regexp.Regexp
	num    float64 // gt/lt 阈值
	seen   int     // duplicated 阈值
}

// compiledNode 条件树的编译形态。叶子内联在父节点的切片里(值类型,不为每个叶子单独分配);
// nodeOpAny 短路命中,nodeOpAll 短路失败。
type compiledNode struct {
	op       uint8          // nodeOpAll / nodeOpAny
	conds    []compiledCond // op 为叶子时的条件
	children []compiledNode // 子组
}

const (
	nodeOpAll uint8 = iota
	nodeOpAny
)

// Compiled 一条预编译后的规则。零关联的 scoped 规则在这里表现为
// appliesAll=false 且 linkIDs 为空,求值期直接跳过(spec D2:不兜底成全局)。
type Compiled struct {
	Rule       store.Rule
	evaluator  ConditionEvaluator // 双层统一求值器接口(Tier-1 nativeVisualEvaluator / Tier-2 exprEvaluator)
	logicAll   bool
	appliesAll bool           // scope=global
	linkIDs    map[int64]bool // scope=links 时的适用短链
	root       compiledNode   // 条件树(扁平形态 = 一层 logicAll 组)
	nested     bool           // 是否真的用了嵌套(用于把「为什么没命中」说成人话)
}

// compileRule 预编译一条规则。conditions 非空但全部被丢弃(字段不认识/正则非法/CIDR 非法)
// 时返回 ok=false:一条坏条件不能拖垮整份快照,但也不能让这条规则退化成"恒命中的兜底"。
func compileRule(r store.Rule, log *slog.Logger) (Compiled, bool) {
	c := Compiled{Rule: r, logicAll: true}
	switch r.Scope {
	case store.RuleScopeGlobal:
		c.appliesAll = true
	default:
		if len(r.LinkIDs) > 0 {
			c.linkIDs = make(map[int64]bool, len(r.LinkIDs))
			for _, id := range r.LinkIDs {
				c.linkIDs[id] = true
			}
		}
	}
	switch r.Logic {
	case store.RuleLogicAny:
		c.logicAll = false
	case store.RuleLogicAll, "":
	default:
		// 库内 CHECK 拦得住脏值,这里再兜一层:不认识的 logic 一律按 all 求值
		log.Warn("规则 logic 非法,按 all 求值", "rule", r.ID, "logic", r.Logic)
	}

	// Tier-2 表达式规则:通过 Expr 字节码虚拟机求值
	if r.RuleType == store.RuleTypeExpression {
		if strings.TrimSpace(r.Expression) == "" {
			log.Warn("表达式规则的 expression 为空,整条规则不参与求值", "rule", r.ID, "name", r.Name)
			return Compiled{}, false
		}
		prog, refs, err := compileExpression(r.Expression)
		if err != nil {
			log.Warn("表达式规则编译失败,整条规则不参与求值", "rule", r.ID, "name", r.Name, "err", err)
			return Compiled{}, false
		}
		c.evaluator = &exprEvaluator{program: prog, refs: refs, log: log}
		return c, true
	}

	declared := r.Conditions.Len()
	kept := 0
	if r.Conditions.IsTree() {
		root, ok := compileNode(*r.Conditions.Root, r.ID, log)
		if !ok {
			log.Warn("规则条件树不可求值,整条规则不参与求值", "rule", r.ID, "name", r.Name)
			return Compiled{}, false
		}
		c.root = root
		c.nested = root.nestedTree()
		kept = len(root.leaves())
	} else {
		// 扁平形态就是"一层 + rules.logic"的树:求值路径只有 root 一条
		root := compiledNode{op: nodeOpFor(c.logicAll)}
		for _, cond := range r.Conditions.Leaves {
			cc, ok := compileCond(cond, r.ID, log)
			if !ok {
				continue
			}
			root.conds = append(root.conds, cc)
			kept++
		}
		c.root = root
	}
	if declared > 0 && kept == 0 {
		log.Warn("规则条件全部不可求值,整条规则不参与求值", "rule", r.ID, "name", r.Name)
		return Compiled{}, false
	}
	c.evaluator = &nativeVisualEvaluator{root: c.root}
	return c, true
}

func nodeOpFor(logicAll bool) uint8 {
	if logicAll {
		return nodeOpAll
	}
	return nodeOpAny
}

// leaves 树里的叶子总数(用于"声明了 N 条却一条都不可求值"的判定)。
func (n compiledNode) leaves() []compiledCond {
	if len(n.children) == 0 {
		return n.conds
	}
	var out []compiledCond
	for i := range n.children {
		out = append(out, n.children[i].leaves()...)
	}
	return out
}

// nestedTree 树里是否出现了子组(一层 all/any 不算嵌套)。
func (n compiledNode) nestedTree() bool {
	if len(n.children) == 0 {
		return false
	}
	if len(n.conds) > 0 {
		return true
	}
	for i := range n.children {
		if len(n.children[i].children) > 0 || n.children[i].nestedTree() {
			return true
		}
	}
	return false
}

// leafCount 叶子总数,不给 leaves() 分配一个临时切片。
func (n compiledNode) leafCount() int {
	if len(n.children) == 0 {
		return len(n.conds)
	}
	total := len(n.conds)
	for i := range n.children {
		total += n.children[i].leafCount()
	}
	return total
}

// compileNode 预编译条件树的一个节点。ok=false 表示这个节点不可求值,
// 调用方要么整条规则不参与求值,要么(叶子层)跳过这一条。
//
// 为什么不是"跳过坏叶子继续":在 or 组里丢一条叶子会静默收窄规则,
// 在 and 组里丢一条会静默放宽规则——后者是 fail-open,比整条规则不参与
// 危险得多。所以非空组编译后变成空组一律判整条规则不可求值。
func compileNode(n store.ConditionNode, ruleID int64, log *slog.Logger) (compiledNode, bool) {
	if n.Group != nil && n.Type != store.ConditionTypeLeaf {
		logic := n.Group.Logic
		if logic == "" {
			// 缺失 logic 按 all:与扁平形态、与库内 CHECK 的约定一致
			logic = "all"
		}
		if logic != "all" && logic != "any" {
			// 库内 CHECK 拦得住脏值;这里兜一层并按 all 求值,和扁平形态同一套策略
			log.Warn("条件组 logic 非法,按 all 求值", "rule", ruleID, "logic", logic)
			logic = "all"
		}
		out := compiledNode{op: nodeOpFor(logic == "all")}
		declared := 0
		for _, child := range n.Group.Children {
			cc, ok := compileNode(child, ruleID, log)
			if !ok {
				// 组里有一个不可求值的叶子,整组作废:在 and 组里丢一条等于把规则放宽
				// (fail-open),在 any 组里丢一条等于把规则收紧——都不是用户写的东西。
				return compiledNode{}, false
			}
			if len(cc.conds) == 0 && len(cc.children) == 0 {
				continue // 结构性空组(本来就没有子节点),不算写坏了
			}
			if len(cc.conds) == 1 && len(cc.children) == 0 {
				// 单叶子直接内联,不为每个叶子分配一个节点
				out.conds = append(out.conds, cc.conds[0])
			} else {
				out.children = append(out.children, cc)
			}
			declared++
		}
		if declared == 0 && len(n.Group.Children) > 0 {
			log.Warn("条件组声明了子节点但一个都不可求值", "rule", ruleID, "logic", logic)
			return compiledNode{}, false
		}
		return out, true
	}
	if n.Leaf == nil {
		return compiledNode{}, false
	}
	cc, ok := compileCond(*n.Leaf, ruleID, log)
	if !ok {
		return compiledNode{}, false
	}
	return compiledNode{op: nodeOpAll, conds: []compiledCond{cc}}, true
}

// compileCond 预编译单条条件;ok=false 表示这条条件被丢弃(调用方记录并继续)。
func compileCond(cond store.RuleCondition, ruleID int64, log *slog.Logger) (compiledCond, bool) {
	drop := func(reason string) (compiledCond, bool) {
		log.Warn("规则条件被丢弃", "rule", ruleID, "field", cond.Field,
			"operator", cond.Operator, "reason", reason)
		return compiledCond{}, false
	}
	if !ValidField(cond.Field) {
		// 字段不在 13 个可求值字段内 = 配了也跑不了;接数据源后重新加载即可自动生效
		return drop("field 不可求值")
	}
	if !ValidOperator(cond.Operator) {
		return drop("operator 不在白名单")
	}
	values := make([]string, 0, len(cond.Values))
	for _, v := range cond.Values {
		v = strings.TrimSpace(v)
		if v == "" {
			continue
		}
		values = append(values, v)
	}
	if len(values) == 0 {
		return drop("values 为空")
	}
	c := compiledCond{field: cond.Field, op: cond.Operator, raw: values}
	// ip 字段的集合比较(in/not_in/eq/neq/in_cidr)走预解析的前缀基数树:
	// 单 IP 与网段都预编译进 IPRadixTree,求值期 O(1) 逐位下钻,零分配。
	// 其余运算符(ip contains / ip regex 等)对 ip 按普通字符串处理。
	if cond.Field == FieldIP {
		switch cond.Operator {
		case OpIn, OpEq, OpNeq, OpNotIn, OpInCIDR, OpNotInCIDR:
			var prefixes []netip.Prefix
			for _, v := range values {
				if p, err := netip.ParsePrefix(v); err == nil {
					prefixes = append(prefixes, p)
					continue
				}
				if a, err := netip.ParseAddr(v); err == nil {
					prefixes = append(prefixes, netip.PrefixFrom(a, a.BitLen()))
					continue
				}
			}
			if len(prefixes) == 0 {
				return drop("ip 值既不是合法 IP 也不是合法 CIDR")
			}
			c.radix = NewIPRadixTree(prefixes)
			return c, true
		}
	}
	if cond.Operator == OpInCIDR || cond.Operator == OpNotInCIDR {
		return drop(cond.Operator + " 运算符仅支持 ip 字段")
	}
	switch cond.Operator {
	case OpGT, OpLT:
		num, err := strconv.ParseFloat(values[0], 64)
		if err != nil {
			return drop("阈值不是数字")
		}
		c.num = num
	case OpDuplicated:
		n, err := strconv.Atoi(values[0])
		if err != nil || n < 1 {
			return drop("重复次数不是正整数")
		}
		c.seen = n
	case OpRegex:
		for _, v := range values {
			re, err := regexp.Compile(v)
			if err != nil {
				// 一个值坏掉不该让整条条件失效:跳过它,剩下的照常编译
				log.Warn("规则条件的正则无法编译,该值被跳过",
					"rule", ruleID, "field", cond.Field, "pattern", v, "err", err)
				continue
			}
			c.res = append(c.res, re)
		}
		if len(c.res) == 0 {
			return drop("正则全部无法编译")
		}
	default:
		c.lits = make([]string, len(values))
		for i, v := range values {
			c.lits[i] = strings.ToLower(v)
		}
		if cond.Operator == OpIn || cond.Operator == OpNotIn {
			c.litMap = make(map[string]struct{}, len(c.lits))
			for _, lit := range c.lits {
				c.litMap[lit] = struct{}{}
			}
		}
	}
	return c, true
}

// match 单条条件是否满足。
// 关键不变式:字段值取不到(空)时恒不命中——包括 not_in / neq / not_contains。
// 否则"取不到数据"会变成"全部命中"的反面,把 GeoIP 未接入的规则变成对全租户流量生效的拦截。
func (c *compiledCond) match(ctx VisitorContext) bool {
	if ctx == nil {
		return false
	}
	raw, ok := ctx.Field(c.field)
	if !ok {
		return false
	}
	switch c.op {
	case OpIn, OpEq, OpNeq, OpNotIn, OpInCIDR, OpNotInCIDR:
		return c.matchSet(ctx, raw)
	case OpStartsWith:
		return c.matchStartsWith(raw)
	case OpEndsWith:
		return c.matchEndsWith(raw)
	case OpContains, OpNotContains:
		return c.matchContains(raw)
	case OpGT, OpLT:
		return c.matchNumber(raw)
	case OpRegex:
		return c.matchRegex(raw)
	case OpDuplicated:
		// 计数来自调用方提供的 Fact.Seen;没有计数数据(=没接数据源)时恒不命中
		if f, ok := ctx.(*Fact); ok && f.Seen != nil {
			return f.Seen[SeenKey(c.field, raw)] >= c.seen
		}
		if f, ok := ctx.(Fact); ok && f.Seen != nil {
			return f.Seen[SeenKey(c.field, raw)] >= c.seen
		}
		return false
	}
	return false
}

// matchSet 处理 in / eq / neq / not_in / in_cidr:values 里任一相等即满足(大小写不敏感)。
// ip 字段特殊:比对的是预解析在 IPRadixTree 里的前缀与单 IP,求值期零分配且 O(1)。
// 多个运算符只共用一个 hit("访客 IP 落没落在集合/网段内"),再由 c.op 决定取反不取反——
// 在 ip 分支里各写各的返回值会漏掉 neq,把它整体判反。
func (c *compiledCond) matchSet(ctx VisitorContext, raw string) bool {
	hit := false
	if c.field == FieldIP {
		addr := ctx.ClientIP()
		if !addr.IsValid() {
			return false
		}
		if c.radix != nil {
			hit = c.radix.Contains(addr)
		}
	} else if c.litMap != nil {
		hit = c.matchLitMap(raw)
	} else {
		for _, lit := range c.lits {
			if equalFoldASCII(raw, lit) {
				hit = true
				break
			}
		}
	}
	switch c.op {
	case OpIn, OpEq, OpInCIDR:
		return hit
	default: // neq / not_in
		return !hit
	}
}

// matchLitMap 对已预编译为小写集合的 map 进行 O(1) 匹配。
// 对 <=64 字节的纯 ASCII 字符串在栈缓冲区上小写化,避免分配堆内存。
// 注意:缓冲区快速路径只覆盖 ASCII——长串与非 ASCII 输入(访客自控的 path/ua/utm
// 才可能落到这里)走下面的线性扫描,以零分配为代价换掉 strings.ToLower 的堆分配;
// 代价是折叠语义收窄为 strings.EqualFold 的简单折叠(非 Unicode 全量小写映射)。
func (c *compiledCond) matchLitMap(raw string) bool {
	if _, ok := c.litMap[raw]; ok {
		return true
	}
	var buf [64]byte
	if len(raw) <= len(buf) {
		hasUpper := false
		isASCII := true
		for i := 0; i < len(raw); i++ {
			b := raw[i]
			if b >= 'A' && b <= 'Z' {
				hasUpper = true
				buf[i] = b + ('a' - 'A')
			} else if b < 0x80 {
				buf[i] = b
			} else {
				isASCII = false
				break
			}
		}
		if isASCII {
			// 全小写时 buf 与 raw 逐字节相同,上面已查过 raw 且未命中,故必不命中。
			if !hasUpper {
				return false
			}
			_, ok := c.litMap[string(buf[:len(raw)])]
			return ok
		}
	}
	for _, lit := range c.lits {
		if equalFoldASCII(raw, lit) {
			return true
		}
	}
	return false
}

// matchStartsWith 处理 starts_with:前缀匹配,大小写不敏感。
func (c *compiledCond) matchStartsWith(raw string) bool {
	for _, lit := range c.lits {
		if hasPrefixFold(raw, lit) {
			return true
		}
	}
	return false
}

// matchEndsWith 处理 ends_with:后缀匹配,大小写不敏感。
func (c *compiledCond) matchEndsWith(raw string) bool {
	for _, lit := range c.lits {
		if hasSuffixFold(raw, lit) {
			return true
		}
	}
	return false
}

// hasPrefixFold 判断 s 是否以 prefix 开头,忽略 ASCII 大小写。零分配。
func hasPrefixFold(s, prefix string) bool {
	if len(s) < len(prefix) {
		return false
	}
	return equalFoldASCII(s[:len(prefix)], prefix)
}

// hasSuffixFold 判断 s 是否以 suffix 结尾,忽略 ASCII 大小写。零分配。
func hasSuffixFold(s, suffix string) bool {
	if len(s) < len(suffix) {
		return false
	}
	return equalFoldASCII(s[len(s)-len(suffix):], suffix)
}

// matchContains 处理 contains / not_contains:子串匹配,大小写不敏感
// (租户写 "bot" 就该命中 "Googlebot")。
func (c *compiledCond) matchContains(raw string) bool {
	hit := false
	for _, lit := range c.lits {
		if containsFold(raw, lit) {
			hit = true
			break
		}
	}
	if c.op == OpContains {
		return hit
	}
	return !hit
}

// matchNumber 处理 gt / lt:两侧都按数字解析,取不到数字(布尔/空串)一律不命中。
func (c *compiledCond) matchNumber(raw string) bool {
	v, err := strconv.ParseFloat(strings.TrimSpace(raw), 64)
	if err != nil {
		return false
	}
	if c.op == OpGT {
		return v > c.num
	}
	return v < c.num
}

// matchRegex 处理 regex:用加载期编译好的正则匹配字段原串
// (不做小写化——需要忽略大小写时租户自己在表达式里写 (?i))。
func (c *compiledCond) matchRegex(raw string) bool {
	for _, re := range c.res {
		if re.MatchString(raw) {
			return true
		}
	}
	return false
}

// matchAll 条件组是否满足:all 需全部满足,any 需任一满足。
// 空条件组视为满足(可以配一条"无条件即执行"的规则);但整条规则
// 不会带着空条件组出现——conditions 非空却被全部丢弃时整条规则已不参与求值。
func (c *Compiled) matchAll(ctx VisitorContext) bool {
	if c.evaluator != nil {
		return c.evaluator.Match(ctx)
	}
	return c.root.match(ctx)
}

// match 递归求值。短路是必须的:and 组里后面的条件可能拿不到数据(白跑一次慢判断,
// 甚至让一次 duplicated 计数被重复触发),any 组里更不该为已经成立的分支继续付出代价。
func (n compiledNode) match(ctx VisitorContext) bool {
	// want = 一旦成立就短路返回的那个值:any 组是 true,all 组是 false。
	// 空节点:all([]) 为真(可配一条"无条件即执行"的规则),any([]) 为假。
	// 内联的叶子先于嵌套组求值(见 compileNode):叶子是用户直接写在父组里的条件,
	// 嵌套组排在后面,与叶子摊平后的顺序一致。
	want := n.op == nodeOpAny
	for i := range n.conds {
		if n.conds[i].match(ctx) == want {
			return want
		}
	}
	for i := range n.children {
		if n.children[i].match(ctx) == want {
			return want
		}
	}
	return !want
}

// applies 规则是否适用于该短链。
// scope=global 恒适用;scope=links 只对显式关联的短链适用,
// 零关联则永不适用(spec D2:不兜底、不静默退化成全局)。
func (c *Compiled) applies(linkID int64) bool {
	if c.appliesAll {
		return true
	}
	if len(c.linkIDs) == 0 {
		return false
	}
	return c.linkIDs[linkID]
}

// containsFold 子串匹配,忽略 ASCII 大小写。
// 不走 strings.ToLower(s)+Contains:那会在每次访问上为整条 UA 分配一份副本,
// 而求值在跳转热路径上(spec D7),这里必须零分配。
// 先按首字节筛候选位置,再用 equalFoldASCII 确认,避免退化到 O(n*m)。
func containsFold(s, sub string) bool {
	if sub == "" {
		return true
	}
	first := lowerASCII(sub[0])
	for i := 0; i+len(sub) <= len(s); i++ {
		if lowerASCII(s[i]) != first {
			continue
		}
		if equalFoldASCII(s[i:i+len(sub)], sub) {
			return true
		}
	}
	return false
}

// equalFoldASCII 快速比较两个字符串是否在忽略 ASCII 大小写下相等。
// 针对 ASCII 进行零分配直接比对;遇到非 ASCII 字符时回退至 strings.EqualFold。
// 回退必须比对整个字符串:循环下标 i 可能落在多字节字符中间,
// 截断后再 EqualFold 会把不同的字符判成相等。
func equalFoldASCII(s, t string) bool {
	if len(s) != len(t) {
		return false
	}
	for i := 0; i < len(s); i++ {
		sb := s[i]
		tb := t[i]
		if sb == tb {
			continue
		}
		if sb >= 0x80 || tb >= 0x80 {
			return strings.EqualFold(s, t)
		}
		if sb >= 'A' && sb <= 'Z' {
			sb += 'a' - 'A'
		}
		if tb >= 'A' && tb <= 'Z' {
			tb += 'a' - 'A'
		}
		if sb != tb {
			return false
		}
	}
	return true
}

// lowerASCII 只折叠 ASCII 大小写(UA/域名/系统名的判定都只涉及 ASCII;
// 非 ASCII 字符原样返回,不影响结果)。
func lowerASCII(b byte) byte {
	if b >= 'A' && b <= 'Z' {
		return b + ('a' - 'A')
	}
	return b
}
