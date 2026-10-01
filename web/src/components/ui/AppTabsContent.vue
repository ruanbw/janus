<template>
  <TabsContent
    :value="value"
    :force-mount="forceMount"
    :class="cn('outline-none focus-visible:ring-2 focus-visible:ring-ring', props.class)"
  >
    <slot />
  </TabsContent>
</template>

<script setup lang="ts">
import { TabsContent } from 'reka-ui';

import { cn } from '@/lib/utils';

const props = withDefaults(
  defineProps<{
    value: string | number;
    forceMount?: boolean;
    class?: any;
  }>(),
  {},
);
</script>
