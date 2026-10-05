/**
 * 原语层封装（components/ui/*.vue）的 **emits 转发契约**。
 *
 * 背景：d1 lane 在查 AppRadioGroup 的 TS2322 时顺藤摸瓜，发现
 * `ui/radio-group.vue` 只声明 defineProps、没有 defineEmits、也没在模板上转抛，
 * 于是调用方的 `onUpdate:modelValue` 被 Vue 的 `filterModelListeners()` 从 fallthrough
 * 里静默剔除（@vue/runtime-core dist ~4791：凡 `update:xxx` 且 `xxx` 已是本组件
 * 声明过的 prop 就剔除），永远到不了 reka 的原语 —— v-model 与 change 全部静默失效。
 *
 * 关键对照：`ui/checkbox.vue` 有 `defineEmits` + `@update:model-value="emit(...)"`，
 * 所以它能工作。差别只在有没有那一行转抛。
 *
 * 本文件对每个受影响封装做 **A/B 实证**（挂载 → 点 → 断言 emit 是否触发），
 * 而非只信静态 grep：静态上「没有 defineEmits」只是嫌疑，
 * 真正要证明的是「事件确实没到调用方」。
 *
 * 用例先记录当前（缺陷）行为，补转抛后改为断言修复后行为 —— 两头都有证据。
 */
import { mount } from '@vue/test-utils';
import { CheckboxRoot, ProgressRoot, RadioGroupRoot, SwitchRoot } from 'reka-ui';
import { defineComponent, h, nextTick } from 'vue';

import { Checkbox } from '@/components/ui/checkbox';
import { Progress } from '@/components/ui/progress';
import { RadioGroup } from '@/components/ui/radio-group';
import { RadioGroupItem } from '@/components/ui/radio-group';
import { Separator } from '@/components/ui/separator';
import { Switch } from '@/components/ui/switch';

/** 点击后把 emit 记录清空前的载荷取出来 */
async function settle(): Promise<void> {
  await nextTick();
  await new Promise((r) => setTimeout(r, 10));
}

describe('ui 封装 — emits 转发：对照组（已正确转抛）', () => {
  it('checkbox.vue：点击触发 update:modelValue', async () => {
    const onUpd = vi.fn();
    const w = mount(Checkbox, {
      props: { modelValue: false, 'onUpdate:modelValue': onUpd } as never,
      attachTo: document.body,
    });

    await w.find('button').trigger('click');
    await settle();

    expect(onUpd).toHaveBeenCalledWith(true);
    w.unmount();
  });

  it('裸 reka 原语本身工作正常 —— 证明问题在封装层而非 reka', async () => {
    const onUpd = vi.fn();
    const w = mount(SwitchRoot, {
      props: { modelValue: false, 'onUpdate:modelValue': onUpd } as never,
      slots: { default: () => h('span', 'x') },
      attachTo: document.body,
    });

    await w.find('button').trigger('click');
    await settle();

    expect(onUpd).toHaveBeenCalledWith(true);
    w.unmount();
  });
});

describe('ui/radio-group.vue — update:modelValue 转发', () => {
  it('点击选项触发 update:modelValue，载荷为被选中项的 value', async () => {
    const onUpd = vi.fn();
    const w = mount(RadioGroup, {
      props: { modelValue: 'a', 'onUpdate:modelValue': onUpd } as never,
      slots: {
        default: () => [h(RadioGroupItem, { value: 'a' }), h(RadioGroupItem, { value: 'b' })],
      },
      attachTo: document.body,
    });

    expect(w.findAll('[role="radio"]').length).toBe(2);
    await w.findAll('[role="radio"]')[1]!.trigger('click');
    await settle();

    expect(onUpd).toHaveBeenCalledWith('b');
    w.unmount();
  });

  it('【回归防线】显式转抛与 A/B 对照组行为一致（不再依赖 fallthrough）', async () => {
    // 与 ui/radio-group.vue 结构一致，唯一差别是显式声明并转抛 emit
    const Fixed = defineComponent({
      components: { RadioGroupRoot },
      props: { modelValue: { type: null, required: false } },
      emits: { 'update:modelValue': (v: unknown) => true },
      setup(props, { emit, slots }) {
        return () =>
          h(RadioGroupRoot, {
            modelValue: props.modelValue,
            'onUpdate:modelValue': (v: unknown) => emit('update:modelValue', v),
          }, slots);
      },
    });

    const onUpd = vi.fn();
    const w = mount(Fixed, {
      props: { modelValue: 'a', 'onUpdate:modelValue': onUpd } as never,
      slots: {
        default: () => [h(RadioGroupItem, { value: 'a' }), h(RadioGroupItem, { value: 'b' })],
      },
      attachTo: document.body,
    });

    await w.findAll('[role="radio"]')[1]!.trigger('click');
    await settle();

    expect(onUpd).toHaveBeenCalledWith('b');
    w.unmount();
  });
});

describe('ui/switch.vue — update:modelValue 转发', () => {
  it('点击触发 update:modelValue', async () => {
    const onUpd = vi.fn();
    const w = mount(Switch, {
      props: { modelValue: false, 'onUpdate:modelValue': onUpd } as never,
      attachTo: document.body,
    });

    expect(w.find('button').exists()).toBe(true);
    await w.find('button').trigger('click');
    await settle();

    expect(onUpd).toHaveBeenCalledWith(true);
    w.unmount();
  });

  it('【回归防线】显式转抛与 A/B 对照组行为一致', async () => {
    const Fixed = defineComponent({
      components: { SwitchRoot },
      props: { modelValue: { type: Boolean, required: false } },
      emits: { 'update:modelValue': (v: unknown) => true },
      setup(props, { emit, slots }) {
        return () =>
          h(SwitchRoot, {
            modelValue: props.modelValue,
            'onUpdate:modelValue': (v: unknown) => emit('update:modelValue', v),
          }, slots);
      },
    });

    const onUpd = vi.fn();
    const w = mount(Fixed, {
      props: { modelValue: false, 'onUpdate:modelValue': onUpd } as never,
      attachTo: document.body,
    });

    await w.find('button').trigger('click');
    await settle();

    expect(onUpd).toHaveBeenCalledWith(true);
    w.unmount();
  });
});

describe('ui/progress.vue — update:modelValue 转发', () => {
  it('裸 reka ProgressRoot 本身会发 update:modelValue（说明封装层必须转抛）', async () => {
    const onUpd = vi.fn();
    const w = mount(ProgressRoot, {
      props: { modelValue: 10, max: 100, 'onUpdate:modelValue': onUpd } as never,
      attachTo: document.body,
    });

    // 直接调原语的 API 触发一次变更，绕开「靠 hover 之类的交互驱动」
    (w.vm as unknown as { $emit: (e: string, v: unknown) => void }).$emit('update:modelValue', [42]);
    await settle();

    // ProgressRoot 自身是声明了 emits 的，$emit 能落到调用方
    expect(onUpd).toHaveBeenCalledWith([42]);
    w.unmount();
  });

  it('ui/progress.vue 转发 update:modelValue / update:max 到调用方', async () => {
    const onUpd = vi.fn();
    const onMax = vi.fn();
    const w = mount(Progress, {
      props: {
        modelValue: 10,
        max: 100,
        'onUpdate:modelValue': onUpd,
        'onUpdate:max': onMax,
      } as never,
      attachTo: document.body,
    });

    const root = w.findComponent(ProgressRoot);
    expect(root.exists()).toBe(true);
    root.vm.$emit('update:modelValue', [42]);
    root.vm.$emit('update:max', 200);
    await settle();

    expect(onUpd).toHaveBeenCalledWith([42]);
    expect(onMax).toHaveBeenCalledWith(200);
    w.unmount();
  });

  it('ui/progress.vue 净化逻辑本身正确（不属本次修复范围，仅记录现状）', () => {
    const w = mount(Progress, {
      props: { modelValue: 9999, max: 100 },
      attachTo: document.body,
    });
    const root = w.findComponent(ProgressRoot);
    // 超界值被夹到 max
    expect(root.props('modelValue')).toBe(100);

    const w2 = mount(Progress, { props: { modelValue: -5, max: 100 }, attachTo: document.body });
    expect(w2.findComponent(ProgressRoot).props('modelValue')).toBe(0);

    const w3 = mount(Progress, { props: { modelValue: null, max: 100 }, attachTo: document.body });
    // null 是 reka 语义里的「不确定进度」，原样透传
    expect(w3.findComponent(ProgressRoot).props('modelValue')).toBeNull();

    w.unmount(); w2.unmount(); w3.unmount();
  });
});

describe('ui/radio-group-item.vue — select 转发', () => {
  it('select 不经 update: 前缀，fallthrough 未被过滤，事件能到达调用方', async () => {
    // 与 radio-group.vue 的关键区别：filterModelListeners 只剔除 `update:xxx`
    // 形态的键，`select` 不在其中，因此虽然封装没写 defineEmits，
    // 事件仍能经 fallthrough 到达调用方 —— 无需修改。
    const onSelect = vi.fn();
    const w = mount(RadioGroup, {
      props: { modelValue: 'a' },
      slots: { default: () => h(RadioGroupItem, { value: 'a', onSelect }) },
      attachTo: document.body,
    });

    w.findComponent(RadioGroupItem).vm.$emit('select', { originalEvent: null });
    await settle();

    expect(onSelect).toHaveBeenCalledTimes(1);
    w.unmount();
  });
});

describe('ui/separator.vue — 无需修改', () => {
  it('reka 的 Separator 根本没有 emits，故不存在转发缺口', () => {
    // reka 的 Separator 源码里没有 emits 字段，其 d.ts 也不导出 SeparatorEmits。
    // 封装无从转发，不属于缺陷。
    const w = mount(Separator, { props: {}, attachTo: document.body });
    expect((w.vm.$options as unknown as { emits?: unknown }).emits ?? null).toBeNull();
    w.unmount();
  });
});