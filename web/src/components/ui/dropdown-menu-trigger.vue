<script lang="ts">
import type { DropdownMenuTriggerProps } from 'reka-ui';

export type { DropdownMenuTriggerProps };
</script>

<script setup lang="ts">
import { DropdownMenuTrigger } from 'reka-ui';

import { cn } from '@/lib/utils';

const props = defineProps<DropdownMenuTriggerProps & { class?: any }>();
</script>

<template>
  <DropdownMenuTrigger
    :disabled="props.disabled"
    :as-child="props.asChild"
    :class="cn('cursor-default outline-none', props.class)"
    :as="props.as"
  >
    <slot />
  </DropdownMenuTrigger>
</template>
