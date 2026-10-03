<template>
  <RadioGroup
    v-bind="rootProps"
    role="radiogroup"
    :aria-labelledby="formItem?.labelId"
    :class="groupClasses"
  >
    <slot />
  </RadioGroup>
</template>

<script setup lang="ts">
import { computed } from 'vue';

import RadioGroup from '@/components/ui/radio-group.vue';
import { cn } from '@/lib/utils';
import { useFormItem } from './form';

const props = withDefaults(
  defineProps<{
    modelValue?: string | number | undefined;
    disabled?: boolean;
    orientation?: 'horizontal' | 'vertical';
    name?: string;
    class?: any;
  }>(),
  { disabled: false, orientation: 'horizontal' },
);

const emit = defineEmits<{
  'update:modelValue': [value: string | number | undefined];
  change: [value: string | number | undefined];
}>();

/**
 * 一组单选按钮没有「一个」可被 label[for] 指向的控件，所以 AppFormItem 的 for 在
 * 这里注定落空。改为 role="radiogroup" + aria-labelledby 指回外层 label，
 * 读屏进入这一组时才会先念「短链类型」，而不是逐个念「单选按钮」。
 */
const formItem = useFormItem();

/**
 * reka 的 RadioGroupRoot 发的是 `AcceptableValue`（string | number | bigint |
 * Record<string, any> | null），比本组件对外声明的 `string | number | undefined` 宽。
 * `ui/radio-group.vue` 补上 emit 转发后，这个宽度就必须在这一层收掉。
 *
 * 收窄是安全的：本组件的插槽只放得下 AppRadio / AppRadioCard，两者的 `value`
 * 都是**必填**的 `string | number`，全仓 18 处用法无一例外，也没有裸reka
 * RadioGroupItem 直接塞进来的情况。故 bigint /对象 / null 在本组件内不可达。
 */
function toModelValue(value: unknown): string | number | undefined {
  return typeof value === 'string' || typeof value === 'number' ? value : undefined;
}

const rootProps = computed(() => ({
  modelValue: props.modelValue,
  disabled: props.disabled,
  orientation: props.orientation,
  name: props.name,
  'onUpdate:modelValue': (value: unknown) => {
    const next = toModelValue(value);
    emit('update:modelValue', next);
    emit('change', next);
  },
}));

const groupClasses = computed(() =>
  cn(
    'flex gap-3',
    props.orientation === 'vertical' ? 'flex-col' : 'flex-wrap items-center',
    props.class,
  ),
);
</script>
