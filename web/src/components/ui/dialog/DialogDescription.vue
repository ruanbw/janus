<script setup lang="ts">
import { DialogDescription, type DialogDescriptionProps } from 'reka-ui';

import { cn } from '@/lib/utils';

const props = defineProps<DialogDescriptionProps & { class?: any }>();
</script>

<template>
  <DialogDescription
    :as-child="props.asChild"
    :class="cn('text-xs leading-relaxed text-muted-foreground', props.class)"
    :as="props.as"
  >
    <slot />
  </DialogDescription>
</template>