/**
 * .vue SFC 挂载冒烟测试:证明 vite.config.ts 的 vue() 插件 + @vue/test-utils 链路通。
 * 选 AppTag:纯展示、无副作用(不读 store / 不发请求)。
 */
import { mount } from '@vue/test-utils';

import AppTag from '@/components/app/AppTag.vue';

describe('AppTag', () => {
  it('渲染 slot 内容', () => {
    const wrapper = mount(AppTag, { slots: { default: 'beta' } });
    expect(wrapper.text()).toBe('beta');
    wrapper.unmount();
  });

  it('color=ok 走语义色预设类', () => {
    const wrapper = mount(AppTag, { props: { color: 'ok' }, slots: { default: 'up' } });
    expect(wrapper.classes()).toContain('text-ok');
    expect(wrapper.classes()).toContain('border-ok/30');
    wrapper.unmount();
  });

  it('color=#rrggbb 走内联样式而非预设类', () => {
    const wrapper = mount(AppTag, {
      props: { color: '#ff8800' },
      slots: { default: 'custom' },
    });
    const style = wrapper.attributes('style') ?? '';
    expect(style).toContain('color: #ff8800');
    expect(style).toContain('border-color: #ff880055');
    expect(style).toContain('background-color: #ff880014');
    expect(wrapper.classes()).not.toContain('text-ok');
    wrapper.unmount();
  });
});