/**
 * AppRadioGroup 的 v-model / change 运行时契约。
 *
 * 背景：d1 lane 放开 .vue 的 props 类型检查后，`RuleFormView.vue:728` 的
 * `@change="setScope"` 报出 TS2322：
 *   Type '(scope: string) => void' is not assignable to
 *   type '(value: string | number | undefined) => any'
 *
 * 顺着这条错误往运行时追查，发现真正的问题不在类型：
 * **AppRadioGroup 的 v-model / change 曾完全不生效。**
 *
 * 机制（已用 A/B 实测确认，非推断）：`ui/radio-group.vue` 只声明了
 * `defineProps<RadioGroupRootProps>()`，既没声明 emits 也没在模板上转抛。
 * 调用方的 `onUpdate:modelValue` 因此进入该组件的 `$attrs`，再经 fallthrough
 * 落到根 vnode —— 而 Vue 在 fallthrough 阶段调用 `filterModelListeners()`
 * （@vue/runtime-core dist ~4791）：凡是 `update:xxx` 且 `xxx` 已是本组件
 * 声明过的 prop 的键，一律从 fallthrough 里剔除，理由是「组件自己应该处理它」。
 * 但该组件并不处理，于是监听器被静默丢弃，永远到不了 reka 的 RadioGroupRoot。
 *
 * 对照组 `ui/checkbox.vue` 显式 `defineEmits` + `@update:model-value` 转抛，所以能工作。
 * 已按同一模式为 `ui/radio-group.vue` 补上转抛，本文件锁住修复后的行为。
 */
import { h } from 'vue';
import { mount } from '@vue/test-utils';

import AppRadioCard from '@/components/app/AppRadioCard.vue';
import AppRadioGroup from '@/components/app/AppRadioGroup.vue';

async function settle(): Promise<void> {
  await new Promise((r) => setTimeout(r, 10));
}

function mountGroup(modelValue: string, extra: Record<string, unknown> = {}) {
  const onChange = vi.fn();
  const onUpdate = vi.fn();
  const wrapper = mount(AppRadioGroup, {
    props: { modelValue, onChange, 'onUpdate:modelValue': onUpdate, ...extra },
    slots: {
      default: () => [
        h(AppRadioCard, { value: 'global', title: '全局' }),
        h(AppRadioCard, { value: 'links', title: '指定短链' }),
      ],
    },
    global: { components: { AppRadioCard } },
    attachTo: document.body,
  });
  return { wrapper, onChange, onUpdate };
}

describe('AppRadioGroup — v-model / change 的运行时转发', () => {
  it('点击选项触发 update:modelValue 与 change，载荷为被选中项的 value', async () => {
    const { wrapper, onChange, onUpdate } = mountGroup('global');

    const radios = wrapper.findAll('[role="radio"]');
    expect(radios.length).toBe(2);
    expect(radios[0]!.attributes('aria-checked')).toBe('true');

    await radios[1]!.trigger('click');
    await settle();

    expect(onUpdate).toHaveBeenCalledWith('links');
    expect(onChange).toHaveBeenCalledWith('links');
    // 选中态是受控的（由 modelValue 决定），此处父级未改modelValue，
    // 故 aria-checked 不翻转；受控链路由下面的 v-model 用例验证。
    expect(radios[1]!.attributes('aria-checked')).toBe('false');

    wrapper.unmount();
  });

  it('v-model 受控用法：父级 model 更新后选中态跟随', async () => {
    const Harness = {
      components: { AppRadioGroup, AppRadioCard },
      data: () => ({ scope: 'global' }),
      template: `<AppRadioGroup v-model="scope">
          <AppRadioCard value="global" title="全局" />
          <AppRadioCard value="links" title="指定短链" />
        </AppRadioGroup><div id="out">{{ scope }}</div>`,
    };
    const w = mount(Harness, { attachTo: document.body });
    expect(w.find('#out').text()).toBe('global');

    await w.findAll('[role="radio"]')[1]!.trigger('click');
    await settle();

    // 这条是本文件的核心断言：父级 model 必须真的被写回
    expect(w.find('#out').text()).toBe('links');
    w.unmount();
  });

  it('载荷恒为被选中项的 value，任何一次点击都不产生 undefined', async () => {
    const { wrapper, onChange } = mountGroup('global');

    const radios = wrapper.findAll('[role="radio"]');

    // 点第二张，再点回第一张 —— 两个方向的载荷都必须是具体的 value
    await radios[1]!.trigger('click');
    await settle();
    await wrapper.findAll('[role="radio"]')[0]!.trigger('click');
    await settle();

    expect(onChange.mock.calls.map((c) => c[0])).toEqual(['links', 'global']);
    // 这是「AppRadioGroup 把载荷声明成 `| undefined` 比实际宽」的运行时依据：
    // 单选组的取值只可能来自 AppRadioCard 那个必填的 value，永远不会是 undefined。
    onChange.mock.calls.forEach((call) => expect(call[0]).not.toBeUndefined());

    wrapper.unmount();
  });
});