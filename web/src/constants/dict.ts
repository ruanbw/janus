// 状态字典:契约枚举 → 后台展示文案
import type {
  CertStatus,
  DomainOrigin,
  DomainStatus,
  LandingSource,
  LinkStatus,
  LinkType,
  RedirectStatus,
  TenantStatus,
} from '@/types/api';

export interface DictItem {
  label: string;
  /** AppTag 语义颜色 */
  color?: string;
}

export const DOMAIN_STATUS: Record<DomainStatus, DictItem> = {
  pending: { label: '待激活', color: 'warn' },
  active: { label: '已激活', color: 'ok' },
  failed: { label: '校验失败', color: 'err' },
  stopped: { label: '已停用', color: 'default' },
};

export const CERT_STATUS: Record<CertStatus, DictItem> = {
  pending: { label: '待签发', color: 'warn' },
  issued: { label: '已签发', color: 'ok' },
  failed: { label: '失败', color: 'err' },
};

export const DOMAIN_ORIGIN: Record<DomainOrigin, DictItem> = {
  self: { label: '自有域名', color: 'brand' },
  platform: { label: '平台默认域名', color: 'info' },
};

export const LINK_STATUS: Record<LinkStatus, DictItem> = {
  enabled: { label: '启用', color: 'ok' },
  disabled: { label: '停用', color: 'default' },
};

export const REDIRECT_STATUS: Record<RedirectStatus, DictItem> = {
  '302': { label: '临时重定向', color: 'info' },
  '301': { label: '永久重定向', color: 'warn' },
};

export const LINK_TYPE: Record<LinkType, DictItem> = {
  redirect: { label: '跳转', color: 'brand' },
  landing: { label: '落地页', color: 'info' },
};

export const LANDING_SOURCE: Record<LandingSource, DictItem> = {
  url: { label: 'URL 地址', color: 'info' },
  upload: { label: '上传压缩包', color: 'brand' },
};

export const TENANT_STATUS: Record<TenantStatus, DictItem> = {
  pending: { label: '待验证', color: 'warn' },
  active: { label: '正常', color: 'ok' },
  banned: { label: '已封禁', color: 'err' },
};

/** 短码字符集:去除易混淆字符 0/O/1/l/I(见 spec Further Notes) */
export const SHORT_CODE_ALPHABET = 'abcdefghijkmnopqrstuvwxyzABCDEFGHJKLMNPQRSTUVWXYZ23456789';

/** slug 规则:小写字母/数字开头结尾,可含连字符,长度 1-63(后端 IsValidSlug) */
export const SLUG_PATTERN = /^[a-z0-9](?:[a-z0-9-]*[a-z0-9])?$/;
export const SLUG_MAX_LENGTH = 63;

/** 自定义短码规则:仅允许字母与数字(易混淆字符由 FORBIDDEN 单独校验),长度 1-64(后端 MaxCodeLen=64) */
export const SHORT_CODE_PATTERN = /^[a-zA-Z0-9]+$/;
/** 短码中的易混淆字符(后端字符集不含 0/O/1/l/I) */
export const SHORT_CODE_FORBIDDEN_PATTERN = /[0O1lI]/;
export const SHORT_CODE_MAX_LENGTH = 64;
/** 自动生成短码的固定长度(后端 domain.AutoCodeLength) */
export const AUTO_SHORT_CODE_LENGTH = 6;
