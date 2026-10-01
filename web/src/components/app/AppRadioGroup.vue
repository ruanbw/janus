<template>
  <RadioGroup
    v-bind="rootProps"
    :class="groupClasses"
  >
    <slot />
  </RadioGroup>
</template>

<script setup lang="ts">
import { computed } from 'vue';

import RadioGroup from '@/components/ui/radio-group.vue';
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

const rootProps = computed(() => ({
  modelValue: props.modelValue,
  disabled: props.disabled,
  orientation: props.orientation,
  name: props.name,
  'onUpdate:modelValue': (value: string | number | undefined) => {
    emit('update:modelValue', value);
    emit('change', value);
  },
}));

const groupClasses = computed(() =>
  cn(
    'flex gap-3',
    props.orientation === 'vertical' ? 'flex-col' : 'flex-wrap items-center',
    props.class,
  ),
);
</script>