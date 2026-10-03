/**
 * 表单回车语义。
 *
 * #4 DomainCreateView 双提交：`AppInput` 的 `@keydown.enter` 没有 preventDefault，
 *    与 `<form>` 的原生隐式提交撞车。form 内没有 submit 按钮，且恰好只有 1 个阻塞
 *    隐式提交的字段（AppInput 是 text；textarea 不阻塞）→ 浏览器在同一次回车里
 *    既派发 keydown.enter 又隐式提交，两条路径都调 onSubmit → 两次 POST /api/domains
 *    → 201 之后紧跟一个 409，误报「域名已被占用」。
 *
 * #5 AccountView 密码表单回车是死代码：表单内 3 个 type="password" 字段远超
 *    「恰好 1 个阻塞字段」的阈值，隐式提交不触发；submit 按钮又在 </AppForm> 之外的
 *    CardFooter 里。于是回车完全没有反应，只能点按钮。
 */
import { flushPromises, mount } from '@vue/test-utils';
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

vi.mock('@/utils/toast', () => ({ message: toastMock }));
vi.mock('@/api/domains', () => domainsApi);
vi.mock('@/api/auth', () => authApi);
vi.mock('@/api/me', () => meApi);

const routerMock = vi.hoisted(() => ({ push: vi.fn(), replace: vi.fn() }));
vi.mock('vue-router', async () => {
  const actual = await vi.importActual<typeof import('vue-router')>('vue-router');
  return { ...actual, useRouter: () => routerMock, useRoute: () => ({ params: {}, query: {} }) };
});

import DomainCreateView from '@/views/domains/DomainCreateView.vue';
import AccountView from '@/views/account/AccountView.vue';
import UI from '@/components/app';

function mountView(component: Parameters<typeof mount>[0], extraStubs: Record<string, unknown> = {}) {
  return mount(component, {
    global: {
      plugins: [createPinia(), UI],
      stubs: { RouterLink: { template: '<a><slot /></a>' }, AppTooltip: { template: '<div><slot /></div>' }, ...extraStubs },
    },
  });
}

beforeEach(() => {
  vi.clearAllMocks();
  setActivePinia(createPinia());
});

describe('DomainCreateView：一次回车恰好提交一次', () => {
  it('keydown.enter 与 <form> 隐式提交同时发生，也只打一次 POST /api/domains', async () => {
    domainsApi.createDomain.mockResolvedValue({ id: 1, fqdn: 'links.example.com' });
    const wrapper = mountView(DomainCreateView);

    const input = wrapper.get('input');
    await input.setValue('links.example.com');

    // 同一次回车的两条路径：AppInput 的 @press-enter，以及 <form> 的原生隐式提交。
    // 两者在浏览器里是同一个事件循环回合内先后发生的，所以这里也不能 await 隔开 ——
    // 隔开就变成「第一次请求已完成」的串行场景，测不到并发重入。
    void input.trigger('keydown.enter');
    void wrapper.get('form').trigger('submit');
    await flushPromises();

    expect(domainsApi.createDomain).toHaveBeenCalledTimes(1);
    wrapper.unmount();
  });

  it('回车时输入非法 → 一次请求都不发', async () => {
    const wrapper = mountView(DomainCreateView);

    const input = wrapper.get('input');
    await input.setValue('not_a_valid_fqdn!!');

    void input.trigger('keydown.enter');
    void wrapper.get('form').trigger('submit');
    await flushPromises();

    expect(domainsApi.createDomain).not.toHaveBeenCalled();
    wrapper.unmount();
  });
});

describe('AccountView：密码表单回车可提交', () => {
  async function fillPasswords(newPwd: string, confirmPwd: string) {
    authApi.fetchMe.mockResolvedValue({ status: 'active', usage: {} });
    authApi.changePassword.mockResolvedValue(undefined);
    const wrapper = mountView(AccountView, { ErrorPagesCard: { template: '<div />' } });
    await flushPromises();
    const inputs = wrapper.findAll('input');
    await inputs[0].setValue('old-password-1');
    await inputs[1].setValue(newPwd);
    await inputs[2].setValue(confirmPwd);
    return wrapper;
  }

  it('在确认密码框按回车能提交（此前 @finish 永不触发，回车完全没反应）', async () => {
    const wrapper = await fillPasswords('newpassword1', 'newpassword1');
    await wrapper.findAll('input')[2].trigger('keydown.enter');
    await flushPromises();
    expect(authApi.changePassword).toHaveBeenCalledTimes(1);
    wrapper.unmount();
  });

  it('回车时密码不合法 → 不调用 changePassword', async () => {
    const wrapper = await fillPasswords('short1', 'short1');
    await wrapper.findAll('input')[2].trigger('keydown.enter');
    await flushPromises();
    expect(authApi.changePassword).not.toHaveBeenCalled();
    wrapper.unmount();
  });
});