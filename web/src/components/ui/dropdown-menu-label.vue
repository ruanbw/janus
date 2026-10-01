<script lang="ts">
import type { DropdownMenuLabelProps } from 'reka-ui';

export type { DropdownMenuLabelProps };
</script>

<script setup lang="ts">
import { DropdownMenuLabel } from 'reka-ui';

import { cn } from '@/lib/utils';

const props = defineProps<DropdownMenuLabelProps & { class?: any }>();
</script>

<template>
  <DropdownMenuLabel :class="cn('px-2 py-1.5 text-xs text-muted-foreground', props.class)">
    <slot />
  </DropdownMenuLabel>
</template>
