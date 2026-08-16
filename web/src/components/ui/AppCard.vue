<template>
  <div
    class="rounded-2xl bg-surface"
    :class="[bordered ? 'border border-line' : 'border border-transparent', size === 'small' ? 'p-4' : 'p-5']"
  >
    <h3 v-if="title" class="mb-4 text-[15px] font-semibold text-ink">{{ title }}</h3>
    <div :class="size === 'small' ? '' : ''"><slot /></div>
  </div>
</template>

<script setup lang="ts">
withDefaults(
  defineProps<{
    title?: string;
    bordered?: boolean;
    size?: 'default' | 'small';
  }>(),
  { bordered: true, size: 'default' },
);
</script>
