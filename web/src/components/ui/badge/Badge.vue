<script lang="ts">
import { cva } from 'class-variance-authority';

export type BadgeVariant = 'default' | 'secondary' | 'destructive' | 'outline';

export const badgeVariants = cva(
  'inline-flex shrink-0 select-none items-center gap-1.5 whitespace-nowrap rounded-md border px-2.5 py-0.5 text-xs font-semibold transition-colors outline-none focus-visible:ring-[3px] focus-visible:ring-ring/50',
  {
    variants: {
      variant: {
        default: 'border-transparent bg-primary text-primary-foreground shadow-xs',
        secondary: 'border-transparent bg-secondary text-secondary-foreground',
        destructive: 'border-transparent bg-destructive text-destructive-foreground shadow-xs',
        outline: 'border-input text-foreground',
      },
    },
    defaultVariants: {
      variant: 'default',
    },
  },
);
</script>

<script setup lang="ts">
import { Primitive, type PrimitiveProps } from 'reka-ui';

import { cn } from '@/lib/utils';

const props = defineProps<
  PrimitiveProps & {
    variant?: BadgeVariant;
    class?: any;
  }
>();
</script>

<template>
  <Primitive
    :as="props.as ?? 'span'"
    :as-child="props.asChild"
    :class="cn(badgeVariants({ variant: props.variant }), props.class)"
  >
    <slot />
  </Primitive>
</template>