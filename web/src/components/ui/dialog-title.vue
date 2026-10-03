<script setup lang="ts">
import { DialogTitle, type DialogTitleProps } from 'reka-ui';

import { cn } from '@/lib/utils';

const props = defineProps<DialogTitleProps & { class?: any }>();
</script>

<template>
  <DialogTitle
    :as-child="props.asChild"
    :class="cn('text-base font-semibold leading-tight', props.class)"
    :as="props.as"
  >
    <slot />
  </DialogTitle>
</template>