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

const rootProps = computed(() => ({
  modelValue: props.modelValue,
  disabled: props.disabled,
  orientation: props.orientation,
  name: props.name,
  'onUpdate:modelValue': (value: string | number | undefined) => {
    emit('update:modelValue', value);
    emit('change', value);
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
