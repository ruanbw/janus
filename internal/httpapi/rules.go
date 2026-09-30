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
	LinkIDs *[]int64 `json:"linkIds"`
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
	linkIDs *[]int64
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
		if cur.Conditions != nil {
			w.conditions = cur.Conditions
		}
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
	// ⑦⑧ 条件字段与运算符必须都在 v1 白名单内(白名单以 rules 包为准,httpapi 只引用)
	if req.Conditions != nil {
		conds := store.RuleConditions(*req.Conditions)
		for i, cond := range conds {
			if !rules.ValidField(cond.Field) {
				return ruleWrite{}, ruleErr("conditions[%d].field 不在可求值字段集内:%s", i, cond.Field)
			}
			if !rules.ValidOperator(cond.Operator) {
				return ruleWrite{}, ruleErr("conditions[%d].operator 不合法:%s", i, cond.Operator)
			}
		}
		if conds == nil {
			conds = store.RuleConditions{}
		}
		w.conditions = conds
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
