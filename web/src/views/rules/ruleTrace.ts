/**
 * 决策链求值：把「一次访问 + 当前规则集」变成逐条规则的判定结果。
 *
 * 规则模拟器与短链访问明细页共用这一份实现。两处各算一次必然漂移，而漂移一次的
 * 决策链比没有决策链更糟——它会让人照着一个假的结论去改规则。
 *
 * 前提：这是**前端等价求值**，不是服务端执行结果。正则（RE2 vs JS）、地理字段
 * （无数据源）两处必然存在差异，页面上必须明说而不是让用户自己发现。
 */
import { listLinks } from '@/api/links';
import { getRule, listLinkRules, listRules } from '@/api/rules';
import type { Link, LinkRule, Rule } from '@/types/api';
import { actionLabel, isBlockingAction } from './ruleMeta';
import {
  buildVisitorFacts,
  evalCondition,
  mapWithConcurrency,
  parseSimTarget,
  type SimInput,
  type TraceStep,
  type VisitorFacts,
} from './ruleSim';

/** 后端 pageSize 上限 100；租户规则上限 200、短链配额上限 500，故最多翻 3 / 5 页 */
const PAGE_SIZE = 100;
const MAX_RULE_PAGES = 3;
const MAX_LINK_PAGES = 5;

export interface DecisionTrace {
  /** 从这次访问解析出的 13 个可判定字段 */
  facts: VisitorFacts;
  steps: TraceStep[];
  /** 回放结论：命中的规则。与「当时真实裁决」可能不同——规则后来被改过 */
  matched: Rule | null;
  scopeNote: string;
  /** 未取到条件而没参与求值的规则数（0 = 全部规则都真的被求值了） */
  skippedForDetail: number;
  /** 真正带条件、参与了求值的规则数 */
  evaluableCount: number;
}

/** 拉全量规则（租户上限 200 条），避免「只能预览列表当前页」这种半吊子预览 */
export async function loadAllRules(): Promise<Rule[]> {
  const items: Rule[] = [];
  for (let page = 1; page <= MAX_RULE_PAGES; page += 1) {
    const res = await listRules({ page, pageSize: PAGE_SIZE });
    items.push(...res.items);
    if (items.length >= res.total || res.items.length < PAGE_SIZE) break;
  }
  return items;
}

/** 按短码 + Host 定位本租户短链：短码在租户内不唯一，必须连域名一起匹配 */
export async function resolveLink(hostname: string, code: string): Promise<Link | undefined> {
  if (!code) return undefined;
  for (let page = 1; page <= MAX_LINK_PAGES; page += 1) {
    const res = await listLinks({ page, pageSize: PAGE_SIZE });
    const found = res.items.find(
      (l) => l.code === code && (l.domains.length === 0 || l.domains.includes(hostname)),
    );
    if (found) return found;
    if (res.items.length < PAGE_SIZE) break;
  }
  return undefined;
}

export interface TraceOptions {
  /** 已知的短链：调用方已经持有时直接传入，省掉逐页扫描 listLinks */
  link?: Link;
  /** 只回放这一条规则（模拟器的「仅看规则 #N」用） */
  onlyRuleId?: number | null;
}

/**
 * 按优先级升序跑一遍规则链，首条命中即定案（First-Match-Wins）。
 *
 * 每条规则的逐条件判定来自 ruleSim.ts，与后端 internal/rules/eval.go 对齐。
 */
export async function buildDecisionTrace(
  input: SimInput,
  opts: TraceOptions = {},
): Promise<DecisionTrace> {
  const facts = buildVisitorFacts(input);

  // 适用范围：能定位到短链时，以该短链实际的适用规则为准（含全局继承项）
  let applicable: Set<number> | null = null;
  let scopeNote: string;
  if (opts.link) {
    const items: LinkRule[] = await listLinkRules(opts.link.id);
    applicable = new Set(items.map((i) => i.id));
    scopeNote = `URL 命中短链 /${opts.link.code}，已按该短链实际的适用规则（含全局继承）求值。`;
  } else {
    const { hostname, code } = parseSimTarget(input.url);
    const link = await resolveLink(hostname, code);
    if (link) {
      const items: LinkRule[] = await listLinkRules(link.id);
      applicable = new Set(items.map((i) => i.id));
      scopeNote = `URL 命中短链 /${link.code}，已按该短链实际的适用规则（含全局继承）求值。`;
    } else {
      applicable = null;
      scopeNote = code
        ? `未在本租户找到短码 /${code}，本次仅按 scope=global 的全局规则求值；「指定短链」的规则不在预览范围内。`
        : 'URL 里没有短码，本次仅按 scope=global 的全局规则求值；「指定短链」的规则不在预览范围内。';
    }
  }

  const all = await loadAllRules();
  const candidates = all
    .filter((r) => r.enabled)
    .filter((r) => opts.onlyRuleId == null || r.id === opts.onlyRuleId)
    .sort((a, b) => a.priority - b.priority);

  // 列表接口不带 conditions：按需取详情（取不到就退出求值，而不是当空条件糊过去）
  const detailed = await mapWithConcurrency(candidates, 8, async (r) => {
    if (r.conditions && r.conditions.length > 0) return r;
    try {
      return await getRule(r.id);
    } catch {
      return r;
    }
  });
  const skippedForDetail = detailed.filter((r) => (r.conditions?.length ?? 0) === 0).length;
  const evaluableCount = detailed.length - skippedForDetail;

  const steps: TraceStep[] = [];
  let matched: Rule | null = null;

  for (const rule of detailed) {
    const key = `step-${rule.id}`;
    const inScope = rule.scope === 'global' || (applicable ? applicable.has(rule.id) : false);
    if (!inScope) {
      steps.push({
        key,
        ruleId: rule.id,
        ruleName: rule.name,
        status: 'skip',
        statusText: '不适用',
        facts: [],
        whyText:
          rule.scope === 'links' && rule.linkCount === 0
            ? '未关联短链 · 不会命中（不会退化为全局规则）'
            : '作用域为「指定短链」，本次请求的短链不在其关联列表中',
      });
      continue;
    }
    if (matched) {
      steps.push({
        key,
        ruleId: rule.id,
        ruleName: rule.name,
        status: 'skip',
        statusText: '已跳过',
        facts: [],
        whyText: '首条命中即裁决（First-Match-Wins），后续规则不再求值',
      });
      continue;
    }

    const factsOfConds = (rule.conditions ?? []).map((c) => evalCondition(c, facts));
    // 空条件组后端视为「无条件即执行」(eval.go matchAll: len(conds)==0 → true),
    // 而且 conditions:[] 是合法落库状态。所以这里必须当命中处理,
    // 否则一条 action=notfound 的空条件规则会显示「无规则命中」而线上拦下全部流量。
    const ruleMatched =
      factsOfConds.length === 0 ||
      (rule.logic === 'all' ? factsOfConds.every((f) => f.hit) : factsOfConds.some((f) => f.hit));

    if (ruleMatched) {
      matched = rule;
      const isBlock = isBlockingAction(rule.action);
      steps.push({
        key,
        ruleId: rule.id,
        ruleName: rule.name,
        status: isBlock ? 'block' : 'hit',
        statusText: '命中 · 裁决',
        facts: factsOfConds,
        whyText:
          factsOfConds.length === 0
            ? `无任何条件，后端视为「无条件即执行」，直接执行动作：${actionLabel(rule.action)}${
                rule.action === 'redirect' ? ` → ${rule.destination || '（未填写目标）'}` : ''
              }`
            : `条件${rule.logic === 'all' ? '全部' : '任一'}满足，执行动作：${actionLabel(rule.action)}${
                rule.action === 'redirect' ? ` → ${rule.destination || '（未填写目标）'}` : ''
              }`,
      });
    } else {
      steps.push({
        key,
        ruleId: rule.id,
        ruleName: rule.name,
        status: 'skip',
        statusText: '未命中',
        facts: factsOfConds,
        whyText: '条件不满足，继续求值下一条规则',
      });
    }
  }

  return { facts, steps, matched, scopeNote, skippedForDetail, evaluableCount };
}

export interface Verdict {
  title: string;
  action: string;
  matched: boolean;
  blocking: boolean;
  actionText: string;
  detailText: string;
}

/** 把回放结论整理成一句话裁决，模拟器与访问明细页共用同一套措辞 */
export function verdictOf(trace: DecisionTrace): Verdict {
  const { evaluableCount } = trace;
  const matched = trace.matched;
  if (matched) {
    const blocking = isBlockingAction(matched.action);
    return {
      title: `命中 #${matched.id} · ${actionLabel(matched.action)}`,
      action: matched.action,
      matched: true,
      blocking,
      actionText:
        matched.action === 'redirect'
          ? `${actionLabel(matched.action)} → ${matched.destination || '（未填写目标）'}`
          : actionLabel(matched.action),
      detailText: `依据规则「${matched.name}」（优先级 ${matched.priority}）判定；命中只计 visits，动作不灌水访问量。`,
    };
  }
  return {
    title: '无规则命中',
    action: 'pass',
    matched: false,
    blocking: false,
    actionText: evaluableCount === 0 ? '无可用规则（没有已启用且带条件的规则）' : '未命中任何规则',
    detailText:
      evaluableCount === 0
        ? '本租户没有已启用且带条件的规则，请先在规则列表中创建并启用。'
        : '全部适用规则均未命中，按 spec D4 继续走短链自身的目标选择流程。',
  };
}
