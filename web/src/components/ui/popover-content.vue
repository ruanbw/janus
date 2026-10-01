<script lang="ts">
import { cva } from 'class-variance-authority';

export const popoverContentVariants = cva(
  'z-50 w-72 rounded-xl border border-border bg-popover p-4 text-popover-foreground shadow-md outline-none data-[state=open]:animate-in data-[state=closed]:animate-out data-[state=closed]:fade-out-0 data-[state=open]:fade-in-0 data-[state=closed]:zoom-out-95 data-[state=open]:zoom-in-95 data-[side=bottom]:slide-in-from-top-2 data-[side=left]:slide-in-from-right-2 data-[side=right]:slide-in-from-left-2 data-[side=top]:slide-in-from-bottom-2',
);
</script>

<script setup lang="ts">
import { PopoverContent, type PopoverContentEmits, type PopoverContentProps } from 'reka-ui';

import { cn } from '@/lib/utils';

const props = defineProps<PopoverContentProps & { class?: any }>();
const emit = defineEmits<PopoverContentEmits>();
</script>

<template>
  <PopoverContent
    :side="props.side"
    :side-offset="props.sideOffset"
    :align="props.align"
    :align-offset="props.alignOffset"
    :avoid-collisions="props.avoidCollisions"
    :collision-padding="props.collisionPadding"
    :arrow-padding="props.arrowPadding"
    :force-mount="props.forceMount"
    :as-child="props.asChild"
    :class="cn(popoverContentVariants(), props.class)"
    @escape-key-down="emit('escapeKeyDown', $event)"
    @pointer-down-outside="emit('pointerDownOutside', $event)"
    @focus-outside="emit('focusOutside', $event)"
    @interact-outside="emit('interactOutside', $event)"
    @open-auto-focus="emit('openAutoFocus', $event)"
    @close-auto-focus="emit('closeAutoFocus', $event)"
  >
    <slot />
  </PopoverContent>
</template>