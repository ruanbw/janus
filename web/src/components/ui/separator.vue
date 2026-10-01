<script lang="ts">
import { cva } from 'class-variance-authority';

export const separatorVariants = cva('shrink-0 bg-border', {
  variants: {
    orientation: {
      horizontal: 'h-px w-full',
      vertical: 'h-full w-px',
    },
  },
  defaultVariants: {
    orientation: 'horizontal',
  },
});
</script>

<script setup lang="ts">
import { Separator, type SeparatorProps } from 'reka-ui';

import { cn } from '@/lib/utils';

const props = defineProps<SeparatorProps & { class?: any }>();
</script>

<template>
  <Separator
    :orientation="props.orientation"
    :decorative="props.decorative"
    :as-child="props.asChild"
    :class="cn(separatorVariants({ orientation: props.orientation }), props.class)"
  >
    <slot />
  </Separator>
</template>