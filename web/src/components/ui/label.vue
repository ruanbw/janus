<script setup lang="ts">
import { Label, type LabelProps } from 'reka-ui';

import { cn } from '@/lib/utils';

const props = defineProps<LabelProps & { class?: any }>();
</script>

<template>
  <Label
    :for="props.for"
    :as-child="props.asChild"
    :as="props.as"
    :class="
      cn(
        'text-sm font-medium leading-none select-none',
        'peer-disabled:cursor-not-allowed peer-disabled:opacity-70',
        props.class,
      )
    ">
    <slot />
  </Label>
</template>