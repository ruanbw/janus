<script setup lang="ts">
import { DialogPortal, type DialogPortalProps } from 'reka-ui';

const props = defineProps<DialogPortalProps>();
</script>

<template>
  <DialogPortal :to="props.to" :disabled="props.disabled">
    <slot />
  </DialogPortal>
</template>