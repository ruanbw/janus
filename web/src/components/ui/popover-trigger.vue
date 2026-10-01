<script setup lang="ts">
import { PopoverTrigger, type PopoverTriggerProps } from 'reka-ui';

const props = defineProps<PopoverTriggerProps>();
</script>

<template>
  <PopoverTrigger :as-child="props.asChild">
    <slot />
  </PopoverTrigger>
</template>