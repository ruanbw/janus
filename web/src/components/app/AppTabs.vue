<template>
  <UiTabs
    :model-value="modelValue"
    :default-value="defaultValue"
    :orientation="orientation"
    :dir="dir"
    :activation-mode="activationMode"
    class="flex flex-col gap-2"
    @update:model-value="emit('update:modelValue', $event)"
  >
    <slot />
  </UiTabs>
</template>

<script setup lang="ts">
import UiTabs from '@/components/ui/tabs.vue';

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
