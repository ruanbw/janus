<script setup lang="ts">
import { DialogPortal, type DialogPortalProps } from 'reka-ui';

const props = defineProps<DialogPortalProps>();
</script>

<template>
  <DialogPortal :to="props.to" :disabled="props.disabled" :defer="props.defer" :force-mount="props.forceMount">
    <slot />
  </DialogPortal>
</template>