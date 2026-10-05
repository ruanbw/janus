<template>
  <CardContentPrimitive :class="cn(props.class)">
    <slot />
  </CardContentPrimitive>
</template>

<script setup lang="ts">
import { CardContent as CardContentPrimitive } from '@/components/ui/card';

import { cn } from '@/lib/utils';

const props = defineProps<{
  class?: any;
}>();
</script>
