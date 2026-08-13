// 平台管理(后台,仅平台管理员;非超管 403)
import { del, get, patch } from '@/utils/request';

import type { Tenant, TenantStatus } from '@/types/api';

/** 租户列表(含用量) */
export function listTenants(): Promise<Tenant[]> {
  return get<Tenant[]>('/admin/tenants');
}

/** 租户详情 */
export function getTenant(id: number): Promise<Tenant> {
  return get<Tenant>(`/admin/tenants/${id}`);
}

/** 封禁/解封、调整等级 */
export function updateTenant(
  id: number,
  data: { status?: TenantStatus; tierId?: number },
): Promise<Tenant> {
  return patch<Tenant>(`/admin/tenants/${id}`, data);
}

/** 平台强删违规域名(解除其短链关联) */
export function removeDomain(id: number): Promise<void> {
  return del<void>(`/admin/domains/${id}`);
}
