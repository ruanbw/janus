/**
 * AppTag 的 size 属性契约。
 *
 * 背景：ErrorPagesCard.vue 给 <AppTag size="small"> 传了 size，但 AppTag 从未声明它。
 * 未声明的属性会作为 fallthrough attr 落到 Badge → Primitive（渲染 <span>）上，
 * 变成一个纯装饰的 size="small" DOM 属性，既不产生任何样式，又污染标记。
 *
 * 契约：AppTag 显式声明 size 并产生真实样式（不再漏到 DOM 上）。
 */
import { mount } from '@vue/test-utils';

import AppTag from '@/components/app/AppTag.vue';

describe('AppTag size 属性', () => {
  it('size="small" 产生小号样式，而不是漏到 DOM 上当装饰属性', () => {
    const wrapper = mount(AppTag, { props: { size: 'small' }, slots: { default: 'beta' } });
    // 原实现：AppTag 无 size prop → size="small" 直接出现在 <span> 上
    expect(wrapper.attributes('size')).toBeUndefined();
    expect(wrapper.classes()).toContain('text-2xs');
    wrapper.unmount();
  });

  it('size="small" 的字号小于默认档', () => {
    const small = mount(AppTag, { props: { size: 'small' }, slots: { default: 'x' } });
    const def = mount(AppTag, { slots: { default: 'x' } });
    expect(small.classes()).not.toEqual(def.classes());
    expect(def.classes()).toContain('text-xs');
    small.unmount();
    def.unmount();
  });

  it('size="default" 与不传 size 等价', () => {
    const withDefault = mount(AppTag, { props: { size: 'default' }, slots: { default: 'x' } });
    const without = mount(AppTag, { slots: { default: 'x' } });
    expect(withDefault.classes()).toEqual(without.classes());
    withDefault.unmount();
    without.unmount();
  });

  it('非法 size 值回落到默认档，不产生未知属性', () => {
    const wrapper = mount(AppTag, {
      props: { size: 'huge' as unknown as 'default' },
      slots: { default: 'x' },
    });
    expect(wrapper.classes()).toContain('text-xs');
    wrapper.unmount();
  });

  it('size 不影响语义色预设', () => {
    const wrapper = mount(AppTag, { props: { size: 'small', color: 'ok' }, slots: { default: 'up' } });
    expect(wrapper.classes()).toContain('text-ok');
    expect(wrapper.classes()).toContain('border-ok/30');
    wrapper.unmount();
  });
});