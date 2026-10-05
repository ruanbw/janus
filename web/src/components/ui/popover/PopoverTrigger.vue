<script setup lang="ts">
import { PopoverTrigger, type PopoverTriggerProps } from 'reka-ui';

const props = defineProps<PopoverTriggerProps>();
</script>

<template>
  <PopoverTrigger :as-child="props.asChild" :as="props.as">
    <slot />
  </PopoverTrigger>
</template>