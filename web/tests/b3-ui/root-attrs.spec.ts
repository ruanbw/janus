/**
 * popover.vue / dropdown-menu.vue 的 fallthrough attrs 落点契约。
 *
 * 背景：这两个根组件虽然「单根」，但链路终点是 reka 的 PopperRoot，
 * 而 PopperRoot 自带 `inheritAttrs: false` 且只 `return renderSlot($slots, 'default')`。
 * 于是根上的 class 与所有 fallthrough attr 被静默丢弃 —— 连 Vue 的
 * extraneous-attrs 告警都触发不了（因为对 PopperRoot 来说它们是合法 attrs）。
 *
 * 修复前实测：<UiPopover class="x" data-p="1"> 渲染出的 DOM 里既没有 x 也没有 data-p。
 */
import { mount } from '@vue/test-utils';
import { h, type Component } from 'vue';

import DropdownMenu from '@/components/ui/dropdown-menu.vue';
import DropdownMenuTrigger from '@/components/ui/dropdown-menu-trigger.vue';
import Popover from '@/components/ui/popover.vue';
import PopoverTrigger from '@/components/ui/popover-trigger.vue';

describe('popover.vue / dropdown-menu.vue — 根上的 attrs 必须落到真实 DOM', () => {
  it('popover：class 落到根 DOM 元素上', () => {
    const wrapper = mount(Popover, {
      attrs: { class: 'popover-root' },
      slots: { default: () => h(PopoverTrigger, { asChild: true }, { default: () => h('button', 'T') }) },
    });

    expect(wrapper.find('.popover-root').exists()).toBe(true);

    wrapper.unmount();
  });

  it('popover：data-* fallthrough attr 落到根 DOM 元素上', () => {
    const wrapper = mount(Popover, {
      attrs: { 'data-testid': 'popover-root' },
      slots: { default: () => h(PopoverTrigger, { asChild: true }, { default: () => h('button', 'T') }) },
    });

    expect(wrapper.find('[data-testid="popover-root"]').exists()).toBe(true);

    wrapper.unmount();
  });

  it('dropdown-menu：class 落到根 DOM 元素上', () => {
    const wrapper = mount(DropdownMenu, {
      attrs: { class: 'menu-root' },
      slots: { default: () => h(DropdownMenuTrigger, { asChild: true }, { default: () => h('button', 'T') }) },
    });

    expect(wrapper.find('.menu-root').exists()).toBe(true);

    wrapper.unmount();
  });

  it('dropdown-menu：data-* fallthrough attr 落到根 DOM 元素上', () => {
    const wrapper = mount(DropdownMenu, {
      attrs: { 'data-testid': 'menu-root' },
      slots: { default: () => h(DropdownMenuTrigger, { asChild: true }, { default: () => h('button', 'T') }) },
    });

    expect(wrapper.find('[data-testid="menu-root"]').exists()).toBe(true);

    wrapper.unmount();
  });

  it('dropdown-menu：id 落到根 DOM 元素上（表单 label 关联依赖它）', () => {
    const wrapper = mount(DropdownMenu, {
      attrs: { id: 'menu-root' },
      slots: { default: () => h(DropdownMenuTrigger, { asChild: true }, { default: () => h('button', 'T') }) },
    });

    expect(wrapper.find('#menu-root').exists()).toBe(true);

    wrapper.unmount();
  });

  it('popover：slot 内容仍正常渲染（加根元素不能吞掉 children）', () => {
    const wrapper = mount(Popover, {
      attrs: { class: 'popover-root' },
      slots: { default: () => h(PopoverTrigger, { asChild: true }, { default: () => h('button', 'go') }) },
    });

    expect(wrapper.text()).toContain('go');

    wrapper.unmount();
  });
});
/**
 * 根必须是「单个元素」而不是 fragment。
 *
 * popover.vue / dropdown-menu.vue 的模板根前面放了一段 HTML 注释来解释
 * display:contents 的来由。Vue 的编译器把根前的注释也算进根节点列表，
 * 于是组件根变成 Fragment 而不是单个 div —— 这正是 Vue 会打出
 * "Extraneous non-props attributes ... could not be automatically inherited
 * because component renders fragment" 的那个条件。
 *
 * 后果是隐性的：任何走 $attrs 自动继承的路径都会失效，且只有加 attr 时才报警。
 * 把说明移到 <script> 里即可，根重新变回单个元素。
 */
describe('popover / dropdown-menu — 根必须是单个元素而非 fragment', () => {
  const cases: Array<[string, Component, Component]> = [
    ['popover', Popover, PopoverTrigger],
    ['dropdown-menu', DropdownMenu, DropdownMenuTrigger],
  ];

  for (const [name, Root, Trigger] of cases) {
    it(`${name}：组件根是元素节点而非 fragment`, () => {
      const wrapper = mount(Root, {
        attrs: { 'data-testid': 'root-probe' },
        slots: { default: () => h(Trigger, { asChild: true }, { default: () => h('button', 'T') }) },
      });

      // Fragment 的 type 是 Symbol(v-fgt)；单元素根的 type 是字符串 'div'
      expect(typeof wrapper.vm.$.subTree.type, `${name} 的根被注释变成了 fragment`).toBe('string');

      wrapper.unmount();
    });

    it(`${name}：根元素上没有多余的注释兄弟节点`, () => {
      const wrapper = mount(Root, {
        attrs: { 'data-testid': 'root-probe' },
        slots: { default: () => h(Trigger, { asChild: true }, { default: () => h('button', 'T') }) },
      });

      // wrapper.element 必须直接就是承接 attrs 的那个 div
      expect((wrapper.element as HTMLElement).tagName).toBe('DIV');
      expect((wrapper.element as HTMLElement).getAttribute('data-testid')).toBe('root-probe');

      wrapper.unmount();
    });
  }
});
