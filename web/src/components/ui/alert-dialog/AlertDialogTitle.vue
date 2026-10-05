<script setup lang="ts">
import { AlertDialogTitle, type AlertDialogTitleProps } from 'reka-ui';

import { cn } from '@/lib/utils';

const props = defineProps<AlertDialogTitleProps & { class?: any }>();
</script>

<template>
  <AlertDialogTitle
    :as-child="props.asChild"
    :class="cn('text-base font-semibold leading-tight', props.class)"
    :as="props.as"
  >
    <slot />
  </AlertDialogTitle>
</template>