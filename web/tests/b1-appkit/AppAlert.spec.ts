/**
 * AppAlert 的 message / description 渲染契约。
 *
 * 背景：headingText = title || message，而 hasBody 只看 description 与插槽。
 * 同时传 title + message 时，message 全文被当成标题吃掉且不进正文块 ——
 * 5 处调用方（AccountView / DomainCreateView / ErrorPagesCard /
 * AdminTenantRemoveDomainView / AdminTenantTierView）都踩了这个。
 */
import { mount } from '@vue/test-utils';

import AppAlert from '@/components/app/AppAlert.vue';

/** 标题节点：AppAlert 用 <h5> 承载 */
function headingTextOf(wrapper: ReturnType<typeof mount>): string | null {
  const h = wrapper.find('h5');
  return h.exists() ? h.text() : null;
}

/** 正文块：由 ui/alert.vue 的 alertDescriptionVariants（text-xs …）标记 */
function bodyTextOf(wrapper: ReturnType<typeof mount>): string | null {
  const body = wrapper.find('div.text-xs');
  return body.exists() ? body.text() : null;
}

describe('AppAlert 文本渲染', () => {
  it('只传 message：无标题块，message 全文进正文', () => {
    const wrapper = mount(AppAlert, { props: { message: '仅有正文消息' } });
    expect(headingTextOf(wrapper)).toBeNull();
    expect(wrapper.text()).toContain('仅有正文消息');
    wrapper.unmount();
  });

  it('title + message：标题归标题，message 全文进正文（不被吃掉）', () => {
    const wrapper = mount(AppAlert, {
      props: { title: '三级页面决议机制说明', message: '页面决议优先级为：单条规则专属页面。' },
    });
    expect(headingTextOf(wrapper)).toBe('三级页面决议机制说明');
    expect(bodyTextOf(wrapper)).toBe('页面决议优先级为：单条规则专属页面。');
    wrapper.unmount();
  });

  it('title + description：description 进正文，message 不参与', () => {
    const wrapper = mount(AppAlert, {
      props: { title: '提示标题', description: '正文描述', message: '不该出现' },
    });
    expect(headingTextOf(wrapper)).toBe('提示标题');
    expect(bodyTextOf(wrapper)).toBe('正文描述');
    expect(wrapper.text()).not.toContain('不该出现');
    wrapper.unmount();
  });

  it('只传 title：无正文块', () => {
    const wrapper = mount(AppAlert, { props: { title: '只有标题' } });
    expect(headingTextOf(wrapper)).toBe('只有标题');
    expect(bodyTextOf(wrapper)).toBeNull();
    wrapper.unmount();
  });

  it('message 走正文时，标题块不额外渲染（避免重复文本）', () => {
    const wrapper = mount(AppAlert, {
      props: { type: 'warning', title: 'DNS 解析配置提醒', message: '添加前请先确认解析记录。' },
    });
    const h = wrapper.find('h5');
    expect(h.exists()).toBe(true);
    expect(h.text()).toBe('DNS 解析配置提醒');
    expect(wrapper.text().match(/DNS 解析配置提醒/g)).toHaveLength(1);
    expect(wrapper.text().match(/添加前请先确认解析记录。/g)).toHaveLength(1);
    wrapper.unmount();
  });
});