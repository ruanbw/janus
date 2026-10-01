<template>
  <SwitchRoot
    :checked="model"
    :disabled="disabled"
    :name="name"
    :class="switchClasses"
    @update:checked="onCheckedChange"
  >
    <SwitchThumb :class="thumbClasses" />
  </SwitchRoot>
</template>

<script lang="ts">
import { cva } from 'class-variance-authority';

export type SwitchSize = 'sm' | 'default';

export const switchVariants = cva(
  'app-switch peer relative inline-flex shrink-0 items-center rounded-full border border-transparent p-[2px] shadow-xs transition-colors outline-none focus-visible:ring-[3px] focus-visible:ring-ring/50 disabled:cursor-not-allowed disabled:opacity-50 data-[state=checked]:bg-primary data-[state=unchecked]:bg-control-track hover:data-[state=unchecked]:bg-control-track-hover',
  {
    variants: {
      size: {
        default: 'h-[19px] w-[34px]',
        sm: 'h-[17px] w-[30px]',
      },
    },
    defaultVariants: {
      size: 'default',
    },
  },
);

export const switchThumbVariants = cva(
  // 关态 thumb 采用 --control-thumb,配合发丝描边 --control-thumb-edge 在明暗两套主题下均有清晰轮廓;
  // 开态统一走 --primary-foreground,随品牌色自适应高对比前景色。
  'pointer-events-none block rounded-full bg-control-thumb shadow-xs ring-1 ring-inset ring-control-thumb-edge transition-transform duration-150 ease-out data-[state=checked]:bg-primary-foreground',
  {
    variants: {
      size: {
        default: 'size-[13px] translate-x-0 data-[state=checked]:translate-x-[15px]',
        sm: 'size-[11px] translate-x-0 data-[state=checked]:translate-x-[13px]',
      },
    },
    defaultVariants: {
      size: 'default',
    },
  },
);
</script>

<script setup lang="ts">
import { computed } from 'vue';
import { SwitchRoot, SwitchThumb } from 'reka-ui';

import { cn } from '@/lib/utils';

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
  cn(switchVariants({ size: props.size }), props.class),
);

const thumbClasses = computed(() => switchThumbVariants({ size: props.size }));

function onCheckedChange(val: boolean): void {
  emit('update:modelValue', val);
  emit('update:checked', val);
  emit('change', val);
}
</script>