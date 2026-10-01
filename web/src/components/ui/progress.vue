<script lang="ts">
import { cva } from 'class-variance-authority';

export const progressVariants = cva(
  'relative h-2 w-full overflow-hidden rounded-full bg-muted',
);

export const progressIndicatorVariants = cva(
  'h-full w-full flex-1 rounded-full bg-primary transition-transform duration-300 ease-in-out',
);
</script>

<script setup lang="ts">
import { ProgressIndicator, ProgressRoot, type ProgressRootProps } from 'reka-ui';

import { cn } from '@/lib/utils';

const props = defineProps<ProgressRootProps & { class?: any; indicatorClass?: any }>();
</script>

<template>
  <ProgressRoot
    :model-value="props.modelValue"
    :max="props.max"
    :get-value-label="props.getValueLabel"
    :get-value-text="props.getValueText"
    :as-child="props.asChild"
    :class="cn(progressVariants(), props.class)"
  >
    <ProgressIndicator :class="cn(progressIndicatorVariants(), props.indicatorClass)">
      <slot />
    </ProgressIndicator>
  </ProgressRoot>
</template>