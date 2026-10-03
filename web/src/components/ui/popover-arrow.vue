<script setup lang="ts">
import { PopoverArrow, type PopoverArrowProps } from 'reka-ui';

import { cn } from '@/lib/utils';

const props = defineProps<PopoverArrowProps & { class?: any }>();
</script>

<template>
  <PopoverArrow
    :width="props.width"
    :height="props.height"
    :rounded="props.rounded"
    :class="cn('fill-popover', props.class)"
    :as-child="props.asChild"
    :as="props.as"
  >
    <slot />
  </PopoverArrow>
</template>