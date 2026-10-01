<script lang="ts">
import { cva } from 'class-variance-authority';

export const alertDialogActionVariants = cva(
  'inline-flex h-9 shrink-0 select-none items-center justify-center gap-1.5 whitespace-nowrap rounded-md px-4 text-sm font-medium transition-colors outline-none focus-visible:ring-[3px] focus-visible:ring-ring/50 disabled:pointer-events-none disabled:cursor-not-allowed disabled:opacity-50',
  {
    variants: {
      variant: {
        default: 'bg-primary text-primary-foreground shadow-xs hover:bg-primary/90',
        destructive:
          'bg-destructive text-destructive-foreground shadow-xs hover:bg-destructive/90',
      },
    },
    defaultVariants: {
      variant: 'default',
    },
  },
);
</script>

<script setup lang="ts">
import { AlertDialogAction, type AlertDialogActionProps } from 'reka-ui';

import { cn } from '@/lib/utils';

const props = defineProps<
  AlertDialogActionProps & {
    variant?: 'default' | 'destructive';
    class?: any;
  }
>();
</script>

<template>
  <AlertDialogAction
    :as-child="props.asChild"
    :class="cn(alertDialogActionVariants({ variant: props.variant }), props.class)"
  >
    <slot />
  </AlertDialogAction>
</template>