<template>
  <div class="relative w-full">
    <input
      :value="display"
      :placeholder="placeholder"
      :disabled="disabled"
      inputmode="decimal"
      :aria-invalid="formItem?.invalid.value ? 'true' : 'false'"
      class="app-field w-full rounded-lg border border-line bg-surface px-3 text-sm text-ink placeholder:text-ink-faint transition-colors focus:border-brand-500 focus:outline-none disabled:cursor-not-allowed disabled:bg-surface-strong disabled:text-ink-faint"
      :class="sizeClasses"
      @input="onInput"
      @blur="onBlur"
    />
  </div>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue';

import { useFormItem } from './form';

const props = withDefaults(
  defineProps<{
    modelValue?: number;
    min?: number;
    max?: number;
    precision?: number;
    step?: number;
    placeholder?: string;
    disabled?: boolean;
    size?: 'large' | 'middle' | 'small' | 'default';
  }>(),
  { size: 'middle', step: 1, disabled: false },
);

const emit = defineEmits<{ 'update:modelValue': [value: number | undefined] }>();

const formItem = useFormItem();
const raw = ref<string>(props.modelValue === undefined ? '' : String(props.modelValue));

const sizeClasses = computed(() => {
  if (props.size === 'large') return 'h-10 text-[15px]';
  if (props.size === 'small') return 'h-8 text-[13px]';
  return 'h-9';
});

const display = computed(() => raw.value);

function onInput(event: Event): void {
  const el = event.target as HTMLInputElement;
  raw.value = el.value;
  formItem?.clearError();
}

function onBlur(): void {
  const trimmed = raw.value.trim();
  if (trimmed === '') {
    emit('update:modelValue', undefined);
    return;
  }
  let value = Number(trimmed);
  if (Number.isNaN(value)) {
    raw.value = props.modelValue === undefined ? '' : String(props.modelValue);
    return;
  }
  if (props.min !== undefined && value < props.min) value = props.min;
  if (props.max !== undefined && value > props.max) value = props.max;
  if (props.precision !== undefined) {
    value = Number(value.toFixed(props.precision));
  }
  raw.value = String(value);
  emit('update:modelValue', value);
}
</script>
