import { mount } from '@vue/test-utils';
import { createPinia } from 'pinia';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { h, type VNode } from 'vue';

import ThemeSwitcher from '@/components/layout/ThemeSwitcher.vue';
import { useThemeStore } from '@/stores/theme';

/**
 * 把 reka-ui 的下拉原语换成能在happy-dom 里直接 emit select 的替身。
 * 组件从 '@/components/ui' 桶文件导入,所以按模块 mock 才拦得住。
 */
vi.mock('@/components/ui', () => {
  const passthrough = (name: string) => ({
    name,
    setup: (_: unknown, ctx: { slots: Record<string, unknown> }) => () =>
      (((ctx.slots.default as (() => VNode) | undefined)?.() ?? []) as VNode[]),
  });
  return {
    DropdownMenuRoot: passthrough('DropdownMenuRoot'),
    DropdownMenuTrigger: passthrough('DropdownMenuTrigger'),
    DropdownMenuPortal: passthrough('DropdownMenuPortal'),
    DropdownMenuContent: passthrough('DropdownMenuContent'),
    // 真实组件只透传 @select(值由 v-for 闭包捕获),替身只需能触发它
    DropdownMenuItem: {
      name: 'DropdownMenuItem',
      emits: ['select'],
      setup(_: unknown, ctx: { emit: (e: string) => void; slots: Record<string, unknown> }) {
        return () =>
          h(
            'button',
            { onClick: () => ctx.emit('select') },
            ((ctx.slots.default as (() => VNode) | undefined)?.() ?? []) as VNode[],
          );
      },
    },
  };
});

/**
 * 收集运行期间逃逸的未处理 promise 拒绝。
 * happy-dom 下未处理的拒绝走 Node 的 process 'unhandledRejection'。
 *
 * 注意:这里不能用 vi.spyOn(...).mockRejectedValue —— vitest 自己会在 mock
 * 返回值上挂 catch,拒绝根本不会逃逸,断言就成了空跑。改用普通函数替换。
 */
async function withRejectionCollector(run: () => void): Promise<unknown[]> {
  const rejections: unknown[] = [];
  const handler = (reason: unknown) => {
    rejections.push(reason);
  };
  process.on('unhandledRejection', handler);
  try {
    run();
    // onSelect 内有两帧 requestAnimationFrame 延迟,等它跑完
    await new Promise((resolve) => setTimeout(resolve, 60));
  } finally {
    process.off('unhandledRejection', handler);
  }
  return rejections;
}

function replaceSetModeAnimated(
  store: ReturnType<typeof useThemeStore>,
  impl: (mode: string, origin: { x: number; y: number }) => Promise<void>,
): void {
  Object.defineProperty(store, 'setModeAnimated', { configurable: true, value: impl });
}

function mountSwitcher(pinia: ReturnType<typeof createPinia>) {
  const wrapper = mount(ThemeSwitcher, { global: { plugins: [pinia] } });
  // 深色选项的文案是「深色」
  const darkItem = wrapper.findAll('button').find((b) => b.text() === '深色');
  expect(darkItem).toBeDefined();
  return { wrapper, darkItem: darkItem! };
}

describe('ThemeSwitcher 切换失败不外泄', () => {
  afterEach(() => {
    vi.restoreAllMocks();
  });

  it('setModeAnimated reject 时不产生未处理的 promise 拒绝', async () => {
    const pinia = createPinia();
    const store = useThemeStore(pinia);
    let called = 0;
    replaceSetModeAnimated(store, () => {
      called += 1;
      return Promise.reject(new Error('动画收尾失败'));
    });

    const { wrapper, darkItem } = mountSwitcher(pinia);
    const rejections = await withRejectionCollector(() => {
      void darkItem.trigger('click');
    });

    expect(called).toBe(1);
    expect(rejections).toEqual([]);
    wrapper.unmount();
  });

  it('正常切换仍会调用 setModeAnimated', async () => {
    const pinia = createPinia();
    const store = useThemeStore(pinia);
    const calls: unknown[][] = [];
    replaceSetModeAnimated(store, (mode, origin) => {
      calls.push([mode, origin]);
      return Promise.resolve();
    });

    const { wrapper, darkItem } = mountSwitcher(pinia);
    await withRejectionCollector(() => {
      void darkItem.trigger('click');
    });

    expect(calls).toEqual([['dark', expect.objectContaining({ x: expect.any(Number) })]]);
    wrapper.unmount();
  });
});