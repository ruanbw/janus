<template>
  <TabsTrigger
    :value="value"
    :disabled="disabled"
    :class="
      cn(
        variant === 'line'
          ? 'flex items-center gap-2 border-b-2 -mb-px px-4 py-2.5 text-sm font-medium transition-colors outline-none focus-visible:ring-[3px] focus-visible:ring-ring/50 border-transparent text-muted-foreground hover:text-foreground cursor-pointer data-[state=active]:border-primary data-[state=active]:text-primary'
          : 'inline-flex items-center justify-center whitespace-nowrap rounded-md px-3 py-1 text-sm font-medium transition-all outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 disabled:pointer-events-none disabled:opacity-50 cursor-pointer data-[state=active]:bg-surface data-[state=active]:text-ink data-[state=active]:shadow-xs',
        props.class,
      )
    "
  >
    <slot />
  </TabsTrigger>
</template>

<script setup lang="ts">
import { TabsTrigger } from 'reka-ui';

import { cn } from '@/lib/utils';

const props = withDefaults(
  defineProps<{
    value: string | number;
    disabled?: boolean;
    variant?: 'line' | 'pill';
    class?: any;
  }>(),
  {
    disabled: false,
    variant: 'line',
  },
);
</script>
