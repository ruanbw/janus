<script setup lang="ts">
import { PopoverRoot, type PopoverRootEmits, type PopoverRootProps } from 'reka-ui';

const props = defineProps<PopoverRootProps>();
const emit = defineEmits<PopoverRootEmits>();
</script>

<template>
  <PopoverRoot
    :open="props.open"
    :default-open="props.defaultOpen"
    :modal="props.modal"
    @update:open="emit('update:open', $event)"
  >
    <slot />
  </PopoverRoot>
</template>