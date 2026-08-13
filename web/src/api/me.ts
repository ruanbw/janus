// 租户设置(后台,会话鉴权)
import { get, patch } from '@/utils/request';

import type { Tenant } from '@/types/api';

/** 当前租户信息(含配额用量 usage) */
export function fetchMyTenant(): Promise<Tenant> {
  return get<Tenant>('/me');
}

/** 更新自动生成短码长度 */
export function updateMyTenant(data: { codeLength: number }): Promise<Tenant> {
  return patch<Tenant>('/me', data);
}
