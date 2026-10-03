/**
 * Progress 的数值净化契约。
 *
 * 背景：AppProgress 原来是 `Math.max(0, Math.min(100, props.percent))`，
 * 而 Math.min(100, NaN) === NaN —— clamp 完全失效，NaN 直达 reka 的 ProgressRoot，
 * 触发 `Invalid prop \`value\` of value NaN supplied to \`ProgressRoot\`` 的 console.error。
 * ui/progress.vue 对 modelValue / max 零校验，越界值同样会刷错误。
 */
import { mount } from '@vue/test-utils';

import AppProgress from '@/components/app/AppProgress.vue';
import UiProgress from '@/components/ui/progress.vue';

/** reka 把净化后的进度写到 role=progressbar 的 aria-valuenow 上 */
function indicatorPercent(wrapper: ReturnType<typeof mount>): number | null {
  const raw = wrapper.find('[role="progressbar"]').attributes('aria-valuenow');
  return raw === undefined ? null : Number(raw);
}

/** 渲染期间收集 console.error（reka 的 validateValue/validateMax 走这条通道） */
async function renderCollectingErrors(
  render: () => ReturnType<typeof mount>,
): Promise<{ wrapper: ReturnType<typeof mount>; errors: string[] }> {
  const errors: string[] = [];
  const spy = vi.spyOn(console, 'error').mockImplementation((...args: unknown[]) => {
    errors.push(args.map(String).join(' '));
  });
  try {
    const wrapper = render();
    await wrapper.vm.$nextTick();
    await Promise.resolve();
    return { wrapper, errors };
  } finally {
    spy.mockRestore();
  }
}

describe('AppProgress 数值净化', () => {
  it('percent=NaN：净化为 0，不把 NaN 传给 ProgressRoot', async () => {
    const { wrapper, errors } = await renderCollectingErrors(() =>
      mount(AppProgress, { props: { percent: Number.NaN } }),
    );
    expect(errors.join('\n')).not.toContain('ProgressRoot');
    expect(indicatorPercent(wrapper)).toBe(0);
    expect(wrapper.text()).toContain('0%');
    wrapper.unmount();
  });

  it('percent=Infinity：收敛到 100', async () => {
    const { wrapper, errors } = await renderCollectingErrors(() =>
      mount(AppProgress, { props: { percent: Number.POSITIVE_INFINITY } }),
    );
    expect(errors.join('\n')).not.toContain('ProgressRoot');
    expect(wrapper.text()).toContain('100%');
    wrapper.unmount();
  });

  it('percent=-Infinity：收敛到 0', async () => {
    const { wrapper, errors } = await renderCollectingErrors(() =>
      mount(AppProgress, { props: { percent: Number.NEGATIVE_INFINITY } }),
    );
    expect(errors.join('\n')).not.toContain('ProgressRoot');
    expect(wrapper.text()).toContain('0%');
    wrapper.unmount();
  });

  it('percent=-20：夹到 0', async () => {
    const wrapper = mount(AppProgress, { props: { percent: -20 } });
    expect(wrapper.text()).toContain('0%');
    expect(indicatorPercent(wrapper)).toBe(0);
    wrapper.unmount();
  });

  it('percent=180：夹到 100', async () => {
    const wrapper = mount(AppProgress, { props: { percent: 180 } });
    expect(wrapper.text()).toContain('100%');
    wrapper.unmount();
  });

  it('percent 为 undefined / null：退到默认值 0，不产生 NaN', async () => {
    const undef = await renderCollectingErrors(() =>
      mount(AppProgress, { props: { percent: undefined } }),
    );
    expect(undef.errors.join('\n')).not.toContain('ProgressRoot');
    expect(undef.wrapper.text()).toContain('0%');
    undef.wrapper.unmount();

    const nul = await renderCollectingErrors(() =>
      mount(AppProgress, { props: { percent: null as unknown as number } }),
    );
    expect(nul.errors.join('\n')).not.toContain('ProgressRoot');
    expect(nul.wrapper.text()).toContain('0%');
    nul.wrapper.unmount();
  });
});

describe('ui/progress 数值净化', () => {
  it('modelValue 越界（负数 / 超过 max）：夹紧且不刷 console.error', async () => {
    const neg = await renderCollectingErrors(() =>
      mount(UiProgress, { props: { modelValue: -10, max: 100 } }),
    );
    expect(neg.errors.join('\n')).not.toContain('ProgressRoot');
    expect(indicatorPercent(neg.wrapper)).toBe(0);
    neg.wrapper.unmount();

    const over = await renderCollectingErrors(() =>
      mount(UiProgress, { props: { modelValue: 500, max: 100 } }),
    );
    expect(over.errors.join('\n')).not.toContain('ProgressRoot');
    expect(indicatorPercent(over.wrapper)).toBe(100);
    over.wrapper.unmount();
  });

  it('modelValue=NaN：净化为 0，不刷 console.error', async () => {
    const { wrapper, errors } = await renderCollectingErrors(() =>
      mount(UiProgress, { props: { modelValue: Number.NaN, max: 100 } }),
    );
    expect(errors.join('\n')).not.toContain('ProgressRoot');
    expect(indicatorPercent(wrapper)).toBe(0);
    wrapper.unmount();
  });

  it('max 非正数 / NaN：净化为 100，不刷 console.error', async () => {
    const zero = await renderCollectingErrors(() =>
      mount(UiProgress, { props: { modelValue: 50, max: 0 } }),
    );
    expect(zero.errors.join('\n')).not.toContain('ProgressRoot');
    zero.wrapper.unmount();

    const nan = await renderCollectingErrors(() =>
      mount(UiProgress, { props: { modelValue: 50, max: Number.NaN } }),
    );
    expect(nan.errors.join('\n')).not.toContain('ProgressRoot');
    nan.wrapper.unmount();
  });
});