<script lang="ts">
import { cva } from 'class-variance-authority';
import type { DropdownMenuItemEmits, DropdownMenuItemProps } from 'reka-ui';

export type { DropdownMenuItemEmits, DropdownMenuItemProps };

export const dropdownMenuItemVariants = cva(
  'relative flex cursor-pointer select-none items-center gap-2 rounded-md px-2 py-1.5 text-sm outline-none transition-colors data-[disabled]:pointer-events-none data-[disabled]:opacity-50 data-[highlighted]:bg-accent data-[highlighted]:text-accent-foreground',
);
</script>

<script setup lang="ts">
import {
  DropdownMenuItem,
  type DropdownMenuItemEmits as ItemEmits,
  type DropdownMenuItemProps as ItemProps,
} from 'reka-ui';

import { cn } from '@/lib/utils';

const props = defineProps<ItemProps & { class?: any }>();
const emit = defineEmits<ItemEmits>();
</script>

<template>
  <DropdownMenuItem
    :disabled="props.disabled"
    :text-value="props.textValue"
    :class="cn(dropdownMenuItemVariants(), props.class)"
    @select="emit('select', $event)"
  >
    <slot />
  </DropdownMenuItem>
</template>
