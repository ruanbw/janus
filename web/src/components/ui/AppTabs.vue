<template>
  <TabsRoot
    :model-value="modelValue"
    :default-value="defaultValue"
    :orientation="orientation"
    :dir="dir"
    :activation-mode="activationMode"
    :class="cn('flex flex-col gap-2', props.class)"
    @update:model-value="emit('update:modelValue', $event)"
  >
    <slot />
  </TabsRoot>
</template>

<script setup lang="ts">
import { TabsRoot } from 'reka-ui';

import { cn } from '@/lib/utils';

const props = withDefaults(
  defineProps<{
    modelValue?: string | number;
    defaultValue?: string | number;
    orientation?: 'horizontal' | 'vertical';
    dir?: 'ltr' | 'rtl';
    activationMode?: 'automatic' | 'manual';
    class?: any;
  }>(),
  {
    orientation: 'horizontal',
    activationMode: 'automatic',
  },
);

const emit = defineEmits<{
  'update:modelValue': [val: string | number];
}>();
</script>
