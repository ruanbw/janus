<template>
  <h3 :class="cn('text-base font-semibold leading-none tracking-tight text-ink', props.class)">
    <slot />
  </h3>
</template>

<script setup lang="ts">
import { cn } from '@/lib/utils';

const props = defineProps<{
  class?: any;
}>();
</script>
