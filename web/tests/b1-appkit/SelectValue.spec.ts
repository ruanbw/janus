/**
 * SelectValue 的 placeholder 回退契约。
 *
 * 背景：reka 的 SelectValue 默认插槽带 fallback（`selectedLabel.join(', ')` 或
 * `props.placeholder`），但 Vue 的 renderSlot 只在**未提供 default slot** 时用 fallback。
 * ui/select-value.vue 无条件提供 default slot（即使外层没传内容也渲染为空），
 * fallback 永不执行 → placeholder 变成纯装饰，单选 Select 无值时触发器显示空白。
 */
import { mount } from '@vue/test-utils';
import {
  SelectContent,
  SelectItem,
  SelectItemText,
  SelectPortal,
  SelectRoot,
  SelectTrigger,
  SelectViewport,
} from 'reka-ui';

import AppSelect from '@/components/app/AppSelect.vue';
import { SelectValue as UiSelectValue } from '@/components/ui/select';
import { SelectValue } from 'reka-ui';

const OPTIONS = [
  { value: 'US', label: '美国' },
  { value: 'BR', label: '巴西' },
];

/**
 * SelectValue 依赖 SelectRoot 上下文（selectedLabel 取自 rootContext.optionsSet，
 * 选项则必须在 SelectContent 内注册）；这里搭一个完整的最小 Select 结构。
 * `withSlot` 用来对比「消费方提供插槽」与「消费方不提供插槽」两条路径。
 */
const Host = {
  components: {
    SelectRoot,
    SelectTrigger,
    SelectPortal,
    SelectContent,
    SelectViewport,
    SelectItem,
    SelectItemText,
    UiSelectValue,
  },
  props: {
    modelValue: { type: String, default: '' },
    placeholder: { type: String, default: '' },
    /** true：传入会渲染出内容的插槽；'empty'：传入渲染为空的插槽；false：不传插槽 */
    slotMode: { type: [String, Boolean], default: false },
  },
  template: `
    <SelectRoot :model-value="modelValue">
      <SelectTrigger data-testid="trigger">
        <UiSelectValue :placeholder="placeholder">
          <template v-if="slotMode === true" #default="slotProps">
            <span class="from-slot">[{{ slotProps.modelValue }}|{{ slotProps.selectedLabel.join(',') }}]</span>
          </template>
          <template v-else-if="slotMode === 'empty'" #default>
            <span class="empty-slot"></span>
          </template>
        </UiSelectValue>
      </SelectTrigger>
      <SelectPortal>
        <SelectContent force-mount>
          <SelectViewport>
            <SelectItem v-for="o in options" :key="o.value" :value="o.value">
              <SelectItemText>{{ o.label }}</SelectItemText>
            </SelectItem>
          </SelectViewport>
        </SelectContent>
      </SelectPortal>
    </SelectRoot>
  `,
  data: () => ({ options: OPTIONS }),
};

/**
 * 这里不能用 stubs.Teleport：那会让 SelectContent 不挂载，选项不注册进
 * rootContext.optionsSet，SelectValue 的 selectedLabel 恒为空，测不到真实回退行为。
 */
function mountHost(props: Record<string, unknown>) {
  return mount(Host, { props, attachTo: document.body });
}

/** 触发器里 SelectValue 渲染出来的文本 */
function triggerText(wrapper: ReturnType<typeof mount>): string {
  return wrapper.find('[role="combobox"]').text();
}

describe('ui/select-value placeholder 回退', () => {
  beforeEach(() => {
    document.body.innerHTML = '';
  });

  it('消费方未提供 default slot：无值时由 reka 的 fallback 渲染 placeholder', () => {
    const wrapper = mountHost({ modelValue: '', placeholder: '留空 = 由后端按 IP 解析' });
    expect(wrapper.text()).toContain('留空 = 由后端按 IP 解析');
    wrapper.unmount();
  });

  it('消费方未提供 default slot 且无 placeholder：渲染为空，而不是 "undefined"', () => {
    const wrapper = mountHost({ modelValue: '', placeholder: '' });
    expect(wrapper.text()).toBe('');
    expect(wrapper.text()).not.toContain('undefined');
    wrapper.unmount();
  });

  it('消费方提供了 default slot：插槽优先，placeholder 不再出现', () => {
    const wrapper = mountHost({ modelValue: '', placeholder: '占位', slotMode: true });
    // 插槽渲染出的标记文本是 `|`，若 placeholder 回退同时生效会出现两段文本
    expect(wrapper.text()).toContain('|');
    expect(wrapper.text()).not.toContain('占位');
    expect(wrapper.find('.from-slot').exists()).toBe(true);
    wrapper.unmount();
  });

  it('插槽参数 modelValue 原样透传给消费方', async () => {
    const wrapper = mountHost({ modelValue: 'US', placeholder: '占位', slotMode: true });
    await wrapper.vm.$nextTick();
    await Promise.resolve();
    // 插槽文本形如 [US|...]：modelValue 必须透传，否则消费方拿不到值
    expect(wrapper.find('.from-slot').text()).toMatch(/^\[US\|/);
    wrapper.unmount();
  });

  it('有值且消费方未提供插槽：渲染选项 label，而不是空白', async () => {
    const wrapper = mountHost({ modelValue: 'US', placeholder: '占位' });
    await wrapper.vm.$nextTick();
    await Promise.resolve();
    // 本封装无条件转发 default 插槽会击穿 reka 的 fallback：有值也渲染不出 label
    expect(wrapper.find('[role="combobox"]').text()).toContain('美国');
    expect(wrapper.find('[role="combobox"]').text()).not.toContain('占位');
    wrapper.unmount();
  });

  it('消费方传入渲染为空的插槽（空元素）：placeholder 不生效，行为与 reka 一致', async () => {
    const wrapper = mountHost({ modelValue: '', placeholder: '占位', slotMode: 'empty' });
    await wrapper.vm.$nextTick();
    await Promise.resolve();
    // 锁住一个被实测确认的事实：击穿 fallback 的是**插槽渲染出真实元素**，
    // 而不是本封装是否转发 default 插槽。Vue 的 renderSlot 会把只含注释的
    // 插槽内容丢掉并退回 fallback，所以无条件的 `<slot v-bind>` 本身无害；
    // AppSelect 里真正的问题是它给 labelOf('') 渲染出了一个空 <span>。
    expect(wrapper.find('[role="combobox"]').text()).toBe('');
    wrapper.unmount();
  });

  /**
   * 结构性契约：消费方不传 default 插槽时，本封装**不能**给 SelectValue 塞一个
   * default 插槽。reka 的 placeholder 放在 SelectValue 默认插槽的 fallback 里，
   * 一旦这里凭空造出一个 default 插槽，fallback 就永远轮不到 —— 而且这是个
   * 静默失效：单选触发器只是显示空白，不报任何错。
   *
   * 断言直接查传给 reka SelectValue 的插槽存不存在，而不是靠「渲染出的文本」
   * 反推（只含注释的插槽会被 renderSlot 丢弃并退回 fallback，文本层面看不出差别，
   * 上一轮就踩了这个假绿）。
   */
  it('消费方不传 default 插槽时，不给 reka SelectValue 塞 default 插槽', () => {
    const wrapper = mountHost({ modelValue: '', placeholder: '占位', slotMode: false });
    const inner = wrapper.findComponent(SelectValue);
    expect(inner.exists()).toBe(true);
    expect(inner.vm.$slots.default).toBeUndefined();
    wrapper.unmount();
  });

  it('消费方传了 default 插槽时，插槽被原样转发给 reka SelectValue', () => {
    const wrapper = mountHost({ modelValue: '', placeholder: '占位', slotMode: true });
    const inner = wrapper.findComponent(SelectValue);
    expect(inner.vm.$slots.default).toBeTypeOf('function');
    wrapper.unmount();
  });
});

describe('AppSelect 单选清除与空值展示', () => {
  it('单选清除写回空字符串而不是 undefined', async () => {
    const wrapper = mount(AppSelect, {
      props: { modelValue: 'US', options: OPTIONS, allowClear: true, placeholder: '选国家' },
      global: { stubs: { Teleport: true } },
    });
    await wrapper.vm.$nextTick();

    const clearBtn = wrapper.find('button[aria-label="清除选择"]');
    expect(clearBtn.exists()).toBe(true);
    await clearBtn.trigger('click');

    expect(wrapper.emitted('update:modelValue')?.[0]).toEqual(['']);
    wrapper.unmount();
  });

  it('多选清除写回空数组', async () => {
    const wrapper = mount(AppSelect, {
      props: { modelValue: ['US'], options: OPTIONS, multiple: true, allowClear: true },
      global: { stubs: { Teleport: true } },
    });
    await wrapper.vm.$nextTick();

    await wrapper.find('button[aria-label="清除选择"]').trigger('click');
    expect(wrapper.emitted('update:modelValue')?.[0]).toEqual([[]]);
    wrapper.unmount();
  });

  it('无值时触发器显示 placeholder 文案（不是空白）', async () => {
    const wrapper = mount(AppSelect, {
      props: { modelValue: '', options: OPTIONS, placeholder: '留空 = 由后端按 IP 解析' },
      global: { stubs: { Teleport: true } },
    });
    await wrapper.vm.$nextTick();
    expect(triggerText(wrapper)).toContain('留空 = 由后端按 IP 解析');
    wrapper.unmount();
  });

  it('modelValue 为 undefined 时同样显示 placeholder，不出现 "undefined" 字样', async () => {
    const wrapper = mount(AppSelect, {
      props: { modelValue: undefined, options: OPTIONS, placeholder: '请选择' },
      global: { stubs: { Teleport: true } },
    });
    await wrapper.vm.$nextTick();
    expect(triggerText(wrapper)).toContain('请选择');
    expect(triggerText(wrapper)).not.toContain('undefined');
    wrapper.unmount();
  });

  it('有值时显示选项 label', async () => {
    const wrapper = mount(AppSelect, {
      props: { modelValue: 'BR', options: OPTIONS, placeholder: '请选择' },
      global: { stubs: { Teleport: true } },
    });
    await wrapper.vm.$nextTick();
    expect(triggerText(wrapper)).toContain('巴西');
    wrapper.unmount();
  });
});