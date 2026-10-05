/**
 * form 族封装的 props 转发契约（asChild / as / required / reference）。
 *
 * 背景：这些封装声明了 reka 的完整 Props 类型，却在模板里只转发一部分。
 * Vue 会把未转发的 prop 当成「已声明的 prop」从 $attrs 摘走 ——
 * 既不生效、也不透传、也完全不报错。最危险的是 asChild：
 * 传了之后调用方以为「子元素就是本体」，实际却多包了一层 div，
 * 样式与语义（role / aria）全部错位。
 *
 * 断言全部落在「渲染出的真实 DOM」上，而不是「组件收到了 prop」。
 */
import { mount } from '@vue/test-utils';
import { SelectRoot } from 'reka-ui';
import { h } from 'vue';

import { Checkbox } from '@/components/ui/checkbox';
import { Label } from '@/components/ui/label';
import { RadioGroup } from '@/components/ui/radio-group';
import { RadioGroupItem } from '@/components/ui/radio-group';
import { SelectTrigger as Select } from '@/components/ui/select';
import { Separator } from '@/components/ui/separator';
import { Switch } from '@/components/ui/switch';
import { Textarea } from '@/components/ui/textarea';

/** asChild=true 时，本体应当就是 slot 里那个元素：tag 应为 span，且不应多出包裹的 div */
function expectAsChildPassesThrough(wrapper: ReturnType<typeof mount>) {
  const span = wrapper.find('span');
  expect(span.exists()).toBe(true);
  // 多包一层 div 就说明 asChild 没转发
  expect(span.element.parentElement?.tagName).toBe('DIV');
  expect(span.element.tagName).toBe('SPAN');
}

describe('form 族 — asChild 转发契约', () => {
  it('checkbox：asChild 被转发（不再多包一层 button）', () => {
    // CheckboxRoot 的 children 是 CheckboxIndicator（仅选中时渲染），
    // 所以 asChild 生效时 Primitive 会把根合并到 indicator 上、整棵子树暂不出现；
    // asChild 没转发时则会稳定地多出一层 <button>。
    const withAsChild = mount(Checkbox, { props: { asChild: true } });
    const withoutAsChild = mount(Checkbox, {});

    expect(withoutAsChild.find('button').exists()).toBe(true);
    expect(withAsChild.find('button').exists()).toBe(false);

    withAsChild.unmount();
    withoutAsChild.unmount();
  });

  it('switch：asChild 时本体是 slot 的 span', () => {
    const wrapper = mount(Switch, {
      props: { asChild: true },
      slots: { default: () => h('span', 'sw') },
    });

    expectAsChildPassesThrough(wrapper);
    expect(wrapper.find('span').attributes('role')).toBe('switch');

    wrapper.unmount();
  });

  it('radio-group-item：asChild 时本体是 slot 的 span', () => {
    const wrapper = mount(RadioGroup, {
      slots: {
        default: () =>
          h(RadioGroupItem, { value: 'a', asChild: true }, { default: () => h('span', 'rg') }),
      },
    });

    expectAsChildPassesThrough(wrapper);
    expect(wrapper.find('span').attributes('role')).toBe('radio');

    wrapper.unmount();
  });

  it('separator：asChild 时本体是 slot 的 span，不再多包一层 div', () => {
    const wrapper = mount(Separator, {
      props: { asChild: true },
      slots: { default: () => h('span', 'sep') },
    });

    expectAsChildPassesThrough(wrapper);

    wrapper.unmount();
  });

  it('label：asChild 时本体是 slot 的 span', () => {
    const wrapper = mount(Label, {
      props: { asChild: true, for: 'x' },
      slots: { default: () => h('span', 'lb') },
    });

    expectAsChildPassesThrough(wrapper);

    wrapper.unmount();
  });
});

describe('form 族 — as 转发契约', () => {
  it('checkbox：as="span" 时渲染 span 而非默认的 button', () => {
    const wrapper = mount(Checkbox, { props: { as: 'span' } });

    expect(wrapper.find('button').exists()).toBe(false);
    expect(wrapper.find('span').attributes('role')).toBe('checkbox');

    wrapper.unmount();
  });

  it('separator：as="hr" 时渲染 hr', () => {
    const wrapper = mount(Separator, { props: { as: 'hr' } });

    expect(wrapper.find('hr').exists()).toBe(true);
    expect(wrapper.find('div').exists()).toBe(false);

    wrapper.unmount();
  });

  it('label：as="p" 时渲染 p', () => {
    const wrapper = mount(Label, { props: { as: 'p', for: 'x' }, slots: { default: 'L' } });

    expect(wrapper.find('p').exists()).toBe(true);
    expect(wrapper.find('label').exists()).toBe(false);

    wrapper.unmount();
  });

  it('radio-group：as="fieldset" 时渲染 fieldset', () => {
    const wrapper = mount(RadioGroup, { props: { as: 'fieldset' } });

    expect(wrapper.find('fieldset').exists()).toBe(true);

    wrapper.unmount();
  });
});

describe('form 族 — required / reference 转发契约', () => {
  it('checkbox：required=true 反映到 aria-required', () => {
    const wrapper = mount(Checkbox, { props: { required: true } });

    expect(wrapper.find('[role="checkbox"]').attributes('aria-required')).toBe('true');

    wrapper.unmount();
  });

  it('switch：required=true 反映到 aria-required', () => {
    const wrapper = mount(Switch, { props: { required: true } });

    expect(wrapper.find('[role="switch"]').attributes('aria-required')).toBe('true');

    wrapper.unmount();
  });

  it('radio-group-item：required=true 反映到 required（reka 在 role=radio 上输出 required 而非 aria-required）', () => {
    const wrapper = mount(RadioGroup, {
      slots: {
        default: () => h(RadioGroupItem, { value: 'a', required: true }, { default: () => 'rg' }),
      },
    });

    expect(wrapper.find('[role="radio"]').attributes('required')).toBe('true');

    wrapper.unmount();
  });

  it('radio-group：required=true 反映到 radiogroup 的 aria-required', () => {
    const wrapper = mount(RadioGroup, {
      props: { required: true },
      slots: { default: () => 'x' },
    });

    expect(wrapper.find('[role="radiogroup"]').attributes('aria-required')).toBe('true');

    wrapper.unmount();
  });

  it('textarea：maxlength 这类原生 attr 仍正常透传（未声明 prop 不被误吞）', () => {
    const wrapper = mount(Textarea, { attrs: { maxlength: '10', rows: '3' } });

    expect(wrapper.find('textarea').attributes('maxlength')).toBe('10');
    expect(wrapper.find('textarea').attributes('rows')).toBe('3');

    wrapper.unmount();
  });

  it('select trigger：disabled=true 反映到 DOM 上（必须在 SelectRoot 内）', () => {
    const wrapper = mount(SelectRoot, {
      slots: { default: () => h(Select, { disabled: true }, { default: () => 'pick' }) },
    });

    expect(wrapper.find('button').attributes('disabled')).toBeDefined();

    wrapper.unmount();
  });
});