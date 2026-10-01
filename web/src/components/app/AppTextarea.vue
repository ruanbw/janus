<template>
  <div class="relative w-full" :class="isInvalid ? 'data-invalid-wrap' : ''">
    <UiTextarea
      :id="fieldId"
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
      class="pointer-events-none absolute right-2.5 bottom-2 rounded bg-surface/80 px-1 py-0.5 font-mono text-2xs text-ink-faint backdrop-blur-xs select-none"
    >
      {{ currentLength }}{{ maxlength !== undefined ? ` / ${maxlength}` : '' }}
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue';

import UiTextarea from '@/components/ui/textarea.vue';

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
    /** 显式 id；缺省时自动跟随外层 AppFormItem */
    id?: string;
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

/** 显式 id 优先；否则跟随所属 AppFormItem */
const fieldId = computed(() => props.id ?? formItem?.id);
const isInvalid = computed(() => Boolean(props.invalid || formItem?.invalid.value));
const currentLength = computed(() => String(props.modelValue ?? '').length);

const textareaClasses = computed(() => {
  return cn(
    // 焦点环与错误描边由 main.css 的 .app-field 单一出处承担，这里只挂类名
    'app-field min-h-20 bg-control-bg transition-[color,background-color,border-color,box-shadow]',
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
