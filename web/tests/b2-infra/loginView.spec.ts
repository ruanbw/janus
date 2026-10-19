/**
 * 登录页契约:账号 + 密码(不再有邮箱验证 / 首登校验码)。
 *
 * 冻结接口契约:
 * - 请求体为 {username,password,rememberMe?};账号规则 3-32 位 [a-z0-9_-]
 * - 登录成功且 tenant.mustChangePassword 为真 → 跳 /account 强制改密
 * - 已删除邮箱验证/忘记密码/重置密码入口,登录页不得再暴露这些入口或字段
 */
import { flushPromises, mount } from '@vue/test-utils';
import { createPinia, setActivePinia } from 'pinia';

import { ApiError } from '@/types/api';
import type { Tenant } from '@/types/api';

const toastMock = vi.hoisted(() => ({
  error: vi.fn(),
  success: vi.fn(),
  info: vi.fn(),
  warning: vi.fn(),
}));
const authApi = vi.hoisted(() => ({
  login: vi.fn(),
  logout: vi.fn(),
  fetchMe: vi.fn(),
  register: vi.fn(),
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

import LoginView from '@/views/auth/LoginView.vue';
import UI from '@/components/app';

const USERNAME = 'janus_admin';
const PASSWORD = 'Passw0rd!';

function tenant(overrides: Partial<Tenant> = {}): Tenant {
  return {
    id: 1,
    username: USERNAME,
    slug: 'janus-admin',
    status: 'active',
    isSuperAdmin: false,
    mustChangePassword: false,
    tier: { id: 1, name: 'free', maxLinks: 10, maxDomains: 1 },
    defaultDomain: 'janus-admin.janus.test',
    createdAt: '2026-01-01T00:00:00Z',
    ...overrides,
  };
}

function mountLogin() {
  return mount(LoginView, {
    global: {
      plugins: [createPinia(), UI],
      stubs: { RouterLink: { template: '<a><slot /></a>' } },
    },
  });
}

type Wrapper = ReturnType<typeof mountLogin>;

/** 表单里可输入的文本框/密码框(排除「记住我」复选框) */
function editableInputs(wrapper: Wrapper) {
  return wrapper.findAll('input').filter((input) => input.attributes('type') !== 'checkbox');
}

async function fillAndSubmit(wrapper: Wrapper, username: string, password: string): Promise<void> {
  const inputs = editableInputs(wrapper);
  await inputs[0]!.setValue(username);
  await inputs[1]!.setValue(password);
  await wrapper.get('form').trigger('submit');
  await flushPromises();
}

beforeEach(() => {
  vi.clearAllMocks();
  setActivePinia(createPinia());
  meApi.fetchMyTenant.mockResolvedValue({});
  configApi.fetchConfig.mockResolvedValue({});
});

describe('LoginView:账号 + 密码', () => {
  it('提交请求体为 {username,password,rememberMe},不含邮箱与 setupToken', async () => {
    authApi.login.mockResolvedValue(tenant());
    meApi.fetchMyTenant.mockResolvedValue(tenant());
    const wrapper = mountLogin();

    await fillAndSubmit(wrapper, USERNAME, PASSWORD);

    expect(authApi.login).toHaveBeenCalledTimes(1);
    expect(authApi.login).toHaveBeenCalledWith({
      username: USERNAME,
      password: PASSWORD,
      rememberMe: true,
    });
    expect(routerMock.push).toHaveBeenCalledWith('/domains');

    wrapper.unmount();
  });

  it('页面不再暴露邮箱验证/忘记密码/首登校验码入口', () => {
    const wrapper = mountLogin();

    expect(wrapper.text()).not.toContain('忘记密码');
    expect(wrapper.text()).not.toContain('首次登录校验码');
    expect(wrapper.text()).not.toContain('邮箱');

    wrapper.unmount();
  });

  it('账号不符合 [a-z0-9_-]{3,32} → 不发起登录请求', async () => {
    const wrapper = mountLogin();

    await fillAndSubmit(wrapper, 'AB', PASSWORD); // 大写且不足 3 位

    expect(authApi.login).not.toHaveBeenCalled();

    wrapper.unmount();
  });

  it('mustChangePassword 为真 → 跳 /account 强制改密', async () => {
    const forced = tenant({ mustChangePassword: true });
    authApi.login.mockResolvedValue(forced);
    meApi.fetchMyTenant.mockResolvedValue(forced);
    const wrapper = mountLogin();

    await fillAndSubmit(wrapper, USERNAME, PASSWORD);

    expect(routerMock.push).toHaveBeenCalledWith('/account');

    wrapper.unmount();
  });

  it('普通「账号或密码错误」→ 内联提示错误且不跳转', async () => {
    authApi.login.mockRejectedValue(new ApiError(401, 'unauthorized', '账号或密码错误'));
    const wrapper = mountLogin();

    await fillAndSubmit(wrapper, USERNAME, PASSWORD);

    expect(routerMock.push).not.toHaveBeenCalled();
    expect(wrapper.text()).toContain('账号或密码错误');

    wrapper.unmount();
  });
});
