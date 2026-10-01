<template>
  <CardTitlePrimitive :class="cn('text-base text-ink', props.class)">
    <slot />
  </CardTitlePrimitive>
</template>

<script setup lang="ts">
import CardTitlePrimitive from '@/components/ui/card-title.vue';

import { cn } from '@/lib/utils';

const props = defineProps<{
  class?: any;
}>();
</script>
