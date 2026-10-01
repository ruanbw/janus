<script setup lang="ts">
import { TooltipArrow, type TooltipArrowProps } from 'reka-ui';

import { cn } from '@/lib/utils';

const props = defineProps<TooltipArrowProps & { class?: any }>();
</script>

<template>
  <TooltipArrow
    :width="props.width"
    :height="props.height"
    :class="cn('fill-foreground', props.class)"
  >
    <slot />
  </TooltipArrow>
</template>