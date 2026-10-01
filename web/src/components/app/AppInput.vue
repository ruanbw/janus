<template>
  <div class="relative flex w-full items-center" :class="wrapperClasses">
    <span
      v-if="$slots.prefix"
      class="pointer-events-none absolute left-3 z-10 flex items-center text-ink-faint [&>svg]:size-4"
    >
      <slot name="prefix" />
    </span>

    <UiInput
      :id="fieldId"
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
        aria-label="清空输入框"
        class="relative flex h-5 w-5 items-center justify-center rounded-sm text-ink-faint transition-colors after:absolute after:-inset-1 after:content-[''] hover:bg-surface-strong hover:text-ink focus-visible:outline-none"
        @click.stop="onClear"
      >
        <X :size="13" />
      </button>

      <button
        v-if="isPassword"
        type="button"
        tabindex="-1"
        :title="showPassword ? '隐藏密码' : '显示密码'"
        :aria-label="showPassword ? '隐藏密码' : '显示密码'"
        class="relative flex h-5 w-5 items-center justify-center rounded-sm text-ink-faint transition-colors after:absolute after:-inset-1 after:content-[''] hover:bg-surface-strong hover:text-ink focus-visible:outline-none"
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

import UiInput from '@/components/ui/input.vue';

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
    /** 显式 id；缺省时自动跟随外层 AppFormItem，保证 <label for> 能命中 */
    id?: string;
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

/** 显式 id 优先；否则跟随所属 AppFormItem，让外层 <label for> 真正指向本控件。 */
const fieldId = computed(() => props.id ?? formItem?.id);
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
    // 焦点环与错误描边由 main.css 的 .app-field 单一出处承担，这里只挂类名
    'app-field bg-control-bg transition-[color,background-color,border-color,box-shadow]',
    isSm ? 'h-control-sm px-2.5 text-xs' : isLg ? 'h-control-lg px-3.5 text-base' : 'h-control-md px-3 text-sm',
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
