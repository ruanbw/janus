// API Key(后台,会话鉴权)
import { del, get, post } from '@/utils/request';

import type { ApiKey } from '@/types/api';

/** API Key 列表(不含明文) */
export function listApiKeys(): Promise<ApiKey[]> {
  return get<ApiKey[]>('/api-keys');
}

/** 生成 API Key:响应含明文 key,仅此一次 */
export function createApiKey(data: { name: string }): Promise<ApiKey> {
  return post<ApiKey>('/api-keys', data);
}

/** 吊销 API Key */
export function deleteApiKey(id: number): Promise<void> {
  return del<void>(`/api-keys/${id}`);
}
