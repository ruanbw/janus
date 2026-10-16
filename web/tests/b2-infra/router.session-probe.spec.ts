/**
 * 路由守卫对「会话探测失败」的处理契约。
 *
 * 背景：fetchMe 只把 401 当作未登录，其余错误置 sessionProbeFailed。
 * 守卫必须据此区分「未登录」(跳登录页)与「登录态未知」(放行,不踢人)。
 */
import { createPinia, setActivePinia } from 'pinia';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import { ApiError } from '@/types/api';

const fetchMeMock = vi.fn();

vi.mock('@/utils/toast', () => ({
  message: { error: vi.fn(), success: vi.fn(), warning: vi.fn(), info: vi.fn() },
}));

// 视图/布局只需可解析,守卫断言不关心它们渲染什么
vi.mock('@/layouts/AdminLayout.vue', () => ({ default: { render: () => null } }));
vi.mock('@/views/overview/OverviewView.vue', () => ({ default: { render: () => null } }));
vi.mock('@/views/account/AccountView.vue', () => ({ default: { render: () => null } }));
vi.mock('@/views/auth/LoginView.vue', () => ({ default: { render: () => null } }));

vi.mock('@/api/auth', () => ({
  fetchMe: () => fetchMeMock(),
  login: vi.fn(),
  logout: vi.fn(),
}));
vi.mock('@/api/me', () => ({ fetchMyTenant: vi.fn() }));
vi.mock('@/api/config', () => ({ fetchConfig: vi.fn() }));

const { default: router } = await import('@/router');

describe('路由守卫:会话探测失败不跳登录页', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    setActivePinia(createPinia());
  });

  it('非 401 探测失败:放行原路由,不重定向到登录页', async () => {
    fetchMeMock.mockRejectedValue(new ApiError(0, 'E_NETWORK', '无法连接后台服务'));

    await router.push('/overview');

    expect(router.currentRoute.value.path).toBe('/overview');
    expect(router.currentRoute.value.query.redirect).toBeUndefined();
  });

  it('401:仍重定向到登录页并保留 redirect', async () => {
    fetchMeMock.mockRejectedValue(new ApiError(401, 'E_UNAUTH', '未登录'));

    // 用不同路径,避免与上一个用例的当前位置相同导致导航被判为重复
    await router.push('/account');

    expect(router.currentRoute.value.path).toBe('/login');
    expect(router.currentRoute.value.query.redirect).toBe('/account');
  });
});
