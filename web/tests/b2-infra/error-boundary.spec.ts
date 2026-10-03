import { mount } from '@vue/test-utils';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { defineComponent, h, nextTick, ref } from 'vue';

import ErrorBoundary from '@/components/layout/ErrorBoundary.vue';

const crashFlag = ref(false);
const FlakyChild = defineComponent({
  name: 'FlakyChild',
  setup() {
    return () => {
      if (crashFlag.value) throw new Error('子组件渲染炸了');
      return h('div', { 'data-testid': 'ok' }, '正常内容');
    };
  },
});

function mountBoundary(props: { resetKey?: string } = {}) {
  return mount(ErrorBoundary, {
    props,
    slots: { default: () => h(FlakyChild) },
    global: { config: { errorHandler: () => {} } },
  });
}

async function crash(): Promise<void> {
  crashFlag.value = true;
  await nextTick();
  await nextTick();
}

describe('ErrorBoundary 行为契约', () => {
  beforeEach(() => {
    crashFlag.value = false;
    vi.spyOn(console, 'error').mockImplementation(() => {});
    vi.spyOn(window.location, 'reload').mockImplementation(() => {});
  });

  afterEach(() => {
    vi.restoreAllMocks();
  });

  it('子组件抛错时渲染降级卡片', async () => {
    const wrapper = mountBoundary();
    await crash();

    expect(wrapper.text()).toContain('页面加载或渲染出错');
    expect(wrapper.text()).toContain('子组件渲染炸了');
  });

  it('resetKey 变化时清空错误,新页面不继承上一页的降级卡片', async () => {
    const wrapper = mountBoundary({ resetKey: '/a' });
    await crash();
    expect(wrapper.text()).toContain('页面加载或渲染出错');

    crashFlag.value = false;
    await wrapper.setProps({ resetKey: '/b' });
    await nextTick();

    expect(wrapper.text()).not.toContain('页面加载或渲染出错');
  });

  it('「重新加载」重建插槽内容,attempt 随重试递增', async () => {
    let renders = 0;
    const Counting = defineComponent({
      name: 'Counting',
      setup() {
        return () => {
          renders += 1;
          if (crashFlag.value) throw new Error('子组件渲染炸了');
          return h('div', { 'data-testid': 'ok' });
        };
      },
    });
    const wrapper = mount(ErrorBoundary, {
      slots: { default: ({ attempt }: { attempt: number }) => h(Counting, { key: attempt }) },
      global: { config: { errorHandler: () => {} } },
    });
    await crash();
    await nextTick();
    const before = renders;
    crashFlag.value = false;

    await wrapper.findAll('button').find((b) => b.text() === '重新加载')!.trigger('click');
    await nextTick();

    expect(renders).toBeGreaterThan(before);
    expect(window.location.reload).not.toHaveBeenCalled();
  });

  it('子组件错误不再冒泡到 app.config.errorHandler:边界负责就地降级,不把整站砖化', async () => {
    const errorHandler = vi.fn();
    mount(ErrorBoundary, {
      slots: { default: () => h(FlakyChild) },
      global: { config: { errorHandler } },
    });
    await crash();

    expect(errorHandler).not.toHaveBeenCalled();
  });

  it('生产包不输出诊断日志(console.error 由 import.meta.env.DEV 把关)', async () => {
    vi.stubEnv('DEV', false);
    mountBoundary();
    await crash();

    expect(console.error).not.toHaveBeenCalled();
    vi.unstubAllEnvs();
  });
});