<script setup lang="ts">
import { cn } from '@/lib/utils';

const props = defineProps<{ class?: any }>();
</script>

<template>
  <tr :class="cn('border-b border-border transition-colors hover:bg-muted/50', props.class)">
    <slot />
  </tr>
</template>