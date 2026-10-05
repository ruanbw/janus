<template>
  <UiTabs
    :model-value="modelValue"
    :default-value="defaultValue"
    :orientation="orientation"
    :dir="dir"
    :activation-mode="activationMode"
    :class="cn('flex flex-col gap-2', props.class)"
    @update:model-value="emit('update:modelValue', $event)"
  >
    <slot />
  </UiTabs>
</template>

<script setup lang="ts">
import { Tabs as UiTabs } from '@/components/ui/tabs';

import { cn } from '@/lib/utils';

const props = defineProps<{
  modelValue?: string | number;
  defaultValue?: string | number;
  orientation?: 'horizontal' | 'vertical';
  dir?: 'ltr' | 'rtl';
  activationMode?: 'automatic' | 'manual';
  class?: any;
}>();

const emit = defineEmits<{
  'update:modelValue': [val: string | number];
}>();
</script>
