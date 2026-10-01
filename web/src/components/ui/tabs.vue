<script setup lang="ts">
import { TabsRoot, type TabsRootEmits, type TabsRootProps } from 'reka-ui';

const props = defineProps<TabsRootProps>();
const emit = defineEmits<TabsRootEmits>();
</script>

<template>
  <TabsRoot
    :default-value="props.defaultValue"
    :orientation="props.orientation"
    :dir="props.dir"
    :activation-mode="props.activationMode"
    :unmount-on-hide="props.unmountOnHide"
    :as-child="props.asChild"
    @update:model-value="emit('update:modelValue', $event)"
  >
    <slot />
  </TabsRoot>
</template>