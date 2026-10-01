<template>
  <div class="flex flex-col items-center gap-3 py-10 text-center">
    <span class="flex h-12 w-12 items-center justify-center rounded-full bg-surface-strong text-ink-faint">
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
