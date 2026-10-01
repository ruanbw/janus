<template>
  <TabsList
    :loop="loop"
    :class="
      cn(
        variant === 'line'
          ? 'flex border-b border-line gap-2'
          : 'inline-flex h-9 items-center justify-center rounded-lg bg-surface-strong p-1 text-ink-soft',
        props.class,
      )
    "
  >
    <slot />
  </TabsList>
</template>

<script setup lang="ts">
import { TabsList } from 'reka-ui';

import { cn } from '@/lib/utils';

const props = withDefaults(
  defineProps<{
    loop?: boolean;
    variant?: 'line' | 'pill';
    class?: any;
  }>(),
  {
    loop: true,
    variant: 'line',
  },
);
</script>
