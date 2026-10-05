<script setup lang="ts">
import { cn } from '@/lib/utils';

const props = defineProps<{ class?: any }>();
</script>

<template>
  <td :class="cn('px-3 py-2.5 align-middle text-sm', props.class)">
    <slot />
  </td>
</template>