<template>
  <CardFooterPrimitive :class="cn(props.class)">
    <slot />
  </CardFooterPrimitive>
</template>

<script setup lang="ts">
import CardFooterPrimitive from '@/components/ui/card-footer.vue';

import { cn } from '@/lib/utils';

const props = defineProps<{
  class?: any;
}>();
</script>
