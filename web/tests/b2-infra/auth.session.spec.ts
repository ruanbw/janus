/**
 * 会话探测的错误分类契约。
 *
 * 背景：fetchMe 原先裸 catch，把超时/断网/5xx 与 401 一视同仁地当成「未登录」，
 * 清空 tenant；配合路由守卫，网络抖动一次就能把持有效会话的用户踢到登录页。
 * 现在只有 401（服务端明确否认）才清空，其余错误保持原状态并置 sessionProbeFailed。
 */
import { createPinia, setActivePinia } from 'pinia';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import { ApiError } from '@/types/api';
import type { Tenant } from '@/types/api';

const fetchMeMock = vi.fn();
const loginMock = vi.fn();
const logoutMock = vi.fn();
const fetchMyTenantMock = vi.fn();
const fetchConfigMock = vi.fn();

vi.mock('@/api/auth', () => ({
  fetchMe: () => fetchMeMock(),
  login: (...args: unknown[]) => loginMock(...args),
  logout: () => logoutMock(),
}));
vi.mock('@/api/me', () => ({
  fetchMyTenant: () => fetchMyTenantMock(),
}));
vi.mock('@/api/config', () => ({
  fetchConfig: () => fetchConfigMock(),
}));

const { useAuthStore } = await import('@/stores/auth');

const TENANT = {
  id: 1,
  email: 'a@example.com',
  slug: 'a',
  status: 'active',
  isSuperAdmin: false,
  tier: 'free',
  defaultDomain: 'a.platform.test',
  createdAt: '2026-01-01T00:00:00Z',
} as unknown as Tenant;

describe('auth store 会话探测', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    setActivePinia(createPinia());
  });

  it('网络抖动(非 401)不清空会话,置 sessionProbeFailed 且返回现有 tenant', async () => {
    loginMock.mockResolvedValue(TENANT);
    fetchMyTenantMock.mockResolvedValue(TENANT);

    const store = useAuthStore();
    await store.login('a@example.com', 'pw');
    expect(store.isAuthenticated).toBe(true);

    fetchMeMock.mockRejectedValue(new ApiError(0, 'E_NETWORK', '无法连接后台服务'));

    const result = await store.fetchMe();

    expect(store.tenant).toEqual(TENANT);
    expect(store.isAuthenticated).toBe(true);
    expect(store.sessionProbeFailed).toBe(true);
    expect(result).toEqual(TENANT);
  });

  it('超时(ECONNABORTED → status 0)同样不清空会话', async () => {
    loginMock.mockResolvedValue(TENANT);
    fetchMyTenantMock.mockResolvedValue(TENANT);

    const store = useAuthStore();
    await store.login('a@example.com', 'pw');

    fetchMeMock.mockRejectedValue(new ApiError(0, 'E_TIMEOUT', '请求超时,请稍后重试'));

    await store.fetchMe();

    expect(store.tenant).toEqual(TENANT);
    expect(store.sessionProbeFailed).toBe(true);
  });

  it('5xx 不清空会话', async () => {
    loginMock.mockResolvedValue(TENANT);
    fetchMyTenantMock.mockResolvedValue(TENANT);

    const store = useAuthStore();
    await store.login('a@example.com', 'pw');

    fetchMeMock.mockRejectedValue(new ApiError(500, 'E_UNKNOWN', '请求失败(HTTP 500)'));

    await store.fetchMe();

    expect(store.tenant).toEqual(TENANT);
    expect(store.sessionProbeFailed).toBe(true);
  });

  it('401 清空会话并复位探测失败标记', async () => {
    loginMock.mockResolvedValue(TENANT);
    fetchMyTenantMock.mockResolvedValue(TENANT);

    const store = useAuthStore();
    await store.login('a@example.com', 'pw');

    fetchMeMock.mockRejectedValue(new ApiError(401, 'E_UNAUTH', '未登录'));

    const result = await store.fetchMe();

    expect(store.tenant).toBeNull();
    expect(store.isAuthenticated).toBe(false);
    expect(store.sessionProbeFailed).toBe(false);
    expect(result).toBeNull();
  });

  it('探测成功后清掉先前的探测失败标记', async () => {
    const store = useAuthStore();
    fetchMeMock.mockRejectedValueOnce(new ApiError(0, 'E_NETWORK', '网络错误'));
    await store.fetchMe();
    expect(store.sessionProbeFailed).toBe(true);

    fetchMeMock.mockResolvedValue(TENANT);
    fetchMyTenantMock.mockResolvedValue(TENANT);
    await store.fetchMe();

    expect(store.sessionProbeFailed).toBe(false);
    expect(store.tenant).toEqual(TENANT);
  });

  it('登出/clear 复位探测失败标记', async () => {
    const store = useAuthStore();
    fetchMeMock.mockRejectedValue(new ApiError(0, 'E_NETWORK', '网络错误'));
    await store.fetchMe();
    expect(store.sessionProbeFailed).toBe(true);

    logoutMock.mockResolvedValue(undefined);
    await store.logout();
    expect(store.sessionProbeFailed).toBe(false);
  });
});
