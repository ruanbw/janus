<script setup lang="ts">
import { AlertDialogTrigger, type AlertDialogTriggerProps } from 'reka-ui';

const props = defineProps<AlertDialogTriggerProps>();
</script>

<template>
  <AlertDialogTrigger :as-child="props.asChild" :as="props.as">
    <slot />
  </AlertDialogTrigger>
</template>