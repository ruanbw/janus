<script lang="ts">
import { cva } from 'class-variance-authority';

export const alertDialogContentVariants = cva(
  'fixed left-1/2 top-1/2 z-50 grid w-full max-w-md -translate-x-1/2 -translate-y-1/2 gap-4 rounded-xl border border-border bg-background text-foreground shadow-lg outline-none data-[state=open]:animate-in data-[state=closed]:animate-out data-[state=closed]:fade-out-0 data-[state=open]:fade-in-0 data-[state=closed]:zoom-out-95 data-[state=open]:zoom-in-95',
);
</script>

<script setup lang="ts">
import {
  AlertDialogContent,
  type AlertDialogContentEmits,
  type AlertDialogContentProps,
} from 'reka-ui';

import { cn } from '@/lib/utils';

const props = defineProps<AlertDialogContentProps & { class?: any }>();
const emit = defineEmits<AlertDialogContentEmits>();
</script>

<template>
  <AlertDialogContent
    :force-mount="props.forceMount"
    :as-child="props.asChild"
    :as="props.as"
    :disable-outside-pointer-events="props.disableOutsidePointerEvents"
    :class="cn(alertDialogContentVariants(), props.class)"
    @escape-key-down="emit('escapeKeyDown', $event)"
    @pointer-down-outside="emit('pointerDownOutside', $event)"
    @focus-outside="emit('focusOutside', $event)"
    @interact-outside="emit('interactOutside', $event)"
    @open-auto-focus="emit('openAutoFocus', $event)"
    @close-auto-focus="emit('closeAutoFocus', $event)"
  >
    <slot />
  </AlertDialogContent>
</template>