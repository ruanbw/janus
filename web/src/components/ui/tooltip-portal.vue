<script setup lang="ts">
import { TooltipPortal, type TooltipPortalProps } from 'reka-ui';

const props = defineProps<TooltipPortalProps>();
</script>

<template>
  <TooltipPortal :to="props.to" :disabled="props.disabled">
    <slot />
  </TooltipPortal>
</template>