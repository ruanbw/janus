<script lang="ts">
import { cva } from 'class-variance-authority';

export const tabsTriggerVariants = cva(
  // 药丸型(默认外观)的悬停：底色走 accent、文字转 accent-foreground,与选中态
  // 的 bg-background + shadow 形成「浮起来」的层级差。
  // 下划线型(AppTabsTrigger 的 LINE_VARIANTS)会用 hover:bg-transparent 抵消底色,
  // 只保留文字变化 —— 两条线型页签不该在悬停时糊出一块底。
  'inline-flex flex-1 select-none items-center justify-center gap-1.5 whitespace-nowrap rounded-md px-3 py-1 text-sm font-medium transition-colors outline-none hover:bg-accent hover:text-accent-foreground focus-visible:ring-[3px] focus-visible:ring-ring/50 disabled:pointer-events-none disabled:cursor-not-allowed disabled:opacity-50 data-[state=active]:bg-background data-[state=active]:text-foreground data-[state=active]:shadow-xs',
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
