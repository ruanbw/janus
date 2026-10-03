<script setup lang="ts">
import { TooltipPortal, type TooltipPortalProps } from 'reka-ui';

const props = defineProps<TooltipPortalProps>();
</script>

<template>
  <TooltipPortal :to="props.to" :disabled="props.disabled" :defer="props.defer" :force-mount="props.forceMount">
    <slot />
  </TooltipPortal>
</template>