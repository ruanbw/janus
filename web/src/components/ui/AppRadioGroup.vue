<template>
  <RadioGroupRoot
    v-model="model"
    :disabled="disabled"
    :orientation="orientation"
    :name="name"
    :class="groupClasses"
  >
    <slot />
  </RadioGroupRoot>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import { RadioGroupRoot } from 'reka-ui';

import { cn } from '@/lib/utils';

const props = withDefaults(
  defineProps<{
    modelValue?: string | number | undefined;
    disabled?: boolean;
    orientation?: 'horizontal' | 'vertical';
    name?: string;
    class?: any;
  }>(),
  { disabled: false, orientation: 'horizontal' },
);

const emit = defineEmits<{
  'update:modelValue': [value: string | number | undefined];
  change: [value: string | number | undefined];
}>();

const model = computed({
  get(): string | number | undefined {
    return props.modelValue;
  },
  set(val: string | number | undefined) {
    emit('update:modelValue', val);
    emit('change', val);
  },
});

const groupClasses = computed(() => {
  return cn(
    'flex gap-3',
    props.orientation === 'vertical' ? 'flex-col' : 'flex-wrap items-center',
    props.class,
  );
});
</script>
