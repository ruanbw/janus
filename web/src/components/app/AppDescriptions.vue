<template>
  <dl class="grid gap-x-8 gap-y-3" :class="column === 1 ? 'grid-cols-1' : 'grid-cols-2'">
    <slot />
  </dl>
</template>

<script setup lang="ts">
withDefaults(
  defineProps<{
    column?: number;
    size?: 'default' | 'small';
  }>(),
  { column: 1, size: 'default' },
);
</script>
