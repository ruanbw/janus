<script setup lang="ts">
import { TabsContent, type TabsContentProps } from 'reka-ui';

import { cn } from '@/lib/utils';

const props = defineProps<TabsContentProps & { class?: any }>();
</script>

<template>
  <TabsContent
    :value="props.value"
    :force-mount="props.forceMount"
    :as-child="props.asChild"
    :class="cn('outline-none focus-visible:ring-[3px] focus-visible:ring-ring/50', props.class)"
  >
    <slot />
  </TabsContent>
</template>