<script lang="ts">
import { cva } from 'class-variance-authority';

export const tooltipContentVariants = cva(
  'z-50 max-w-xs overflow-hidden rounded-md bg-foreground px-3 py-1.5 text-xs leading-relaxed text-background shadow-md select-none data-[state=delayed-open]:animate-in data-[state=closed]:animate-out data-[state=closed]:fade-out-0 data-[state=delayed-open]:fade-in-0 data-[state=closed]:zoom-out-95 data-[state=delayed-open]:zoom-in-95 data-[side=bottom]:slide-in-from-top-2 data-[side=left]:slide-in-from-right-2 data-[side=right]:slide-in-from-left-2 data-[side=top]:slide-in-from-bottom-2',
);
</script>

<script setup lang="ts">
import { TooltipContent, type TooltipContentEmits, type TooltipContentProps } from 'reka-ui';

import { cn } from '@/lib/utils';

const props = defineProps<TooltipContentProps & { class?: any }>();
const emit = defineEmits<TooltipContentEmits>();
</script>

<template>
  <TooltipContent
    :side="props.side"
    :side-offset="props.sideOffset"
    :align="props.align"
    :align-offset="props.alignOffset"
    :avoid-collisions="props.avoidCollisions"
    :collision-padding="props.collisionPadding"
    :collision-boundary="props.collisionBoundary"
    :arrow-padding="props.arrowPadding"
    :aria-label="props.ariaLabel"
    :position-strategy="props.positionStrategy"
    :update-position-strategy="props.updatePositionStrategy"
    :sticky="props.sticky"
    :hide-when-detached="props.hideWhenDetached"
    :force-mount="props.forceMount"
    :as-child="props.asChild"
    :as="props.as"
    :class="cn(tooltipContentVariants(), props.class)"
    @escape-key-down="emit('escapeKeyDown', $event)"
    @pointer-down-outside="emit('pointerDownOutside', $event)"
  >
    <slot />
  </TooltipContent>
</template>