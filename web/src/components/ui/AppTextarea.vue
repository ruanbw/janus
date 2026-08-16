<template>
  <div class="relative w-full">
    <textarea
      :value="modelValue"
      :placeholder="placeholder"
      :maxlength="maxlength"
      :rows="rows"
      :disabled="disabled"
      :aria-invalid="formItem?.invalid.value ? 'true' : 'false'"
      class="app-field w-full resize-y rounded-lg border border-line bg-surface px-3 py-2 text-sm leading-relaxed text-ink placeholder:text-ink-faint transition-colors focus:border-brand-500 focus:outline-none disabled:cursor-not-allowed disabled:bg-surface-strong disabled:text-ink-faint"
      @input="onInput"
      @blur="formItem?.clearError()"
    />
    <span v-if="showCount" class="pointer-events-none absolute bottom-1.5 right-2.5 text-[11px] text-ink-faint">
      {{ String(modelValue ?? '').length }}{{ maxlength !== undefined ? '/' + maxlength : '' }}
    </span>
  </div>
</template>

<script setup lang="ts">
import { useFormItem } from './form';

withDefaults(
  defineProps<{
    modelValue?: string;
    placeholder?: string;
    maxlength?: number;
    rows?: number;
    disabled?: boolean;
    showCount?: boolean;
  }>(),
  { modelValue: '', rows: 2, showCount: false, disabled: false },
);

const emit = defineEmits<{ 'update:modelValue': [value: string] }>();

const formItem = useFormItem();

function onInput(event: Event): void {
  const el = event.target as HTMLTextAreaElement;
  emit('update:modelValue', el.value);
  formItem?.clearError();
}
</script>
