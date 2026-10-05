<script setup lang="ts">
import { DialogRoot, type DialogRootEmits, type DialogRootProps } from 'reka-ui';

const props = defineProps<DialogRootProps>();
const emit = defineEmits<DialogRootEmits>();
</script>

<template>
  <DialogRoot
    :open="props.open"
    :default-open="props.defaultOpen"
    :modal="props.modal"
    :unmount-on-hide="props.unmountOnHide"
    @update:open="emit('update:open', $event)"
  >
    <slot />
  </DialogRoot>
</template>