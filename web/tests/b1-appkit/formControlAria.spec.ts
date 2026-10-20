/**
 * 表单控件的无障碍名必须落在**控件本身**。
 *
 * 被钉住的不变式（Given-When-Then）：
 *   Given 调用方写了 `:aria-label="..."`（表格行勾选框、无可见 label 的搜索框）
 *   When  组件渲染成 DOM
 *   Then  该属性出现在 `role=checkbox` 的按钮 / `<input>` 上，
 *         而不是落在外面那层 <label> 或 <div> 上，也不是干脆消失
 *
 * 为什么值得单独钉住：这两个包装组件（AppCheckbox / AppInput）都以外层元素为根，
 * 而 `aria-label` 又恰好都是「必须挂在可聚焦控件上才生效」的属性。落错位置时
 * 界面看起来毫无异常：读屏软件只会念出一个没有名字的复选框/文本框，
 * 视觉用户永远发现不了。
 *
 * 真实触发路径（修复前）：
 *   - AppCheckbox 把 prop 声明成带引号的 `'aria-label'`，Vue 在 props 里归一化成
 *     `ariaLabel`，模板却读 `$props['aria-label']` —— 永远 undefined，属性直接消失；
 *   - AppInput 根本没声明该 prop，attr 走 fallthrough 落在最外层 <div> 上。
 */
import { mount } from '@vue/test-utils';

import AppCheckbox from '@/components/app/AppCheckbox.vue';
import AppInput from '@/components/app/AppInput.vue';
import AppSelect from '@/components/app/AppSelect.vue';

describe('控件的无障碍名', () => {
  it('Given AppCheckbox 传了 aria-label, Then 它落在 role=checkbox 的按钮上', () => {
    const wrapper = mount(AppCheckbox, { props: { ariaLabel: '选中短链 abc123' } });

    const box = wrapper.get('[role="checkbox"]');
    expect(box.attributes('aria-label')).toBe('选中短链 abc123');
    wrapper.unmount();
  });

  it('Given 调用方写 kebab-case 的 :aria-label, Then 同样生效', () => {
    // 调用点实际都是 `:aria-label="..."` 写法，kebab → camel 的映射不能丢
    const wrapper = mount(AppCheckbox, {
      attrs: { 'aria-label': '选中短链 def456' },
    });

    expect(wrapper.get('[aria-label="选中短链 def456"]').attributes('role')).toBe('checkbox');
    wrapper.unmount();
  });

  it('Given AppInput 传了 aria-label, Then 它落在 <input> 上而不是外层 div', () => {
    const wrapper = mount(AppInput, {
      props: { modelValue: '', ariaLabel: '搜索短链' },
    });

    const input = wrapper.get('input');
    expect(input.attributes('aria-label')).toBe('搜索短链');
    // 外层 div 不该再重复携带：同一个名字挂在两个节点上会让可访问名来源难以判断
    expect(wrapper.element.hasAttribute('aria-label')).toBe(false);
    wrapper.unmount();
  });

  it('Given AppSelect 传了 aria-label, Then 它落在 combobox 触发器上', () => {
    const wrapper = mount(AppSelect, {
      props: { options: [{ value: 'all', label: '全部' }], ariaLabel: '按类型过滤' },
    });

    expect(wrapper.get('[role="combobox"]').attributes('aria-label')).toBe('按类型过滤');
    wrapper.unmount();
  });

  it('Given 没有传无障碍名, Then 不渲染空的 aria-label(空名字等于没名字)', () => {
    const checkbox = mount(AppCheckbox);
    expect(checkbox.get('[role="checkbox"]').attributes('aria-label')).toBeUndefined();
    checkbox.unmount();

    const input = mount(AppInput);
    expect(input.get('input').attributes('aria-label')).toBeUndefined();
    input.unmount();
  });
});
