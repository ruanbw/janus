/**
 * AppTooltip / ui/tooltip 的属性透传契约。
 *
 * 背景：
 *  1. AppTooltip 声明了 class prop 但模板从未使用 → 外部传入的 class 被静默吞掉
 *     （LinksView.vue:251 给长 URL 传的 `max-w-md whitespace-normal break-all`
 *      整条换行样式链消失）。
 *  2. ui/tooltip 声明了 open / disabled / ignoreNonKeyboardFocus 三个 prop 但完全不转发
 *     → v-model:open 静默失效。
 *  3. ui/tooltip 的根是 TooltipProvider，而 TooltipProvider 自带 inheritAttrs:false
 *     且只 renderSlot，未声明的 attr 全部丢弃。
 */
import { mount } from '@vue/test-utils';
import { TooltipContent, TooltipPortal, TooltipRoot, TooltipTrigger } from 'reka-ui';

import AppTooltip from '@/components/app/AppTooltip.vue';
import { Tooltip as UiTooltip } from '@/components/ui/tooltip';
import { TooltipContent as UiTooltipContent } from '@/components/ui/tooltip';

beforeEach(() => {
  document.body.innerHTML = '';
});

/**
 * 在 UiTooltip 的插槽里放真实的 TooltipTrigger / TooltipPortal / TooltipContent。
 * 不能用一个字面量 <div role="tooltip"> 冒充：那它无论 open 传没传都会渲染，
 * 断言就变成假绿。
 */
const SLOT = `
  <TooltipTrigger><button>触发</button></TooltipTrigger>
  <TooltipPortal>
    <TooltipContent>提示内容</TooltipContent>
  </TooltipPortal>
`;

const COMPONENTS = { TooltipTrigger, TooltipPortal, TooltipContent };

async function mountTooltip(props: Record<string, unknown>) {
  const wrapper = mount(UiTooltip, {
    props,
    slots: { default: SLOT },
    attachTo: document.body,
    global: { components: COMPONENTS },
  });
  await wrapper.vm.$nextTick();
  await Promise.resolve();
  return wrapper;
}

describe('ui/tooltip 属性转发', () => {
  it('open prop 被转发到 TooltipRoot：open=true 时 tooltip 出现', async () => {
    const wrapper = await mountTooltip({ open: true });
    // 转发前 open 被吞（TooltipRoot 只拿到 defaultOpen=false），tooltip 永远不出现
    expect(document.querySelector('[role="tooltip"]')?.textContent).toContain('提示内容');
    wrapper.unmount();
  });

  it('open=false 时 tooltip 不出现', async () => {
    const wrapper = await mountTooltip({ open: false });
    expect(document.querySelector('[role="tooltip"]')).toBeNull();
    wrapper.unmount();
  });

  it('disabled prop 被转发到 TooltipRoot', async () => {
    const wrapper = await mountTooltip({ open: true, disabled: true });
    // disabled 在 reka 里只影响「能否被触发」，不撤销已受控的 open，
    // 所以这里断言它确实到达 TooltipRoot，而不是「tooltip 不出现」。
    expect(wrapper.findComponent(TooltipRoot).props('disabled')).toBe(true);
    wrapper.unmount();
  });

  it('update:open 由真实的 TooltipRoot 冒泡上来，v-model:open 才能成立', async () => {
    const wrapper = await mountTooltip({ open: false });
    // 不能直接 wrapper.vm.$emit —— 那是组件自己往自己身上打，与转发无关，
    // 原实现（声明了 emit 却没绑定 @update:open）照样能“通过”。
    // 必须从 TooltipRoot 真实触发，事件才有路径走到 UiTooltip。
    const root = wrapper.findComponent(TooltipRoot);
    expect(root.exists()).toBe(true);
    root.vm.$emit('update:open', true);
    await wrapper.vm.$nextTick();
    expect(wrapper.emitted('update:open')).toEqual([[true]]);
    wrapper.unmount();
  });

  it('TooltipRoot 真的拿到了 open=false，而不是被吞掉退回 defaultOpen', async () => {
    const wrapper = await mountTooltip({ open: false });
    expect(wrapper.findComponent(TooltipRoot).props('open')).toBe(false);
    wrapper.unmount();
  });

  it('ignoreNonKeyboardFocus 被声明并转发到 TooltipRoot', async () => {
    const wrapper = await mountTooltip({ open: true, ignoreNonKeyboardFocus: true });
    expect(wrapper.findComponent(TooltipRoot).props('ignoreNonKeyboardFocus')).toBe(true);
    wrapper.unmount();
  });

  it('未声明的 attr 无法落到 DOM：整条 reka 根链都是无渲染根', async () => {
    const wrapper = await mountTooltip({ open: true, 'data-testid': 'tooltip-host' });
    // 实测：TooltipProvider → TooltipRoot → PopperRoot 一路 inheritAttrs:false 且只
    // renderSlot，链上没有任何元素能承载 attr。这条用例把该约束钉死，
    // 避免下一个调用方误以为传 class/data-* 能生效（那正是 LinksView 踩的坑）。
    expect(wrapper.find('[data-testid="tooltip-host"]').exists()).toBe(false);
    wrapper.unmount();
  });
});

describe('AppTooltip class 应用', () => {
  it('class 透传给 ui/tooltip-content，而不是被静默吞掉', async () => {
    const wrapper = mount(AppTooltip, {
      props: { title: '提示', class: 'my-custom-class' },
      slots: { default: '<span>x</span>' },
      attachTo: document.body,
      // TooltipPortal 用真渲染替换（不是 stub:true），否则子树被整体禁用，
      // 内容节点根本不会出现，断言就是空转。
      global: { stubs: { TooltipPortal: { template: '<div><slot /></div>' } } },
    });
    await wrapper.vm.$nextTick();

    const content = wrapper.findComponent(UiTooltipContent);
    expect(content.exists()).toBe(true);
    // 原实现：AppTooltip 从不把 props.class 用到任何节点上，这里恒为 undefined
    expect(content.props('class')).toContain('my-custom-class');
    wrapper.unmount();
  });
});