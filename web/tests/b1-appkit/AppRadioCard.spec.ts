/**
 * AppRadioCard 的 DOM 内容模型契约。
 *
 * 背景：UiRadioGroupItem 渲染的是 <button role="radio">，button 的内容模型只允许
 * phrasing content。原实现把 <div> / <p> 直接塞进去（非法），浏览器解析时的
 * foster parenting 会把节点挪出按钮，焦点环与 aria 关系随之失效。
 */
import { h, type Component } from 'vue';
import { mount } from '@vue/test-utils';

import AppRadioCard from '@/components/app/AppRadioCard.vue';
import AppRadioGroup from '@/components/app/AppRadioGroup.vue';

/** button 只允许 phrasing content：div / p / ul 等块级元素一律非法 */
const BLOCK_TAGS = ['div', 'p', 'ul', 'ol', 'li', 'section', 'article', 'table'];

function blockDescendants(el: Element): string[] {
  return Array.from(el.querySelectorAll(BLOCK_TAGS.join(',')))
    .filter((node) => el.contains(node))
    .map((node) => node.tagName.toLowerCase());
}

describe('AppRadioCard DOM 内容模型', () => {
  /**
   * UiRadioGroupItem 依赖 RadioGroupRoot 上下文，所以每个用例都在 AppRadioGroup 内挂载。
   */
  function mountCard(
    props: { value: string | number; title?: string; description?: string; icon?: Component },
    slots?: Record<string, () => unknown>,
  ) {
    return mount(AppRadioGroup, {
      props: { modelValue: props.value },
      slots: { default: () => h(AppRadioCard, props, slots) },
      global: { components: { AppRadioCard } },
      attachTo: document.body,
    });
  }

  it('带 title + description + icon 时，按钮内不出现块级元素', () => {
    const wrapper = mountCard(
      { value: 'global', title: '全局', description: '对本租户的全部短链生效。', icon: { template: '<svg />' } },
    );
    const button = wrapper.find('button');
    expect(button.exists()).toBe(true);
    expect(button.attributes('role')).toBe('radio');
    expect(blockDescendants(button.element)).toEqual([]);
    wrapper.unmount();
  });

  it('#extra 插槽传入 AppTag 这类 span 时仍合法，且内容渲染出来', () => {
    const wrapper = mountCard(
      { value: '302', title: '302 临时重定向', description: '不缓存跳转' },
      { extra: () => h('span', { class: 'tag' }, '推荐') },
    );
    const button = wrapper.find('button');
    expect(blockDescendants(button.element)).toEqual([]);
    expect(button.find('.tag').text()).toBe('推荐');
    wrapper.unmount();
  });

  it('#title 插槽渲染内容', () => {
    const wrapper = mountCard({ value: 'x' }, { title: () => h('span', '插槽标题') });
    expect(wrapper.find('button').text()).toContain('插槽标题');
    expect(blockDescendants(wrapper.find('button').element)).toEqual([]);
    wrapper.unmount();
  });

  it('description 文案与 title 都渲染在按钮内', () => {
    const wrapper = mountCard({ value: 'global', title: '全局', description: '全部短链参与求值' });
    const button = wrapper.find('button');
    expect(button.text()).toContain('全局');
    expect(button.text()).toContain('全部短链参与求值');
    wrapper.unmount();
  });

  it('放在 AppRadioGroup 里，每张 role=radio 按钮内无块级元素', () => {
    const wrapper = mount(AppRadioGroup, {
      props: { modelValue: 'global' },
      slots: {
        default: `
          <AppRadioCard value="global" title="全局" description="全部短链" />
          <AppRadioCard value="links" title="指定短链" description="只对指定短链生效" />
        `,
      },
      global: { components: { AppRadioCard } },
      attachTo: document.body,
    });
    const buttons = wrapper.findAll('button[role="radio"]');
    expect(buttons.length).toBe(2);
    for (const btn of buttons) {
      expect(blockDescendants(btn.element)).toEqual([]);
    }
    wrapper.unmount();
  });
});