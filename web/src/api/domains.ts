// 域名 API(后台,会话鉴权)
import { del, get, patch, post } from '@/utils/request';

import type { Domain, DomainStatus } from '@/types/api';

/** 域名列表(含平台默认域名) */
export function listDomains(): Promise<Domain[]> {
  return get<Domain[]>('/domains');
}

/** 添加自有域名(403 域名配额超限;409 已被占用;400 非法/平台保留/描述过长) */
export function createDomain(data: { fqdn: string; description?: string }): Promise<Domain> {
  return post<Domain>('/domains', data);
}

/** 手动重新 DNS 校验(202) */
export function recheckDomain(id: number): Promise<void> {
  return post<void>(`/domains/${id}/recheck`);
}

/** 停用/恢复(status: stopped | active) */
export function updateDomainStatus(id: number, status: DomainStatus): Promise<Domain> {
  return patch<Domain>(`/domains/${id}`, { status });
}

/** 删除自有域名(存在未删除短链 → 409;平台默认域名 → 400) */
export function deleteDomain(id: number): Promise<void> {
  return del<void>(`/domains/${id}`);
}
