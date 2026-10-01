package httpapi

// 规则(租户级访问处置规则)与「规则 ↔ 短链」关联的 API 层。
//
// 关联模型(spec D1):关联只存在规则一侧。规则的 scope 决定生效范围——
// scope=global 对本租户全部短链生效,scope=links 只对 rule_links 里显式列出的短链生效。
// 短链表单里的勾选与规则编辑器里的多选写的是同一份 rule_links,不存在两份会漂移的数据。
// 零关联的 scope=links 规则是合法配置(spec D2):它永远不会命中,后端不拦,
// 界面必须显式标出「未关联短链 · 不会命中」——静默兜底成全局是不可逆的线上事故。
//
// 三条容易出事、因而在这里显式对待的不变式:
//  1. 读必须完整:GET /api/rules/{id} 回完整未截断的 conditions 与 linkIds。
//     前端编辑器是"原样回显、整体替换保存",只回前 N 个会被原样写回,
//     静默截断真实条件/关联且不报错——租户看不出任何异常,一保存规则就失效了。
//     (linkNames 是唯一允许截断的字段:它只是给人看的徽标,完整关联由 linkIds 给出。)
//  2. 写必须能区分"省略"与"空":PATCH 的 linkIds 是 *[]int64。省略 = 不动关联,
//     [] = 解除全部。裸 []int64 会把 [] 吃成"没传",于是"改成全局"变成
//     "先清空关联再改作用域",规则从"作用于全部短链"静默退化成"一条都不命中"。
//  3. 变更后必须失效快照:规则集合按租户缓存在内存里供跳转热路径求值,
//     漏调 Invalidate 会让刚保存的规则在快照重建前完全不生效,且没有任何报错。

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/netip"
	"net/url"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"cloak/internal/rules"
	"cloak/internal/store"
)

// maxRuleLinkNames 列表里回传的关联短链可读标识上限(完整关联由 linkIds 给出)。
const maxRuleLinkNames = 3

// defaultRulePriority 新建规则未指定优先级时的取值(与 DDL 默认一致:数字小者先评估)。
const defaultRulePriority = 100

// ---------- 请求体 ----------

// ruleReq POST /api/rules 与 PATCH /api/rules/{id} 共用的请求体。
// 全部字段用指针:必须能区分"没传这个字段"(PATCH 不动它)与"传了零值"
// (如 enabled=false、destination=""、conditions:[])。任一字段用裸值都会把其中一种吃掉。
type ruleReq struct {
	Name        *string               `json:"name"`
	Description *string               `json:"description"`
	Priority    *int                  `json:"priority"`
	Scope       *string               `json:"scope"`
	Enabled     *bool                 `json:"enabled"`
	Logic       *string               `json:"logic"`
	Action      *string               `json:"action"`
	Destination *string               `json:"destination"`
	Conditions  *store.RuleConditions `json:"conditions"`
	// LinkIDs 必须是 *[]int64(指针到切片),理由见文件头不变式 ②。
	LinkIDs    *[]int64 `json:"linkIds"`
	PageMode   *string  `json:"pageMode"`
	CustomHTML *string  `json:"customHtml"`
	RuleType   *string  `json:"ruleType"`
	Expression *string  `json:"expression"`
}

// ruleWrite 归一化并校验后的待写入规则(是"合成结果"而不是"请求增量":
// PATCH 也要按合成后的值校验,否则"只改 action"会被漏判成没有 destination 的 redirect 规则)。
type ruleWrite struct {
	name        string
	description string
	priority    int
	scope       string
	enabled     bool
	logic       string
	action      string
	destination string
	conditions  store.RuleConditions
	// linkIDs 非 nil 时整体替换关联;nil = 不动关联。scope=global 时恒指向空切片
	// (关联与"作用于全部短链"同时成立是自相矛盾的数据状态,见 store.RuleUpdate 注释)。
	linkIDs    *[]int64
	pageMode   string
	customHTML string
	ruleType   string
	expression string
}

// ruleErr 构造一条 400 校验错误(apiErr 由 writeAPIError 统一序列化)。
func ruleErr(format string, args ...any) error {
	return apiErr{http.StatusBadRequest, errValidation, fmt.Sprintf(format, args...), nil}
}

// resolveRule 把请求体与规则现值合成为待写入的完整规则,并逐条校验契约里的 400 条件。
// cur 为 nil 表示新建。校验失败返回 apiErr(→ 400);数据库错误原样返回(→ 500)。
func (a *API) resolveRule(ctx context.Context, tenantID int64, cur *store.Rule, req ruleReq) (ruleWrite, error) {
	w := ruleWrite{
		name:        strings.TrimSpace(curStr(cur, func(r *store.Rule) string { return r.Name })),
		description: curStr(cur, func(r *store.Rule) string { return r.Description }),
		priority:    defaultRulePriority,
		scope:       store.RuleScopeGlobal,
		enabled:     true,
		logic:       store.RuleLogicAll,
		destination: "",
		conditions:  store.RuleConditions{},
		pageMode:    "default",
		customHTML:  "",
	}
	if cur != nil {
		// 现值原样带过来(不靠"零值即未设置"去猜):priority=0 是合法配置,
		// 若用 `if cur.Priority != 0` 区分,一次不改优先级的 PATCH 就会把它重置成默认值。
		w.priority = cur.Priority
		w.scope = firstNonEmpty(cur.Scope, store.RuleScopeGlobal)
		w.enabled = cur.Enabled
		w.logic = firstNonEmpty(cur.Logic, store.RuleLogicAll)
		w.action = cur.Action
		w.destination = cur.Destination
		w.pageMode = firstNonEmpty(cur.PageMode, "default")
		w.customHTML = cur.CustomHTML
		w.ruleType = firstNonEmpty(cur.RuleType, store.RuleTypeVisual)
		w.expression = cur.Expression
		// 现值无条件时就是零值,直接带过来(条件列不再是切片,靠 IsZero 区分未设置)
		w.conditions = cur.Conditions
	} else {
		w.ruleType = store.RuleTypeVisual
		w.expression = ""
	}
	// ① name 必填且非空白(新建时缺失同样算非法)
	if req.Name != nil {
		w.name = strings.TrimSpace(*req.Name)
	}
	if w.name == "" {
		return ruleWrite{}, ruleErr("name 不能为空")
	}
	if req.Description != nil {
		w.description = *req.Description
	}
	// ② priority 不得为负(数字小者先评估,负数会让它排在所有规则之前且无意义)
	if req.Priority != nil {
		w.priority = *req.Priority
	}
	if w.priority < 0 {
		return ruleWrite{}, ruleErr("priority 不能为负数")
	}
	// ③ scope 只能是 global / links
	if req.Scope != nil && *req.Scope != "" {
		w.scope = *req.Scope
	}
	if w.scope != store.RuleScopeGlobal && w.scope != store.RuleScopeLinks {
		return ruleWrite{}, ruleErr("scope 必须为 global 或 links")
	}
	if req.Enabled != nil {
		w.enabled = *req.Enabled
	}
	// ⑥ logic 只能是 all / any
	if req.Logic != nil && *req.Logic != "" {
		w.logic = *req.Logic
	}
	if w.logic != store.RuleLogicAll && w.logic != store.RuleLogicAny {
		return ruleWrite{}, ruleErr("logic 必须为 all 或 any")
	}
	// ④ action 必须是四个处置之一(新建时必填)
	if req.Action != nil && *req.Action != "" {
		w.action = *req.Action
	}
	switch w.action {
	case store.RuleActionPass, store.RuleActionRedirect,
		store.RuleActionNotfound, store.RuleActionThrottle:
	case "":
		return ruleWrite{}, ruleErr("action 不能为空")
	default:
		return ruleWrite{}, ruleErr("action 必须为 pass / redirect / notfound / throttle")
	}
	if req.Destination != nil {
		w.destination = *req.Destination
	}
	// ⑤ action=redirect 时 destination 必须是合法 URL(沿用 validTargetURL:拒控制字符)
	if w.action == store.RuleActionRedirect && !validTargetURL(w.destination) {
		return ruleWrite{}, ruleErr("action=redirect 时 destination 必须是合法 URL(不能为空、超长或包含控制字符)")
	}
	// ⑩ pageMode 与 customHtml 校验
	if req.PageMode != nil && *req.PageMode != "" {
		w.pageMode = *req.PageMode
	}
	if w.pageMode != "default" && w.pageMode != "custom" {
		return ruleWrite{}, ruleErr("pageMode 必须为 default 或 custom")
	}
	if req.CustomHTML != nil {
		w.customHTML = *req.CustomHTML
	}
	if len(w.customHTML) > 512*1024 {
		return ruleWrite{}, ruleErr("customHtml 超过大小上限(512KB)")
	}
	// ⑪ ruleType 与 expression 校验
	if req.RuleType != nil && *req.RuleType != "" {
		w.ruleType = *req.RuleType
	}
	if w.ruleType != store.RuleTypeVisual && w.ruleType != store.RuleTypeExpression {
		return ruleWrite{}, ruleErr("ruleType 必须为 visual 或 expression")
	}
	if req.Expression != nil {
		w.expression = *req.Expression
	}
	if w.ruleType == store.RuleTypeExpression {
		if strings.TrimSpace(w.expression) == "" {
			return ruleWrite{}, ruleErr("ruleType=expression 时 expression 不能为空")
		}
		if err := rules.ValidateExpression(w.expression); err != nil {
			return ruleWrite{}, ruleErr("expression 非法: %v", err)
		}
	}
	// ⑦⑧ 条件字段与运算符必须都在 v1 白名单内(白名单以 rules 包为准,httpapi 只引用)。
	// 条件树里的每个叶子都要校验,报错带 JSON 路径——不指明位置的话用户没法改。
	if req.Conditions != nil {
		if err := validateConditionTree(*req.Conditions); err != nil {
			return ruleWrite{}, err
		}
		w.conditions = *req.Conditions
	}
	// ⑨ linkIds:只接受本租户、未逻辑删除的短链。scope=global 时不校验也不保留——
	// 那些 id 马上会被丢弃,为一次注定不发生的写入报错只会给出误导性的失败原因。
	w.linkIDs = req.LinkIDs
	if w.scope == store.RuleScopeGlobal {
		empty := []int64{}
		w.linkIDs = &empty
	} else if req.LinkIDs != nil {
		ids, err := dedupeLinkIDs(*req.LinkIDs)
		if err != nil {
			return ruleWrite{}, err
		}
		// ⑨ 关联只能指向本租户、未逻辑删除的短链(已删除的也算非法:
		// 关联一条看不见的短链只会在界面上造成误解,且不会有任何提示)
		n, err := a.store.CountOwnedLiveLinks(ctx, tenantID, ids)
		if err != nil {
			return ruleWrite{}, err
		}
		if n != len(ids) {
			return ruleWrite{}, ruleErr("linkIds 中存在不存在、已逻辑删除或不属于当前租户的短链")
		}
		w.linkIDs = &ids
	}
	return w, nil
}

// validateConditionTree 校验一组条件(扁平或树形)。每个叶子都跑一遍
// 字段/运算符白名单,每个组都校验 logic——库内 CHECK 只拦得住脏值,拦不住
// 写错的意图,而一条被静默丢弃的条件意味着规则行为和用户以为的不一样。
func validateConditionTree(conds store.RuleConditions) error {
	if !conds.IsTree() {
		for i, cond := range conds.Leaves {
			if err := validateConditionLeaf(cond, fmt.Sprintf("conditions[%d]", i)); err != nil {
				return err
			}
		}
		return nil
	}
	return validateConditionNode(*conds.Root, "conditions")
}

func validateConditionNode(n store.ConditionNode, path string) error {
	if n.Leaf != nil {
		return validateConditionLeaf(*n.Leaf, path)
	}
	if n.Group == nil {
		return ruleErr("%s 不是合法条件节点(既没有 leaf 也没有 group)", path)
	}
	if n.Group.Logic != store.RuleLogicAll && n.Group.Logic != store.RuleLogicAny {
		return ruleErr("%s.logic 只能是 all 或 any:%s", path, n.Group.Logic)
	}
	if len(n.Group.Children) == 0 {
		return ruleErr("%s 是空条件组", path)
	}
	for i, child := range n.Group.Children {
		if err := validateConditionNode(child, fmt.Sprintf("%s.children[%d]", path, i)); err != nil {
			return err
		}
	}
	return nil
}

func validateConditionLeaf(cond store.RuleCondition, path string) error {
	if !rules.ValidField(cond.Field) {
		return ruleErr("%s.field 不在可求值字段集内:%s", path, cond.Field)
	}
	if !rules.ValidOperator(cond.Operator) {
		return ruleErr("%s.operator 不合法:%s", path, cond.Operator)
	}
	return nil
}

// firstNonEmpty 空串回退到 DDL 默认值(库内 CHECK 保证不为空,这里双保险)。
func firstNonEmpty(v, def string) string {
	if v == "" {
		return def
	}
	return v
}

// curStr 取规则现值(新建时 cur 为 nil,返回空串,随后由请求体或默认值填充)。
func curStr(cur *store.Rule, get func(*store.Rule) string) string {
	if cur == nil {
		return ""
	}
	return get(cur)
}

// dedupeLinkIDs 校验并去重短链 id 列表(保持请求顺序)。
// 非正整数同样拒绝:它是"不存在的短链 id"的一种,静默丢弃会让租户以为规则挂上了。
func dedupeLinkIDs(ids []int64) ([]int64, error) {
	seen := make(map[int64]struct{}, len(ids))
	out := make([]int64, 0, len(ids))
	for _, id := range ids {
		if id <= 0 {
			return nil, ruleErr("linkIds 只能包含正整数")
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out, nil
}

// ---------- 规则 CRUD ----------

// fillRuleView 补齐规则的展示字段:24h 命中 + 关联短链可读标识截断。
//
// hits24h 读时从 visits 按 rule_id 聚合(spec D9):rules 表上不存计数器,
// 命中发生在跳转热路径上,每命中一次写一次库就是给最高 QPS 的链路加写放大。
// 截断只作用于 linkNames——linkIds 与 conditions 一律保持完整(见文件头不变式 ①)。
func (a *API) fillRuleView(ctx context.Context, rules []*store.Rule) error {
	ids := make([]int64, 0, len(rules))
	for _, r := range rules {
		ids = append(ids, r.ID)
	}
	hits, err := a.store.CountRuleHits24h(ctx, ids)
	if err != nil {
		return err
	}
	for _, r := range rules {
		r.Hits24h = hits[r.ID]
		if len(r.LinkNames) > maxRuleLinkNames {
			r.LinkNames = r.LinkNames[:maxRuleLinkNames]
		}
	}
	return nil
}

// invalidateRules 丢弃该租户规则快照:任何规则写入或关联变更后都要调用,
// 否则新配置在快照重建前不生效(spec D7)。
func (a *API) invalidateRules(tenantID int64) {
	a.ruleCache.Invalidate(tenantID)
}

// handleListRules GET /api/rules — 规则列表(分页,含 scope/linkCount/linkNames/hits24h)。
func (a *API) handleListRules(c *gin.Context) {
	t, _, ok := a.requireSession(c)
	if !ok {
		return
	}
	page, pageSize := pageParams(c)
	items, total, err := a.store.ListRules(c.Request.Context(), t.ID, page, pageSize)
	if err != nil {
		writeErr(c, http.StatusInternalServerError, errInternal, "internal error")
		return
	}
	out := make([]*store.Rule, 0, len(items))
	for i := range items {
		out = append(out, &items[i])
	}
	if err := a.fillRuleView(c.Request.Context(), out); err != nil {
		writeErr(c, http.StatusInternalServerError, errInternal, "internal error")
		return
	}
	writeJSON(c, http.StatusOK, map[string]any{"items": out, "total": total})
}

// handleGetRule GET /api/rules/{id} — 规则详情(含完整 conditions 与 linkIds,不截断)。
func (a *API) handleGetRule(c *gin.Context) {
	t, _, ok := a.requireSession(c)
	if !ok {
		return
	}
	id, ok := pathID(c)
	if !ok {
		return
	}
	r, err := a.store.GetRule(c.Request.Context(), t.ID, id)
	if err != nil {
		writeRuleErr(c, err)
		return
	}
	if err := a.fillRuleView(c.Request.Context(), []*store.Rule{r}); err != nil {
		writeErr(c, http.StatusInternalServerError, errInternal, "internal error")
		return
	}
	writeJSON(c, http.StatusOK, r)
}

// handleCreateRule POST /api/rules — 新建规则;scope=links 时用 linkIds 建立关联
// (允许为空:零关联是合法状态,告警由界面负责暴露)。
func (a *API) handleCreateRule(c *gin.Context) {
	t, sess, ok := a.requireSession(c)
	if !ok {
		return
	}
	if !a.requireCSRF(c, sess) {
		return
	}
	var req ruleReq
	if err := json.NewDecoder(c.Request.Body).Decode(&req); err != nil {
		writeErr(c, http.StatusBadRequest, errValidation, "invalid JSON body")
		return
	}
	w, err := a.resolveRule(c.Request.Context(), t.ID, nil, req)
	if err != nil {
		writeAPIError(c, err)
		return
	}
	// 规则数上限(spec D7):规则集合整租户常驻内存供热路径求值,没有上限会让快照无限膨胀
	n, err := a.store.CountTenantRules(c.Request.Context(), t.ID)
	if err != nil {
		writeErr(c, http.StatusInternalServerError, errInternal, "internal error")
		return
	}
	if n >= store.MaxRulesPerTenant {
		writeErrDetails(c, http.StatusForbidden, errQuota,
			"规则数量已达上限", map[string]any{"usage": n, "limit": store.MaxRulesPerTenant})
		return
	}
	created, err := a.store.CreateRule(c.Request.Context(), t.ID, store.Rule{
		Name: w.name, Description: w.description, Priority: w.priority,
		Scope: w.scope, Enabled: w.enabled, Logic: w.logic, Action: w.action,
		Destination: w.destination, Conditions: w.conditions, LinkIDs: derefIDs(w.linkIDs),
		PageMode: w.pageMode, CustomHTML: w.customHTML,
		RuleType: w.ruleType, Expression: w.expression,
	})
	if err != nil {
		a.writeRuleWriteErr(c, err)
		return
	}
	a.invalidateRules(t.ID)
	if err := a.fillRuleView(c.Request.Context(), []*store.Rule{created}); err != nil {
		writeErr(c, http.StatusInternalServerError, errInternal, "internal error")
		return
	}
	writeJSON(c, http.StatusCreated, created)
}

// handlePatchRule PATCH /api/rules/{id} — 改规则。
// 传 linkIds 时整体替换关联(省略则不动关联);scope 从 links 改回 global 时关联被清空。
func (a *API) handlePatchRule(c *gin.Context) {
	t, sess, ok := a.requireSession(c)
	if !ok {
		return
	}
	if !a.requireCSRF(c, sess) {
		return
	}
	id, ok := pathID(c)
	if !ok {
		return
	}
	var req ruleReq
	if err := json.NewDecoder(c.Request.Body).Decode(&req); err != nil {
		writeErr(c, http.StatusBadRequest, errValidation, "invalid JSON body")
		return
	}
	// 先取现值:跨租户/不存在 → 404,同时用于"合成后再校验"(例如只改 action 也要看旧 destination)
	cur, err := a.store.GetRule(c.Request.Context(), t.ID, id)
	if err != nil {
		writeRuleErr(c, err)
		return
	}
	w, err := a.resolveRule(c.Request.Context(), t.ID, cur, req)
	if err != nil {
		writeAPIError(c, err)
		return
	}
	upd := store.RuleUpdate{
		Name: &w.name, Description: &w.description, Priority: &w.priority,
		Scope: &w.scope, Enabled: &w.enabled, Logic: &w.logic, Action: &w.action,
		Destination: &w.destination, Conditions: &w.conditions, LinkIDs: w.linkIDs,
		PageMode: &w.pageMode, CustomHTML: &w.customHTML,
		RuleType: &w.ruleType, Expression: &w.expression,
	}
	updated, err := a.store.UpdateRule(c.Request.Context(), t.ID, id, upd)
	if err != nil {
		a.writeRuleWriteErr(c, err)
		return
	}
	a.invalidateRules(t.ID)
	if err := a.fillRuleView(c.Request.Context(), []*store.Rule{updated}); err != nil {
		writeErr(c, http.StatusInternalServerError, errInternal, "internal error")
		return
	}
	writeJSON(c, http.StatusOK, updated)
}

// handleDeleteRule DELETE /api/rules/{id} — 删规则(rule_links 由库内 CASCADE 一并消失)。
func (a *API) handleDeleteRule(c *gin.Context) {
	t, sess, ok := a.requireSession(c)
	if !ok {
		return
	}
	if !a.requireCSRF(c, sess) {
		return
	}
	id, ok := pathID(c)
	if !ok {
		return
	}
	if err := a.store.DeleteRule(c.Request.Context(), t.ID, id); err != nil {
		writeRuleErr(c, err)
		return
	}
	a.invalidateRules(t.ID)
	writeNoContent(c)
}

// ruleOption 规则下拉项(契约 ruleOption)。
// 契约只要求 {id,name,scope,hits24h};这里多回 action/priority/enabled ——
// 短链表单的「适用规则」勾选区要按处置语义与优先级展示,让前端再拉一次详情不值得。
type ruleOption struct {
	ID       int64  `json:"id"`
	Name     string `json:"name"`
	Scope    string `json:"scope"`
	Action   string `json:"action"`
	Priority int    `json:"priority"`
	Enabled  bool   `json:"enabled"`
	Hits24h  int64  `json:"hits24h"`
}

// handleRuleOptions GET /api/rules/options — 租户全部规则的精简列表(下拉用,不分页)。
func (a *API) handleRuleOptions(c *gin.Context) {
	t, _, ok := a.requireSession(c)
	if !ok {
		return
	}
	items, err := a.store.ListAllRules(c.Request.Context(), t.ID)
	if err != nil {
		writeErr(c, http.StatusInternalServerError, errInternal, "internal error")
		return
	}
	ptrs := make([]*store.Rule, 0, len(items))
	for i := range items {
		ptrs = append(ptrs, &items[i])
	}
	if err := a.fillRuleView(c.Request.Context(), ptrs); err != nil {
		writeErr(c, http.StatusInternalServerError, errInternal, "internal error")
		return
	}
	out := make([]ruleOption, 0, len(ptrs))
	for _, r := range ptrs {
		out = append(out, ruleOption{
			ID: r.ID, Name: r.Name, Scope: r.Scope, Action: r.Action,
			Priority: r.Priority, Enabled: r.Enabled, Hits24h: r.Hits24h,
		})
	}
	writeJSON(c, http.StatusOK, out)
}

// ---------- 短链 ↔ 规则关联 ----------

// linkRule 某条短链适用的规则(契约 linkRule)。
// Source 是权威字段,不可由前端推断:inherited = 来自 scope=global(在短链侧恒为勾选且不可取消),
// scoped = 来自 rule_links。判错就直接变成"全局规则被取消得掉"的假象。
type linkRule struct {
	ID       int64  `json:"id"`
	Name     string `json:"name"`
	Scope    string `json:"scope"`
	Action   string `json:"action"`
	Priority int    `json:"priority"`
	Enabled  bool   `json:"enabled"`
	Source   string `json:"source"`
	// LinkCount:scope=links 时该规则还关联着多少条短链(全局规则恒为 0)。
	// 短链列表页要在这里就地启停规则,没有这个数字就没法告诉用户「关掉它会连带影响别的短链」。
	LinkCount int `json:"linkCount"`
}

// 关联来源标记。
const (
	linkRuleSourceInherited = "inherited" // 来自 scope=global
	linkRuleSourceScoped    = "scoped"    // 来自 rule_links
)

// linkRulesFor 返回某条短链适用的规则(全局继承 + 显式关联),按求值顺序。
func (a *API) linkRulesFor(c *gin.Context, tenantID, linkID int64) ([]linkRule, error) {
	rs, err := a.store.RulesForLink(c.Request.Context(), tenantID, linkID)
	if err != nil {
		return nil, err
	}
	out := make([]linkRule, 0, len(rs))
	for _, r := range rs {
		source := linkRuleSourceScoped
		if r.Scope == store.RuleScopeGlobal {
			source = linkRuleSourceInherited
		}
		out = append(out, linkRule{
			ID: r.ID, Name: r.Name, Scope: r.Scope, Action: r.Action,
			Priority: r.Priority, Enabled: r.Enabled, Source: source,
			// RulesForLink 已经 fillRuleLinkMeta 填好了 linkCount,直接用,不加查询
			LinkCount: r.LinkCount,
		})
	}
	return out, nil
}

// handleListLinkRules GET /api/links/{id}/rules — 该短链适用的规则。
func (a *API) handleListLinkRules(c *gin.Context) {
	t, _, ok := a.requireSession(c)
	if !ok {
		return
	}
	id, ok := pathID(c)
	if !ok {
		return
	}
	if _, err := a.store.GetLinkByID(c.Request.Context(), t.ID, id); err != nil {
		writeLinkErr(c, err)
		return
	}
	items, err := a.linkRulesFor(c, t.ID, id)
	if err != nil {
		writeErr(c, http.StatusInternalServerError, errInternal, "internal error")
		return
	}
	writeJSON(c, http.StatusOK, map[string]any{"items": items})
}

type putLinkRulesReq struct {
	RuleIDs []int64 `json:"ruleIds"`
}

// handlePutLinkRules PUT /api/links/{id}/rules — 整体替换该短链与指定短链作用域规则的关联。
// 空数组 = 解除全部。scope=global 的规则既不接受被取消,也不受本端点影响:
// 传它的 id → 400(它不该被"关掉",误传说明调用方理解错了)。
func (a *API) handlePutLinkRules(c *gin.Context) {
	t, sess, ok := a.requireSession(c)
	if !ok {
		return
	}
	if !a.requireCSRF(c, sess) {
		return
	}
	id, ok := pathID(c)
	if !ok {
		return
	}
	if _, err := a.store.GetLinkByID(c.Request.Context(), t.ID, id); err != nil {
		writeLinkErr(c, err)
		return
	}
	var req putLinkRulesReq
	if err := json.NewDecoder(c.Request.Body).Decode(&req); err != nil {
		writeErr(c, http.StatusBadRequest, errValidation, "invalid JSON body")
		return
	}
	ids, err := dedupeLinkIDs(req.RuleIDs)
	if err != nil {
		writeErr(c, http.StatusBadRequest, errValidation, err.Error())
		return
	}
	if !a.checkScopedRuleIDs(c, t.ID, ids) {
		return
	}
	if err := a.store.SetLinkRules(c.Request.Context(), t.ID, id, ids); err != nil {
		a.writeRuleWriteErr(c, err)
		return
	}
	a.invalidateRules(t.ID)
	items, err := a.linkRulesFor(c, t.ID, id)
	if err != nil {
		writeErr(c, http.StatusInternalServerError, errInternal, "internal error")
		return
	}
	writeJSON(c, http.StatusOK, map[string]any{"items": items})
}

// checkScopedRuleIDs 校验 ruleIds 全部是本租户 scope=links 的规则,否则写 400 并返回 false。
// 两种 400 分开给出可读原因,但都不静默跳过——配置写入丢一条,租户在界面上看到的是
// "已关联"却一条都没生效,比直接报错难查得多。
func (a *API) checkScopedRuleIDs(c *gin.Context, tenantID int64, ids []int64) bool {
	if len(ids) == 0 {
		return true
	}
	rs, err := a.store.RulesByIDs(c.Request.Context(), tenantID, ids)
	if err != nil {
		writeErr(c, http.StatusInternalServerError, errInternal, "internal error")
		return false
	}
	byID := make(map[int64]store.Rule, len(rs))
	for _, r := range rs {
		byID[r.ID] = r
	}
	for _, id := range ids {
		r, ok := byID[id]
		if !ok {
			writeErr(c, http.StatusBadRequest, errValidation, "ruleIds 中存在不存在或不属于当前租户的规则")
			return false
		}
		if r.Scope != store.RuleScopeLinks {
			writeErr(c, http.StatusBadRequest, errValidation,
				"ruleIds 中不能包含 scope=global 的规则(全局规则对本短链恒生效,不受关联开关影响)")
			return false
		}
	}
	return true
}

// ---------- 错误与共用小工具 ----------

// pathID 解析路径参数 :id,非法时写 400。
func pathID(c *gin.Context) (int64, bool) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		writeErr(c, http.StatusBadRequest, errValidation, "invalid id")
		return 0, false
	}
	return id, true
}

// writeRuleErr 把 store 的取数错误映射成响应(规则不存在/不属于本租户一律 404,
// 不泄露"别人家确实有这么一条规则")。
func writeRuleErr(c *gin.Context, err error) {
	writeNotFoundOrInternal(c, err, "rule not found")
}

// writeLinkErr 同上,但用于短链(短链侧的端点要给出"link not found")。
func writeLinkErr(c *gin.Context, err error) {
	writeNotFoundOrInternal(c, err, "link not found")
}

// writeNotFoundOrInternal ErrNotFound → 404,其余 → 500。
func writeNotFoundOrInternal(c *gin.Context, err error, msg string) {
	if errors.Is(err, store.ErrNotFound) {
		writeErr(c, http.StatusNotFound, errNotFound, msg)
		return
	}
	writeErr(c, http.StatusInternalServerError, errInternal, "internal error")
}

// writeRuleWriteErr 写路径的错误映射:同租户重名 409,关联的短链/规则不归属 400。
func (a *API) writeRuleWriteErr(c *gin.Context, err error) {
	if errors.Is(err, store.ErrForeignLink) {
		writeErr(c, http.StatusBadRequest, errValidation, "linkIds 中存在不存在、已逻辑删除或不属于当前租户的短链")
		return
	}
	if errors.Is(err, store.ErrForeignRule) {
		writeErr(c, http.StatusBadRequest, errValidation, "ruleIds 中存在不存在或不属于当前租户的规则")
		return
	}
	if store.IsUniqueViolation(err) {
		writeErr(c, http.StatusConflict, errConflict, "同租户已存在同名规则")
		return
	}
	writeRuleErr(c, err)
}

// derefIDs 取待写入的关联 id 列表(未指定关联时为 nil)。
func derefIDs(ids *[]int64) []int64 {
	if ids == nil {
		return nil
	}
	return *ids
}

// ---------- 规则仿真(诊断) ----------

// 规则仿真只回答一个问题:「这个访客会被哪条规则拦下,凭什么」。
// 它不是第二套判定:结论来自 rules.Snapshot.Simulate,而 Simulate 走的是
// Evaluate 同一套顺序与同一批 compiledCond.match。所以这里只做三件事:
// 把请求里的访客画像拼成一个"假想请求"、定位它命中的短链、把裁决翻成人话。
//
// 三条不能破的线:
//  1. 租户来自会话,不来自请求体:拿别人 tenant 的规则集试规则等于越权。
//  2. 访客画像全部由调用方给定,GeoIP 也只在建上下文那一刻注入一次,
//     求值期内零 IO(引擎是纯内存的,诊断不能比线上还慢)。
//  3. 只读快照:草稿与 onlyRuleId 只在这一条推演里生效,绝不写回热路径共享的快照。

// simTarget 目标 URL 拆出来的东西:域名 + 短码 + 路径/查询串。
type simTarget struct {
	host  string
	code  string
	path  string
	query string
}

// maxSimURLLen 目标 URL 长度上限(与 validTargetURL 的 4096 同一量级)。
const maxSimURLLen = 4096

// scopeNoteGlobal 没定位到短链时的统一说明。
const scopeNoteGlobal = "本次仅按 scope=global 的全局规则求值;「指定短链」的规则不在推演范围内。"

type simulateReq struct {
	// URL 必填:完整 URL(https://域名/短码?x=1)或单个短码(/短码、短码)。
	URL string `json:"url"`
	// IP 为空时取当前请求的来源 IP(再兜底 127.0.0.1)。
	IP string `json:"ip"`
	// UserAgent / AcceptLanguage / Referrer 为空即"这次访客没有这个头"。
	UserAgent      string `json:"userAgent"`
	AcceptLanguage string `json:"acceptLanguage"`
	Referrer       string `json:"referrer"`
	// ManualCountry 手工指定国家码:诊断环境常常拿不到真实 GeoIP。
	// 只作用于本次推演,不写入任何持久数据。
	ManualCountry string `json:"manualCountry"`
	// OnlyRuleID 只回放这一条存量规则(诊断"单条规则自己生效吗")。
	OnlyRuleID *int64 `json:"onlyRuleId"`
	// DraftRule 未保存的草稿规则:按 id 顶替同 id 的存量规则参与本次推演。
	DraftRule *simulateDraftRule `json:"draftRule"`
}

// simulateDraftRule 未落库的规则草稿。字段与创建/更新接口同构:
// 省略即沿用现值(PATCH 语义),所以"只改了一个字段"也能试。
type simulateDraftRule struct {
	ID *int64 `json:"id"`
	ruleReq
}

// simulateResp 诊断结果。facts 原样回显这次访客的 13 个可求值字段。
type simulateResp struct {
	Facts     map[string]string   `json:"facts"`
	ScopeNote string              `json:"scopeNote"`
	Steps     []rules.StepTrace   `json:"steps"`
	Verdict   simulateVerdictResp `json:"verdict"`
	Error     string              `json:"error,omitempty"`
}

// simulateVerdictResp 裁决结论。Blocked = 会直接掐断访问(notfound / throttle)。
type simulateVerdictResp struct {
	Matched     bool   `json:"matched"`
	Blocked     bool   `json:"blocked"`
	RuleID      int64  `json:"ruleId"`
	RuleName    string `json:"ruleName"`
	Action      string `json:"action"`
	Destination string `json:"destination"`
	Priority    int    `json:"priority"`
	Message     string `json:"message"`
}

func (a *API) handleSimulateRules(c *gin.Context) {
	t, sess, ok := a.requireSession(c)
	if !ok {
		return
	}
	if !a.requireCSRF(c, sess) {
		return
	}
	var req simulateReq
	if err := json.NewDecoder(c.Request.Body).Decode(&req); err != nil {
		writeErr(c, http.StatusBadRequest, errValidation, "invalid JSON body")
		return
	}
	target, err := parseSimTarget(req.URL)
	if err != nil {
		writeAPIError(c, err)
		return
	}
	ip, err := simClientIP(c, req.IP)
	if err != nil {
		writeAPIError(c, err)
		return
	}
	country, err := simManualCountry(req.ManualCountry)
	if err != nil {
		writeAPIError(c, err)
		return
	}
	var asn string
	if country == "" && a.geo != nil {
		// GeoIP 只在这里查一次:求值器本身不碰任何 IO。
		info := a.geo.Lookup(ip)
		country, asn = info.Country, info.ASN
	}
	onlyID, err := a.simOnlyRuleID(c, t.ID, req.OnlyRuleID)
	if err != nil {
		writeAPIError(c, err)
		return
	}
	draft, err := a.simDraft(c, t.ID, req.DraftRule)
	if err != nil {
		writeAPIError(c, err)
		return
	}
	link, note := a.simResolveLink(c.Request.Context(), t.ID, target)
	var linkID int64
	if link != nil {
		linkID = link.ID
	}

	// 假想请求走的是 LazyVisitorContext:UA/语言/来源站点的解析路径与线上跳转完全一致,
	// 仿真不会另造一套解析(另造一套迟早会和线上分叉)。
	vctx := rules.AcquireVisitorContext(
		target.request(req.UserAgent, req.AcceptLanguage, req.Referrer), country, asn).WithIP(ip)
	defer rules.ReleaseVisitorContext(vctx)

	res := a.ruleCache.Get(c.Request.Context(), t.ID).Simulate(vctx, linkID, onlyID, draft)
	steps := res.Steps
	if steps == nil {
		steps = []rules.StepTrace{}
	}
	writeJSON(c, http.StatusOK, simulateResp{
		Facts:     vctx.ToFact().Fields(),
		ScopeNote: note,
		Steps:     steps,
		Verdict:   simVerdict(res, link),
		Error:     res.Error,
	})
}

// parseSimTarget 解析目标 URL。两种写法都接受:完整 URL,或光秃秃的短码。
// 后者没有域名——path 仍参与 path 条件求值,但解析不出短链归属,
// 也就无法把 domain 条件与短链作用域对上(scopeNote 会讲清楚)。
func parseSimTarget(raw string) (simTarget, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return simTarget{}, ruleErr("url 不能为空")
	}
	if len(raw) > maxSimURLLen {
		return simTarget{}, ruleErr("url 过长(上限 4096 字符)")
	}
	if !strings.Contains(raw, "://") {
		code := strings.Trim(raw, "/")
		if code == "" || strings.Contains(code, "/") {
			return simTarget{}, ruleErr("url 需要是完整 URL(如 https://域名/短码)或单个短码")
		}
		return simTarget{code: code, path: "/" + code}, nil
	}
	u, err := url.Parse(raw)
	if err != nil {
		return simTarget{}, ruleErr("url 不合法")
	}
	scheme := strings.ToLower(u.Scheme)
	if scheme != "http" && scheme != "https" {
		return simTarget{}, ruleErr("url 只支持 http/https")
	}
	host := hostOnly(u.Host)
	if host == "" {
		return simTarget{}, ruleErr("url 缺少域名")
	}
	path := u.Path
	if path == "" {
		path = "/"
	}
	code := strings.Trim(path, "/")
	if i := strings.Index(code, "/"); i >= 0 {
		code = code[:i]
	}
	return simTarget{host: host, code: code, path: path, query: u.RawQuery}, nil
}

// request 把目标 URL 拼成一个"假想访问",交给访客上下文求值。
func (t simTarget) request(ua, lang, referrer string) *http.Request {
	r := &http.Request{
		Method: http.MethodGet,
		Host:   t.host,
		URL:    &url.URL{Path: t.path, RawQuery: t.query},
		Header: make(http.Header, 3),
	}
	if ua != "" {
		r.Header.Set("User-Agent", ua)
	}
	if lang != "" {
		r.Header.Set("Accept-Language", lang)
	}
	if referrer != "" {
		r.Header.Set("Referer", referrer)
	}
	return r
}

// simClientIP 校验访客 IP;为空时取当前请求来源 IP(诊断页多半在本机打开)。
func simClientIP(c *gin.Context, raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		if ip, err := netip.ParseAddr(clientIP(c.Request)); err == nil {
			return ip.String(), nil
		}
		return "127.0.0.1", nil
	}
	if len(raw) > 64 {
		return "", ruleErr("ip 过长")
	}
	ip, err := netip.ParseAddr(raw)
	if err != nil {
		return "", ruleErr("ip 不合法")
	}
	return ip.String(), nil
}

// simManualCountry 校验手工指定的国家码(两位字母,大小写不敏感)。
func simManualCountry(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", nil
	}
	if len(raw) != 2 || !isAlpha(raw) {
		return "", ruleErr("manualCountry 必须是两位国家码(如 CN)")
	}
	return strings.ToUpper(raw), nil
}

func isAlpha(s string) bool {
	for i := 0; i < len(s); i++ {
		c := s[i]
		if (c < 'a' || c > 'z') && (c < 'A' || c > 'Z') {
			return false
		}
	}
	return true
}

// simOnlyRuleID 校验 onlyRuleId:只允许回放本租户自己的规则。
func (a *API) simOnlyRuleID(c *gin.Context, tenantID int64, id *int64) (*int64, error) {
	if id == nil {
		return nil, nil
	}
	if *id <= 0 {
		return nil, ruleErr("onlyRuleId 必须是正整数")
	}
	if _, err := a.store.GetRule(c.Request.Context(), tenantID, *id); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil, apiErr{status: http.StatusNotFound, code: errNotFound, message: "rule not found"}
		}
		return nil, err
	}
	return id, nil
}

// simDraft 把草稿合成为完整规则并预编译。校验与落库走同一个 resolveRule:
// "试过了"必须等价于"存得下",否则诊断接口就成了比线上宽松的另一套校验。
// 草稿带 id 时顶替同 id 的存量规则;停用的草稿会占一步并标成 disabled。
func (a *API) simDraft(c *gin.Context, tenantID int64, d *simulateDraftRule) (*rules.Compiled, error) {
	if d == nil {
		return nil, nil
	}
	ctx := c.Request.Context()
	var cur *store.Rule
	var id int64
	if d.ID != nil && *d.ID > 0 {
		got, err := a.store.GetRule(ctx, tenantID, *d.ID)
		if err != nil {
			if errors.Is(err, store.ErrNotFound) {
				return nil, apiErr{status: http.StatusNotFound, code: errNotFound, message: "rule not found"}
			}
			return nil, err
		}
		cur, id = got, got.ID
	}
	w, err := a.resolveRule(ctx, tenantID, cur, d.ruleReq)
	if err != nil {
		return nil, err
	}
	draft, ok := rules.CompileDraft(store.Rule{
		ID: id, Name: w.name, Description: w.description, Priority: w.priority,
		Scope: w.scope, Enabled: w.enabled, Logic: w.logic, Action: w.action,
		Destination: w.destination, Conditions: w.conditions, LinkIDs: derefIDs(w.linkIDs),
		PageMode: w.pageMode, CustomHTML: w.customHTML,
		RuleType: w.ruleType, Expression: w.expression,
	}, nil)
	if !ok {
		return nil, ruleErr("草稿规则的条件全部不可求值(检查字段与运算符),无法推演")
	}
	return draft, nil
}

// simResolveLink 定位目标 URL 命中的短链。只在本租户的 active 域名下找:
// 别的租户的短链 id 不该出现在本租户的诊断结果里。
// 取不到不是错误——诊断一个还没建出来的短链是正常用法,scopeNote 会说明推演范围。
func (a *API) simResolveLink(ctx context.Context, tenantID int64, target simTarget) (*store.Link, string) {
	if target.host == "" || target.code == "" {
		return nil, "URL 里没有短码," + scopeNoteGlobal
	}
	d, err := a.store.GetDomainByFQDN(ctx, target.host)
	if err != nil || d.Status != "active" || d.TenantID != tenantID {
		return nil, fmt.Sprintf("域名 %s 不在本租户(或未激活),%s", target.host, scopeNoteGlobal)
	}
	link, _, reason, err := a.store.LookupLinkForVisit(ctx, d.ID, target.code)
	if err != nil || link == nil || link.TenantID != tenantID {
		return nil, fmt.Sprintf("未在本租户找到短链 /%s,%s", target.code, scopeNoteGlobal)
	}
	if reason != "" {
		return link, fmt.Sprintf("短链 /%s 当前不可用(%s),线上不会走到这一步;以下按它的关联规则推演。",
			target.code, reason)
	}
	note := fmt.Sprintf("URL 命中短链 /%s(ID %d),已按该短链实际适用的规则(含全局继承)求值。", target.code, link.ID)
	if !link.RulesEnabled {
		note += "该短链已关闭规则求值(rulesEnabled=false),线上不会应用任何规则。"
	}
	return link, note
}

// simVerdict 把裁决翻成人话。短链关闭了规则求值时结论按"无线上规则"给:
// 跳转热路径会先看 link.RulesEnabled,诊断不能报一个线上不会发生的拦截。
func simVerdict(res rules.SimulationResult, link *store.Link) simulateVerdictResp {
	var v simulateVerdictResp
	if link != nil && !link.RulesEnabled {
		v.Message = "该短链已关闭规则求值(rulesEnabled=false),线上访客按原目标正常跳转,不会应用任何规则。"
		return v
	}
	if !res.Matched {
		v.Message = "该访客不命中任何规则,按原目标正常跳转。"
		return v
	}
	v.Matched = true
	v.Blocked = res.Verdict.Action == store.RuleActionNotfound || res.Verdict.Action == store.RuleActionThrottle
	v.RuleID = res.Verdict.RuleID
	v.RuleName = res.Verdict.Name
	v.Action = res.Verdict.Action
	v.Destination = res.Verdict.Destination
	v.Priority = res.Verdict.Priority
	v.Message = fmt.Sprintf("命中「%s」(优先级 %d):%s", res.Verdict.Name, res.Verdict.Priority, actionVerdict(res.Verdict))
	return v
}

func actionVerdict(d rules.Decision) string {
	switch d.Action {
	case store.RuleActionRedirect:
		return fmt.Sprintf("跳转到 %s。", d.Destination)
	case store.RuleActionNotfound:
		return "直接返回 404。"
	case store.RuleActionThrottle:
		return "限流,返回 429。"
	default:
		return "放行,按原目标正常跳转。"
	}
}

// validateExprReq 表达式校验请求体。
type validateExprReq struct {
	Expression string `json:"expression"`
}

// validateExprResp 表达式校验响应体。
type validateExprResp struct {
	Valid   bool   `json:"valid"`
	Message string `json:"message,omitempty"`
}

// handleValidateExpr POST /api/rules/validate-expr — 校验 Expr 表达式语法与返回值类型。
func (a *API) handleValidateExpr(c *gin.Context) {
	_, _, ok := a.requireSession(c)
	if !ok {
		return
	}
	var req validateExprReq
	if err := json.NewDecoder(c.Request.Body).Decode(&req); err != nil {
		writeErr(c, http.StatusBadRequest, errValidation, "invalid JSON body")
		return
	}
	if strings.TrimSpace(req.Expression) == "" {
		writeJSON(c, http.StatusOK, validateExprResp{
			Valid:   false,
			Message: "expression 不能为空",
		})
		return
	}
	if err := rules.ValidateExpression(req.Expression); err != nil {
		writeJSON(c, http.StatusOK, validateExprResp{
			Valid:   false,
			Message: err.Error(),
		})
		return
	}
	writeJSON(c, http.StatusOK, validateExprResp{Valid: true})
}

