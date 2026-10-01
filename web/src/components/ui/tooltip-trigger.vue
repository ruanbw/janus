<script setup lang="ts">
import { TooltipTrigger, type TooltipTriggerProps } from 'reka-ui';

const props = defineProps<TooltipTriggerProps>();
</script>

<template>
  <TooltipTrigger :as-child="props.asChild" :reference="props.reference">
    <slot />
  </TooltipTrigger>
</template>