<script lang="ts">
import type { DropdownMenuSeparatorProps } from 'reka-ui';

export type { DropdownMenuSeparatorProps };
</script>

<script setup lang="ts">
import { DropdownMenuSeparator } from 'reka-ui';

import { cn } from '@/lib/utils';

const props = defineProps<DropdownMenuSeparatorProps & { class?: any }>();
</script>

<template>
  <DropdownMenuSeparator :class="cn('-mx-1 my-1 h-px bg-border', props.class)" :as-child="props.asChild" :as="props.as" />
</template>
