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
