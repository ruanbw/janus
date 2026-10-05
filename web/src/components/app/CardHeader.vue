<template>
  <!-- gap-0 抵消 ui/card-header 的 gap-1.5:本组件用 space-y-1.5 控子项间距,
       两者同时生效会把间距叠成两倍 -->
  <CardHeaderPrimitive :class="cn('gap-0 space-y-1.5', props.class)">
    <slot />
  </CardHeaderPrimitive>
</template>

<script setup lang="ts">
import { CardHeader as CardHeaderPrimitive } from '@/components/ui/card';

import { cn } from '@/lib/utils';

const props = defineProps<{
  class?: any;
}>();
</script>
