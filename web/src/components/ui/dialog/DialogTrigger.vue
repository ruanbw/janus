<script setup lang="ts">
import { DialogTrigger, type DialogTriggerProps } from 'reka-ui';

const props = defineProps<DialogTriggerProps>();
</script>

<template>
  <DialogTrigger :as-child="props.asChild" :as="props.as">
    <slot />
  </DialogTrigger>
</template>