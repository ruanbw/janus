/**
 * 路由守卫:强制改密(mustChangePassword)拦截契约。
 *
 * 后端在 mustChangePassword 为真时,除 /auth/me、/auth/change-password、/auth/logout
 * 外一律 403(E_PASSWORD_CHANGE_REQUIRED)。前端必须把用户锁在 /account,
 * 避免进入一个只会报错的页面;已登录访问 /login 时也应回到 /account 而不是后台首页。
 */
import { createPinia, setActivePinia } from 'pinia';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import type { Tenant } from '@/types/api';

const fetchMeMock = vi.fn();
const fetchMyTenantMock = vi.fn();
const fetchConfigMock = vi.fn();

vi.mock('@/utils/toast', () => ({
  message: { error: vi.fn(), success: vi.fn(), warning: vi.fn(), info: vi.fn() },
}));

// 视图/布局只需可解析,守卫断言不关心它们渲染什么
vi.mock('@/layouts/AdminLayout.vue', () => ({ default: { render: () => null } }));
vi.mock('@/views/overview/OverviewView.vue', () => ({ default: { render: () => null } }));
vi.mock('@/views/account/AccountView.vue', () => ({ default: { render: () => null } }));
vi.mock('@/views/auth/LoginView.vue', () => ({ default: { render: () => null } }));
vi.mock('@/views/domains/DomainsView.vue', () => ({ default: { render: () => null } }));

vi.mock('@/api/auth', () => ({
  fetchMe: () => fetchMeMock(),
  login: vi.fn(),
  logout: vi.fn(),
}));
vi.mock('@/api/me', () => ({ fetchMyTenant: () => fetchMyTenantMock() }));
vi.mock('@/api/config', () => ({ fetchConfig: () => fetchConfigMock() }));

const { default: router } = await import('@/router');

function tenant(mustChangePassword: boolean): Tenant {
  return {
    id: 1,
    username: 'janus_admin',
    slug: 'janus-admin',
    status: 'active',
    isSuperAdmin: false,
    mustChangePassword,
    tier: { id: 1, name: 'free', maxLinks: 10, maxDomains: 1 },
    defaultDomain: 'janus-admin.janus.test',
    createdAt: '2026-01-01T00:00:00Z',
  };
}

/** 让本次会话探测返回给定租户(fetchMe 与 /api/me 同构) */
function authenticatedAs(mustChangePassword: boolean): void {
  const me = tenant(mustChangePassword);
  fetchMeMock.mockResolvedValue(me);
  fetchMyTenantMock.mockResolvedValue(me);
}

describe('路由守卫:强制改密只放行 /account', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    setActivePinia(createPinia());
    fetchConfigMock.mockResolvedValue({});
  });

  it('mustChangePassword 为真时访问 /overview → 重定向 /account', async () => {
    authenticatedAs(true);

    await router.push('/overview');

    expect(router.currentRoute.value.path).toBe('/account');
  });

  it('mustChangePassword 为真时 /account 本身放行', async () => {
    authenticatedAs(true);

    await router.push('/account');

    expect(router.currentRoute.value.path).toBe('/account');
  });

  it('mustChangePassword 为假时 /overview 正常放行', async () => {
    authenticatedAs(false);

    await router.push('/overview');

    expect(router.currentRoute.value.path).toBe('/overview');
  });

  it('mustChangePassword 为真且已登录时访问 /login → 回 /account 而不是后台首页', async () => {
    authenticatedAs(true);

    await router.push('/login');

    expect(router.currentRoute.value.path).toBe('/account');
  });
});
