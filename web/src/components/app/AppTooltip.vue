<template>
  <UiTooltip :delay-duration="delayDuration">
    <UiTooltipTrigger as-child><slot /></UiTooltipTrigger>
    <UiTooltipPortal>
      <UiTooltipContent
      :side="side"
      :side-offset="sideOffset"
      :class="props.class"
    >
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
    /**
     * 作用在气泡内容上（不是触发器上）。
     * 原实现声明了 class 却从未使用，LinksView.vue 给长 URL 传的
     * `max-w-md whitespace-normal break-all` 整条换行样式链被静默吞掉。
     */
    class?: any;
  }>(),
  { placement: 'top', delayDuration: 120, sideOffset: 5 },
);

const side = computed(() => props.placement);
</script>
