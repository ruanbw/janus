<script lang="ts">
import { cva } from 'class-variance-authority';
import type { DropdownMenuContentEmits, DropdownMenuContentProps } from 'reka-ui';

export type { DropdownMenuContentEmits, DropdownMenuContentProps };

export const dropdownMenuContentVariants = cva(
  'z-50 min-w-40 overflow-hidden rounded-lg border border-border bg-popover p-1 text-popover-foreground shadow-lg outline-none data-[state=open]:animate-in data-[state=closed]:animate-out data-[state=closed]:fade-out-0 data-[state=open]:fade-in-0 data-[state=closed]:zoom-out-95 data-[state=open]:zoom-in-95 data-[side=bottom]:slide-in-from-top-2 data-[side=left]:slide-in-from-right-2 data-[side=right]:slide-in-from-left-2 data-[side=top]:slide-in-from-bottom-2',
);
</script>

<script setup lang="ts">
import {
  DropdownMenuContent,
  type DropdownMenuContentEmits as ContentEmits,
  type DropdownMenuContentProps as ContentProps,
} from 'reka-ui';

import { cn } from '@/lib/utils';

const props = defineProps<ContentProps & { class?: any }>();
const emit = defineEmits<ContentEmits>();
</script>

<template>
  <DropdownMenuContent
    :side="props.side"
    :side-offset="props.sideOffset"
    :align="props.align"
    :align-offset="props.alignOffset"
    :avoid-collisions="props.avoidCollisions"
    :collision-padding="props.collisionPadding"
    :collision-boundary="props.collisionBoundary"
    :arrow-padding="props.arrowPadding"
    :update-position-strategy="props.updatePositionStrategy"
    :sticky="props.sticky"
    :hide-when-detached="props.hideWhenDetached"
    :prioritize-position="props.prioritizePosition"
    :disable-update-on-layout-shift="props.disableUpdateOnLayoutShift"
    :memo-dependencies="props.memoDependencies"
    :side-flip="props.sideFlip"
    :align-flip="props.alignFlip"
    :hide-shifted-arrow="props.hideShiftedArrow"
    :position-strategy="props.positionStrategy"
    :reference="props.reference"
    :loop="props.loop"
    :force-mount="props.forceMount"
    :as-child="props.asChild"
    :as="props.as"
    :class="cn(dropdownMenuContentVariants(), props.class)"
    @close-auto-focus="emit('closeAutoFocus', $event)"
    @escape-key-down="emit('escapeKeyDown', $event)"
    @pointer-down-outside="emit('pointerDownOutside', $event)"
    @focus-outside="emit('focusOutside', $event)"
    @interact-outside="emit('interactOutside', $event)"
  >
    <slot />
  </DropdownMenuContent>
</template>
