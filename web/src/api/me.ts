// 租户设置(后台,会话鉴权)
import { get, patch } from '@/utils/request';

import type { Tenant, TenantErrorPages, TenantErrorPagesUpdatePayload } from '@/types/api';

/** 当前租户信息(含配额用量 usage) */
export function fetchMyTenant(): Promise<Tenant> {
  return get<Tenant>('/me');
}

/** 更新自动生成短码长度 */
export function updateMyTenant(data: { codeLength: number }): Promise<Tenant> {
  return patch<Tenant>('/me', data);
}

/** 获取租户全局错误页面配置 */
export function fetchTenantErrorPages(): Promise<TenantErrorPages> {
  return get<TenantErrorPages>('/me/error-pages');
}

/** 更新租户全局错误页面配置 */
export function updateTenantErrorPages(data: TenantErrorPagesUpdatePayload): Promise<TenantErrorPages> {
  return patch<TenantErrorPages>('/me/error-pages', data);
}

