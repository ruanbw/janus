<template>
  <div :class="wrapperClasses" class="relative w-full">
    <span v-if="$slots.prefix" class="pointer-events-none absolute inset-y-0 left-3 flex items-center text-ink-faint">
      <slot name="prefix" />
    </span>
    <input
      :type="inputType"
      :value="modelValue"
      :placeholder="placeholder"
      :maxlength="maxlength"
      :disabled="disabled"
      :readonly="readonly"
      :autocomplete="autocomplete"
      :aria-invalid="formItem?.invalid.value ? 'true' : 'false'"
      class="app-field w-full rounded-lg border border-line bg-surface text-ink placeholder:text-ink-faint transition-colors focus:border-brand-500 focus:outline-none disabled:cursor-not-allowed disabled:bg-surface-strong disabled:text-ink-faint"
      :class="[sizeClasses, $slots.prefix ? 'pl-9' : '', $slots.suffix || isPassword ? 'pr-10' : '']"
      @input="onInput"
      @change="onInput"
      @blur="onBlur"
      @keydown.enter="emit('pressEnter')"
    />
    <button
      v-if="isPassword"
      type="button"
      class="absolute inset-y-0 right-2.5 flex items-center text-ink-faint transition-colors hover:text-ink-soft"
      tabindex="-1"
      @click="showPassword = !showPassword"
    >
      <EyeOff v-if="showPassword" :size="15" />
      <Eye v-else :size="15" />
    </button>
    <span v-else-if="$slots.suffix" class="absolute inset-y-0 right-3 flex items-center text-ink-faint">
      <slot name="suffix" />
    </span>
  </div>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue';
import { Eye, EyeOff } from '@lucide/vue';

import { useFormItem } from './form';

const props = withDefaults(
  defineProps<{
    modelValue?: string | number;
    type?: string;
    placeholder?: string;
    maxlength?: number;
    disabled?: boolean;
    readonly?: boolean;
    autocomplete?: string;
    size?: 'large' | 'middle' | 'small' | 'default';
  }>(),
  { modelValue: '', type: 'text', size: 'middle' },
);

const emit = defineEmits<{
  'update:modelValue': [value: string];
  pressEnter: [];
  blur: [event: FocusEvent];
}>();

const formItem = useFormItem();
const showPassword = ref(false);

const isPassword = computed(() => props.type === 'password');
const inputType = computed(() => (isPassword.value && showPassword.value ? 'text' : props.type));

const sizeClasses = computed(() => {
  if (props.size === 'large') return 'h-10 text-[15px]';
  if (props.size === 'small') return 'h-8 text-[13px]';
  return 'h-9 text-sm';
});

const wrapperClasses = computed(() => (formItem?.invalid.value ? 'data-invalid-wrap' : ''));

function onInput(event: Event): void {
  const el = event.target as HTMLInputElement;
  emit('update:modelValue', el.value);
  formItem?.clearError();
}

function onBlur(event: FocusEvent): void {
  emit('blur', event);
}
</script>
