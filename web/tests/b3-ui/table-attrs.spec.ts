/**
 * table.vue 的 attrs 落点契约。
 *
 * 背景：table.vue 是两层结构 —— 外层滚动 <div class="relative w-full overflow-x-auto">
 * 里面才是真正的 <table>。修复前 `class` 落在内层 <table>，而其余 fallthrough
 * 属性（data-* / id / aria-* / style）落在外层 <div>，属性被拆散到两个元素上：
 * 既不好定位，也让「给 Table 传 id」这种写法名不副实。
 *
 * 契约：Table 的 class 与全部 fallthrough 属性统一落在真正承载语义的 <table> 上；
 * 外层滚动容器只负责 overflow，不接收调用方的属性。
 */
import { mount } from '@vue/test-utils';

import Table from '@/components/ui/table.vue';

describe('table.vue — attrs 落点必须统一在 <table> 上', () => {
  it('class 落在 <table> 上', () => {
    const wrapper = mount(Table, { props: { class: 'w-full' } });

    expect(wrapper.find('table').classes()).toContain('w-full');

    wrapper.unmount();
  });

  it('data-* 落在同一个 <table> 上，而不是外层滚动 div', () => {
    const wrapper = mount(Table, { attrs: { 'data-testid': 'my-table' } });

    expect(wrapper.find('table').attributes('data-testid')).toBe('my-table');

    wrapper.unmount();
  });

  it('id 与其它 fallthrough attr 也落在 <table> 上', () => {
    const wrapper = mount(Table, { attrs: { id: 'my-table', 'aria-label': '数据表' } });

    expect(wrapper.find('table').attributes('id')).toBe('my-table');
    expect(wrapper.find('table').attributes('aria-label')).toBe('数据表');

    wrapper.unmount();
  });

  it('class 与 fallthrough attr 落在同一个元素上（不再分裂）', () => {
    const wrapper = mount(Table, {
      props: { class: 'w-full' },
      attrs: { 'data-testid': 'my-table' },
    });

    const table = wrapper.find('table');
    expect(table.classes()).toContain('w-full');
    expect(table.attributes('data-testid')).toBe('my-table');
    // 外层滚动 div 不应沾到调用方的属性
    expect(wrapper.find('div').attributes('data-testid')).toBeUndefined();

    wrapper.unmount();
  });

  it('外层滚动容器仍保留自身的 overflow 行为', () => {
    const wrapper = mount(Table);

    expect(wrapper.find('div').classes()).toContain('overflow-x-auto');
    expect(wrapper.find('table').classes()).toContain('w-full');

    wrapper.unmount();
  });

  it('slot 内容仍渲染在 <table> 内', () => {
    const wrapper = mount(Table, { slots: { default: '<tbody><tr><td>x</td></tr></tbody>' } });

    expect(wrapper.find('table td').text()).toBe('x');

    wrapper.unmount();
  });
});