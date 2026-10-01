<script setup lang="ts">
import { AlertDialogPortal, type AlertDialogPortalProps } from 'reka-ui';

const props = defineProps<AlertDialogPortalProps>();
</script>

<template>
  <AlertDialogPortal :to="props.to" :disabled="props.disabled">
    <slot />
  </AlertDialogPortal>
</template>