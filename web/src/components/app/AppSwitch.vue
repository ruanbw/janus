<template>
  <UiSwitch
    :id="fieldId"
    :model-value="model"
    :disabled="disabled"
    :name="name"
    :class="switchClasses"
    @update:model-value="onCheckedChange"
  />
</template>

<script lang="ts">
import { switchThumbVariants, switchVariants as uiSwitchVariants } from '@/components/ui/switch.vue';

import { cn } from '@/lib/utils';

export type SwitchSize = 'sm' | 'default';

export { switchThumbVariants };

/**
 * 轨道尺寸。default 档必须显式给尺寸：ui/switch 原语的根元素只有 p-0.5,
 * 宽度是 shrink-to-fit(thumb size-4 + padding),只有 20px,而 thumb 勾选态
 * translate-x-4 位移 16px,右边缘到 34px —— 会直接溢出轨道压到旁边文案上。
 * 两档都在项目层锁定 h/w,thumb 用后代选择器同步缩放。
 */
const SWITCH_SIZE_CLASSES: Record<SwitchSize, string> = {
  default: 'h-5 w-9 p-0.5',
  sm: 'h-[17px] w-[30px] p-0 [&>span]:size-[11px] [&>span]:data-[state=checked]:translate-x-[13px]',
};

export const switchVariants = (options: {
  size?: SwitchSize;
  class?: any;
} = {}): string => {
  return cn(
    uiSwitchVariants(),
    // 原语层没有关态轨道的 hover 反馈，用控件状态层的 --control-track-hover 补回
    'hover:data-[state=unchecked]:bg-control-track-hover',
    SWITCH_SIZE_CLASSES[options.size ?? 'default'],
    options.class,
  );
};
</script>

<script setup lang="ts">
import { computed } from 'vue';

import UiSwitch from '@/components/ui/switch.vue';

import { useFormItem } from './form';

const props = withDefaults(
  defineProps<{
    modelValue?: boolean;
    checked?: boolean;
    name?: string;
    disabled?: boolean;
    size?: SwitchSize;
    /** 显式 id；缺省时自动跟随外层 AppFormItem */
    id?: string;
    class?: any;
  }>(),
  { disabled: false },
);

const emit = defineEmits<{
  'update:modelValue': [val: boolean];
  'update:checked': [val: boolean];
  change: [val: boolean];
}>();

const model = computed(() => {
  if (props.modelValue !== undefined) return props.modelValue;
  if (props.checked !== undefined) return props.checked;
  return false;
});

/** 显式 id 优先；否则跟随所属 AppFormItem，让外层 <label for> 能命中 */
const formItem = useFormItem();
const fieldId = computed(() => props.id ?? formItem?.id);

const switchClasses = computed(() =>
  // 关态 thumb 用 --control-thumb + 发丝描边 --control-thumb-edge，与 --primary-foreground
  // 的开态一起构成原语默认外观；这里只补项目语义层的轨道色与尺寸。
  switchVariants({ size: props.size, class: props.class }),
);

function onCheckedChange(val: boolean): void {
  emit('update:modelValue', val);
  emit('update:checked', val);
  emit('change', val);
}
</script>
