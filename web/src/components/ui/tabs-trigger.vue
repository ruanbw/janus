<script lang="ts">
import { cva } from 'class-variance-authority';

export const tabsTriggerVariants = cva(
  'inline-flex flex-1 select-none items-center justify-center gap-1.5 whitespace-nowrap rounded-md px-3 py-1 text-sm font-medium transition-colors outline-none focus-visible:ring-[3px] focus-visible:ring-ring/50 disabled:pointer-events-none disabled:cursor-not-allowed disabled:opacity-50 data-[state=active]:bg-background data-[state=active]:text-foreground data-[state=active]:shadow-xs',
);
</script>

<script setup lang="ts">
import { TabsTrigger, type TabsTriggerProps } from 'reka-ui';

import { cn } from '@/lib/utils';

const props = defineProps<TabsTriggerProps & { class?: any }>();
</script>

<template>
  <TabsTrigger
    :value="props.value"
    :disabled="props.disabled"
    :as-child="props.asChild"
    :class="cn(tabsTriggerVariants(), props.class)"
  >
    <slot />
  </TabsTrigger>
</template>