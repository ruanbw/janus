<template>
  <div class="relative w-full" :class="isInvalid ? 'data-invalid-wrap' : ''">
    <textarea
      :value="modelValue"
      :placeholder="placeholder"
      :maxlength="maxlength"
      :rows="rows"
      :disabled="disabled"
      :readonly="readonly"
      :aria-invalid="isInvalid ? 'true' : 'false'"
      :class="textareaClasses"
      @input="onInput"
      @blur="onBlur"
      @focus="emit('focus', $event)"
    />
    <div
      v-if="showCount"
      class="pointer-events-none absolute bottom-2 right-2.5 rounded bg-surface/80 px-1 py-0.5 text-2xs font-mono text-ink-faint backdrop-blur-xs select-none"
    >
      {{ currentLength }}{{ maxlength !== undefined ? ` / ${maxlength}` : '' }}
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue';

import { cn } from '@/lib/utils';
import { useFormItem } from './form';

const props = withDefaults(
  defineProps<{
    modelValue?: string;
    placeholder?: string;
    maxlength?: number;
    rows?: number;
    disabled?: boolean;
    readonly?: boolean;
    showCount?: boolean;
    invalid?: boolean;
    class?: any;
  }>(),
  { modelValue: '', rows: 3, showCount: false, disabled: false, readonly: false },
);

const emit = defineEmits<{
  'update:modelValue': [value: string];
  blur: [event: FocusEvent];
  focus: [event: FocusEvent];
}>();

const formItem = useFormItem();
const isInvalid = computed(() => Boolean(props.invalid || formItem?.invalid.value));
const currentLength = computed(() => String(props.modelValue ?? '').length);

const textareaClasses = computed(() => {
  return cn(
    'app-field flex min-h-[80px] w-full resize-y rounded-md border border-input bg-transparent px-3 py-2 text-sm shadow-xs transition-[color,background-color,border-color,box-shadow] placeholder:text-muted-foreground disabled:cursor-not-allowed disabled:opacity-50 dark:bg-input/30',
    props.showCount && 'pb-7',
    isInvalid.value && 'border-err',
    props.class,
  );
});

function onInput(event: Event): void {
  const el = event.target as HTMLTextAreaElement;
  emit('update:modelValue', el.value);
  formItem?.clearError();
}

function onBlur(event: FocusEvent): void {
  emit('blur', event);
  formItem?.clearError();
}
</script>
