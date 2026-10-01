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
  | 'brand'
  | 'info'
  | 'ok'
  | 'warn'
  | 'err'
  // 历史别名兼容
  | 'success'
  | 'warning'
  | 'error';

export const badgeVariants = cva(
  'inline-flex select-none items-center gap-1.5 rounded-md border px-2.5 py-0.5 text-xs font-semibold whitespace-nowrap transition-colors outline-none focus-visible:ring-[3px] focus-visible:ring-ring/50',
  {
    variants: {
      variant: {
        default:
          'border-transparent bg-primary text-primary-foreground shadow-2xs hover:bg-primary/90',
        secondary:
          'border-transparent bg-secondary text-secondary-foreground hover:bg-secondary/80',
        destructive:
          'border-transparent bg-destructive text-destructive-foreground shadow-2xs hover:bg-destructive/90',
        outline: 'border-input text-foreground bg-transparent',
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
  default: 'border-line bg-surface-strong text-ink-soft',
  brand: 'border-primary/30 bg-primary/10 text-primary',
  info: 'border-info/30 bg-info/10 text-info',
  ok: 'border-ok/30 bg-ok/10 text-ok',
  warn: 'border-warn/30 bg-warn/10 text-warn',
  err: 'border-err/30 bg-err/10 text-err',
  // 兼容别名映射
  success: 'border-ok/30 bg-ok/10 text-ok',
  warning: 'border-warn/30 bg-warn/10 text-warn',
  error: 'border-err/30 bg-err/10 text-err',
  green: 'border-ok/30 bg-ok/10 text-ok',
  orange: 'border-warn/30 bg-warn/10 text-warn',
  red: 'border-err/30 bg-err/10 text-err',
  blue: 'border-primary/30 bg-primary/10 text-primary',
  cyan: 'border-info/30 bg-info/10 text-info',
  purple: 'border-primary/30 bg-primary/10 text-primary',
  geekblue: 'border-primary/30 bg-primary/10 text-primary',
  gold: 'border-warn/30 bg-warn/10 text-warn',
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
