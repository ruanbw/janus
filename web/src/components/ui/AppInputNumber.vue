<template>
  <div :class="containerClasses">
    <input
      ref="inputRef"
      :value="raw"
      :placeholder="placeholder"
      :disabled="disabled"
      :readonly="readonly"
      inputmode="decimal"
      :aria-invalid="isInvalid ? 'true' : 'false'"
      class="app-field w-full min-w-0 bg-transparent px-3 py-1 text-ink placeholder:text-ink-faint focus:outline-none disabled:cursor-not-allowed"
      :class="inputSizeClass"
      @input="onInput"
      @blur="onBlur"
      @focus="emit('focus', $event)"
      @keydown.up.prevent="stepUp"
      @keydown.down.prevent="stepDown"
    />
    <div
      v-if="!readonly"
      class="flex h-full w-6 shrink-0 flex-col divide-y divide-line border-l border-line select-none"
    >
      <button
        type="button"
        tabindex="-1"
        :disabled="disabled || isAtMax"
        class="flex flex-1 items-center justify-center text-ink-faint transition-colors hover:bg-surface-strong hover:text-ink disabled:pointer-events-none disabled:opacity-30"
        @click="stepUp"
      >
        <ChevronUp :size="12" />
      </button>
      <button
        type="button"
        tabindex="-1"
        :disabled="disabled || isAtMin"
        class="flex flex-1 items-center justify-center text-ink-faint transition-colors hover:bg-surface-strong hover:text-ink disabled:pointer-events-none disabled:opacity-30"
        @click="stepDown"
      >
        <ChevronDown :size="12" />
      </button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue';
import { ChevronDown, ChevronUp } from '@lucide/vue';

import { cn } from '@/lib/utils';
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
    readonly?: boolean;
    size?: 'large' | 'middle' | 'small' | 'default' | 'sm' | 'lg';
    invalid?: boolean;
    class?: any;
  }>(),
  { size: 'middle', step: 1, disabled: false, readonly: false },
);

const emit = defineEmits<{
  'update:modelValue': [value: number | undefined];
  change: [value: number | undefined];
  blur: [event: FocusEvent];
  focus: [event: FocusEvent];
}>();

const formItem = useFormItem();
const inputRef = ref<HTMLInputElement | null>(null);

const raw = ref<string>(props.modelValue === undefined ? '' : String(props.modelValue));

watch(
  () => props.modelValue,
  (val) => {
    raw.value = val === undefined ? '' : String(val);
  },
);

const isInvalid = computed(() => Boolean(props.invalid || formItem?.invalid.value));

const isAtMax = computed(() => {
  if (props.max === undefined) return false;
  const num = Number(raw.value);
  return !Number.isNaN(num) && num >= props.max;
});

const isAtMin = computed(() => {
  if (props.min === undefined) return false;
  const num = Number(raw.value);
  return !Number.isNaN(num) && num <= props.min;
});

const containerClasses = computed(() => {
  const isSm = props.size === 'small' || props.size === 'sm';
  const isLg = props.size === 'large' || props.size === 'lg';

  return cn(
    'relative flex w-full items-center overflow-hidden rounded-md border border-input bg-control-bg shadow-xs transition-[color,background-color,border-color,box-shadow] focus-within:border-ring focus-within:ring-[3px] focus-within:ring-ring/50',
    isSm ? 'h-8' : isLg ? 'h-10' : 'h-9',
    props.disabled && 'cursor-not-allowed opacity-50',
    isInvalid.value && 'border-err focus-within:border-err focus-within:ring-err/40',
    props.class,
  );
});

const inputSizeClass = computed(() => {
  if (props.size === 'small' || props.size === 'sm') return 'text-xs';
  if (props.size === 'large' || props.size === 'lg') return 'text-base';
  return 'text-sm';
});

function clampAndRound(val: number): number {
  let res = val;
  if (props.min !== undefined && res < props.min) res = props.min;
  if (props.max !== undefined && res > props.max) res = props.max;
  if (props.precision !== undefined) {
    res = Number(res.toFixed(props.precision));
  } else {
    // 处理浮点精度
    res = Math.round(res * 1e10) / 1e10;
  }
  return res;
}

function commit(val: number | undefined): void {
  raw.value = val === undefined ? '' : String(val);
  emit('update:modelValue', val);
  emit('change', val);
  formItem?.clearError();
}

function stepUp(): void {
  if (props.disabled || props.readonly) return;
  const current = raw.value.trim() === '' ? (props.min ?? 0) : Number(raw.value);
  const base = Number.isNaN(current) ? 0 : current;
  const next = clampAndRound(base + props.step);
  commit(next);
}

function stepDown(): void {
  if (props.disabled || props.readonly) return;
  const current = raw.value.trim() === '' ? (props.min ?? 0) : Number(raw.value);
  const base = Number.isNaN(current) ? 0 : current;
  const next = clampAndRound(base - props.step);
  commit(next);
}

function onInput(event: Event): void {
  const el = event.target as HTMLInputElement;
  raw.value = el.value;
  formItem?.clearError();
}

function onBlur(event: FocusEvent): void {
  emit('blur', event);
  const trimmed = raw.value.trim();
  if (trimmed === '') {
    commit(undefined);
    return;
  }
  let value = Number(trimmed);
  if (Number.isNaN(value)) {
    raw.value = props.modelValue === undefined ? '' : String(props.modelValue);
    return;
  }
  value = clampAndRound(value);
  commit(value);
}
</script>
