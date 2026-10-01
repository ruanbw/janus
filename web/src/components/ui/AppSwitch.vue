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
  'app-switch relative inline-flex shrink-0 items-center rounded-full border border-line-strong bg-surface-muted p-[2px] transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500 focus-visible:ring-offset-2 ring-offset-surface disabled:cursor-not-allowed disabled:opacity-50 data-[state=checked]:border-brand-600 data-[state=checked]:bg-brand-600 dark:data-[state=checked]:border-brand-500 dark:data-[state=checked]:bg-brand-500',
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
  'pointer-events-none block rounded-full bg-white shadow-xs transition-transform duration-150 ease-out',
  {
    variants: {
      size: {
        default: 'size-[13px] data-[state=checked]:translate-x-[15px]',
        sm: 'size-[11px] data-[state=checked]:translate-x-[13px]',
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