<script lang="ts">
import { cva } from 'class-variance-authority';

export const dialogOverlayVariants = cva(
  'fixed inset-0 z-50 bg-black/60 backdrop-blur-[2px] data-[state=open]:animate-in data-[state=closed]:animate-out data-[state=closed]:fade-out-0 data-[state=open]:fade-in-0',
);
</script>

<script setup lang="ts">
import { DialogOverlay, type DialogOverlayProps } from 'reka-ui';

import { cn } from '@/lib/utils';

const props = defineProps<DialogOverlayProps & { class?: any }>();
</script>

<template>
  <DialogOverlay
    :force-mount="props.forceMount"
    :as-child="props.asChild"
    :class="cn(dialogOverlayVariants(), props.class)"
    :as="props.as"
  />
</template>