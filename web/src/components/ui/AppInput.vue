<template>
  <div :class="wrapperClasses" class="relative flex w-full items-center">
    <span
      v-if="$slots.prefix"
      class="pointer-events-none absolute left-3 z-10 flex items-center text-ink-faint [&>svg]:size-4"
    >
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
      :aria-invalid="isInvalid ? 'true' : 'false'"
      :class="inputClasses"
      @input="onInput"
      @change="onInput"
      @focus="emit('focus', $event)"
      @blur="onBlur"
      @keydown.enter="emit('pressEnter')"
    />

    <div
      v-if="hasRightElements"
      class="absolute right-2.5 z-10 flex items-center gap-1.5 text-ink-faint"
    >
      <button
        v-if="showClear"
        type="button"
        tabindex="-1"
        title="清空"
        class="flex h-5 w-5 items-center justify-center rounded-sm transition-colors hover:bg-surface-strong hover:text-ink focus-visible:outline-none"
        @click.stop="onClear"
      >
        <X :size="13" />
      </button>

      <button
        v-if="isPassword"
        type="button"
        tabindex="-1"
        :title="showPassword ? '隐藏密码' : '显示密码'"
        class="flex h-5 w-5 items-center justify-center rounded-sm transition-colors hover:bg-surface-strong hover:text-ink focus-visible:outline-none"
        @click.stop="showPassword = !showPassword"
      >
        <EyeOff v-if="showPassword" :size="14" />
        <Eye v-else :size="14" />
      </button>

      <span v-if="$slots.suffix" class="flex items-center [&>svg]:size-4">
        <slot name="suffix" />
      </span>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, ref, useSlots } from 'vue';
import { Eye, EyeOff, X } from '@lucide/vue';

import { cn } from '@/lib/utils';
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
    size?: 'large' | 'middle' | 'small' | 'default' | 'sm' | 'lg';
    allowClear?: boolean;
    invalid?: boolean;
    class?: any;
  }>(),
  { modelValue: '', type: 'text', size: 'middle', allowClear: false },
);

const emit = defineEmits<{
  'update:modelValue': [value: string];
  pressEnter: [];
  blur: [event: FocusEvent];
  focus: [event: FocusEvent];
  clear: [];
}>();

const slots = useSlots();
const formItem = useFormItem();
const showPassword = ref(false);

const isPassword = computed(() => props.type === 'password');
const inputType = computed(() => (isPassword.value && showPassword.value ? 'text' : props.type));
const isInvalid = computed(() => Boolean(props.invalid || formItem?.invalid.value));

const showClear = computed(() => {
  return (
    props.allowClear &&
    !props.disabled &&
    !props.readonly &&
    props.modelValue !== undefined &&
    props.modelValue !== null &&
    String(props.modelValue).length > 0
  );
});

const hasRightElements = computed(() => {
  return showClear.value || isPassword.value || Boolean(slots.suffix);
});

const inputClasses = computed(() => {
  const isSm = props.size === 'small' || props.size === 'sm';
  const isLg = props.size === 'large' || props.size === 'lg';

  let rightPadding = '';
  if (hasRightElements.value) {
    let count = 0;
    if (showClear.value) count++;
    if (isPassword.value) count++;
    if (slots.suffix) count++;
    rightPadding = count > 1 ? 'pr-16' : 'pr-9';
  }

  return cn(
    'app-field flex w-full rounded-md border border-input bg-transparent text-foreground shadow-xs transition-[color,background-color,border-color,box-shadow] placeholder:text-muted-foreground disabled:cursor-not-allowed disabled:opacity-50 dark:bg-input/30',
    isSm ? 'h-8 px-2.5 text-xs' : isLg ? 'h-10 px-3.5 text-base' : 'h-9 px-3 py-1 text-sm',
    slots.prefix && 'pl-9',
    rightPadding,
    isInvalid.value && 'border-err',
    props.class,
  );
});

const wrapperClasses = computed(() => (isInvalid.value ? 'data-invalid-wrap' : ''));

function onInput(event: Event): void {
  const el = event.target as HTMLInputElement;
  emit('update:modelValue', el.value);
  formItem?.clearError();
}

function onBlur(event: FocusEvent): void {
  emit('blur', event);
}

function onClear(): void {
  emit('update:modelValue', '');
  emit('clear');
  formItem?.clearError();
}
</script>
