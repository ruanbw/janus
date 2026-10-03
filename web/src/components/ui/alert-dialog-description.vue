<script setup lang="ts">
import { AlertDialogDescription, type AlertDialogDescriptionProps } from 'reka-ui';

import { cn } from '@/lib/utils';

const props = defineProps<AlertDialogDescriptionProps & { class?: any }>();
</script>

<template>
  <AlertDialogDescription
    :as-child="props.asChild"
    :class="cn('text-xs leading-relaxed text-muted-foreground', props.class)"
    :as="props.as"
  >
    <slot />
  </AlertDialogDescription>
</template>