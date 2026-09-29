<template>
  <p :class="cn('text-sm text-ink-faint leading-relaxed', props.class)">
    <slot />
  </p>
</template>

<script setup lang="ts">
import { cn } from '@/lib/utils';

const props = defineProps<{
  class?: any;
}>();
</script>
