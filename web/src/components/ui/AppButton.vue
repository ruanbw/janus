<template>
  <button
    :type="htmlType"
    :disabled="disabled || loading"
    :class="btnClasses"
  >
    <Loader2 v-if="loading" :size="iconSize" class="animate-spin shrink-0" />
    <span v-else-if="$slots.icon" class="inline-flex shrink-0"><slot name="icon" /></span>
    <span v-if="$slots.default" class="inline-flex items-center gap-1.5"><slot /></span>
  </button>
</template>

<script lang="ts">
import { cva } from 'class-variance-authority';

export type ButtonVariant =
  | 'default'
  | 'primary'
  | 'destructive'
  | 'outline'
  | 'secondary'
  | 'ghost'
  | 'link'
  | 'dashed';

export type ButtonType = 'primary' | 'default' | 'text' | 'dashed' | 'ghost';
export type ButtonSize = 'default' | 'small' | 'middle' | 'large' | 'sm' | 'lg' | 'icon';

export const buttonVariants = cva(
  'app-btn inline-flex select-none items-center justify-center gap-1.5 whitespace-nowrap rounded-md font-medium transition-all active:scale-[0.99] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500 focus-visible:ring-offset-2 ring-offset-surface disabled:pointer-events-none disabled:cursor-not-allowed disabled:opacity-50 [&_svg]:pointer-events-none [&_svg]:shrink-0',
  {
    variants: {
      variant: {
        default:
          'bg-brand-600 text-white shadow-xs hover:bg-brand-500 dark:bg-brand-500 dark:hover:bg-brand-400',
        primary:
          'bg-brand-600 text-white shadow-xs hover:bg-brand-500 dark:bg-brand-500 dark:hover:bg-brand-400',
        destructive:
          'border border-transparent bg-err text-white shadow-xs hover:bg-err/90 focus-visible:ring-err',
        outline:
          'border border-line bg-surface text-ink shadow-xs hover:border-brand-400 hover:bg-surface-muted hover:text-ink dark:bg-surface-strong/60 dark:hover:bg-surface-strong',
        secondary:
          'bg-surface-strong text-ink shadow-xs hover:bg-surface-strong/80',
        ghost:
          'text-ink-soft hover:bg-surface-strong hover:text-ink dark:hover:bg-surface-strong/80',
        link:
          'text-brand-600 underline-offset-4 hover:underline dark:text-brand-400',
        dashed:
          'border border-dashed border-line-strong bg-transparent text-ink-soft hover:border-brand-400 hover:text-brand-600',
      },
      size: {
        default: 'h-9 px-4 py-2 text-sm',
        sm: 'h-8 rounded-md px-3 text-xs',
        lg: 'h-10 rounded-md px-6 text-sm font-semibold',
        icon: 'h-9 w-9 p-0',
      },
    },
    defaultVariants: {
      variant: 'default',
      size: 'default',
    },
  },
);
</script>

<script setup lang="ts">
import { computed } from 'vue';
import { Loader2 } from '@lucide/vue';

import { cn } from '@/lib/utils';

const props = withDefaults(
  defineProps<{
    variant?: ButtonVariant;
    type?: ButtonType;
    size?: ButtonSize;
    danger?: boolean;
    block?: boolean;
    loading?: boolean;
    disabled?: boolean;
    htmlType?: 'button' | 'submit' | 'reset';
    class?: any;
  }>(),
  {
    type: 'default',
    danger: false,
    block: false,
    loading: false,
    disabled: false,
    htmlType: 'button',
  },
);

const resolvedVariant = computed<ButtonVariant>(() => {
  if (props.variant !== undefined) {
    return props.variant;
  }
  if (props.danger) {
    return 'destructive';
  }
  if (props.type === 'primary') {
    return 'primary';
  }
  if (props.type === 'text') {
    return 'ghost';
  }
  if (props.type === 'dashed') {
    return 'dashed';
  }
  if (props.type === 'ghost') {
    return 'ghost';
  }
  return 'outline';
});

const resolvedSize = computed<'default' | 'sm' | 'lg' | 'icon'>(() => {
  if (props.size === 'small' || props.size === 'sm') return 'sm';
  if (props.size === 'large' || props.size === 'lg') return 'lg';
  if (props.size === 'icon') return 'icon';
  return 'default';
});

const iconSize = computed(() => {
  if (resolvedSize.value === 'sm') return 14;
  if (resolvedSize.value === 'lg') return 18;
  return 15;
});

const btnClasses = computed(() => {
  return cn(
    buttonVariants({ variant: resolvedVariant.value, size: resolvedSize.value }),
    props.block && 'w-full',
    props.class,
  );
});
</script>
