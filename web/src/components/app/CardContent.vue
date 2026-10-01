<template>
  <CardContentPrimitive :class="cn(props.class)">
    <slot />
  </CardContentPrimitive>
</template>

<script setup lang="ts">
import CardContentPrimitive from '@/components/ui/card-content.vue';

import { cn } from '@/lib/utils';

const props = defineProps<{
  class?: any;
}>();
</script>
