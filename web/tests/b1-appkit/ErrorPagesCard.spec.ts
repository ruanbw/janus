/**
 * ErrorPagesCard 的两类契约：
 *  1. 保存保护：加载未完成 / 加载失败时不能保存，否则会用空值覆盖服务端已有配置。
 *  2. Tab 结构：role="tab" 的 aria-controls 必须指向真实存在的面板 id。
 *
 * 组件直接 import 了 api/me 与 utils/toast，这里整体 mock 掉，测真实的渲染与事件。
 */
import { flushPromises, mount } from '@vue/test-utils';

import UI from '@/components/app';
import ErrorPagesCard from '@/views/settings/ErrorPagesCard.vue';

const fetchTenantErrorPages = vi.fn();
const updateTenantErrorPages = vi.fn();
const toastError = vi.fn();
const toastSuccess = vi.fn();

vi.mock('@/api/me', () => ({
  fetchTenantErrorPages: (...args: unknown[]) => fetchTenantErrorPages(...args),
  updateTenantErrorPages: (...args: unknown[]) => updateTenantErrorPages(...args),
}));

vi.mock('@/utils/toast', () => ({
  message: {
    error: (...args: unknown[]) => toastError(...args),
    success: (...args: unknown[]) => toastSuccess(...args),
    warning: vi.fn(),
  },
}));

const SERVER_STATE = { custom404Html: '<h1>404</h1>', custom429Html: '<h1>429</h1>' };

/**
 * 应用在 main.ts 里用 app.use(UI) 全局注册 App* / Card* 组件，
 * 测试里必须同样装上这个插件，否则视图会渲染成 <appcard> 这类未知标签。
 */
const GLOBAL = { plugins: [UI], stubs: { Teleport: true } };

/** 挂载并等异步的 onMounted 加载跑完 */
async function mountCard() {
  // attachTo 必须开：断言用 document.getElementById 校验 aria-controls 指向的节点存在
  const wrapper = mount(ErrorPagesCard, { attachTo: document.body, global: GLOBAL });
  await flushPromises();
  return wrapper;
}

function saveButton(wrapper: Awaited<ReturnType<typeof mountCard>>) {
  return wrapper.findAll('button').find((b) => b.text().includes('保存配置'));
}

describe('ErrorPagesCard 保存保护', () => {
  beforeEach(() => {
    fetchTenantErrorPages.mockReset();
    updateTenantErrorPages.mockReset();
    toastError.mockReset();
    toastSuccess.mockReset();
    fetchTenantErrorPages.mockResolvedValue(SERVER_STATE);
  });

  it('加载未完成时保存按钮禁用，不会发出覆盖请求', async () => {
    let resolveFetch: (v: typeof SERVER_STATE) => void = () => {};
    fetchTenantErrorPages.mockReturnValue(
      new Promise<typeof SERVER_STATE>((resolve) => {
        resolveFetch = resolve;
      }),
    );

    const wrapper = mount(ErrorPagesCard, { attachTo: document.body, global: GLOBAL });
    await wrapper.vm.$nextTick();

    const btn = saveButton(wrapper);
    expect(btn).toBeDefined();
    expect(btn?.attributes('disabled')).toBeDefined();

    await btn?.trigger('click');
    expect(updateTenantErrorPages).not.toHaveBeenCalled();

    resolveFetch(SERVER_STATE);
    await flushPromises();
    wrapper.unmount();
  });

  it('加载失败后保存按钮保持禁用，不允许用空值覆盖服务端配置', async () => {
    fetchTenantErrorPages.mockRejectedValue(new Error('boom'));

    const wrapper = await mountCard();
    const btn = saveButton(wrapper);
    expect(btn).toBeDefined();
    expect(btn?.attributes('disabled')).toBeDefined();

    await btn?.trigger('click');
    expect(updateTenantErrorPages).not.toHaveBeenCalled();

    wrapper.unmount();
  });

  it('加载成功后保存按钮可用，且提交服务端已有内容而不是空串', async () => {
    updateTenantErrorPages.mockResolvedValue(SERVER_STATE);
    const wrapper = await mountCard();

    const btn = saveButton(wrapper);
    expect(btn?.attributes('disabled')).toBeUndefined();

    await btn?.trigger('click');
    await flushPromises();

    expect(updateTenantErrorPages).toHaveBeenCalledWith({
      custom404Html: SERVER_STATE.custom404Html,
      custom429Html: SERVER_STATE.custom429Html,
    });

    wrapper.unmount();
  });
});

describe('ErrorPagesCard Tab 结构', () => {
  beforeEach(() => {
    fetchTenantErrorPages.mockReset();
    updateTenantErrorPages.mockReset();
    fetchTenantErrorPages.mockResolvedValue(SERVER_STATE);
  });

  it('每个 role="tab" 的 aria-controls 都指向真实存在的面板 id', async () => {
    const wrapper = await mountCard();
    const tabs = wrapper.findAll('[role="tab"]');
    expect(tabs.length).toBe(2);

    for (const tab of tabs) {
      const controls = tab.attributes('aria-controls');
      // reka 的 TabsTrigger 在对应 TabsContent 不存在时给 undefined / 空串，两种都算未关联
      expect(controls, `tab「${tab.text()}」的 aria-controls 不能为空`).toBeTruthy();
      expect(document.getElementById(controls ?? ''), `aria-controls=${controls} 指向的节点不存在`).not.toBeNull();
    }

    wrapper.unmount();
  });
});