<script setup lang="ts">
import { TooltipProvider, TooltipRoot, type TooltipProviderProps, type TooltipRootProps } from 'reka-ui';

const props = defineProps<TooltipProviderProps & TooltipRootProps>();
</script>

<template>
  <TooltipProvider
    :delay-duration="props.delayDuration"
    :skip-delay-duration="props.skipDelayDuration"
    :disable-hoverable-content="props.disableHoverableContent"
    :disable-closing-trigger="props.disableClosingTrigger"
  >
    <TooltipRoot :default-open="props.defaultOpen" :delay-duration="props.delayDuration">
      <slot />
    </TooltipRoot>
  </TooltipProvider>
</template>