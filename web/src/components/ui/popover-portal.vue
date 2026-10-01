<script setup lang="ts">
import { PopoverPortal, type PopoverPortalProps } from 'reka-ui';

const props = defineProps<PopoverPortalProps>();
</script>

<template>
  <PopoverPortal :to="props.to" :disabled="props.disabled">
    <slot />
  </PopoverPortal>
</template>