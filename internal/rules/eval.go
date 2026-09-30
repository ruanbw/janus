// Package rules 是规则求值引擎:把"一条规则 + 一次访问"判定成"命中/未命中",
// 并按优先级给出裁决。求值发生在跳转热路径上(spec D7),因此:
//   - 求值期是纯内存的:条件组遍历 + 已预编译的字面量/CIDR/正则,零 parse、零 IO;
//   - 任何异常都 fail-open(放行并记日志):风控规则不该把线上短链打成 500;
//   - 取不到数据的字段(未收录网段的 country、当前无数据源的 asn)恒不命中,
//     绝不"默认放行"或"默认拦截"。
package rules

import (
	"log/slog"
	"net"
	"regexp"
	"strconv"
	"strings"

	"cloak/internal/store"
)

// 条件运算符白名单(spec D5)。落库时只接受这 10 个,
// 其余一律在加载期被丢弃并记日志——不认识的运算符绝不能退化成"恒真"。
const (
	OpIn          = "in"           // 字段值 ∈ values
	OpNotIn       = "not_in"       // 字段值 ∉ values
	OpEq          = "eq"           // 字段值 = values[0]
	OpNeq         = "neq"          // 字段值 ≠ values[0]
	OpContains    = "contains"     // 字段值包含任一 values(子串,忽略大小写)
	OpNotContains = "not_contains" // 字段值不包含任一 values
	OpGT          = "gt"           // 数值大于 values[0]
	OpLT          = "lt"           // 数值小于 values[0]
	OpRegex       = "regex"        // 字段值匹配任一 values(加载期已编译)
	OpDuplicated  = "duplicated"   // 该字段值的出现次数 ≥ values[0](计数来自 Fact.Seen)
)

// ValidOperator 判断运算符是否在白名单内(API 层入参校验复用)。
func ValidOperator(op string) bool {
	switch op {
	case OpIn, OpNotIn, OpEq, OpNeq, OpContains, OpNotContains, OpGT, OpLT, OpRegex, OpDuplicated:
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
	field string
	op    string
	lits  []string     // 小写归一后的字面量(in/eq/contains/...)
	ips   []net.IP     // 预解析的单 IP(仅 ip 字段)
	nets  []*net.IPNet // 预解析的 CIDR(仅 ip 字段)
	res   []*regexp.Regexp
	num   float64 // gt/lt 阈值
	seen  int     // duplicated 阈值
}

// Compiled 一条预编译后的规则。零关联的 scoped 规则在这里表现为
// appliesAll=false 且 linkIDs 为空,求值期直接跳过(spec D2:不兜底成全局)。
type Compiled struct {
	Rule       store.Rule
	logicAll   bool
	appliesAll bool           // scope=global
	linkIDs    map[int64]bool // scope=links 时的适用短链
	conds      []compiledCond
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
	declared := len(r.Conditions)
	kept := 0
	for _, cond := range r.Conditions {
		cc, ok := compileCond(cond, r.ID, log)
		if !ok {
			continue
		}
		c.conds = append(c.conds, cc)
		kept++
	}
	if declared > 0 && kept == 0 {
		log.Warn("规则条件全部不可求值,整条规则不参与求值", "rule", r.ID, "name", r.Name)
		return Compiled{}, false
	}
	return c, true
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
	c := compiledCond{field: cond.Field, op: cond.Operator}
	// ip 字段的集合比较(不属于)走预解析的 IP/CIDR:单个 IP 与网段都支持。
	// 其余运算符(ip contains / ip regex 等)对 ip 按普通字符串处理。
	if cond.Field == FieldIP {
		switch cond.Operator {
		case OpIn, OpEq, OpNeq, OpNotIn:
			for _, v := range values {
				if _, n, err := net.ParseCIDR(v); err == nil {
					c.nets = append(c.nets, n)
					continue
				}
				if ip := net.ParseIP(v); ip != nil {
					c.ips = append(c.ips, ip)
				}
			}
			if len(c.ips) == 0 && len(c.nets) == 0 {
				return drop("ip 值既不是 IP 也不是 CIDR")
			}
			return c, true
		}
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
	}
	return c, true
}

// match 单条条件是否满足。
// 关键不变式:字段值取不到(空)时恒不命中——包括 not_in / neq / not_contains。
// 否则"取不到数据"会变成"全部命中"的反面,把 GeoIP 未接入的规则变成对全租户流量生效的拦截。
func (c *compiledCond) match(f *Fact) bool {
	raw, ok := f.value(c.field)
	if !ok {
		return false
	}
	switch c.op {
	case OpIn, OpEq, OpNeq, OpNotIn:
		return c.matchSet(f, raw)
	case OpContains, OpNotContains:
		return c.matchContains(raw)
	case OpGT, OpLT:
		return c.matchNumber(raw)
	case OpRegex:
		return c.matchRegex(raw)
	case OpDuplicated:
		// 计数来自调用方提供的 Fact.Seen;没有计数数据(=没接数据源)时恒不命中
		if f.Seen == nil {
			return false
		}
		return f.Seen[SeenKey(c.field, raw)] >= c.seen
	}
	return false
}

// matchSet 处理 in / eq / neq / not_in:values 里任一相等即满足(大小写不敏感)。
// ip 字段特殊:比对的是预解析的 IP 集合与 CIDR 集合
// (配置侧的 IP/CIDR 在加载期就解析好了,求值期只解析访客自己的地址,且只解析一次)。
// 四个运算符只共用一个 hit("访客 IP 落没落在集合里"),再由 c.op 决定取反不取反——
// 在 ip 分支里各写各的返回值会漏掉 neq,把它整体判反。
func (c *compiledCond) matchSet(f *Fact, raw string) bool {
	hit := false
	if c.field == FieldIP {
		ip := f.parsedIP()
		if ip == nil {
			return false
		}
		for _, want := range c.ips {
			if want.Equal(ip) {
				hit = true
				break
			}
		}
		if !hit {
			for _, n := range c.nets {
				if n.Contains(ip) {
					hit = true
					break
				}
			}
		}
	} else {
		for _, lit := range c.lits {
			if strings.EqualFold(raw, lit) {
				hit = true
				break
			}
		}
	}
	switch c.op {
	case OpIn, OpEq:
		return hit
	default: // neq / not_in
		return !hit
	}
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
func (c *Compiled) matchAll(f *Fact) bool {
	if len(c.conds) == 0 {
		return true
	}
	if c.logicAll {
		for i := range c.conds {
			if !c.conds[i].match(f) {
				return false
			}
		}
		return true
	}
	for i := range c.conds {
		if c.conds[i].match(f) {
			return true
		}
	}
	return false
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
// 先按首字节筛候选位置,再用 EqualFold 确认,避免退化到 O(n*m)。
func containsFold(s, sub string) bool {
	if sub == "" {
		return true
	}
	first := lowerASCII(sub[0])
	for i := 0; i+len(sub) <= len(s); i++ {
		if lowerASCII(s[i]) != first {
			continue
		}
		if strings.EqualFold(s[i:i+len(sub)], sub) {
			return true
		}
	}
	return false
}

// lowerASCII 只折叠 ASCII 大小写(UA/域名/系统名的判定都只涉及 ASCII;
// 非 ASCII 字符原样返回,不影响结果)。
func lowerASCII(b byte) byte {
	if b >= 'A' && b <= 'Z' {
		return b + ('a' - 'A')
	}
	return b
}
