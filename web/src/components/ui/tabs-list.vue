<script lang="ts">
import { cva } from 'class-variance-authority';

export const tabsListVariants = cva(
  'relative inline-flex h-9 shrink-0 items-center justify-center rounded-lg bg-muted p-1 text-muted-foreground',
);
</script>

<script setup lang="ts">
import { TabsList, type TabsListProps } from 'reka-ui';

import { cn } from '@/lib/utils';

const props = defineProps<TabsListProps & { class?: any }>();
</script>

<template>
  <TabsList
    :loop="props.loop"
    :as-child="props.asChild"
    :class="cn(tabsListVariants(), props.class)"
  >
    <slot />
  </TabsList>
</template>