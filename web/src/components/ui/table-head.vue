<script setup lang="ts">
import { cn } from '@/lib/utils';

const props = defineProps<{ class?: any }>();
</script>

<template>
  <th
    :class="
      cn(
        'h-10 px-3 text-left align-middle text-sm font-medium text-muted-foreground whitespace-nowrap',
        props.class,
      )
    "
  >
    <slot />
  </th>
</template>