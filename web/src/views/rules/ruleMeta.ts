/**
 * 规则的静态元数据：字段 / 运算符 / 动作 / 判定逻辑的可选集合与文案。
 *
 * 规则列表、规则表单、规则模拟器三处共用同一份定义——条件的中文标签一旦
 * 各处自己拼,同一个 `devtype` 会在列表里叫「设备类型」、在决策链里叫「UA 类型」,
 * 排障时对不上号。
 */
import type { Rule, RuleAction, RuleCondition, RuleField, RuleLogic, RuleOperator } from '@/types/api';

export interface FieldOption {
  value: RuleField;
  label: string;
  defaultOp: RuleOperator;
  placeholder: string;
  /** 取值示例,展示在条件区提示里 */
  hint?: string;
  /** 数据源待接入:后端恒取不到值,置灰不可选(spec D5) */
  pending?: boolean;
  /** 接入后的数据来源,写进代码注释便于将来接回 */
  source: string;
}

/**
 * 条件判定字段(spec D5):收敛到后端从请求即可真实求值的 13 个。
 * 原型里的 18 个字段里,region / city / tz / screen / tls(JA3) / canvas / cookie
 * 后端当前没有任何数据源,已移出 v1 —— 不把不可求值的字段落库,否则就是「能配不能跑」的假能力。
 * country 已接后端内嵌的离线 GeoIP 库(取不到时恒空、恒不命中);
 * asn 仍无数据源,保留占位,UI 上置灰并显式说明。
 */
export const FIELD_OPTIONS: FieldOption[] = [
  { value: 'ip', label: 'IP 地址 / CIDR', defaultOp: 'in', placeholder: '198.51.100.0/24, 203.0.113.7', hint: '支持 CIDR 网段与单个 IP', source: 'X-Forwarded-For / RemoteAddr' },
  { value: 'ipattr', label: 'IP 属性', defaultOp: 'in', placeholder: 'private, loopback, linklocal', hint: 'private / loopback / linklocal', source: 'net.IP 判定' },
  { value: 'country', label: '国家 / 地区', defaultOp: 'in', placeholder: 'US, CN', hint: 'ISO 国家码', source: '访客 IP 的离线 GeoIP 库（ip2region）' },
  { value: 'asn', label: 'ASN / 运营商', defaultOp: 'in', placeholder: 'AS15169, AS16509', hint: 'AS 号', pending: true, source: 'visits.asn（ASN mmdb 接入前恒空）' },
  { value: 'lang', label: '语言 (Accept-Language)', defaultOp: 'in', placeholder: 'zh-CN, pt-BR', hint: '取首个语言标签', source: 'Accept-Language 首标签' },
  { value: 'ref', label: 'Referrer 主机名', defaultOp: 'in', placeholder: 'facebook.com, google.com', hint: '取主机名，不含协议与路径', source: 'Referer 主机名' },
  { value: 'utm', label: 'UTM 来源', defaultOp: 'eq', placeholder: 'wechat, google', hint: '取 utm_source 查询参数', source: 'utm_source 查询参数' },
  { value: 'ua', label: 'User-Agent', defaultOp: 'contains', placeholder: 'bot, spider, crawler', hint: 'User-Agent 原串', source: 'User-Agent 原串' },
  { value: 'devtype', label: '设备类型', defaultOp: 'in', placeholder: 'bot, mobile, tablet, desktop', hint: 'bot / mobile / tablet / desktop', source: 'UA 轻量判定' },
  { value: 'os', label: '操作系统', defaultOp: 'in', placeholder: 'iOS, Android, Windows, macOS, Linux', hint: 'iOS / Android / Windows / macOS / Linux / 其他', source: 'UA 轻量判定' },
  { value: 'browser', label: '浏览器', defaultOp: 'in', placeholder: 'Chrome, Safari, Firefox, Edge', hint: 'Chrome / Safari / Firefox / Edge / 其他', source: 'UA 轻量判定' },
  { value: 'path', label: '请求路径', defaultOp: 'in', placeholder: '/promo, /black-friday', hint: '形如 /abc 的短码路径', source: '请求路径' },
  { value: 'domain', label: '请求域名 (Host)', defaultOp: 'in', placeholder: 'go.example.com', hint: '访问所用的域名', source: '请求 Host' },
];

/**
 * 运算符选项：后端 Evaluate 白名单的**可放出子集**。
 *
 * 刻意不含 `duplicated`：后端白名单接受它、求值也已接线到 Fact.Seen 通道,
 * 但平台没有指纹/计数器数据源,Fact.Seen 恒为 nil → 该运算符恒不命中。
 * 留着就是「能配不能跑」的假能力,而它在每条条件的下拉里都出现,
 * 一个永远点不亮的死选项比没有更糟:后端接线已完成,
 * **待接入访问计数数据源(滑动窗口)后再放出**。
 * 类型 RuleOperator 仍保留该值,用于读回经 API 写入的历史数据。
 */
export const OPERATOR_OPTIONS: { value: RuleOperator; label: string; hint: string }[] = [
  { value: 'in', label: '属于', hint: '访客值落在任一取值内（ip 字段按 CIDR / 字面量匹配）' },
  { value: 'not_in', label: '不属于', hint: 'in 的反面:任一取值命中即不成立' },
  { value: 'eq', label: '等于', hint: '与取值完全相等（忽略大小写）' },
  { value: 'neq', label: '不等于', hint: '与取值均不相等' },
  { value: 'contains', label: '包含', hint: '访客值包含任一取值' },
  { value: 'not_contains', label: '不包含', hint: '不包含任一取值' },
  { value: 'starts_with', label: '开头是', hint: '访客值以任一取值开头（忽略大小写）' },
  { value: 'ends_with', label: '结尾是', hint: '访客值以任一取值结尾（忽略大小写）' },
  { value: 'gt', label: '大于', hint: '按数值比较,非数值恒不命中' },
  { value: 'lt', label: '小于', hint: '按数值比较,非数值恒不命中' },
  { value: 'regex', label: '正则匹配', hint: '后端为 RE2 语法且大小写敏感（需忽略大小写请在表达式里写 (?i)）；复杂表达式的前端预览结果可能与线上略有差异' },
  { value: 'in_cidr', label: '落在 IP 网段', hint: '访客 IP 落在任一 CIDR 网段内（仅 ip 字段可用）' },
  { value: 'not_in_cidr', label: '不在 IP 网段（白名单）', hint: '访客 IP 未落在任一 CIDR 网段内（仅 ip 字段可用，用于白名单拦截）' },
];

/** 命中动作:收敛为四项(spec D4 裁决顺序) */
export const ACTION_OPTIONS: { value: RuleAction; label: string; desc: string }[] = [
  { value: 'pass', label: '放行', desc: '记录命中后继续走短链自身的跳转 / 落地页流程（不改写目标）。' },
  { value: 'redirect', label: '重定向到指定 URL', desc: '把访问改写到下方填写的目标 URL；该地址不参与短链目标池轮询。' },
  { value: 'notfound', label: '直接 404', desc: '视为未命中短链,记 outcome=failed、reason=rule_blocked。' },
  { value: 'throttle', label: '限流 429', desc: '记 outcome=failed、reason=rule_throttled,不计入访问量。' },
];

export const LOGIC_OPTIONS: { value: RuleLogic; label: string }[] = [
  { value: 'all', label: '全部满足' },
  { value: 'any', label: '任一满足' },
];

export function actionLabel(action: RuleAction): string {
  return ACTION_OPTIONS.find((o) => o.value === action)?.label ?? action;
}

/** 阻断类动作:列表标签色与模拟器裁决卡片共用同一套语义色 */
export function isBlockingAction(action: RuleAction): boolean {
  return action === 'notfound' || action === 'throttle';
}

/** 列表里的动作标签色(AppTag color) */
export function actionTagColor(action: RuleAction): string {
  switch (action) {
    case 'pass':
      return 'ok';
    case 'redirect':
      return 'info';
    case 'notfound':
      return 'err';
    case 'throttle':
      return 'warn';
    default:
      return 'default';
  }
}

export function logicLabel(logic: RuleLogic): string {
  return LOGIC_OPTIONS.find((o) => o.value === logic)?.label ?? logic;
}

export function fieldOption(field: string): FieldOption | undefined {
  return FIELD_OPTIONS.find((f) => f.value === field);
}

export function operatorLabel(operator: string): string {
  return OPERATOR_OPTIONS.find((o) => o.value === operator)?.label ?? operator;
}

/** 单条条件的一句话描述,列表摘要与模拟器决策链共用 */
export function describeCondition(cond: RuleCondition): string {
  const label = fieldOption(cond.field)?.label ?? cond.field;
  const op = operatorLabel(cond.operator);
  const values = (cond.values || []).join(' / ') || '（空）';
  return `${label} ${op} ${values}`;
}

/** 列表接口不回传 conditions,没有条件就不编造摘要 */
export function conditionSummary(rule: Rule): string {
  if (!rule.conditions || rule.conditions.length === 0) return '—';
  return rule.conditions.map(describeCondition).join(' · ');
}

/** 设备类型预设选项（与后端 internal/rules/fields.go 保持严格一致） */
export const DEVTYPE_OPTIONS = [
  { value: 'bot', label: '爬虫 / Bot (bot)' },
  { value: 'mobile', label: '手机 (mobile)' },
  { value: 'tablet', label: '平板 (tablet)' },
  { value: 'desktop', label: '桌面端 / PC (desktop)' },
];

/** 操作系统预设选项（后端识别的 5 大系统及其他） */
export const OS_OPTIONS = [
  { value: 'iOS', label: 'iOS' },
  { value: 'Android', label: 'Android' },
  { value: 'Windows', label: 'Windows' },
  { value: 'macOS', label: 'macOS' },
  { value: 'Linux', label: 'Linux' },
  { value: '其他', label: '其他操作系统' },
];

/** 浏览器预设选项（后端识别的 4 大浏览器及其他） */
export const BROWSER_OPTIONS = [
  { value: 'Chrome', label: 'Chrome' },
  { value: 'Safari', label: 'Safari' },
  { value: 'Firefox', label: 'Firefox' },
  { value: 'Edge', label: 'Edge' },
  { value: '其他', label: '其他浏览器' },
];

/** IP 属性预设选项（后端仅识别三种私有/局域属性，非此类为空） */
export const IPATTR_OPTIONS = [
  { value: 'private', label: '私网 / 局域网 (private)' },
  { value: 'loopback', label: '回环地址 (loopback)' },
  { value: 'linklocal', label: '链路本地 (linklocal)' },
];

/** 常用 Accept-Language 选项（后端按首标签小写化比较） */
export const LANG_OPTIONS = [
  { value: 'zh-cn', label: '中文（简体）zh-cn' },
  { value: 'zh-tw', label: '中文（繁体）zh-tw' },
  { value: 'zh-hk', label: '中文（香港）zh-hk' },
  { value: 'zh', label: '中文通用 zh' },
  { value: 'en-us', label: '英语（美国）en-us' },
  { value: 'en-gb', label: '英语（英国）en-gb' },
  { value: 'en', label: '英语通用 en' },
  { value: 'ja', label: '日语 ja / ja-jp' },
  { value: 'ko', label: '韩语 ko / ko-kr' },
  { value: 'es', label: '西班牙语 es' },
  { value: 'pt-br', label: '葡萄牙语（巴西）pt-br' },
  { value: 'pt', label: '葡萄牙语 pt' },
  { value: 'de', label: '德语 de' },
  { value: 'fr', label: '法语 fr' },
  { value: 'ru', label: '俄语 ru' },
  { value: 'vi', label: '越南语 vi' },
  { value: 'th', label: '泰语 th' },
  { value: 'id', label: '印尼语 id' },
  { value: 'ar', label: '阿拉伯语 ar' },
];
