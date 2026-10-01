<script lang="ts">
import { cva } from 'class-variance-authority';
import type { DropdownMenuRootEmits, DropdownMenuRootProps } from 'reka-ui';

export { default as DropdownMenuTrigger } from './dropdown-menu-trigger.vue';
export { default as DropdownMenuPortal } from './dropdown-menu-portal.vue';
export { default as DropdownMenuContent } from './dropdown-menu-content.vue';
export { default as DropdownMenuItem } from './dropdown-menu-item.vue';
export { default as DropdownMenuLabel } from './dropdown-menu-label.vue';
export { default as DropdownMenuSeparator } from './dropdown-menu-separator.vue';

export type { DropdownMenuRootEmits, DropdownMenuRootProps };
</script>

<script setup lang="ts">
import { DropdownMenuRoot } from 'reka-ui';

import { cn } from '@/lib/utils';

const props = defineProps<DropdownMenuRootProps & { class?: any }>();
const emit = defineEmits<DropdownMenuRootEmits>();
</script>

<template>
  <DropdownMenuRoot
    :default-open="props.defaultOpen"
    :class="cn('relative', props.class)"
    @update:open="emit('update:open', $event)"
  >
    <slot />
  </DropdownMenuRoot>
</template>
