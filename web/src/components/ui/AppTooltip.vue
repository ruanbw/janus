<template>
  <TooltipProvider :delay-duration="120">
    <TooltipRoot>
      <TooltipTrigger as-child><slot /></TooltipTrigger>
      <TooltipPortal>
        <TooltipContent
          :side="side"
          :side-offset="5"
          class="z-[80] max-w-xs rounded-lg bg-ink px-2.5 py-1.5 text-xs leading-relaxed text-surface shadow-lg"
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
import {
  TooltipArrow,
  TooltipContent,
  TooltipPortal,
  TooltipProvider,
  TooltipRoot,
  TooltipTrigger,
} from 'reka-ui';
import { computed } from 'vue';

const props = withDefaults(
  defineProps<{
    title?: string;
    placement?: 'top' | 'bottom' | 'left' | 'right';
  }>(),
  { placement: 'top' },
);

const side = computed(() => props.placement);
</script>
