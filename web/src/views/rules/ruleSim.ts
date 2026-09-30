/**
 * 规则模拟器的求值引擎：把一次「假想访问」解析成 13 个可判定字段,
 * 再用与后端 `internal/rules` 同一套语义逐条判定条件。
 *
 * 这里是**纯函数**模块——不碰 API、不碰路由、不碰组件状态,
 * 因此既能喂给规则模拟器页面,也能被将来任何「解释某条规则为什么命中」的功能直接复用。
 *
 * 前端等价实现的边界必须写在代码里(而不是留给用户猜):
 * - 地理字段(country / asn)恒空,依赖它们的条件恒不命中,与后端一致;
 * - 正则后端是 RE2 且大小写敏感,JS 的近似见 evalCondition 的 regex 分支;
 * - 后端求值发生在短链可用性之后,这里只看规则侧。
 */
import { fieldOption, operatorLabel } from '@/views/rules/ruleMeta';
import type { RuleCondition, RuleField } from '@/types/api';

/** 模拟器左侧的输入项 */
export interface SimInput {
  url: string;
  ip: string;
  ua: string;
  lang: string;
  ref: string;
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
  status: 'hit' | 'block' | 'skip';
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
    // GeoIP / ASN 数据源未接入,恒空 → 依赖这两个字段的条件恒不命中(spec D5)
    country: '',
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
    { field: 'country', label: '国家 / 地区', value: '—', note: '数据源待接入 · 恒不命中', pending: true },
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
