/**
 * 规则模拟器的**展示层**：把后端算好的访客画像与决策链映射成页面要的渲染形态。
 *
 * 求值不再在前端做。`internal/rules` 是唯一求值实现,前端曾经有一份等价实现
 * (buildVisitorFacts + evalCondition),两处各算一次必然漂移——RE2 与 JS 正则、
 * GeoIP 与浏览器无数据源,都会让模拟结论与线上裁决对不上,而对不上的诊断比没有诊断更糟:
 * 它会让人照着假结论去改规则。现在规则模拟器直接调 `POST /api/rules/simulate`,
 * 与线上 Evaluate 共用同一套求值顺序与同一批条件判定函数。
 *
 * 本文件剩下的等价实现仍被短链访问明细页使用(它要解释的是**历史**某次访问,
 * 与现在的规则状态无关,不能回放),等那一页也切到接口后再删。
 *
 * 这里是纯函数模块——不碰 API、不碰路由、不碰组件状态。
 */
import type { SimulateStep, SimulateVerdict } from '@/api/rules';
import { actionLabel, fieldOption, operatorLabel } from '@/views/rules/ruleMeta';
import type { RuleAction, RuleCondition, RuleField } from '@/types/api';

/** 模拟器左侧的输入项 */
export interface SimInput {
  url: string;
  ip: string;
  ua: string;
  lang: string;
  ref: string;
  /** ISO 3166-1 alpha-2 国家码。通常留空,后端会用离线库按 IP 解析;
   *  只有想验证「假如这个访客来自 XX 国」时才手填覆盖(手填会盖过 GeoIP 结果)。 */
  country: string;
}

/** 从请求中解析出的 13 个可判定字段 */
export interface VisitorFacts {
  ip: string;
  ipattr: string;
  country: string;
  asn: string;
  lang: string;
  ref: string;
  utm: string;
  ua: string;
  devtype: string;
  os: string;
  browser: string;
  path: string;
  domain: string;
}

export interface TraceFact {
  text: string;
  hit: boolean;
}

/** 决策链的一步:一条规则在这个访客身上的判定结果 */
export interface TraceStep {
  key: string;
  ruleId: number;
  ruleName: string;
  status: 'hit' | 'block' | 'skip' | 'disabled';
  statusText: string;
  facts: TraceFact[];
  whyText: string;
}

export function classifyIpAttr(ip: string): string {
  const parts = ip.trim().split('.');
  if (parts.length !== 4) return '';
  const nums = parts.map((p) => Number(p));
  if (nums.some((n) => !Number.isInteger(n) || n < 0 || n > 255)) return '';
  const [a, b] = nums;
  if (a === 127) return 'loopback';
  if (a === 10) return 'private';
  if (a === 192 && b === 168) return 'private';
  if (a === 172 && b !== undefined && b >= 16 && b <= 31) return 'private';
  if (a === 169 && b === 254) return 'linklocal';
  return '';
}

/**
 * UA 判定。与后端 internal/rules/fields.go 的 uaFacts 逐条对齐——
 * 刻意不用 ua-parser-js:后端是纯 token 匹配,两套判定必然漂移,
 * 而模拟器的价值就在于「所见即线上所判」,漂移一次它就变成骗人的东西。
 * 改这份列表时必须同步改 fields.go 里的 botTokens/tabletTokens/... 。
 */
const BOT_TOKENS = [
  'bot', 'spider', 'crawler', 'slurp', 'curl/', 'wget/', 'python-requests',
  'python-urllib', 'go-http-client', 'headlesschrome', 'phantomjs',
  'facebookexternalhit', 'bingpreview', 'ahrefs', 'semrush', 'mj12', 'dotbot',
  'petalbot', 'yandex', 'baiduspider', 'duckduckbot',
];
const TABLET_TOKENS = [
  'ipad', 'tablet', 'kindle', 'silk', 'playbook', 'sm-t', 'nexus 7', 'nexus 9', 'nexus 10',
];
const MOBILE_TOKENS = [
  'mobile', 'iphone', 'ipod', 'android', 'windows phone', 'blackberry',
  'opera mini', 'iemobile', 'webos',
];
const OS_IOS = ['iphone', 'ipad', 'ipod', 'ios'];
const OS_ANDROID = ['android'];
const OS_WINDOWS = ['windows nt', 'windows phone', 'win64', 'win32', 'windows'];
const OS_MAC = ['mac os x', 'macintosh', 'macos'];
const OS_LINUX = ['linux', 'x11'];
// 顺序有意义:Edge 的 UA 里也含 Chrome/Safari 字样,Firefox 的 UA 里也含 Safari
const EDGE_TOKENS = ['edg/', 'edga/', 'edgios/', 'edge/'];
const FIREFOX_TOKENS = ['firefox/', 'fxios/'];
const CHROME_TOKENS = ['crios/', 'chrome/', 'chromium/'];
const SAFARI_TOKENS = ['safari/'];
// 内核像 Chrome 但浏览器不是 Chrome 的一票否决名单(Opera/Brave 等)
const CHROMIUM_FORKS = [
  'opr/', 'opera', 'vivaldi', 'brave/', 'yabrowser', 'samsungbrowser', 'ucbrowser',
  'quark/', 'miuibrowser', 'heytapbrowser', 'electron',
];

function containsAny(lower: string, tokens: string[]): boolean {
  return tokens.some((t) => lower.includes(t));
}

/**
 * UA 轻量判定,输出与后端一致的取值词表。
 * UA 为空时三个字段恒返回空串(不是猜成 desktop)——
 * 后端把这条当不变式:「空值恒不命中」才能让条件语义保持干净,
 * 猜成桌面会让「devtype in desktop」这类规则在无 UA 的 API 客户端上假成立。
 */
export function detectDevice(ua: string): { devtype: string; os: string; browser: string } {
  if (ua === '') return { devtype: '', os: '', browser: '' };
  const lower = ua.toLowerCase();

  let devtype = 'desktop';
  if (containsAny(lower, BOT_TOKENS)) devtype = 'bot';
  else if (containsAny(lower, TABLET_TOKENS)) devtype = 'tablet';
  else if (containsAny(lower, MOBILE_TOKENS)) devtype = 'mobile';

  let os = '其他';
  if (containsAny(lower, OS_IOS)) os = 'iOS';
  else if (containsAny(lower, OS_ANDROID)) os = 'Android';
  else if (containsAny(lower, OS_WINDOWS)) os = 'Windows';
  else if (containsAny(lower, OS_MAC)) os = 'macOS';
  else if (containsAny(lower, OS_LINUX)) os = 'Linux';

  let browser = '其他';
  if (containsAny(lower, EDGE_TOKENS)) browser = 'Edge';
  else if (containsAny(lower, FIREFOX_TOKENS)) browser = 'Firefox';
  else if (containsAny(lower, CHROME_TOKENS) && !containsAny(lower, CHROMIUM_FORKS)) browser = 'Chrome';
  else if (containsAny(lower, SAFARI_TOKENS) && !containsAny(lower, CHROMIUM_FORKS)) browser = 'Safari';

  return { devtype, os, browser };
}

/** 与后端 firstLangTag 同口径:先截 "," 再截 ";"、trim、小写 */
function firstLangTag(raw: string): string {
  let first = raw.split(',')[0] ?? '';
  const semi = first.indexOf(';');
  if (semi >= 0) first = first.slice(0, semi);
  return first.trim().toLowerCase();
}

export function buildVisitorFacts(input: SimInput): VisitorFacts {
  const url = input.url.trim();
  let domain = '';
  let path = '';
  let utm = '';
  try {
    const u = new URL(url);
    domain = u.hostname;
    path = u.pathname || '/';
    utm = u.searchParams.get('utm_source') ?? '';
  } catch {
    // URL 不合法时按裸路径处理
    path = url.startsWith('/') ? url : '';
  }

  let ref = '';
  try {
    if (input.ref.trim()) ref = new URL(input.ref.trim()).hostname;
  } catch {
    ref = '';
  }

  const ua = input.ua;
  const { devtype, os, browser } = detectDevice(ua);

  return {
    ip: input.ip.trim(),
    ipattr: classifyIpAttr(input.ip),
    // 手填值,后端存的就是这个形态的码(ISO 3166-1 alpha-2);
    // 留空 = 取不到 = 依赖它的条件恒不命中,与后端同一条不变式。
    country: input.country.trim().toUpperCase(),
    // 当前无 ASN 数据源,恒空 → 依赖它的条件恒不命中(ADR 0009)
    asn: '',
    lang: firstLangTag(input.lang),
    ref,
    utm,
    ua,
    devtype,
    os,
    browser,
    path,
    domain,
  };
}

export interface VisitorFieldView {
  field: RuleField;
  label: string;
  value: string;
  note: string;
  pending: boolean;
}

/**
 * 后端扁平化的 13 个字段 → 页面画像结构。
 * 后端对取不到数据的字段回空串(空值恒不命中),这里保持空串不补默认值:
 * 补一个猜测值会让画像看起来有数据,而依赖它的条件其实永不成立。
 */
export function visitorFactsFromServer(facts: Record<string, string>): VisitorFacts {
  return {
    ip: facts.ip || '',
    ipattr: facts.ipattr || '',
    country: facts.country || '',
    asn: facts.asn || '',
    lang: facts.lang || '',
    ref: facts.ref || '',
    utm: facts.utm || '',
    ua: facts.ua || '',
    devtype: facts.devtype || '',
    os: facts.os || '',
    browser: facts.browser || '',
    path: facts.path || '',
    domain: facts.domain || '',
  };
}

/**
 * 后端决策链 → 页面步骤。判词(text/whyText)全部来自后端:
 * 前端自己组织一句解释,就等于在诊断页里塞进第二套语义。
 */
export function traceStepsFromServer(steps: SimulateStep[], winner: SimulateVerdict): TraceStep[] {
  return steps.map((step, i) => {
    const conditions = step.conditions || [];
    const isWinner = step.status === 'hit' && step.ruleId === winner.ruleId;
    // hit 意味着「这条规则参与了裁决」。阻断类裁决要在链上标红,
    // 否则 notfound 与 redirect 在页面上看不出区别。
    const status: TraceStep['status'] = isWinner && winner.blocked ? 'block' : step.status;
    return {
      // 草稿规则没有 id,带下标保证 key 唯一
      key: `${step.draft ? 'draft' : 'rule'}-${step.ruleId}-${i}`,
      ruleId: step.ruleId,
      ruleName: step.ruleName,
      status,
      statusText: stepStatusText(step.status, conditions.length > 0),
      facts: conditions.map((c) => ({ text: c.description || '', hit: c.matched === true })),
      whyText: step.reason || '',
    };
  });
}

/**
 * skip 有两种:作用域/优先级不适用,和真比了没比过。不区分的话
 * 用户看到一串「跳过」无法判断该改作用域还是改条件。
 * 服务端只在真正求值时才回传条件痕迹,据此区分。
 */
function stepStatusText(status: SimulateStep['status'], evaluated: boolean): string {
  if (status === 'disabled') return '已停用';
  if (status === 'hit') return '命中 · 裁决';
  return evaluated ? '未命中' : '不适用';
}

/** 裁决的页面渲染形态。只留 id 不带整条规则:预览页现取,省掉一次无用的回传 */
export interface VerdictView {
  title: string;
  action: string;
  matched: boolean;
  blocking: boolean;
  actionText: string;
  detailText: string;
  matchedRuleId: number | null;
}

export function verdictFromServer(v: SimulateVerdict): VerdictView {
  const action = v.action as RuleAction;
  if (!v.matched) {
    return {
      title: '无规则命中',
      action,
      matched: false,
      blocking: false,
      actionText: '未命中任何规则 · 访客看到原短链',
      detailText: v.message || '',
      matchedRuleId: null,
    };
  }
  const label = actionLabel(action);
  return {
    title: `命中 #${v.ruleId} · ${v.ruleName || label}`,
    action,
    matched: true,
    blocking: v.blocked,
    actionText: v.destination ? `${label} → ${v.destination}` : label,
    detailText: v.message || '',
    matchedRuleId: v.ruleId,
  };
}

/** 访客画像:13 个字段的展示形态(空值显式标出,不静默留白) */
export function visitorFieldViews(facts: VisitorFacts | null): VisitorFieldView[] {
  const f = facts;
  return [
    { field: 'ip', label: 'IP 地址', value: f?.ip || '—', note: '', pending: false },
    {
      field: 'ipattr',
      label: 'IP 属性',
      // 空串就是空串。后端对公网 IP 恒返回空,空值恒不命中(spec D5),
      // 展示成「公网 IP」会诱导用户去配 ipattr eq 公网——那条规则永不成立。
      value: f?.ipattr || '—',
      note: f?.ipattr ? '' : '非 private / loopback / linklocal · 该字段空值恒不命中',
      pending: false,
    },
    {
      field: 'country',
      label: '国家 / 地区',
      // 显示原始国家码而不是中文名:条件里配的就是码,翻译过反而对不上
      value: f?.country || '—',
      // 国家由后端离线库按 IP 解析,前端不再自己猜。回空串只有两种可能:IP 是私网/保留段,
      // 或离线库里没有该段——两种都不该配国家条件,写清楚比显示一个中文名有用。
      note: f?.country ? '' : '后端离线库未解析出国家(私网 IP 或库中无此段) · 留空按取不到处理',
      pending: false,
    },
    { field: 'asn', label: 'ASN / 运营商', value: '—', note: '数据源待接入 · 恒不命中', pending: true },
    { field: 'lang', label: '语言', value: f?.lang || '—', note: '', pending: false },
    { field: 'ref', label: 'Referrer 主机', value: f?.ref || '—', note: '', pending: false },
    { field: 'utm', label: 'UTM 来源', value: f?.utm || '—', note: '', pending: false },
    { field: 'ua', label: 'User-Agent', value: f?.ua || '—', note: '', pending: false },
    { field: 'devtype', label: '设备类型', value: f?.devtype || '—', note: '', pending: false },
    { field: 'os', label: '操作系统', value: f?.os || '—', note: '', pending: false },
    { field: 'browser', label: '浏览器', value: f?.browser || '—', note: '', pending: false },
    { field: 'path', label: '请求路径', value: f?.path || '—', note: '', pending: false },
    { field: 'domain', label: '请求 Host', value: f?.domain || '—', note: '', pending: false },
  ];
}

function ipToInt(ip: string): number | null {
  const parts = ip.trim().split('.');
  if (parts.length !== 4) return null;
  let out = 0;
  for (const p of parts) {
    if (!/^\d{1,3}$/.test(p)) return null;
    const n = Number(p);
    if (n > 255) return null;
    out = out * 256 + n;
  }
  return out;
}

/** CIDR 匹配(仅 IPv4,与后端 net.IP.ParseCIDR + Contains 一致) */
function ipInCidr(ip: string, cidr: string): boolean {
  const [net, bitsRaw] = cidr.split('/');
  const netInt = ipToInt(net ?? '');
  const ipInt = ipToInt(ip);
  if (netInt === null || ipInt === null) return false;
  const bits = Number(bitsRaw);
  if (!Number.isInteger(bits) || bits < 0 || bits > 32) return false;
  if (bits === 0) return true;
  const mask = (0xffffffff << (32 - bits)) >>> 0;
  return (netInt & mask) === (ipInt & mask);
}

function matchValue(value: string, pattern: string): boolean {
  if (pattern.includes('/') && /^[0-9./]+$/.test(pattern)) {
    return ipInCidr(value, pattern);
  }
  return value.toLowerCase() === pattern.toLowerCase();
}

function containsValue(value: string, pattern: string): boolean {
  return value.toLowerCase().includes(pattern.toLowerCase());
}

export function evalCondition(cond: RuleCondition, facts: VisitorFacts): TraceFact {
  const actual = facts[cond.field] ?? '';
  // 与后端 compileCond 同口径:每个值先 TrimSpace 再丢空串,
  // 归一后的值才是参与比较的值(否则手写进 API 的 ' zh-CN ' 会出现模拟器不命中、线上命中)。
  const values = (cond.values || []).map((v) => v.trim()).filter(Boolean);
  const valuesText = values.join(' / ') || '（空）';
  const label = fieldOption(cond.field)?.label ?? cond.field;
  const prefix = `${label} = ${actual || '（空）'}`;
  let note = '';

  let hit = false;
  switch (cond.operator) {
    case 'in':
    case 'eq':
    case 'neq':
    case 'not_in': {
      // ip 字段的集合比较走后端的 IP/CIDR 口径:实际值不是合法 IP 时恒不命中
      if (cond.field === 'ip' && ipToInt(actual) === null && !actual.includes(':')) {
        note = '（IP 不可解析）';
        break;
      }
      if (cond.field === 'ip' && actual.includes(':')) {
        // 后端用 net.ParseIP + net.IPNet.Contains，能正确处理 IPv6；
        // 模拟器的 CIDR 实现只支持 IPv4，IPv6 只能做字面量全等比较。
        note = '（IPv6 仅支持字面量匹配，CIDR 判定以后端为准）';
      }
      const anyHit = values.some((v) => matchValue(actual, v));
      if (cond.operator === 'in' || cond.operator === 'eq') {
        hit = anyHit;
      } else {
        // 实际值为空(数据源缺失)时 not_in / neq 也判不成立,避免「恒命中」的假拦截
        hit = actual !== '' && !anyHit;
      }
      break;
    }
    case 'contains':
      hit = values.some((v) => containsValue(actual, v));
      break;
    case 'not_contains':
      hit = actual !== '' && values.every((v) => !containsValue(actual, v));
      break;
    case 'gt':
    case 'lt': {
      const left = Number(actual);
      const right = Number(values[0]);
      if (actual === '' || values.length === 0 || Number.isNaN(left) || Number.isNaN(right)) {
        hit = false;
      } else {
        hit = cond.operator === 'gt' ? left > right : left < right;
      }
      break;
    }
    case 'regex': {
      // 后端是 RE2 且大小写敏感(要忽略大小写得自己写 (?i));JS 不认 (?i),
      // 因此先按原样试,编译不过再退化为「剥掉 (?i) + 不区分大小写」的近似匹配。
      let patternBroken = false;
      hit = values.some((v) => {
        try {
          return new RegExp(v).test(actual);
        } catch {
          // 继续尝试近似
        }
        try {
          return new RegExp(v.replace(/^\(\?i\)/, ''), 'i').test(actual);
        } catch {
          patternBroken = true;
          return false;
        }
      });
      if (patternBroken) note = '（正则无法在前端编译,已按不命中处理）';
      break;
    }
    case 'duplicated':
      // 不在 OPERATOR_OPTIONS 里放出（恒不命中的假能力,见该常量注释）;
      // 仅为读回经 API 写入的历史条件而保留:没有计数数据源时恒判不成立
      hit = false;
      break;
    default:
      hit = false;
  }

  return {
    text: `${prefix} ${operatorLabel(cond.operator)} ${valuesText} → ${hit ? '成立' : '不成立'}${note}`,
    hit,
  };
}

/** 简单的并发限制映射（模拟器要取详情,避免一次性打满浏览器连接） */
export async function mapWithConcurrency<T, R>(
  items: T[],
  limit: number,
  fn: (item: T) => Promise<R>,
): Promise<R[]> {
  const results: R[] = [];
  let cursor = 0;
  const workers = Array.from({ length: Math.max(1, Math.min(limit, items.length)) }, async () => {
    while (cursor < items.length) {
      const idx = cursor;
      cursor += 1;
      results[idx] = await fn(items[idx]);
    }
  });
  await Promise.all(workers);
  return results;
}

/** 从模拟器输入的 URL 里取出短码与 Host,用于定位本租户短链 */
export function parseSimTarget(url: string): { hostname: string; code: string } {
  let hostname = '';
  let pathname = '';
  try {
    const u = new URL(url.trim());
    hostname = u.hostname;
    pathname = u.pathname;
  } catch {
    return { hostname: '', code: '' };
  }
  const code = pathname.replace(/^\/+/, '').split('/')[0] ?? '';
  return { hostname, code };
}
