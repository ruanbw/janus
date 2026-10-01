<template>
  <UiTabsContent :value="value" :force-mount="forceMount">
    <slot />
  </UiTabsContent>
</template>

<script setup lang="ts">
import UiTabsContent from '@/components/ui/tabs-content.vue';

const props = defineProps<{
  value: string | number;
  forceMount?: boolean;
  class?: any;
}>();
</script>
