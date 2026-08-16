<template>
  <div class="flex flex-wrap items-center" :style="{ gap: gapPx + 'px' }">
    <slot />
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue';

const props = withDefaults(
  defineProps<{
    size?: 'small' | 'middle' | 'large' | number;
    direction?: 'horizontal' | 'vertical';
  }>(),
  { size: 'small', direction: 'horizontal' },
);

const gapPx = computed(() => {
  if (typeof props.size === 'number') return props.size;
  switch (props.size) {
    case 'large':
      return 24;
    case 'middle':
      return 16;
    default:
      return 8;
  }
});
</script>
