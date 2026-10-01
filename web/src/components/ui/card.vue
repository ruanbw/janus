<script setup lang="ts">
import { Primitive, type PrimitiveProps } from 'reka-ui';

import { cn } from '@/lib/utils';

const props = defineProps<PrimitiveProps & { class?: any }>();
</script>

<template>
  <Primitive
    :as="props.as ?? 'div'"
    :as-child="props.asChild"
    :class="
      cn(
        'flex flex-col rounded-lg border border-border bg-card text-card-foreground shadow-xs',
        props.class,
      )
    "
  >
    <slot />
  </Primitive>
</template>