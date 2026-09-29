<template>
  <TooltipProvider :delay-duration="delayDuration">
    <TooltipRoot>
      <TooltipTrigger as-child><slot /></TooltipTrigger>
      <TooltipPortal>
        <TooltipContent
          :side="side"
          :side-offset="sideOffset"
          :class="tooltipClasses"
        >
          <slot v-if="$slots.title" name="title" />
          <template v-else>{{ title }}</template>
          <TooltipArrow class="fill-ink" :width="10" :height="5" />
        </TooltipContent>
      </TooltipPortal>
    </TooltipRoot>
  </TooltipProvider>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import {
  TooltipArrow,
  TooltipContent,
  TooltipPortal,
  TooltipProvider,
  TooltipRoot,
  TooltipTrigger,
} from 'reka-ui';

import { cn } from '@/lib/utils';

const props = withDefaults(
  defineProps<{
    title?: string;
    placement?: 'top' | 'bottom' | 'left' | 'right';
    delayDuration?: number;
    sideOffset?: number;
    class?: any;
  }>(),
  { placement: 'top', delayDuration: 120, sideOffset: 5 },
);

const side = computed(() => props.placement);

const tooltipClasses = computed(() => {
  return cn(
    'z-[80] max-w-xs overflow-hidden rounded-md bg-ink px-3 py-1.5 text-xs leading-relaxed text-surface shadow-md select-none',
    'data-[state=delayed-open]:animate-in data-[state=closed]:animate-out data-[state=closed]:fade-out-0 data-[state=delayed-open]:fade-in-0 data-[state=closed]:zoom-out-95 data-[state=delayed-open]:zoom-in-95',
    'data-[side=bottom]:slide-in-from-top-2 data-[side=left]:slide-in-from-right-2 data-[side=right]:slide-in-from-left-2 data-[side=top]:slide-in-from-bottom-2',
    props.class,
  );
});
</script>
