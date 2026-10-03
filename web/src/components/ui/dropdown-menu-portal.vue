<script lang="ts">
import type { DropdownMenuPortalProps } from 'reka-ui';

export type { DropdownMenuPortalProps };
</script>

<script setup lang="ts">
import { DropdownMenuPortal } from 'reka-ui';

const props = defineProps<DropdownMenuPortalProps>();
</script>

<template>
  <DropdownMenuPortal
    :to="props.to"
    :disabled="props.disabled"
    :force-mount="props.forceMount"
    :defer="props.defer"
  >
    <slot />
  </DropdownMenuPortal>
</template>
