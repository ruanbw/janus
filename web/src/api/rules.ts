// 规则 API(后台,会话鉴权)
// 关联模型(spec D1):规则侧声明作用域,rule_links 是唯一关联数据;
// 短链表单里的勾选操作本质是改写同一条 rule_links 记录,两边看到的是同一份数据。
import { del, get, patch, post, put } from '@/utils/request';

import type {
  LinkRule,
  PageResult,
  Rule,
  RuleAction,
  RuleCondition,
  RuleLogic,
  RuleOption,
  RulePageMode,
  RuleScope,
} from '@/types/api';

export interface RuleListQuery {
  page?: number;
  pageSize?: number;
}

/** 新建规则的请求体。scope='links' 时 linkIds 建立 rule_links 关联(允许为空,零关联是合法状态) */
export interface RuleCreatePayload {
  name: string;
  description?: string;
  priority?: number;
  scope: RuleScope;
  enabled?: boolean;
  logic: RuleLogic;
  action: RuleAction;
  /** action='redirect' 时的改写目标 */
  destination?: string;
  /** 错误响应页面模式: default 继承全局 / custom 专属页面 */
  pageMode?: RulePageMode;
  /** 专属自定义 HTML(限 512KB) */
  customHtml?: string;
  conditions: RuleCondition[];
  linkIds?: number[];
}

/** 编辑规则的请求体。传 linkIds 即整体替换关联(不传则不动关联) */
export type RuleUpdatePayload = Partial<RuleCreatePayload>;

/** 下拉/列表接口统一回 { items } 的形状 */
interface ItemsResult<T> {
  items: T[];
}

/** 后端极端情况下可能漏回数组字段,统一给出契约默认值,页面无需再兜底 */
function normalizeRule(rule: Rule): Rule {
  return {
    ...rule,
    description: rule.description || '',
    priority: typeof rule.priority === 'number' ? rule.priority : 100,
    scope: rule.scope === 'links' ? 'links' : 'global',
    enabled: rule.enabled !== false,
    logic: rule.logic === 'any' ? 'any' : 'all',
    action: rule.action || 'pass',
    destination: rule.destination || '',
    pageMode: rule.pageMode === 'custom' ? 'custom' : 'default',
    customHtml: rule.customHtml || '',
    conditions: Array.isArray(rule.conditions) ? rule.conditions : [],
    linkCount: typeof rule.linkCount === 'number' ? rule.linkCount : 0,
    linkNames: Array.isArray(rule.linkNames) ? rule.linkNames : [],
    linkIds: Array.isArray(rule.linkIds) ? rule.linkIds : [],
    hits24h: typeof rule.hits24h === 'number' ? rule.hits24h : 0,
  };
}

function normalizeLinkRule(item: LinkRule): LinkRule {
  return {
    ...item,
    scope: item.scope === 'links' ? 'links' : 'global',
    action: item.action || 'pass',
    enabled: item.enabled !== false,
    source: item.source === 'inherited' ? 'inherited' : 'scoped',
    linkCount: typeof item.linkCount === 'number' ? item.linkCount : 0,
  };
}

/** 规则列表(分页,含 scope / linkCount / linkNames / hits24h) */
export function listRules(query: RuleListQuery = {}): Promise<PageResult<Rule>> {
  return get<PageResult<Rule>>('/rules', { ...query }).then((res) => ({
    total: typeof res.total === 'number' ? res.total : res.items?.length ?? 0,
    items: (res.items ?? []).map(normalizeRule),
  }));
}

/** 新建规则(403 单租户规则数超上限;409 同名规则) */
export function createRule(data: RuleCreatePayload): Promise<Rule> {
  return post<Rule>('/rules', data).then(normalizeRule);
}

/** 规则详情(含完整 conditions) */
export function getRule(id: number): Promise<Rule> {
  return get<Rule>('/rules/' + id).then(normalizeRule);
}

/** 编辑规则:传 linkIds 时整体替换 rule_links 关联 */
export function updateRule(id: number, data: RuleUpdatePayload): Promise<Rule> {
  return patch<Rule>('/rules/' + id, data).then(normalizeRule);
}

/** 删除规则(级联删除 rule_links;历史访问明细的 rule_id 置空保留) */
export function deleteRule(id: number): Promise<void> {
  return del<void>('/rules/' + id);
}

/** 规则下拉选项(精简字段,供表单勾选区使用) */
export function ruleOptions(): Promise<RuleOption[]> {
  return get<ItemsResult<RuleOption>>('/rules/options').then((res) =>
    (Array.isArray(res) ? res : (res?.items ?? [])).map((o) => ({
      ...o,
      scope: o.scope === 'links' ? 'links' : 'global',
      action: o.action || 'pass',
      enabled: o.enabled !== false,
      priority: typeof o.priority === 'number' ? o.priority : 100,
      hits24h: typeof o.hits24h === 'number' ? o.hits24h : 0,
    })),
  );
}

/** 某条短链适用的规则:含 scope=global 的继承项(source=inherited)与显式关联项(source=scoped) */
export function listLinkRules(linkId: number): Promise<LinkRule[]> {
  return get<ItemsResult<LinkRule>>(`/links/${linkId}/rules`).then((res) =>
    (Array.isArray(res) ? res : (res?.items ?? [])).map(normalizeLinkRule),
  );
}

/** 设置某条短链的 scoped 规则关联(整体替换)。
 *  ruleIds 只接受 scope='links' 的规则 id:传全局规则 id 或跨租户/不存在的 id 都会 400
 *  (不做静默跳过,静默丢弃会让租户误以为规则已挂上);空数组 = 解除全部 scoped 关联。 */
export function setLinkRules(linkId: number, ruleIds: number[]): Promise<LinkRule[]> {
  return put<ItemsResult<LinkRule>>(`/links/${linkId}/rules`, { ruleIds }).then((res) =>
    (Array.isArray(res) ? res : (res?.items ?? [])).map(normalizeLinkRule),
  );
}

/* ---------- 规则仿真(诊断) ---------- */

// 仿真的判定完全在后端跑:Simulate 与线上 Evaluate 共用同一套求值顺序与
// 同一批条件求值函数。前端曾经自己实现过一份等价求值,两处各算一次必然漂移,
// 而漂移一次的决策链比没有决策链更糟——它会让人照着一个假的结论去改规则。

/** 单条条件的求值痕迹:这次比的是「实际值」对「期望值」 */
export interface SimulateCondition {
  field: string;
  operator: string;
  /** 租户写下的原始字面量(未归一),用于回显「你配的是什么」 */
  expected?: string[];
  actual?: string;
  /** 该字段本次是否取到了数据。false = 条件恒不成立(空值不参与匹配) */
  available?: boolean;
  matched?: boolean;
  seen?: number;
  /** 后端给出的人话解释——判定在前端复刻一遍就会漂,文案由后端出 */
  description?: string;
}

/** 决策链上的一条规则 */
export interface SimulateStep {
  ruleId: number;
  ruleName: string;
  priority?: number;
  scope?: string;
  action?: string;
  status: 'hit' | 'skip' | 'disabled';
  reason?: string;
  logic?: string;
  /** true = 这是未落库的草稿规则 */
  draft?: boolean;
  conditions?: SimulateCondition[];
}

export interface SimulateVerdict {
  matched: boolean;
  /** 会直接掐断访问(notfound / throttle) */
  blocked: boolean;
  ruleId: number;
  ruleName: string;
  action: string;
  destination: string;
  priority: number;
  message: string;
}

export interface SimulateRulesResp {
  /** 本次访客的 13 个可判定字段;空串 = 该字段没有数据源 */
  facts: Record<string, string>;
  /** 推演范围说明(定位到短链还是只按全局规则) */
  scopeNote: string;
  steps: SimulateStep[];
  verdict: SimulateVerdict;
  /** 求值期异常被兜住时的说明(如实上报,线上仍然 fail-open) */
  error?: string;
}

/** 仿真请求。url 必填:完整 URL 或单个短码;其余留空 = 「这个访客没有这个头」 */
export interface SimulateRulesReq {
  url: string;
  ip?: string;
  userAgent?: string;
  acceptLanguage?: string;
  referrer?: string;
  /** 手工指定国家码:留空则由后端用离线 GeoIP 库按 ip 解析 */
  manualCountry?: string;
  /** 只回放这一条存量规则 */
  onlyRuleId?: number | null;
}

/** 后端极端情况下可能漏字段,统一给出契约默认值,页面无需再兜底 */
function normalizeSimulateStep(step: SimulateStep): SimulateStep {
  return {
    ...step,
    ruleName: step.ruleName || '',
    status: step.status === 'hit' || step.status === 'disabled' ? step.status : 'skip',
    reason: step.reason || '',
    conditions: Array.isArray(step.conditions) ? step.conditions : [],
  };
}

/** 规则仿真:后端按与线上完全相同的语义回放一遍,返回访客画像、决策链与裁决 */
export function simulateRules(data: SimulateRulesReq): Promise<SimulateRulesResp> {
  return post<SimulateRulesResp>('/rules/simulate', data).then((res) => ({
    facts: res.facts && typeof res.facts === 'object' ? res.facts : {},
    scopeNote: res.scopeNote || '',
    steps: (Array.isArray(res.steps) ? res.steps : []).map(normalizeSimulateStep),
    verdict: {
      matched: res.verdict?.matched === true,
      blocked: res.verdict?.blocked === true,
      ruleId: res.verdict?.ruleId ?? 0,
      ruleName: res.verdict?.ruleName || '',
      action: res.verdict?.action || 'pass',
      destination: res.verdict?.destination || '',
      priority: res.verdict?.priority ?? 0,
      message: res.verdict?.message || '',
    },
    error: res.error || '',
  }));
}
