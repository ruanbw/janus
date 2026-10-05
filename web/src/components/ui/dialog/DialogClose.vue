<script setup lang="ts">
import { DialogClose, type DialogCloseProps } from 'reka-ui';

const props = defineProps<DialogCloseProps>();
</script>

<template>
  <DialogClose :as-child="props.asChild" :as="props.as">
    <slot />
  </DialogClose>
</template>