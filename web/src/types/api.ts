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
  createdAt: string;
}

/** 访问记录 */
export interface Visit {
  id: number;
  linkId: number;
  domain: string;
  /** 访问者 IP(X-Forwarded-For 优先,回退 RemoteAddr) */
  ip: string;
  userAgent: string;
  referer: string;
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
