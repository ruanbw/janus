<script lang="ts">
import { cva } from 'class-variance-authority';

export const dialogContentVariants = cva(
  'fixed left-1/2 top-1/2 z-50 grid w-full max-w-lg -translate-x-1/2 -translate-y-1/2 gap-4 rounded-xl border border-border bg-background p-6 text-foreground shadow-lg outline-none data-[state=open]:animate-in data-[state=closed]:animate-out data-[state=closed]:fade-out-0 data-[state=open]:fade-in-0 data-[state=closed]:zoom-out-95 data-[state=open]:zoom-in-95',
);
</script>

<script setup lang="ts">
import {
  DialogContent,
  type DialogContentEmits,
  type DialogContentProps,
} from 'reka-ui';

import { cn } from '@/lib/utils';

const props = defineProps<DialogContentProps & { class?: any }>();
const emit = defineEmits<DialogContentEmits>();
</script>

<template>
  <DialogContent
    :force-mount="props.forceMount"
    :as-child="props.asChild"
    :class="cn(dialogContentVariants(), props.class)"
    @escape-key-down="emit('escapeKeyDown', $event)"
    @pointer-down-outside="emit('pointerDownOutside', $event)"
    @focus-outside="emit('focusOutside', $event)"
    @interact-outside="emit('interactOutside', $event)"
    @open-auto-focus="emit('openAutoFocus', $event)"
    @close-auto-focus="emit('closeAutoFocus', $event)"
  >
    <slot />
  </DialogContent>
</template>