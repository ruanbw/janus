<template>
  <UiSwitch
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
 * 轨道尺寸。default 档与 ui/switch 原生一致（p-0.5 + thumb size-4 + translate-x-4），
 * 只有 sm 档需要收窄——thumb 由 ui/switch 内部渲染，项目层拿不到它的 class，
 * 因此 sm 档用后代选择器把 thumb 一并缩小，保证轨道与滑块同步。
 */
const SWITCH_SIZE_CLASSES: Record<SwitchSize, string> = {
  default: '',
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

const props = withDefaults(
  defineProps<{
    modelValue?: boolean;
    checked?: boolean;
    name?: string;
    disabled?: boolean;
    size?: SwitchSize;
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