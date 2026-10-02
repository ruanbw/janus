// 规则(租户级风控/分流策略)与「规则 ↔ 短链」关联的数据访问层。
// 关联模型见 spec D1:关联只存在规则一侧(scope=global 对租户全部短链生效,
// scope=links 只对 rule_links 里显式列出的短链生效),短链不持有规则列表,
// 两边看到的是同一份数据,不会漂移。
// 关键不变式:所有按 id 取/改/删规则的语句都带 tenant_id(租户隔离);
// scope='links' 且关联为空的规则永远不命中(spec D2,零关联不兜底成全局)。
package store

import (
	"bytes"
	"context"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"gorm.io/gorm/schema"
)

// MaxRulesPerTenant 单租户规则条数上限(spec D7)。
// 规则集合整租户常驻内存供跳转热路径求值,没有上限会让快照无限膨胀;
// 超出时由 API 层返回 403 拒绝创建。
const MaxRulesPerTenant = 200

// 规则作用域:global 对该租户全部短链生效;links 只对显式关联的短链生效。
const (
	RuleScopeGlobal = "global"
	RuleScopeLinks  = "links"
)

// 条件组之间的关系:all 全部满足;any 任一满足。
const (
	RuleLogicAll = "all"
	RuleLogicAny = "any"
)

// 命中后的处置(spec D4):pass 记录命中并继续原跳转流程;redirect 改写目标 URL;
// notfound 返回 404;throttle 返回 429。
const (
	RuleActionPass     = "pass"
	RuleActionRedirect = "redirect"
	RuleActionNotfound = "notfound"
	RuleActionThrottle = "throttle"
)

// ErrForeignLink 传入的短链 id 不属于该租户(或不存在)。
// 关联写入不接受静默丢弃:调用方会以为规则挂上了,实际一条都没生效。
var ErrForeignLink = errors.New("link not owned by tenant")

// ErrForeignRule 对称于 ErrForeignLink:传入的规则 id 不属于该租户(或不存在)。
var ErrForeignRule = errors.New("rule not owned by tenant")

// RuleLimitError 表示该租户的规则条数已达 MaxRulesPerTenant。
//
// 它由 CreateRule 在**持有租户行锁**的事务内返回,因此"已达上限"这个判断与
// 随后的插入是原子的,不存在并发绕过(旧实现是先 Count 再 Create,两者都在
// 事务外,计数停在 199 时可以并发写出远超上限的规则 —— 而规则集合整租户
// 常驻内存供热路径求值,超上限直接变成内存与求值成本问题)。
//
// Count/Limit 供调用方展示当前用量与上限,与 QuotaError 的 Usage 同构。
type RuleLimitError struct {
	Count int
	Limit int
}

func (e *RuleLimitError) Error() string {
	return fmt.Sprintf("rule limit exceeded: %d/%d", e.Count, e.Limit)
}

// IsRuleLimitExceeded 判断错误是否为规则条数超限(API 层映射成 403 用)。
func IsRuleLimitExceeded(err error) bool {
	var e *RuleLimitError
	return errors.As(err, &e)
}

// ConditionLeafNode 一条叶子条件(如 country in ['US','CA'])。
// 值只接受字面量:CIDR 列表、逗号分隔枚举、正则等(spec D6 名单库暂缓,条件值不支持名单引用)。
type ConditionLeafNode struct {
	Field    string   `json:"field"`
	Operator string   `json:"operator"`
	Values   []string `json:"values"`
}

// RuleCondition 是叶子条件的旧名,与 ConditionLeafNode 是同一个类型,
// 不是两份定义——存量代码(store/httpapi/tests)继续用 RuleCondition,无需改名。
type RuleCondition = ConditionLeafNode

// 条件树节点的两种 Type。只认这两个:不认识的 Type 按叶子处理。
const (
	ConditionTypeGroup = "group"
	ConditionTypeLeaf  = "leaf"
)

// ConditionGroupNode 一个条件组。Logic 为 all / any(空按 all),Children 为子节点。
type ConditionGroupNode struct {
	Logic    string          `json:"logic"`
	Children []ConditionNode `json:"children"`
}

// ConditionNode 条件树节点。Type="group" 时看 Group,Type="leaf" 时看 Leaf。
// 解析时容忍两种省略写法:对象里直接铺 field/operator/values 当叶子,
// 或者省略 type 只给 group——存量数据与前端手写 JSON 两种都见过。
type ConditionNode struct {
	Type  string              `json:"type"`
	Group *ConditionGroupNode `json:"group,omitempty"`
	Leaf  *ConditionLeafNode  `json:"leaf,omitempty"`
}

// RuleConditions 规则条件组(conditions 列,JSONB)。
//
// 两种形态并存,且都读、且写回原形态:
//
//	扁平:[{field,operator,values}, …]                      ← 存量数据,语义在 rules.logic(all/any)
//	树:  {"type":"group","logic":"any","children":[…]}     ← 复合条件,语义在树上
//
// 存量扁平行不迁移、不改写:一次无关的改名不会把 conditions 列从数组变成对象,
// 也就不会让所有历史行的 JSONB 产生无谓的重写与膨胀。
//
// 字段:
//   - Leaves 按遍历序展开的叶子。扁平形态下它就是载荷本身;树形态下是只读快照,
//     供详情回显、API 出参、决策链留痕直接用,不必每次重新遍历树。
//   - Root 仅在树形态下非 nil。Root==nil 即扁平形态,all/any 仍取 rules.logic,
//     求值语义与加条件树之前逐字相同。
//
// 两者不会同时是"权威":扁平看 Leaves,树看 Root。
type RuleConditions struct {
	Leaves []RuleCondition
	Root   *ConditionNode
}

// Conditions 组装一条扁平条件组(一层 all)。
func Conditions(leaves ...RuleCondition) RuleConditions {
	return RuleConditions{Leaves: leaves}
}

// Group 造一个条件组节点。
func Group(logic string, children ...ConditionNode) ConditionNode {
	return ConditionNode{Type: ConditionTypeGroup, Group: &ConditionGroupNode{Logic: logic, Children: children}}
}

// Leaf 造一个叶子节点。
func Leaf(c RuleCondition) ConditionNode {
	return ConditionNode{Type: ConditionTypeLeaf, Leaf: &c}
}

// Tree 造一条使用条件树的规则条件组;Leaves 由 flattenRoot 顺带算出。
func Tree(root ConditionNode) RuleConditions {
	c := RuleConditions{Root: &root}
	c.Leaves = flattenNode(root)
	return c
}

// IsTree 是否使用了条件树(否则按扁平形态求值)。
func (c RuleConditions) IsTree() bool { return c.Root != nil }

// Len 叶子条件条数(两种形态下同义)。
func (c RuleConditions) Len() int { return len(c.Leaves) }

// IsZero 是否为零值。零值与「显式清空条件」要区分:前者表示本次更新不动这一列。
func (c RuleConditions) IsZero() bool { return c.Root == nil && c.Leaves == nil }

// IsEmpty 无条件(无叶子):无条件组在求值侧等价于"恒成立的兜底规则",由调用方决定是否放行。
func (c RuleConditions) IsEmpty() bool { return len(c.Leaves) == 0 && !c.IsTree() }

// flattenRoot 深度优先展开树里的叶子(顺序 = 求值顺序,与短路语义一致)。
func flattenRoot(root ConditionNode) []RuleCondition { return flattenNode(root) }

func flattenNode(n ConditionNode) []RuleCondition {
	if n.Leaf != nil && n.Type != ConditionTypeGroup {
		return []RuleCondition{*n.Leaf}
	}
	if n.Group == nil {
		return nil
	}
	out := make([]RuleCondition, 0, len(n.Group.Children))
	for _, child := range n.Group.Children {
		out = append(out, flattenNode(child)...)
	}
	return out
}

// rawGroup / rawNode 是解析用的中间形态。Children 存 json.RawMessage 而不是
// ConditionNode,是为了让每个子节点都回到 nodeFromRaw 重新判型——否则
// "裸叶子"写法({field,operator,values} 直接铺在 children 数组的元素上)
// 只在顶层被认得,嵌在组里的那种会被当成废节点静默丢掉。
type rawGroup struct {
	Logic    string            `json:"logic"`
	Children []json.RawMessage `json:"children"`
}

type rawNode struct {
	Type     string             `json:"type"`
	Group    *rawGroup          `json:"group"`
	Leaf     *ConditionLeafNode `json:"leaf"`
	Field    string             `json:"field"`
	Operator string             `json:"operator"`
	Values   []string           `json:"values"`
	Logic    string             `json:"logic"`
	Children []json.RawMessage  `json:"children"`
}

// nodeFromRaw 认出它是一组还是一个叶子,并把子节点逐个归一化。
func nodeFromRaw(raw []byte) (ConditionNode, bool) {
	var probe rawNode
	if err := json.Unmarshal(raw, &probe); err != nil {
		return ConditionNode{}, false
	}
	switch {
	case probe.Group != nil:
		return normalizeGroup(*probe.Group), true
	case probe.Leaf != nil:
		return normalizeLeaf(*probe.Leaf), true
	case probe.Type == ConditionTypeGroup || probe.Children != nil:
		return normalizeGroup(rawGroup{Logic: probe.Logic, Children: probe.Children}), true
	case probe.Field != "":
		// 裸叶子:{field,operator,values} 直接铺在节点上
		return normalizeLeaf(ConditionLeafNode{Field: probe.Field, Operator: probe.Operator, Values: probe.Values}), true
	}
	return ConditionNode{}, false
}

// normalizeGroup 递归归一化子节点(丢掉既不是组也不是叶子的废节点),
// 并把缺失的 logic 补成 all。
func normalizeGroup(g rawGroup) ConditionNode {
	kids := make([]ConditionNode, 0, len(g.Children))
	for _, child := range g.Children {
		if n, ok := nodeFromRaw(child); ok {
			kids = append(kids, n)
		}
	}
	if g.Logic == "" {
		g.Logic = "all"
	}
	return ConditionNode{
		Type:  ConditionTypeGroup,
		Group: &ConditionGroupNode{Logic: g.Logic, Children: kids},
	}
}

func normalizeLeaf(l ConditionLeafNode) ConditionNode {
	return ConditionNode{Type: ConditionTypeLeaf, Leaf: &l}
}

// UnmarshalJSON 兼容两种历史形态:扁平数组与条件树对象。
// 解析失败一律降级为"无条件组"而不是报错:一条脏数据不该把整份规则加载拖垮
// (与 Scan 的既有约定一致)。结构性废节点(既无 group 也无 leaf)在解析期丢掉,
// 字段/运算符是否合法留给求值侧判定——那里才知道 13 个字段与运算符白名单。
func (c *RuleConditions) UnmarshalJSON(b []byte) error {
	*c = RuleConditions{}
	trimmed := bytes.TrimSpace(b)
	if len(trimmed) == 0 || string(trimmed) == "null" {
		return nil
	}
	switch trimmed[0] {
	case '[':
		var leaves []RuleCondition
		if err := json.Unmarshal(trimmed, &leaves); err != nil {
			return nil
		}
		if leaves == nil {
			leaves = []RuleCondition{}
		}
		c.Leaves = leaves
		return nil
	case '{':
		root, ok := nodeFromRaw(trimmed)
		if !ok {
			return nil
		}
		c.Root = &root
		c.Leaves = flattenNode(root)
		return nil
	}
	return nil
}

// MarshalJSON 写回原形态:树形态写对象,扁平形态写数组。
// 回写扁平而非一律写成树,是为了不把存量行在一次无关更新里改写成新形态。
func (c RuleConditions) MarshalJSON() ([]byte, error) {
	if c.IsTree() {
		return json.Marshal(c.Root)
	}
	if c.Leaves == nil {
		return []byte("[]"), nil
	}
	return json.Marshal(c.Leaves)
}

// Value 序列化为 JSONB 文本(空条件组写 "[]" 而不是 null)。
func (c RuleConditions) Value() (driver.Value, error) {
	b, err := c.MarshalJSON()
	if err != nil {
		return nil, err
	}
	return string(b), nil
}

// Scan 读回条件组;NULL 与非 JSON 一律归一为"无条件"(空条件组),
// 不让一条历史脏数据把整个规则加载拖垮。
func (c *RuleConditions) Scan(v any) error {
	switch n := v.(type) {
	case nil:
		*c = RuleConditions{}
		return nil
	case []byte:
		return c.decode(n)
	case string:
		return c.decode([]byte(n))
	}
	return fmt.Errorf("store: 无法把 %T 读成规则条件组", v)
}

func (c *RuleConditions) decode(b []byte) error {
	if len(bytes.TrimSpace(b)) == 0 {
		*c = RuleConditions{}
		return nil
	}
	if err := c.UnmarshalJSON(b); err != nil {
		*c = RuleConditions{}
	}
	return nil
}

// GormDBDataType 声明该字段在 postgres 中的列类型为 jsonb
// (与 Tag 里的 type:jsonb 二选一即可,这里写一份是为了 schema 自解释)。
func (RuleConditions) GormDBDataType(db *gorm.DB, field *schema.Field) string {
	if db.Dialector.Name() == "postgres" {
		return "jsonb"
	}
	return "json"
}

// 规则类型(Tier-1 visual 默认可视化条件树; Tier-2 expression Expr 表达式)
const (
	RuleTypeVisual     = "visual"
	RuleTypeExpression = "expression"
)

// Rule 租户级规则。条件字段收敛为后端从请求即可真实求值的 13 个(spec D5),
// 不可求值的字段不落库,避免"能配不能跑"的假能力。
// LinkIDs / LinkNames / LinkCount / Hits24h 是投影字段(查询后填充,不落库):
// 前三个来自 rule_links(LinkNames 形如「短码@域名」,见 fillRuleLinkMeta),
// Hits24h 由 CountRuleHits24h 从 visits 读时聚合(spec D9,不在本表存计数器)。
// 写入侧注意:bool/int 字段没有"零值即未设置"的区分,CreateRule 会把 Enabled=false
// 原样写进库(不是 DDL 默认的 true)——需要默认启用的调用方自己显式置 true。
type Rule struct {
	ID          int64          `json:"id" gorm:"primaryKey"`
	TenantID    int64          `json:"-" gorm:"column:tenant_id"`
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Priority    int            `json:"priority" gorm:"column:priority"`
	Scope       string         `json:"scope"`
	Enabled     bool           `json:"enabled"`
	Logic       string         `json:"logic"`
	Action      string         `json:"action"`
	Destination string         `json:"destination"`
	PageMode    string         `json:"pageMode" gorm:"column:page_mode"`
	CustomHTML  string         `json:"customHtml" gorm:"column:custom_html"`
	RuleType    string         `json:"ruleType" gorm:"column:rule_type"`
	Expression  string         `json:"expression" gorm:"column:expression"`
	Conditions  RuleConditions `json:"conditions" gorm:"column:conditions;type:jsonb"`
	LinkIDs     []int64        `json:"linkIds" gorm:"-"`
	LinkNames   []string       `json:"linkNames" gorm:"-"`
	LinkCount   int            `json:"linkCount" gorm:"-"`
	Hits24h     int64          `json:"hits24h" gorm:"-"`
	CreatedAt   time.Time      `json:"createdAt" gorm:"column:created_at"`
	UpdatedAt   time.Time      `json:"updatedAt" gorm:"column:updated_at"`
}

// RuleLink 规则-短链关联(唯一约束 (rule_id, link_id))。
// 两侧都是 ON DELETE CASCADE:短链物理删除或规则删除,关联一并消失。
type RuleLink struct {
	ID     int64 `json:"id" gorm:"primaryKey"`
	RuleID int64 `json:"ruleId" gorm:"column:rule_id"`
	LinkID int64 `json:"linkId" gorm:"column:link_id"`
}

// fillRuleLinkMeta 批量补充一组规则的关联短链 id / 可读标识 / 关联数(列表页避免 N+1)。
// 可读标识形如「短码@域名」而不是只有短码:短码在租户内不唯一(唯一性只在"同一域名下"),
// 只给短码会让同码的两条短链在界面上无法区分,编辑器回填关联时也就选错了。
// 取该短链第一个域名(link_domains 按 domain id 升序的第一个)作为标识里的域名,
// 关联本身与域名无关——短链挂了哪个域名都算"这条规则适用于它"。
// 逻辑删除的短链也照实列出:关联行只在物理删除时随 CASCADE 消失,
// 求值侧由 spec D4 保证"短链不可用 → 404,规则不参与",这里不做可见性过滤。
func (s *Store) fillRuleLinkMeta(ctx context.Context, rules []*Rule) error {
	if len(rules) == 0 {
		return nil
	}
	ids := make([]int64, 0, len(rules))
	for _, r := range rules {
		ids = append(ids, r.ID)
	}
	type row struct {
		RuleID int64  `gorm:"column:rule_id"`
		LinkID int64  `gorm:"column:link_id"`
		Code   string `gorm:"column:code"`
		FQDN   string `gorm:"column:fqdn"`
	}
	var rows []row
	// fqdn 是相关子查询(按 domain id 取第一个),不是在 link_domains 上做 join ——
	// 一条短链可以挂多个域名,join 会把同一行复制多份,关联数被放大。
	if err := s.db.WithContext(ctx).Table("rule_links rl").
		Select(`rl.rule_id, rl.link_id, l.code,
			(SELECT d.fqdn FROM link_domains ld JOIN domains d ON d.id = ld.domain_id
			  WHERE ld.link_id = l.id ORDER BY d.id LIMIT 1) AS fqdn`).
		Joins("JOIN links l ON l.id = rl.link_id").
		Where("rl.rule_id IN ?", ids).
		Order("rl.rule_id, rl.link_id").Scan(&rows).Error; err != nil {
		return err
	}
	idMap := make(map[int64][]int64, len(rules))
	nameMap := make(map[int64][]string, len(rules))
	for _, row := range rows {
		idMap[row.RuleID] = append(idMap[row.RuleID], row.LinkID)
		nameMap[row.RuleID] = append(nameMap[row.RuleID], ruleLinkName(row.Code, row.FQDN))
	}
	for _, r := range rules {
		r.LinkIDs = idMap[r.ID]
		r.LinkNames = nameMap[r.ID]
		r.LinkCount = len(r.LinkIDs)
	}
	return nil
}

// ruleLinkName 拼关联短链的可读标识「短码@域名」。
// 域名缺失(短链被物理删除到只剩孤儿关联行的极端情况)时退回只给短码,而不是给一个尾部悬空的
// "@"——可读标识是给人看的,不必为了格式完整给出误导性的字符串。
func ruleLinkName(code, fqdn string) string {
	if fqdn == "" {
		return code
	}
	return code + "@" + fqdn
}

// dedupeIDs 去重并保持顺序(短链 id 列表的公共前置处理)。
func dedupeIDs(ids []int64) []int64 {
	seen := make(map[int64]struct{}, len(ids))
	out := make([]int64, 0, len(ids))
	for _, id := range ids {
		if id <= 0 {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out
}

// checkLinkOwnership 校验短链 id 全部属于该租户(库内不存在或属于他人一律报错)。
// 刻意不静默丢弃(spec:"配置写入,静默丢弃会让租户以为规则挂上了")。
// 计数含逻辑删除的短链:关联是否清除由物理删除决定,与 CASCADE 语义一致。
func checkLinkOwnership(tx *gorm.DB, tenantID int64, linkIDs []int64) error {
	ids := dedupeIDs(linkIDs)
	if len(ids) == 0 {
		return nil
	}
	var owned []int64
	if err := tx.Model(&Link{}).Where("tenant_id = ? AND id IN ?", tenantID, ids).
		Pluck("id", &owned).Error; err != nil {
		return err
	}
	if len(owned) != len(ids) {
		return ErrForeignLink
	}
	return nil
}

// replaceRuleLinks 整体替换某条规则的关联短链(先删后插,在调用方的事务内执行)。
// 先校验归属,再落库:半条关联比没有关联更难排查。
func replaceRuleLinks(tx *gorm.DB, tenantID, ruleID int64, linkIDs []int64) error {
	if err := checkLinkOwnership(tx, tenantID, linkIDs); err != nil {
		return err
	}
	if err := tx.Where("rule_id = ?", ruleID).Delete(&RuleLink{}).Error; err != nil {
		return err
	}
	for _, id := range dedupeIDs(linkIDs) {
		rl := RuleLink{RuleID: ruleID, LinkID: id}
		if err := tx.Create(&rl).Error; err != nil {
			return err
		}
	}
	return nil
}

// WithRuleCountCheckInTx 在一个事务里完成「锁租户行 → 数规则 → 判上限 → 执行写入」。
//
// 为什么需要它(与 quota.go 的 WithQuotaInTx 同构,但刻意不复用):
//  1. 规则条数上限 MaxRulesPerTenant 是**写死的产品上限**,不是 tier 配额 ——
//     它不进 Usage、不进套餐页、不参与 admin 改档。把它塞进 QuotaKind 等于
//     给"可配置的东西"凭空加一个不可配置的兄弟,后面每次动配额都要重新判断
//     它算不算 kind。
//  2. quota.go 归另一个代理所有,本波不改它。
//
// 锁的粒度:SELECT ... FOR UPDATE 锁住 tenants 行,把同一租户的所有规则创建
// 串行化;后到的请求必须等前一个事务提交,然后在锁内重新数一遍(此时已经含前一笔)。
// 不同租户之间互不阻塞。
//
// fn 必须使用传入的 tx 写入,用 s.db 会跑在事务外,锁就白加了。
func (s *Store) WithRuleCountCheckInTx(ctx context.Context, tenantID int64, fn func(tx *gorm.DB) error) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// FOR UPDATE:同租户串行,跨租户并行。租户不存在时 First 报错(→ 404)。
		var locked Tenant
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id = ?", tenantID).
			First(&locked).Error; err != nil {
			return err
		}
		var n int64
		if err := tx.Model(&Rule{}).Where("tenant_id = ?", tenantID).Count(&n).Error; err != nil {
			return err
		}
		if int(n) >= MaxRulesPerTenant {
			return &RuleLimitError{Count: int(n), Limit: MaxRulesPerTenant}
		}
		return fn(tx)
	})
}

// CreateRule 创建规则;r.LinkIDs 非空时在同事务内建立关联。
// tenantID 以参数为准(不从入参对象上读),空 scope/logic 归一为 DDL 里的默认值。
// 唯一约束冲突(同租户重名)返回唯一约束错误(整个创建回滚,由 IsUniqueViolation 判定)。
//
// 规则条数上限(MaxRulesPerTenant)在**事务内、持租户行锁**判定,见
// WithRuleCountCheckInTx:超限返回 *RuleLimitError,不写任何数据。
func (s *Store) CreateRule(ctx context.Context, tenantID int64, r Rule) (*Rule, error) {
	if r.Conditions.IsZero() {
		r.Conditions = Conditions()
	}
	// DDL 有默认值但 GORM 会把零值一并 INSERT,CHECK 约束不接受空串,这里补上默认值
	if r.Scope == "" {
		r.Scope = RuleScopeGlobal
	}
	if r.Logic == "" {
		r.Logic = RuleLogicAll
	}
	if r.PageMode == "" {
		r.PageMode = "default"
	}
	if r.RuleType == "" {
		r.RuleType = RuleTypeVisual
	}
	r.TenantID = tenantID
	err := s.WithRuleCountCheckInTx(ctx, tenantID, func(tx *gorm.DB) error {
		if err := tx.Create(&r).Error; err != nil {
			return err
		}
		return replaceRuleLinks(tx, tenantID, r.ID, r.LinkIDs)
	})
	if err != nil {
		return nil, err
	}
	return s.GetRule(ctx, tenantID, r.ID)
}

// GetRule 按 id 取规则(租户隔离),并填充关联短链。
func (s *Store) GetRule(ctx context.Context, tenantID, id int64) (*Rule, error) {
	var r Rule
	if err := s.db.WithContext(ctx).Where("id = ? AND tenant_id = ?", id, tenantID).First(&r).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	if err := s.fillRuleLinkMeta(ctx, []*Rule{&r}); err != nil {
		return nil, err
	}
	return &r, nil
}

// ListRules 分页列出租户规则,按求值顺序(priority 升序, id 升序)。
// 返回规则切片与总数;关联短链一次性批量填充。
func (s *Store) ListRules(ctx context.Context, tenantID int64, page, pageSize int) ([]Rule, int, error) {
	var total int64
	if err := s.db.WithContext(ctx).Model(&Rule{}).Where("tenant_id = ?", tenantID).Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var out []*Rule
	if err := s.db.WithContext(ctx).Where("tenant_id = ?", tenantID).
		Order("priority, id").
		Limit(pageSize).Offset((page - 1) * pageSize).Find(&out).Error; err != nil {
		return nil, 0, err
	}
	if err := s.fillRuleLinkMeta(ctx, out); err != nil {
		return nil, 0, err
	}
	rules := make([]Rule, 0, len(out))
	for _, r := range out {
		rules = append(rules, *r)
	}
	return rules, int(total), nil
}

// RulesByIDs 按 id 批量取本租户的规则(租户隔离;不存在的 id 不返回)。
// 用于把"传进来的 id 到底指哪条规则"一次性查清,进而区分两种完全不同的 400:
// 混入 scope=global 的规则,和跨租户/不存在的 id。
// ids 为空时直接返回空切片,不生成非法的 IN () 条件。
func (s *Store) RulesByIDs(ctx context.Context, tenantID int64, ids []int64) ([]Rule, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	var out []Rule
	err := s.db.WithContext(ctx).Where("tenant_id = ? AND id IN ?", tenantID, ids).
		Order("priority, id").Find(&out).Error
	return out, err
}

// ListAllRules 列出租户全部规则(不分页),按求值顺序。
// 用于下拉选项这类"要把整个规则集合交出去"的读路径——租户规则数有 MaxRulesPerTenant 上限,
// 不存在需要翻页的几千条。
func (s *Store) ListAllRules(ctx context.Context, tenantID int64) ([]Rule, error) {
	var out []*Rule
	if err := s.db.WithContext(ctx).Where("tenant_id = ?", tenantID).
		Order("priority, id").Find(&out).Error; err != nil {
		return nil, err
	}
	if err := s.fillRuleLinkMeta(ctx, out); err != nil {
		return nil, err
	}
	rules := make([]Rule, 0, len(out))
	for _, r := range out {
		rules = append(rules, *r)
	}
	return rules, nil
}

// CountRuleHits24h 按 rule_id 聚合近 24 小时的命中次数(spec D9,读时聚合)。
//
// 为什么不存计数器:命中发生在跳转热路径上,每命中一次 UPDATE 一次 rules 就是给链路上
// QPS 最高的部分加一次写放大;而且计数器重启即丢,还会与明细表对不上。
// 这里数的是"带 rule_id 的明细行",包含被规则拦下的(failed)访问——
// 拦住了就是这条规则起了作用,租户在规则页要看到的正是这个。
// 零关联的规则永远不会被命中,自然恒为 0。
// ruleIDs 为空时直接返回空 map,不生成非法的 IN () 条件。
func (s *Store) CountRuleHits24h(ctx context.Context, ruleIDs []int64) (map[int64]int64, error) {
	out := make(map[int64]int64, len(ruleIDs))
	if len(ruleIDs) == 0 {
		return out, nil
	}
	type hitRow struct {
		RuleID int64 `gorm:"column:rule_id"`
		Hits   int64 `gorm:"column:hits"`
	}
	var rows []hitRow
	if err := s.db.WithContext(ctx).Table("visits").
		Select("rule_id, count(*) AS hits").
		Where("rule_id IN ? AND created_at > now() - interval '24 hours'", ruleIDs).
		Group("rule_id").Scan(&rows).Error; err != nil {
		return nil, err
	}
	for _, row := range rows {
		out[row.RuleID] = row.Hits
	}
	return out, nil
}

// RuleUpdate 规则局部更新字段(指针非空才更新)。
// LinkIDs 非 nil 时整体替换关联短链(空数组 = 清空关联);为 nil 表示不动关联。
//
// 例外:scope 切到 global 时**强制清空关联**,且忽略同时传入的 LinkIDs。
// 理由:rule_links 的一行表示「这条规则适用于这条短链」,而 scope=global 已经表示
// 「适用于全部短链」,两者同时成立是数据模型在自相矛盾。留着矛盾行的代价是租户日后
// 切回 links 时,早已忘记的旧关联会静默复活,一条他以为已收窄的规则突然又只管那几条链。
type RuleUpdate struct {
	Name        *string
	Description *string
	Priority    *int
	Scope       *string
	Enabled     *bool
	Logic       *string
	Action      *string
	Destination *string
	PageMode    *string
	CustomHTML  *string
	RuleType    *string
	Expression  *string
	Conditions  *RuleConditions
	LinkIDs     *[]int64
}

// UpdateRule 更新规则(租户隔离)。传 LinkIDs 时在同一事务内替换关联。
func (s *Store) UpdateRule(ctx context.Context, tenantID, id int64, upd RuleUpdate) (*Rule, error) {
	// 先按租户取一次:既确认存在(跨租户/不存在 → ErrNotFound),又拿到现值做合并
	cur, err := s.GetRule(ctx, tenantID, id)
	if err != nil {
		return nil, err
	}
	fields := map[string]any{}
	put := func(col string, set bool, v any) {
		if set {
			fields[col] = v
		}
	}
	put("name", upd.Name != nil, derefOr(cur.Name, upd.Name))
	put("description", upd.Description != nil, derefOr(cur.Description, upd.Description))
	put("priority", upd.Priority != nil, derefOr(cur.Priority, upd.Priority))
	put("scope", upd.Scope != nil, derefOr(orDefault(cur.Scope, RuleScopeGlobal), upd.Scope))
	put("enabled", upd.Enabled != nil, derefOr(cur.Enabled, upd.Enabled))
	put("logic", upd.Logic != nil, derefOr(orDefault(cur.Logic, RuleLogicAll), upd.Logic))
	put("action", upd.Action != nil, derefOr(cur.Action, upd.Action))
	put("destination", upd.Destination != nil, derefOr(cur.Destination, upd.Destination))
	put("page_mode", upd.PageMode != nil, derefOr(orDefault(cur.PageMode, "default"), upd.PageMode))
	put("custom_html", upd.CustomHTML != nil, derefOr(cur.CustomHTML, upd.CustomHTML))
	put("rule_type", upd.RuleType != nil, derefOr(orDefault(cur.RuleType, RuleTypeVisual), upd.RuleType))
	put("expression", upd.Expression != nil, derefOr(cur.Expression, upd.Expression))
	if upd.Conditions != nil && !upd.Conditions.IsZero() {
		fields["conditions"] = *upd.Conditions
	}
	newScope := orDefault(cur.Scope, RuleScopeGlobal)
	if upd.Scope != nil {
		newScope = *upd.Scope
	}
	// scope 切到 global 时强制清空关联(见 RuleUpdate 注释的取舍说明);
	// scopeToGlobal 的判定延迟到事务内锁定行之后做(见下)。
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// 在锁内重读现值:scope→global 的清关联判定不能依赖事务外的旧快照,
		// 否则与并发的 add-links 交错时,global 规则上会残留 rule_links。
		var locked Rule
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id = ? AND tenant_id = ?", id, tenantID).First(&locked).Error; err != nil {
			return err
		}
		scopeToGlobal := newScope == RuleScopeGlobal && locked.Scope != RuleScopeGlobal
		linkIDs := upd.LinkIDs
		if scopeToGlobal {
			empty := []int64{}
			linkIDs = &empty
		}
		if len(fields) > 0 {
			if err := tx.Model(&Rule{}).Where("id = ? AND tenant_id = ?", id, tenantID).
				Updates(fields).Error; err != nil {
				return err
			}
		}
		if linkIDs != nil {
			return replaceRuleLinks(tx, tenantID, id, *linkIDs)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return s.GetRule(ctx, tenantID, id)
}

// DeleteRule 删除规则(租户隔离),关联行随库内 ON DELETE CASCADE 消失。
func (s *Store) DeleteRule(ctx context.Context, tenantID, id int64) error {
	res := s.db.WithContext(ctx).Where("id = ? AND tenant_id = ?", id, tenantID).Delete(&Rule{})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

// SetRuleLinks 整体替换规则的关联短链(租户隔离 + 归属校验)。
// 传空切片即清空关联(spec D2:清空后规则不再命中,但它仍然是合法配置)。
// 传入跨租户/不存在的短链 id 返回 ErrForeignLink,不静默丢弃。
func (s *Store) SetRuleLinks(ctx context.Context, tenantID, ruleID int64, linkIDs []int64) error {
	var exists int64
	if err := s.db.WithContext(ctx).Model(&Rule{}).
		Where("id = ? AND tenant_id = ?", ruleID, tenantID).
		Pluck("id", &exists).Error; err != nil {
		return err
	}
	if exists == 0 {
		return ErrNotFound
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return replaceRuleLinks(tx, tenantID, ruleID, linkIDs)
	})
}

// SetLinkRules 整体替换某条短链与 scoped 规则(rule_links)的关联(短链侧的写入口)。
//
// 与 SetRuleLinks 是同一份数据的两个写入口(spec D1 规则侧是唯一写入口,短链表单的勾选
// 本质是改写同一条关联),区别只在方向:这里按 link_id 删多余、补缺少,不需要先知道
// 该短链原先关联了哪些规则。
// 事务内先校验规则归属(任一 id 不属于该租户/不存在 → ErrForeignRule,整笔回滚):
// 配置写入不接受静默丢弃,否则界面显示"已关联"而库里一条都没有。
// 调用方另需自行校验这些规则的 scope——库里没有"全局规则不应有关联行"的 CHECK,
// 作用域判定属于业务语义(store 层只保证引用的是本租户真实存在的规则)。
func (s *Store) SetLinkRules(ctx context.Context, tenantID, linkID int64, ruleIDs []int64) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		ids := dedupeIDs(ruleIDs)
		var owned []int64
		if err := tx.Model(&Rule{}).Where("tenant_id = ? AND id IN ?", tenantID, ids).
			Pluck("id", &owned).Error; err != nil {
			return err
		}
		if len(owned) != len(ids) {
			return ErrForeignRule
		}
		// 空数组 = 解除全部关联:此时不能生成 "rule_id NOT IN ()" 这种非法条件
		if len(ids) == 0 {
			return tx.Where("link_id = ?", linkID).Delete(&RuleLink{}).Error
		}
		if err := tx.Where("link_id = ? AND rule_id NOT IN ?", linkID, ids).
			Delete(&RuleLink{}).Error; err != nil {
			return err
		}
		for _, id := range ids {
			rl := RuleLink{RuleID: id, LinkID: linkID}
			// ON CONFLICT DO NOTHING:已存在的关联不该因为"整体替换"的语义被重复插入
			if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&rl).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

// CountOwnedLiveLinks 统计给定短链 id 中"属于该租户且未逻辑删除"的条数。
// 规则关联写入前用它做归属校验:与去重后的 id 数相等即全部命中。
// 与 checkLinkOwnership 的差别正是"排除逻辑删除":契约要求已逻辑删除的短链 id
// 也算非法(关联一条看不见的短链只会在界面上造成误解)。
// ids 为空时返回 0,不生成非法的 IN () 条件。
func (s *Store) CountOwnedLiveLinks(ctx context.Context, tenantID int64, ids []int64) (int, error) {
	if len(ids) == 0 {
		return 0, nil
	}
	var n int64
	if err := s.db.WithContext(ctx).Model(&Link{}).
		Where("tenant_id = ? AND id IN ? AND deleted_at IS NULL", tenantID, ids).
		Count(&n).Error; err != nil {
		return 0, err
	}
	return int(n), nil
}

// ListRuleLinks 列出某条规则的全部关联行(按 link_id 升序)。
func (s *Store) ListRuleLinks(ctx context.Context, ruleID int64) ([]RuleLink, error) {
	var out []RuleLink
	err := s.db.WithContext(ctx).Where("rule_id = ?", ruleID).Order("link_id").Find(&out).Error
	return out, err
}

// RuleIDsForLink 返回显式关联到该短链的规则 id(仅 scope='links' 的规则,按 link_id 升序)。
func (s *Store) RuleIDsForLink(ctx context.Context, linkID int64) ([]int64, error) {
	var out []int64
	err := s.db.WithContext(ctx).Model(&RuleLink{}).Where("link_id = ?", linkID).
		Order("id").Pluck("rule_id", &out).Error
	return out, err
}

// RulesForLink 返回适用于该短链的规则:全局规则(必然生效) + 显式关联的 scoped 规则,
// 按求值顺序(priority 升序, id 升序)。
// 包含停用规则并由 Enabled 字段表达状态——"适用"与"启用"是两件事,后台要能看见停用中的规则。
// 租户隔离:规则与短链两侧都限定 tenant_id,传入他人的短链 id 只会得到该租户自己的全局规则。
func (s *Store) RulesForLink(ctx context.Context, tenantID, linkID int64) ([]Rule, error) {
	var out []*Rule
	err := s.db.WithContext(ctx).Model(&Rule{}).
		Where(`tenant_id = ? AND (
			scope = ?
			OR EXISTS (SELECT 1 FROM rule_links rl JOIN links l ON l.id = rl.link_id
			           WHERE rl.rule_id = rules.id AND rl.link_id = ? AND l.tenant_id = ?)
		)`, tenantID, RuleScopeGlobal, linkID, tenantID).
		Order("priority, id").Find(&out).Error
	if err != nil {
		return nil, err
	}
	if err := s.fillRuleLinkMeta(ctx, out); err != nil {
		return nil, err
	}
	rules := make([]Rule, 0, len(out))
	for _, r := range out {
		rules = append(rules, *r)
	}
	return rules, nil
}

// CountTenantRules 统计租户规则数(用于 MaxRulesPerTenant 配额校验)。
func (s *Store) CountTenantRules(ctx context.Context, tenantID int64) (int, error) {
	var n int64
	if err := s.db.WithContext(ctx).Model(&Rule{}).Where("tenant_id = ?", tenantID).Count(&n).Error; err != nil {
		return 0, err
	}
	return int(n), nil
}

// RulesForTenant 取本租户全部启用规则(含关联短链 id),按求值顺序排列——
// 跳转热路径上规则集合由内存快照提供,这份查询只在快照失效/首次加载时执行一次。
// 停用规则不进快照(它们不参与求值),空集合同样是合法快照。
// 签名与 internal/rules.Cache 的 Loader 一致,可直接以方法值注入。
func (s *Store) RulesForTenant(ctx context.Context, tenantID int64) ([]Rule, error) {
	var out []*Rule
	if err := s.db.WithContext(ctx).Where("tenant_id = ? AND enabled = true", tenantID).
		Order("priority, id").Find(&out).Error; err != nil {
		return nil, err
	}
	if err := s.fillRuleLinkMeta(ctx, out); err != nil {
		return nil, err
	}
	rules := make([]Rule, 0, len(out))
	for _, r := range out {
		rules = append(rules, *r)
	}
	return rules, nil
}

// derefOr 取更新值;指针为空时退回现值(便于用 map 一次性更新,零值也能写入)。
func derefOr[T any](cur T, upd *T) T {
	if upd != nil {
		return *upd
	}
	return cur
}

// orDefault 空串退回 DDL 默认值(CHECK 约束不接受空串)。
func orDefault(v, def string) string {
	if v == "" {
		return def
	}
	return v
}
