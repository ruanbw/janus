<template>
  <UiTooltip :delay-duration="delayDuration">
    <UiTooltipTrigger as-child><slot /></UiTooltipTrigger>
    <UiTooltipPortal>
      <UiTooltipContent :side="side" :side-offset="sideOffset">
        <slot v-if="$slots.title" name="title" />
        <template v-else>{{ title }}</template>
        <UiTooltipArrow :width="10" :height="5" />
      </UiTooltipContent>
    </UiTooltipPortal>
  </UiTooltip>
</template>

<script setup lang="ts">
import { computed } from 'vue';

import UiTooltip from '@/components/ui/tooltip.vue';
import UiTooltipArrow from '@/components/ui/tooltip-arrow.vue';
import UiTooltipContent from '@/components/ui/tooltip-content.vue';
import UiTooltipPortal from '@/components/ui/tooltip-portal.vue';
import UiTooltipTrigger from '@/components/ui/tooltip-trigger.vue';

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
</script>
