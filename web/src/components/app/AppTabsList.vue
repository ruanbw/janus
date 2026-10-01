<template>
  <TabsList :loop="loop" :class="cn(listClasses, props.class)">
    <slot />
  </TabsList>
</template>

<script setup lang="ts">
import { computed } from 'vue';

import TabsList from '@/components/ui/tabs-list.vue';
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

/**
 * pill 直接用原语默认外观:原语的 bg-muted / text-muted-foreground
 * 与本组件原先的 bg-surface-strong / text-ink-soft 在两套主题下取值相同。
 * line 只需抹掉药丸底色与内边距,换成下边框。
 */
const listClasses = computed(() =>
  props.variant === 'line'
    ? 'flex h-auto rounded-none border-b border-line bg-transparent p-0'
    : '',
);
</script>