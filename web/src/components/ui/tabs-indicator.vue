<script lang="ts">
import { cva } from 'class-variance-authority';

export const tabsIndicatorVariants = cva(
  'absolute left-0 z-0 h-full w-full rounded-md bg-background shadow-xs transition-all duration-200',
);
</script>

<script setup lang="ts">
import { TabsIndicator, type TabsIndicatorProps } from 'reka-ui';

import { cn } from '@/lib/utils';

const props = defineProps<TabsIndicatorProps & { class?: any }>();
</script>

<template>
  <TabsIndicator :as-child="props.asChild" :class="cn(tabsIndicatorVariants(), props.class)" :as="props.as">
    <slot />
  </TabsIndicator>
</template>