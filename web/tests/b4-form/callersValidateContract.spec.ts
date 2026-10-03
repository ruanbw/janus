/**
 * 四个调用点的 validate() 契约回归网 —— 防复发的真正防线。
 *
 * `AppForm.validate()` resolve boolean、永不 reject。四个视图此前写成
 * `try { await validate() } catch { return }`：catch 是死代码，校验失败被完全忽略，
 * 请求照发（表单非法时仍打后端，表现为 400/409 之类的误报）。
 *
 * 每个用例都断言同一个契约：**输入非法 → 对应 API 一个都不能被调用**。
 * 全部 src/api/* 均被 mock，不打真网络。
 */
import { flushPromises, mount } from '@vue/test-utils';
import { nextTick } from 'vue';
import { createPinia, setActivePinia } from 'pinia';

const toastMock = vi.hoisted(() => ({
  error: vi.fn(),
  success: vi.fn(),
  info: vi.fn(),
  warning: vi.fn(),
}));
const domainsApi = vi.hoisted(() => ({ createDomain: vi.fn() }));
const authApi = vi.hoisted(() => ({ changePassword: vi.fn(), fetchMe: vi.fn() }));
const meApi = vi.hoisted(() => ({ fetchMyTenant: vi.fn() }));
const adminApi = vi.hoisted(() => ({ getTenant: vi.fn(), removeDomain: vi.fn() }));
const linksApi = vi.hoisted(() => ({
  createLink: vi.fn(),
  updateLink: vi.fn(),
  getLink: vi.fn(),
  listDomains: vi.fn(),
  uploadLanding: vi.fn(),
  listLinks: vi.fn(),
}));

vi.mock('@/utils/toast', () => ({ message: toastMock }));
// 确认框直接放行 true：否则「强删」类用例会卡在二次确认上而假绿，
// 测不到「校验被跳过 → 破坏性操作照常执行」这个真正的 bug。
vi.mock('@/components/app/confirm', () => ({
  confirm: vi.fn(),
  confirmAsync: vi.fn(async () => true),
  closeConfirm: vi.fn(),
}));
vi.mock('@/api/domains', () => domainsApi);
vi.mock('@/api/auth', () => authApi);
vi.mock('@/api/me', () => meApi);
vi.mock('@/api/admin', () => adminApi);
vi.mock('@/api/links', () => linksApi);

const routerMock = vi.hoisted(() => ({
  push: vi.fn(),
  replace: vi.fn(),
  currentRoute: { value: { params: { id: '7' }, query: {} } },
}));
vi.mock('vue-router', async () => {
  const actual = await vi.importActual<typeof import('vue-router')>('vue-router');
  return {
    ...actual,
    useRouter: () => routerMock,
    useRoute: () => routerMock.currentRoute.value,
  };
});

import DomainCreateView from '@/views/domains/DomainCreateView.vue';
import AccountView from '@/views/account/AccountView.vue';
import AdminTenantRemoveDomainView from '@/views/admin/AdminTenantRemoveDomainView.vue';
import LinkFormView from '@/views/links/LinkFormView.vue';
// 与 main.ts 一致：全局注册 App*/Card* 组件，否则视图模板里的组件解析不了
import UI from '@/components/app';

/** 只额外 stub 掉与断言无关的重组件 */
function mountView(component: Parameters<typeof mount>[0], extraStubs: Record<string, unknown> = {}) {
  return mount(component, {
    global: {
      plugins: [createPinia(), UI],
      stubs: {
        RouterLink: { template: '<a><slot /></a>' },
        AppTooltip: { template: '<div><slot /></div>' },
        ...extraStubs,
      },
    },
  });
}

/**
 * 点主操作按钮 —— 这才是真实用户路径，也是 bug 的入口：
 * 按钮的 @click 直接调视图的 onSubmit，完全绕过 AppForm 内部「校验不过就不 emit finish」
 * 的闸门。（若改走 form 的原生 submit 事件，那条路径 AppForm 已经拦住了，测不出本 bug。）
 */
async function clickPrimaryButton(wrapper: ReturnType<typeof mountView>, label: RegExp): Promise<void> {
  const btn = wrapper.findAll('button').find((b) => label.test(b.text()));
  if (!btn) throw new Error(`未找到按钮 ${label}`);
  await btn.trigger('click');
  await flushPromises();
}

beforeEach(() => {
  vi.clearAllMocks();
  setActivePinia(createPinia());
  toastMock.error.mockReset();
});

describe('DomainCreateView：域名非法时不得发出 POST /api/domains', () => {
  it('FQDN 为空 → createDomain 未被调用', async () => {
    const wrapper = mountView(DomainCreateView);
    await clickPrimaryButton(wrapper, /立即添加域名/);
    expect(domainsApi.createDomain).not.toHaveBeenCalled();
    wrapper.unmount();
  });

  it('FQDN 格式非法（不以点分标签结尾）→ createDomain 未被调用', async () => {
    const wrapper = mountView(DomainCreateView);
    await wrapper.get('input').setValue('not_a_valid_fqdn!!');
    await clickPrimaryButton(wrapper, /立即添加域名/);
    expect(domainsApi.createDomain).not.toHaveBeenCalled();
    wrapper.unmount();
  });

  it('输入合法时 createDomain 被调用一次（对照组，确保断言不是因为根本没走提交路径）', async () => {
    domainsApi.createDomain.mockResolvedValue({ id: 1, fqdn: 'links.example.com' });
    const wrapper = mountView(DomainCreateView);
    await wrapper.get('input').setValue('links.example.com');
    await clickPrimaryButton(wrapper, /立即添加域名/);
    expect(domainsApi.createDomain).toHaveBeenCalledTimes(1);
    wrapper.unmount();
  });
});

describe('AccountView：密码不合法时不得调用 changePassword', () => {
  it('新密码短于 8 位 → changePassword 未被调用', async () => {
    authApi.fetchMe.mockResolvedValue({ status: 'active', usage: {} });
    const wrapper = mountView(AccountView, { ErrorPagesCard: { template: '<div />' } });
    await flushPromises();

    const inputs = wrapper.findAll('input');
    await inputs[0].setValue('old-password-1');
    await inputs[1].setValue('short1'); // 含字母数字但不足 8 位
    await inputs[2].setValue('short1');
    await clickPrimaryButton(wrapper, /确认修改密码/);

    expect(authApi.changePassword).not.toHaveBeenCalled();
    wrapper.unmount();
  });

  it('两次输入的新密码不一致 → changePassword 未被调用', async () => {
    authApi.fetchMe.mockResolvedValue({ status: 'active', usage: {} });
    const wrapper = mountView(AccountView, { ErrorPagesCard: { template: '<div />' } });
    await flushPromises();

    const inputs = wrapper.findAll('input');
    await inputs[0].setValue('old-password-1');
    await inputs[1].setValue('newpassword1');
    await inputs[2].setValue('different1');
    await clickPrimaryButton(wrapper, /确认修改密码/);

    expect(authApi.changePassword).not.toHaveBeenCalled();
    wrapper.unmount();
  });

  it('输入合法时 changePassword 被调用一次（对照组）', async () => {
    authApi.fetchMe.mockResolvedValue({ status: 'active', usage: {} });
    authApi.changePassword.mockResolvedValue(undefined);
    const wrapper = mountView(AccountView, { ErrorPagesCard: { template: '<div />' } });
    await flushPromises();

    const inputs = wrapper.findAll('input');
    await inputs[0].setValue('old-password-1');
    await inputs[1].setValue('newpassword1');
    await inputs[2].setValue('newpassword1');
    await clickPrimaryButton(wrapper, /确认修改密码/);

    expect(authApi.changePassword).toHaveBeenCalledTimes(1);
    wrapper.unmount();
  });
});

describe('AdminTenantRemoveDomainView：域名 ID 非法时不得调用 removeDomain', () => {
  it('域名 ID 为 0（小于最小值 1）→ 二次确认框不弹、removeDomain 未被调用', async () => {
    adminApi.getTenant.mockResolvedValue({ id: 7, email: 'a@b.c', slug: 'a', tier: { name: 'free' } });
    const wrapper = mountView(AdminTenantRemoveDomainView);
    await flushPromises();

    // 直接把 model 置成非法值，绕过 AppInputNumber 的 UI 交互；随后等一拍让按钮的 disabled 重算
    const vm = wrapper.vm as unknown as { form: { domainId: number } };
    vm.form.domainId = 0;
    await nextTick();
    await clickPrimaryButton(wrapper, /强制移除违规域名/);

    expect(adminApi.removeDomain).not.toHaveBeenCalled();
    wrapper.unmount();
  });
});

describe('LinkFormView：目标 URL 非法时不得调用 createLink', () => {
  it('目标 URL 为空 → createLink 未被调用', async () => {
    linksApi.listDomains.mockResolvedValue([]);
    const wrapper = mountView(LinkFormView);
    await flushPromises();
    await clickPrimaryButton(wrapper, /创建短链/);
    expect(linksApi.createLink).not.toHaveBeenCalled();
    wrapper.unmount();
  });
});