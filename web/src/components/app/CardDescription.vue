<template>
  <CardDescriptionPrimitive :class="cn('text-ink-faint leading-relaxed', props.class)">
    <slot />
  </CardDescriptionPrimitive>
</template>

<script setup lang="ts">
import { CardDescription as CardDescriptionPrimitive } from '@/components/ui/card';

import { cn } from '@/lib/utils';

const props = defineProps<{
  class?: any;
}>();
</script>
