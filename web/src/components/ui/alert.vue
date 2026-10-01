<script lang="ts">
import { cva } from 'class-variance-authority';

export type AlertVariant = 'default' | 'destructive';

export const alertVariants = cva(
  'relative grid w-full grid-cols-[0_1fr] items-start gap-3 rounded-lg border p-4 text-sm has-[>svg]:grid-cols-[calc(var(--spacing)*4)_1fr]',
  {
    variants: {
      variant: {
        default: 'bg-background text-foreground',
        destructive:
          'border-destructive/50 bg-destructive/10 text-destructive [&>svg]:text-destructive',
      },
    },
    defaultVariants: {
      variant: 'default',
    },
  },
);

export const alertTitleVariants = cva('mb-1 font-medium leading-none tracking-tight');

export const alertDescriptionVariants = cva('text-xs leading-relaxed opacity-90');
</script>

<script setup lang="ts">
import { cn } from '@/lib/utils';

const props = defineProps<{
  variant?: AlertVariant;
  class?: any;
}>();
</script>

<template>
  <div
    role="alert"
    :class="cn(alertVariants({ variant: props.variant }), props.class)"
  >
    <slot />
  </div>
</template>