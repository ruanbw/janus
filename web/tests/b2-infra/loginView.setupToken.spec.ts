/**
 * 超管首次登录(FirstLoginSetup)的登录页契约。
 *
 * 背景:后端对 firstLoginSetup 的租户要求登录请求携带一次性 setup token
 * (internal/httpapi/auth.go `verifyCredentials` → `consumeSetupToken`),
 * 但前端 `login()` 从不发送 setupToken,登录页也没有该输入框。
 * 结果是:设了 `JANUS_SUPERADMIN_EMAIL` 的部署,管理员只能用 curl 完成首登,
 * 浏览器侧第一次登录必然 401 —— 而 LoginView 里「成功后 firstLoginSetup → /account」
 * 的分支(为旧的免密首登写的)成了永远走不到的死代码。
 *
 * 契约(Given-When-Then):
 * 1. Given 后端 401 且文案指明需要 setup token,
 *    When 提交登录,Then 页面出现「首次登录校验码」输入框,并在 toast 里提示校验码(不是普通密码错误)。
 * 2. Given 校验码输入框已出现但为空,When 再次提交,
 *    Then 不发请求(登录有账号级退避,空提交会白吃限流额度),并提示校验码必填。
 * 3. Given 已填写校验码,When 提交,Then login 请求带 setupToken,成功后按
 *    firstLoginSetup 跳 /account(引导设初始密码)。
 * 4. Given 普通「邮箱或密码错误」401,When 提交,Then 不出现校验码输入框(常态登录不受影响)。
 *
 * 字段契约:校验码表单项标签为「首次登录校验码」,是表单里第 3 个非复选框输入框。
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
  resendVerification: vi.fn(),
  verifyEmail: vi.fn(),
  changePassword: vi.fn(),
  forgotPassword: vi.fn(),
  resetPassword: vi.fn(),
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

/** 后端在首登缺 token 时返回的原文(见 internal/httpapi/auth.go consumeSetupToken) */
const SETUP_REQUIRED_MSG =
  '超管首次登录需要邮箱中的一次性 setup token(已重新发送到 admin@example.com)';
const EMAIL = 'admin@example.com';
const PASSWORD = 'Passw0rd!';

const firstLoginTenant = {
  id: 1,
  email: EMAIL,
  status: 'active',
  firstLoginSetup: true,
} as unknown as Tenant;

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

/** 所有 toast 文案(不区分级别:实现可以用 info 也可以用 warning) */
function toastText(): string {
  return [...toastMock.info.mock.calls, ...toastMock.warning.mock.calls, ...toastMock.error.mock.calls]
    .flat()
    .filter((v): v is string => typeof v === 'string')
    .join(' | ');
}

async function fillCredentials(wrapper: Wrapper): Promise<void> {
  const inputs = editableInputs(wrapper);
  await inputs[0]!.setValue(EMAIL);
  await inputs[1]!.setValue(PASSWORD);
}

async function submit(wrapper: Wrapper): Promise<void> {
  await wrapper.get('form').trigger('submit');
  await flushPromises();
}

/** 场景 1 的收尾:把页面推到「已出现校验码输入框」的状态 */
async function reachSetupTokenStep(wrapper: Wrapper): Promise<void> {
  authApi.login.mockRejectedValueOnce(new ApiError(401, 'unauthorized', SETUP_REQUIRED_MSG));
  await fillCredentials(wrapper);
  await submit(wrapper);
}

beforeEach(() => {
  vi.clearAllMocks();
  setActivePinia(createPinia());
  meApi.fetchMyTenant.mockResolvedValue({});
  configApi.fetchConfig.mockResolvedValue({});
});

describe('LoginView:超管首次登录需要校验码', () => {
  it('后端要求 setup token 时,页面出现「首次登录校验码」输入框并提示去查收', async () => {
    const wrapper = mountLogin();
    await reachSetupTokenStep(wrapper);

    // 第一次提交确实发出去了(用户填的是邮箱+密码)
    expect(authApi.login).toHaveBeenCalledTimes(1);
    // 校验码输入框出现:邮箱、密码之外多出第 3 个输入框
    expect(editableInputs(wrapper)).toHaveLength(3);
    expect(wrapper.text()).toContain('首次登录校验码');
    // 提示落在 toast 上,而不是把这条 401 当成「密码错误」
    expect(toastText()).toContain('校验码');

    wrapper.unmount();
  });

  it('校验码为空时再次提交 → 不发请求,并提示必填', async () => {
    const wrapper = mountLogin();
    await reachSetupTokenStep(wrapper);

    await submit(wrapper);

    expect(authApi.login).toHaveBeenCalledTimes(1);
    expect(toastText()).toContain('校验码');

    wrapper.unmount();
  });

  it('填写校验码后提交 → 请求带 setupToken,成功后跳 /account 引导设密码', async () => {
    const wrapper = mountLogin();
    await reachSetupTokenStep(wrapper);

    authApi.login.mockResolvedValueOnce(firstLoginTenant);
    // 真实链路:store.login 成功后会用 GET /api/me 回填 tenant,而 /api/me 返回的是
    // 同一个 Tenant 结构(含 firstLoginSetup),所以这里桩它也不能丢该标志位
    meApi.fetchMyTenant.mockResolvedValueOnce(firstLoginTenant);

    const inputs = editableInputs(wrapper);
    await inputs[2]!.setValue('setup-token-from-mail');
    await submit(wrapper);

    expect(authApi.login).toHaveBeenLastCalledWith({
      email: EMAIL,
      password: PASSWORD,
      rememberMe: true,
      setupToken: 'setup-token-from-mail',
    });
    expect(routerMock.push).toHaveBeenCalledWith('/account');

    wrapper.unmount();
  });

  it('普通「邮箱或密码错误」→ 不暴露校验码输入框', async () => {
    authApi.login.mockRejectedValueOnce(new ApiError(401, 'unauthorized', '邮箱或密码错误'));
    const wrapper = mountLogin();

    await fillCredentials(wrapper);
    await submit(wrapper);

    expect(editableInputs(wrapper)).toHaveLength(2);
    expect(wrapper.text()).not.toContain('首次登录校验码');

    wrapper.unmount();
  });
});
