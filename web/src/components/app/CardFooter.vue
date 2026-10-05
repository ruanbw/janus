<template>
  <CardFooterPrimitive :class="cn(props.class)">
    <slot />
  </CardFooterPrimitive>
</template>

<script setup lang="ts">
import { CardFooter as CardFooterPrimitive } from '@/components/ui/card';

import { cn } from '@/lib/utils';

const props = defineProps<{
  class?: any;
}>();
</script>
