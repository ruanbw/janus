<template>
  <div :class="containerClasses">
    <UiInput
      :id="fieldId"
      :value="raw"
      :placeholder="placeholder"
      :disabled="disabled"
      :readonly="readonly"
      inputmode="decimal"
      :aria-invalid="isInvalid ? 'true' : 'false'"
      :class="inputClasses"
      @input="onInput"
      @blur="onBlur"
      @focus="emit('focus', $event)"
      @keydown.up.prevent="stepUp"
      @keydown.down.prevent="stepDown"
    />
    <div
      v-if="!readonly"
      class="flex h-full w-7 shrink-0 flex-col divide-y divide-line border-l border-line select-none"
    >
      <button
        type="button"
        tabindex="-1"
        aria-label="增加"
        title="增加（也可聚焦输入框后按 ↑）"
        :disabled="disabled || isAtMax"
        class="flex flex-1 items-center justify-center text-ink-faint transition-colors hover:bg-surface-strong hover:text-ink disabled:pointer-events-none disabled:opacity-30"
        @click="stepUp"
      >
        <ChevronUp :size="12" />
      </button>
      <button
        type="button"
        tabindex="-1"
        aria-label="减少"
        title="减少（也可聚焦输入框后按 ↓）"
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

import { Input as UiInput } from '@/components/ui/input';

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
    /** 显式 id；缺省时自动跟随外层 AppFormItem */
    id?: string;
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

/** 显式 id 优先；否则跟随所属 AppFormItem */
const fieldId = computed(() => props.id ?? formItem?.id);

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

/**
 * 步进按钮是一个独立的侧栏而不是浮层，所以容器自己保留边框，
 * ui/input 在内部退化为「无边框的输入区」——这是原语层唯一没覆盖到的排版形态。
 */
const containerClasses = computed(() => {
  const isSm = props.size === 'small' || props.size === 'sm';
  const isLg = props.size === 'large' || props.size === 'lg';

  return cn(
    'relative flex w-full items-center overflow-hidden rounded-md border border-input bg-control-bg transition-[color,background-color,border-color,box-shadow]',
    isSm ? 'h-control-sm' : isLg ? 'h-control-lg' : 'h-control-md',
    'focus-within:border-ring',
    props.disabled && 'cursor-not-allowed',
    isInvalid.value && 'border-err',
    props.class,
  );
});

const inputClasses = computed(() => {
  const isSm = props.size === 'small' || props.size === 'sm';
  const isLg = props.size === 'large' || props.size === 'lg';

  return cn(
    // 焦点环与错误描边由 main.css 的 .app-field 单一出处承担，这里只挂类名
    'app-field h-full w-full min-w-0 rounded-none border-0 bg-transparent py-1 disabled:cursor-not-allowed',
    isSm ? 'px-2.5 text-xs' : isLg ? 'px-3.5 text-base' : 'px-3 text-sm',
  );
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
