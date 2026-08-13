// 状态字典:契约枚举 → 后台展示文案(遵循 CONTEXT.md 词汇表术语)
import type {
  CertStatus,
  DomainOrigin,
  DomainStatus,
  LinkStatus,
  RedirectStatus,
  TenantStatus,
} from '@/types/api';

export interface DictItem {
  label: string;
  /** antd Tag 颜色 */
  color?: string;
}

export const DOMAIN_STATUS: Record<DomainStatus, DictItem> = {
  pending: { label: '待激活', color: 'warning' },
  active: { label: '已激活', color: 'success' },
  failed: { label: '校验失败', color: 'error' },
  stopped: { label: '已停用', color: 'default' },
};

export const CERT_STATUS: Record<CertStatus, DictItem> = {
  pending: { label: '待签发', color: 'warning' },
  issued: { label: '已签发', color: 'success' },
  failed: { label: '失败', color: 'error' },
};

export const DOMAIN_ORIGIN: Record<DomainOrigin, DictItem> = {
  self: { label: '自有域名', color: 'blue' },
  platform: { label: '平台默认域名', color: 'purple' },
};

export const LINK_STATUS: Record<LinkStatus, DictItem> = {
  enabled: { label: '启用', color: 'success' },
  disabled: { label: '停用', color: 'default' },
};

export const REDIRECT_STATUS: Record<RedirectStatus, DictItem> = {
  '302': { label: '临时重定向', color: 'blue' },
  '301': { label: '永久重定向', color: 'orange' },
};

export const TENANT_STATUS: Record<TenantStatus, DictItem> = {
  pending: { label: '待验证', color: 'warning' },
  active: { label: '正常', color: 'success' },
  banned: { label: '已封禁', color: 'error' },
};

/** 短码字符集:去除易混淆字符 0/O/1/l/I(见 spec Further Notes) */
export const SHORT_CODE_ALPHABET = 'abcdefghijkmnopqrstuvwxyzABCDEFGHJKLMNPQRSTUVWXYZ23456789';

/** slug 规则:小写字母/数字开头结尾,可含连字符 */
export const SLUG_PATTERN = /^[a-z0-9](?:[a-z0-9-]*[a-z0-9])?$/;

/** 自定义短码规则:仅允许短码字符集内字符,长度 1-32 */
export const SHORT_CODE_PATTERN = /^[a-zA-Z0-9]+$/;
export const SHORT_CODE_MAX_LENGTH = 32;
