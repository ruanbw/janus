import { mount } from '@vue/test-utils';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { defineComponent, h, nextTick, ref } from 'vue';

import App from '@/App.vue';

/**
 * 可开关抛错的 router-view 替身。
 * 异常由 shouldThrow 驱动、在 mount 之后才发生:@vue/test-utils 会把
 * 「挂载期间」的错误重新抛给调用方,测不到挂载后的降级与恢复。
 */
function createFlakyRouteView() {
  const shouldThrow = ref(false);
  const view = defineComponent({
    name: 'FlakyRouteView',
    setup() {
      return () => {
        if (shouldThrow.value) throw new Error('页面渲染炸了');
        return h('div', { 'data-testid': 'page-content' }, '页面内容');
      };
    },
  });
  return { view, shouldThrow };
}

/** ConfirmHost 被卸载会让 await confirmAsync(...) 的调用方永久挂起,这里记录卸载 */
let confirmHostUnmounted = false;

const ConfirmHostStub = defineComponent({
  name: 'ConfirmHostStub',
  setup: () => () => h('div', { 'data-testid': 'confirm-host' }),
  unmounted() {
    confirmHostUnmounted = true;
  },
});

const ToasterStub = defineComponent({
  name: 'ToasterStub',
  setup: () => () => h('div', { 'data-testid': 'toaster' }),
});

// 未指定时也要兜住:否则 Vue dev 模式会把错误重新抛回测试进程
async function mountApp(errorHandler: (err: unknown, ...rest: unknown[]) => void = () => {}) {
  const { view, shouldThrow } = createFlakyRouteView();
  const wrapper = mount(App, {
    global: {
      stubs: { 'router-view': view, ConfirmHost: ConfirmHostStub, Toaster: ToasterStub },
      config: { errorHandler },
    },
  });
  await nextTick();
  return { wrapper, shouldThrow };
}

async function crash(shouldThrow: { value: boolean }): Promise<void> {
  shouldThrow.value = true;
  await nextTick();
  await nextTick();
}

async function clickRetry(wrapper: { findAll: (s: string) => { text: () => string; trigger: (e: string) => Promise<unknown> }[] }): Promise<void> {
  const button = wrapper.findAll('button').find((b) => b.text() === '重试');
  expect(button).toBeDefined();
  await button!.trigger('click');
  await nextTick();
}

describe('App.vue 致命错误可恢复', () => {
  beforeEach(() => {
    vi.spyOn(console, 'error').mockImplementation(() => {});
    vi.spyOn(window.location, 'reload').mockImplementation(() => {});
    confirmHostUnmounted = false;
  });

  afterEach(() => {
    vi.restoreAllMocks();
  });

  it('子组件抛错时显示降级卡片', async () => {
    const { wrapper, shouldThrow } = await mountApp();
    await crash(shouldThrow);

    expect(wrapper.text()).toContain('系统暂时无法加载');
    expect(wrapper.text()).toContain('页面渲染炸了');
  });

  it('点击「重试」能在不刷新页面的前提下恢复页面', async () => {
    const { wrapper, shouldThrow } = await mountApp();
    await crash(shouldThrow);
    shouldThrow.value = false;

    await clickRetry(wrapper);

    expect(wrapper.find('[data-testid="page-content"]').exists()).toBe(true);
    expect(wrapper.text()).not.toContain('系统暂时无法加载');
    expect(window.location.reload).not.toHaveBeenCalled();
  });

  it('「重试」强制重建子树(靠 key 而非仅清错误标记)', async () => {
    let renders = 0;
    const crashFlag = ref(false);
    const CountingView = defineComponent({
      name: 'CountingView',
      setup() {
        return () => {
          renders += 1;
          if (crashFlag.value) throw new Error('页面渲染炸了');
          return h('div', { 'data-testid': 'page-content' });
        };
      },
    });
    const wrapper = mount(App, {
      global: {
        stubs: { 'router-view': CountingView, ConfirmHost: ConfirmHostStub, Toaster: ToasterStub },
        config: { errorHandler: () => {} },
      },
    });
    await nextTick();
    crashFlag.value = true;
    await nextTick();
    await nextTick();
    const rendersBeforeRetry = renders;

    crashFlag.value = false;
    await clickRetry(wrapper);

    // 光清错误标记不够:必须换 key 重建,否则坏掉的子树状态会被复用
    expect(renders).toBeGreaterThan(rendersBeforeRetry);
    expect(wrapper.find('[data-testid="page-content"]').exists()).toBe(true);
  });

  it('崩溃期间 ConfirmHost 保持挂载:打开中的确认框不会被卸载而挂死', async () => {
    const { wrapper, shouldThrow } = await mountApp();
    await crash(shouldThrow);

    expect(confirmHostUnmounted).toBe(false);
    expect(wrapper.find('[data-testid="confirm-host"]').exists()).toBe(true);
  });
});

describe('App.vue 错误上报口径', () => {
  beforeEach(() => {
    vi.spyOn(console, 'error').mockImplementation(() => {});
    vi.spyOn(window.location, 'reload').mockImplementation(() => {});
  });

  afterEach(() => {
    vi.restoreAllMocks();
  });

  it('App 捕获的错误继续冒泡到 app.config.errorHandler', async () => {
    const errorHandler = vi.fn();
    const { shouldThrow } = await mountApp(errorHandler);

    await crash(shouldThrow);

    expect(errorHandler).toHaveBeenCalledWith(
      expect.objectContaining({ message: '页面渲染炸了' }),
      expect.anything(),
      expect.any(String),
    );
  });
});