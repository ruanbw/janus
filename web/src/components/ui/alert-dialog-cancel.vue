<script lang="ts">
import { cva } from 'class-variance-authority';

export const alertDialogCancelVariants = cva(
  'inline-flex h-9 shrink-0 select-none items-center justify-center gap-1.5 whitespace-nowrap rounded-md border border-input bg-background px-4 text-sm font-medium shadow-xs transition-colors outline-none hover:bg-accent hover:text-accent-foreground focus-visible:ring-[3px] focus-visible:ring-ring/50 disabled:pointer-events-none disabled:cursor-not-allowed disabled:opacity-50',
);
</script>

<script setup lang="ts">
import { AlertDialogCancel, type AlertDialogCancelProps } from 'reka-ui';

import { cn } from '@/lib/utils';

const props = defineProps<AlertDialogCancelProps & { class?: any }>();
</script>

<template>
  <AlertDialogCancel
    :as-child="props.asChild"
    :class="cn(alertDialogCancelVariants(), props.class)"
    :as="props.as"
  >
    <slot />
  </AlertDialogCancel>
</template>