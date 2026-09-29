<template>
  <span
    :class="tagClasses"
    :style="customStyle"
  >
    <slot />
  </span>
</template>

<script lang="ts">
import { cva } from 'class-variance-authority';

export type TagVariant = 'default' | 'secondary' | 'destructive' | 'outline';
export type TagColor =
  | 'default'
  | 'blue'
  | 'cyan'
  | 'green'
  | 'success'
  | 'orange'
  | 'warning'
  | 'red'
  | 'error'
  | 'purple'
  | 'gold'
  | 'geekblue';

export const badgeVariants = cva(
  'inline-flex select-none items-center gap-1.5 rounded-md border px-2.5 py-0.5 text-xs font-semibold whitespace-nowrap transition-colors focus:outline-none focus:ring-2 focus:ring-brand-500 focus:ring-offset-2',
  {
    variants: {
      variant: {
        default:
          'border-transparent bg-brand-600 text-white shadow-2xs hover:bg-brand-500 dark:bg-brand-500 dark:hover:bg-brand-400',
        secondary:
          'border-transparent bg-surface-strong text-ink hover:bg-surface-strong/80',
        destructive:
          'border-transparent bg-err text-white shadow-2xs hover:bg-err/90',
        outline: 'border-line text-ink bg-transparent',
      },
    },
    defaultVariants: {
      variant: 'default',
    },
  },
);
</script>

<script setup lang="ts">
import { computed } from 'vue';

import { cn } from '@/lib/utils';

const props = withDefaults(
  defineProps<{
    variant?: TagVariant;
    color?: TagColor | string;
    class?: any;
  }>(),
  { color: 'default' },
);

const PRESETS: Record<string, string> = {
  default: 'border-line bg-surface-strong/70 text-ink-soft dark:bg-surface-strong',
  blue: 'border-blue-200 bg-blue-50 text-blue-700 dark:border-blue-500/30 dark:bg-blue-500/10 dark:text-blue-300',
  cyan: 'border-cyan-200 bg-cyan-50 text-cyan-700 dark:border-cyan-500/30 dark:bg-cyan-500/10 dark:text-cyan-300',
  green:
    'border-green-200 bg-green-50 text-green-700 dark:border-green-500/30 dark:bg-green-500/10 dark:text-green-300',
  success:
    'border-green-200 bg-green-50 text-green-700 dark:border-green-500/30 dark:bg-green-500/10 dark:text-green-300',
  orange:
    'border-orange-200 bg-orange-50 text-orange-700 dark:border-orange-500/30 dark:bg-orange-500/10 dark:text-orange-300',
  warning:
    'border-orange-200 bg-orange-50 text-orange-700 dark:border-orange-500/30 dark:bg-orange-500/10 dark:text-orange-300',
  red: 'border-red-200 bg-red-50 text-red-700 dark:border-red-500/30 dark:bg-red-500/10 dark:text-red-300',
  error:
    'border-red-200 bg-red-50 text-red-700 dark:border-red-500/30 dark:bg-red-500/10 dark:text-red-300',
  purple:
    'border-purple-200 bg-purple-50 text-purple-700 dark:border-purple-500/30 dark:bg-purple-500/10 dark:text-purple-300',
  gold: 'border-amber-200 bg-amber-50 text-amber-700 dark:border-amber-500/30 dark:bg-amber-500/10 dark:text-amber-300',
  geekblue:
    'border-indigo-200 bg-indigo-50 text-indigo-700 dark:border-indigo-500/30 dark:bg-indigo-500/10 dark:text-indigo-300',
};

const tagClasses = computed(() => {
  if (props.variant !== undefined) {
    return cn(badgeVariants({ variant: props.variant }), props.class);
  }
  const isCustomHex = props.color && /^#[0-9a-fA-F]{6}$/.test(props.color);
  return cn(
    'inline-flex select-none items-center gap-1.5 rounded-md border px-2.5 py-0.5 text-xs font-semibold whitespace-nowrap transition-colors',
    isCustomHex ? '' : (PRESETS[props.color] ?? PRESETS.default),
    props.class,
  );
});

const customStyle = computed(() => {
  if (props.variant !== undefined) return undefined;
  if (PRESETS[props.color] !== undefined) return undefined;
  const hex = props.color;
  if (/^#[0-9a-fA-F]{6}$/.test(hex) === false) return undefined;
  return {
    color: hex,
    borderColor: hex + '55',
    backgroundColor: hex + '14',
  };
});
</script>
