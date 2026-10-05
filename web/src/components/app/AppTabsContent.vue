<template>
  <UiTabsContent :value="value" :force-mount="forceMount" :class="props.class">
    <slot />
  </UiTabsContent>
</template>

<script setup lang="ts">
import { TabsContent as UiTabsContent } from '@/components/ui/tabs';

const props = defineProps<{
  value: string | number;
  forceMount?: boolean;
  class?: any;
}>();
</script>
