<script setup lang="ts">
import {
  AlertDialogRoot,
  type AlertDialogEmits,
  type AlertDialogProps,
} from 'reka-ui';

const props = defineProps<AlertDialogProps>();
const emit = defineEmits<AlertDialogEmits>();
</script>

<template>
  <AlertDialogRoot
    :open="props.open"
    :default-open="props.defaultOpen"
    :unmount-on-hide="props.unmountOnHide"
    @update:open="emit('update:open', $event)"
  >
    <slot />
  </AlertDialogRoot>
</template>