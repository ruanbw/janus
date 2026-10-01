<template>
  <div class="flex flex-col items-center gap-3 py-10 text-center">
    <span class="flex size-12 items-center justify-center rounded-full bg-secondary text-muted-foreground">
      <Inbox :size="22" />
    </span>
    <p class="text-xs text-ink-faint">{{ description }}</p>
  </div>
</template>

<script setup lang="ts">
import { Inbox } from '@lucide/vue';

withDefaults(
  defineProps<{
    description?: string;
  }>(),
  { description: '暂无数据' },
);
</script>
