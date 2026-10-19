/**
 * 注册页契约:账号 + 密码 + 默认域名前缀(slug),注册即生效(不再有验证邮件)。
 *
 * 冻结接口契约:
 * - 请求体为 {username,password,slug};账号规则 3-32 位 [a-z0-9_-],slug 沿用既有规则
 * - 成功后直接进后台;若返回体 mustChangePassword 为真则先跳 /account
 * - 不再有邮箱字段、验证邮件文案与 /verify-email 跳转
 */
import { flushPromises, mount } from '@vue/test-utils';
import { createPinia, setActivePinia } from 'pinia';

import { useAuthStore } from '@/stores/auth';
import { ApiError } from '@/types/api';
import type { Tenant } from '@/types/api';

const toastMock = vi.hoisted(() => ({
  error: vi.fn(),
  success: vi.fn(),
  info: vi.fn(),
  warning: vi.fn(),
}));
const authApi = vi.hoisted(() => ({
  register: vi.fn(),
  login: vi.fn(),
  logout: vi.fn(),
  fetchMe: vi.fn(),
  changePassword: vi.fn(),
}));
const meApi = vi.hoisted(() => ({ fetchMyTenant: vi.fn() }));
const configApi = vi.hoisted(() => ({ fetchConfig: vi.fn() }));

vi.mock('@/utils/toast', () => ({ message: toastMock }));
vi.mock('@/api/auth', () => authApi);
vi.mock('@/api/me', () => meApi);
vi.mock('@/api/config', () => configApi);

const routerMock = vi.hoisted(() => ({ push: vi.fn(), replace: vi.fn() }));
vi.mock('vue-router', async () => {
  const actual = await vi.importActual<typeof import('vue-router')>('vue-router');
  return { ...actual, useRouter: () => routerMock, useRoute: () => ({ query: {} }) };
});

import RegisterView from '@/views/auth/RegisterView.vue';
import UI from '@/components/app';

const USERNAME = 'my_brand-1';
const PASSWORD = 'Passw0rd!';
const SLUG = 'mybrand';

function tenant(overrides: Partial<Tenant> = {}): Tenant {
  return {
    id: 2,
    username: USERNAME,
    slug: SLUG,
    status: 'active',
    isSuperAdmin: false,
    mustChangePassword: false,
    tier: { id: 1, name: 'free', maxLinks: 10, maxDomains: 1 },
    defaultDomain: `${SLUG}.janus.test`,
    createdAt: '2026-01-01T00:00:00Z',
    ...overrides,
  };
}

function mountRegister() {
  return mount(RegisterView, {
    global: {
      plugins: [createPinia(), UI],
      stubs: { RouterLink: { template: '<a><slot /></a>' } },
    },
  });
}

type Wrapper = ReturnType<typeof mountRegister>;

async function fill(
  wrapper: Wrapper,
  values: { username: string; password: string; confirmPassword: string; slug: string },
): Promise<void> {
  const [username, password, confirmPassword, slug] = wrapper.findAll('input');
  await username!.setValue(values.username);
  await password!.setValue(values.password);
  await confirmPassword!.setValue(values.confirmPassword);
  await slug!.setValue(values.slug);
}

async function submit(wrapper: Wrapper): Promise<void> {
  await wrapper.get('form').trigger('submit');
  await flushPromises();
}

beforeEach(() => {
  vi.clearAllMocks();
  setActivePinia(createPinia());
});

describe('RegisterView:账号 + 密码 + 域名前缀', () => {
  it('提交请求体为 {username,password,slug},成功后写入会话并进后台', async () => {
    authApi.register.mockResolvedValue(tenant());
    const wrapper = mountRegister();

    await fill(wrapper, { username: USERNAME, password: PASSWORD, confirmPassword: PASSWORD, slug: SLUG });
    await submit(wrapper);

    expect(authApi.register).toHaveBeenCalledWith({ username: USERNAME, password: PASSWORD, slug: SLUG });
    expect(routerMock.push).toHaveBeenCalledWith('/domains');
    expect(useAuthStore().isAuthenticated).toBe(true);

    wrapper.unmount();
  });

  it('返回体 mustChangePassword 为真 → 跳 /account', async () => {
    authApi.register.mockResolvedValue(tenant({ mustChangePassword: true }));
    const wrapper = mountRegister();

    await fill(wrapper, { username: USERNAME, password: PASSWORD, confirmPassword: PASSWORD, slug: SLUG });
    await submit(wrapper);

    expect(routerMock.push).toHaveBeenCalledWith('/account');

    wrapper.unmount();
  });

  it('账号不合法 → 不发起注册请求', async () => {
    const wrapper = mountRegister();

    await fill(wrapper, { username: 'AB', password: PASSWORD, confirmPassword: PASSWORD, slug: SLUG });
    await submit(wrapper);

    expect(authApi.register).not.toHaveBeenCalled();

    wrapper.unmount();
  });

  it('两次密码不一致 → 不发起注册请求', async () => {
    const wrapper = mountRegister();

    await fill(wrapper, { username: USERNAME, password: PASSWORD, confirmPassword: 'Different1', slug: SLUG });
    await submit(wrapper);

    expect(authApi.register).not.toHaveBeenCalled();

    wrapper.unmount();
  });

  it('slug 不合法 → 不发起注册请求', async () => {
    const wrapper = mountRegister();

    await fill(wrapper, { username: USERNAME, password: PASSWORD, confirmPassword: PASSWORD, slug: '-bad-' });
    await submit(wrapper);

    expect(authApi.register).not.toHaveBeenCalled();

    wrapper.unmount();
  });

  it('页面不再有邮箱字段与验证邮件文案', () => {
    const wrapper = mountRegister();

    expect(wrapper.findAll('input[autocomplete="email"]')).toHaveLength(0);
    expect(wrapper.text()).not.toContain('验证邮件');
    expect(wrapper.text()).not.toContain('邮箱');

    wrapper.unmount();
  });

  it('注册冲突(409)→ 提示被占用且不跳转', async () => {
    authApi.register.mockRejectedValue(new ApiError(409, 'E_CONFLICT', '账号或 slug 已被占用'));
    const wrapper = mountRegister();

    await fill(wrapper, { username: USERNAME, password: PASSWORD, confirmPassword: PASSWORD, slug: SLUG });
    await submit(wrapper);

    expect(routerMock.push).not.toHaveBeenCalled();
    expect([...toastMock.error.mock.calls].flat().join(' | ')).toContain('已被占用');

    wrapper.unmount();
  });
});
