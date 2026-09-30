// CLOAK API 契约类型定义(对齐 .scratch/cloak/api-contract.md)
// 枚举:
//   tenant.status: pending | active | banned
//   domain.status: pending | active | failed | stopped
//   domain.origin: self | platform
//   domain.certStatus: pending | issued | failed
//   link.status: enabled | disabled
//   link.redirectStatus: '302' | '301'

export type TenantStatus = 'pending' | 'active' | 'banned';
export type DomainStatus = 'pending' | 'active' | 'failed' | 'stopped';
export type DomainOrigin = 'self' | 'platform';
export type CertStatus = 'pending' | 'issued' | 'failed';
export type LinkStatus = 'enabled' | 'disabled';
export type RedirectStatus = '302' | '301';
/** 短链类型:跳转型 redirect / 落地页型 landing(默认 redirect) */
export type LinkType = 'redirect' | 'landing';
/** 落地页来源:url 地址 / upload 上传压缩包(默认 url,仅 landing 型有意义) */
export type LandingSource = 'url' | 'upload';
/** 访问动作:跳转型的一次跳转 / 落地页型的一次落地页视图 / 落地页按钮经 SDK 回传的一次点击 */
export type VisitAction = 'redirect' | 'landing_view' | 'click';
/** 访问结果:成功 / 失败(失败时 reason 必非空,且该行不计入 link.visits) */
export type VisitOutcome = 'success' | 'failed';
/** 失败原因:link_disabled 短链已停用 / link_deleted 短链已删除 /
 *  no_target 无可用目标 / landing_missing 落地页文件缺失 */
export type VisitReason = 'link_disabled' | 'link_deleted' | 'no_target' | 'landing_missing';

/* ── 规则引擎 ─────────────────────────────────────────────────────────── */
/** 规则作用域:global 对租户全部短链生效 / links 仅对 rule_links 显式关联的短链生效 */
export type RuleScope = 'global' | 'links';
/** 多个条件之间的关系:all 全部满足 / any 任一满足 */
export type RuleLogic = 'all' | 'any';
/** 命中动作:pass 记录命中并走原目标 / redirect 改写目标 URL /
 *  notfound 直接 404 / throttle 限流 429 */
export type RuleAction = 'pass' | 'redirect' | 'notfound' | 'throttle';
/** 短链侧规则的来源:inherited 继承自 scope=global 的全局规则 / scoped 来自 rule_links 显式关联 */
export type RuleSource = 'inherited' | 'scoped';
/** 条件判定字段:v1 收敛到后端从请求即可真实求值的 13 个
 *  (country / asn 依赖 GeoIP 数据源,接入前恒不命中) */
export type RuleField =
  | 'ip'
  | 'ipattr'
  | 'country'
  | 'asn'
  | 'lang'
  | 'ref'
  | 'utm'
  | 'ua'
  | 'devtype'
  | 'os'
  | 'browser'
  | 'path'
  | 'domain';
/**
 * 条件运算符(后端白名单;传白名单外的值会 400)。
 *
 * `duplicated` 的特殊状态:后端**接受**它(在 ValidOperator 白名单内、求值也已接线到
 * Fact.Seen 通道),但平台目前没有指纹/计数器数据源,Fact.Seen 恒为 nil,该运算符恒不命中。
 * 即“能提交、不能生效”的假能力,故 v1 不在 ruleMeta.ts 的 OPERATOR_OPTIONS 里放出
 * (后端接线已完成,接入访问计数滑动窗口后再放出)。保留在类型里是因为接口层确实可能
 * 回传它——经 API 写入的历史数据仍要能读得出来。
 */
export type RuleOperator =
  | 'in'
  | 'not_in'
  | 'eq'
  | 'neq'
  | 'contains'
  | 'not_contains'
  | 'gt'
  | 'lt'
  | 'regex'
  | 'duplicated';

/** 单条条件:values 为复数形式(逗号 / 换行分隔录入) */
export interface RuleCondition {
  field: RuleField;
  operator: RuleOperator;
  values: string[];
}

/** 规则。conditions 仅在 GET /api/rules/{id} 详情中保证完整,列表响应可能不带 */
export interface Rule {
  id: number;
  name: string;
  description: string;
  priority: number;
  scope: RuleScope;
  enabled: boolean;
  logic: RuleLogic;
  action: RuleAction;
  /** action=redirect 时的改写目标,其余动作为空串 */
  destination: string;
  conditions?: RuleCondition[];
  /** scope=links 时已关联的短链数(全局规则恒为 0) */
  linkCount: number;
  /** 已关联短链的可读标识，形如 `短码@域名`(短码在租户内不唯一，必须带域名)，列表最多回传前 3 个 */
  linkNames: string[];
  /** 已关联短链的完整 id 列表(不受 linkNames 前 3 个限制)，供编辑器精确回填关联 */
  linkIds: number[];
  hits24h: number;
  createdAt: string;
  updatedAt: string;
}

/** 规则下拉选项(GET /api/rules/options) */
export interface RuleOption {
  id: number;
  name: string;
  scope: RuleScope;
  action: RuleAction;
  priority: number;
  enabled: boolean;
  hits24h: number;
}

/** 短链适用的规则(GET /api/links/{id}/rules),含全局继承项与来源标记 */
export interface LinkRule {
  id: number;
  name: string;
  scope: RuleScope;
  action: RuleAction;
  priority: number;
  enabled: boolean;
  source: RuleSource;
}

/** 等级:决定租户的短链与域名数量上限 */
export interface Tier {
  id: number;
  name: string;
  maxLinks: number;
  maxDomains: number;
}

/** 配额用量(见 GET /api/me) */
export interface QuotaUsage {
  links: number;
  domains: number;
  maxLinks: number;
  maxDomains: number;
}

/** 前端启动配置(GET /api/config,按当前租户返回) */
export interface AppConfig {
  /** 本服务器公网 IP,DNS 校验指向地址(CLOAK_SERVER_PUBLIC_IP) */
  serverIp: string;
  /** 平台域名,如 cloak.test */
  platformDomain: string;
  /** 当前租户配额用量 */
  usage: QuotaUsage;
}

/** 租户 */
export interface Tenant {
  id: number;
  email: string;
  slug: string;
  status: TenantStatus;
  isSuperAdmin: boolean;
  codeLength: number;
  tier: Tier;
  /** 平台默认域名,形如 "<slug>.<平台域名>" */
  defaultDomain: string;
  createdAt: string;
  /** 仅超管首次登录(尚无密码)时为 true */
  firstLoginSetup?: boolean;
  /** 用量(仅 /api/me 与超管列表返回) */
  usage?: QuotaUsage;
}

/** 域名 */
export interface Domain {
  id: number;
  fqdn: string;
  /** 域名描述/备注,创建时填写,可空 */
  description: string;
  origin: DomainOrigin;
  status: DomainStatus;
  certStatus: CertStatus;
  activatedAt?: string;
  createdAt: string;
}

/** 短链 */
export interface Link {
  id: number;
  code: string;
  /** 目标 URL 列表(至少 1 个,顺序即轮询顺序) */
  targetUrls: string[];
  redirectStatus: RedirectStatus;
  /** 短链类型(创建时选定、可修改) */
  linkType: LinkType;
  /** 落地页来源(仅 landing 型有意义) */
  landingSource: LandingSource;
  /** 落地页地址(仅 landing+url 来源非空) */
  landingUrl: string;
  status: LinkStatus;
  /** 关联域名列表 */
  domains: string[];
  /** 访问数 */
  visits: number;
  /** 点击数(仅 landing 型增长,redirect 型恒 0) */
  clicks: number;
  /** landing+upload 来源且已成功上传 zip 时为 true */
  landingUploaded: boolean;
  /** 适用的规则条数(全局规则 + 显式关联的规则) */
  ruleCount: number;
  /** 适用的规则名,后端最多回传前 3 个 */
  ruleNames: string[];
  createdAt: string;
}

/** 访问记录(一行 = 一次动作;action='click' 的行不计入 link.visits) */
export interface Visit {
  id: number;
  linkId: number;
  domain: string;
  /** 访问者 IP(X-Forwarded-For 优先,回退 RemoteAddr) */
  ip: string;
  userAgent: string;
  referer: string;
  /** 本次触发的动作:跳转 / 落地页视图 / 点击回传 */
  action: VisitAction;
  /** 本次动作是否达成:成功 / 失败 */
  outcome: VisitOutcome;
  /** 失败原因,仅 outcome='failed' 时非空(见 VisitReason) */
  reason: string;
  /** 本次动作最终抵达的地址(跳转目标 / 落地页 URL / 点击后的目标) */
  targetUrl: string;
  /** 国家(地理占位,数据源待接入,当前恒为空字符串) */
  country: string;
  /** 是否数据中心出口(地理占位,当前恒为 false) */
  isDatacenter: boolean;
  /** 自治系统号(地理占位,当前恒为空字符串) */
  asn: string;
  /** Accept-Language 首标签，如 zh-CN（空表示未携带该请求头） */
  lang: string;
  /**
   * 本次访问真实命中的规则 ID（无规则参与时为 null）。
   *
   * 可空是因为规则被删后 ON DELETE SET NULL：历史明细保留，但不再指向任何规则。
   * 这是后端当时记下的事实，不是前端重算的结果。
   */
  ruleId: number | null;
  /** 本次访问真实执行的规则动作：pass / redirect / notfound / throttle（无规则参与时为空） */
  ruleAction: string;
  createdAt: string;
}

/** 分页响应 */
export interface PageResult<T> {
  items: T[];
  total: number;
}

/** 统一错误响应:{ code, message, details? } */
export interface ApiErrorBody {
  code: string;
  message: string;
  details?: unknown;
}

/** 配额超限错误详情(后端 403 E_DOMAIN_LIMIT / E_LINK_LIMIT 附 details.usage) */
export interface QuotaErrorDetails {
  usage?: QuotaUsage;
  field?: string;
}

/** 从统一错误 details 中提取配额用量(结构与 QuotaUsage 一致时返回) */
export function getQuotaUsage(details: unknown): QuotaUsage | undefined {
  if (!details || typeof details !== 'object') return undefined;
  const usage = (details as { usage?: unknown }).usage;
  if (!usage || typeof usage !== 'object') return undefined;
  const u = usage as Record<string, unknown>;
  if (
    typeof u.links === 'number' &&
    typeof u.domains === 'number' &&
    typeof u.maxLinks === 'number' &&
    typeof u.maxDomains === 'number'
  ) {
    return {
      links: u.links,
      domains: u.domains,
      maxLinks: u.maxLinks,
      maxDomains: u.maxDomains,
    };
  }
  return undefined;
}

/** 前端统一的 API 错误 */
export class ApiError extends Error {
  code: string;
  status: number;
  details?: unknown;

  constructor(status: number, code: string, message: string, details?: unknown) {
    super(message);
    this.name = 'ApiError';
    this.status = status;
    this.code = code;
    this.details = details;
  }
}
