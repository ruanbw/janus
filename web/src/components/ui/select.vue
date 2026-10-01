<script lang="ts">
import { cva } from 'class-variance-authority';

export const selectTriggerVariants = cva(
  'flex h-9 w-full items-center justify-between gap-2 rounded-md border border-input bg-background px-3 py-2 text-sm shadow-xs transition-colors data-[placeholder]:text-muted-foreground outline-none focus-visible:border-ring focus-visible:ring-[3px] focus-visible:ring-ring/50 disabled:cursor-not-allowed disabled:opacity-50 [&>span]:line-clamp-1 [&>span]:text-left',
);
</script>

<script setup lang="ts">
import { SelectTrigger, type SelectTriggerProps } from 'reka-ui';

import { cn } from '@/lib/utils';

const props = defineProps<SelectTriggerProps & { class?: any }>();
</script>

<template>
  <SelectTrigger
    :disabled="props.disabled"
    :as="props.as"
    :as-child="props.asChild"
    :class="cn(selectTriggerVariants(), props.class)"
  >
    <slot />
  </SelectTrigger>
</template>