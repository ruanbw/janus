<template>
  <SelectRoot
    v-model="model"
    :multiple="multiple"
    :disabled="disabled || loading"
    v-model:open="open"
    :name="name"
    class="relative w-full"
  >
    <SelectTrigger
      class="app-field flex w-full items-center justify-between gap-2 rounded-lg border border-line bg-surface px-3 text-sm text-ink transition-colors focus:border-brand-500 focus:outline-none disabled:cursor-not-allowed disabled:bg-surface-strong disabled:text-ink-faint"
      :class="[sizeClasses, formItem?.invalid.value ? 'border-err' : '']"
    >
      <!-- 多选:值标签 + 溢出计数 -->
      <div v-if="multiple" class="flex min-w-0 flex-1 flex-wrap items-center gap-1 py-1">
        <template v-for="tag in visibleTags" :key="tag.value">
          <span
            class="inline-flex max-w-[160px] items-center gap-1 truncate rounded bg-brand-50 px-1.5 py-0.5 text-xs text-brand-700 dark:bg-brand-500/15 dark:text-brand-300"
          >
            <span class="truncate">{{ tag.label }}</span>
            <button
              type="button"
              class="shrink-0 opacity-60 hover:opacity-100"
              @click.stop="removeTag(tag.value)"
            >
              <X :size="11" />
            </button>
          </span>
        </template>
        <span v-if="hiddenCount > 0" class="text-xs text-ink-faint">+{{ hiddenCount }}</span>
        <span v-if="multipleValues.length === 0" class="text-ink-faint">{{ placeholder }}</span>
      </div>
      <!-- 单选:值/占位 -->
      <SelectValue v-else :placeholder="placeholder" class="min-w-0 flex-1 truncate text-left">
        <template #default="{ modelValue: v }">
          <span class="block truncate">{{ labelOf(v) }}</span>
        </template>
      </SelectValue>

      <Loader2 v-if="loading" :size="14" class="shrink-0 animate-spin text-ink-faint" />
      <button
        v-else-if="allowClear && (multiple ? multipleValues.length > 0 : model !== undefined && model !== null && model !== '')"
        type="button"
        class="shrink-0 text-ink-faint hover:text-ink"
        @click.stop="clearValue"
      >
        <X :size="13" />
      </button>
      <SelectIcon v-else class="shrink-0 text-ink-faint">
        <ChevronDown :size="15" />
      </SelectIcon>
    </SelectTrigger>

    <SelectPortal>
      <SelectContent
        :side-offset="4"
        position="popper"
        class="z-50 max-h-72 min-w-[var(--reka-select-trigger-width)] overflow-hidden rounded-xl border border-line bg-surface p-1 shadow-xl"
        @open-auto-focus="onOpenAutoFocus"
      >
        <SelectViewport class="max-h-64 overflow-y-auto p-0.5">
          <div v-if="showSearch" class="px-1 pb-1 pt-0.5">
            <input
              ref="searchRef"
              v-model="search"
              type="text"
              placeholder="搜索…"
              class="w-full rounded-md border border-line bg-surface-muted px-2.5 py-1.5 text-[13px] text-ink placeholder:text-ink-faint focus:border-brand-500 focus:outline-none"
              @click.stop
              @keydown.stop
            />
          </div>
          <SelectItem
            v-for="opt in filteredOptions"
            :key="String(opt.value)"
            :value="opt.value"
            :disabled="opt.disabled"
            class="flex cursor-pointer items-center justify-between gap-2 rounded-md px-2.5 py-1.5 text-[13px] text-ink outline-none data-[disabled]:cursor-not-allowed data-[disabled]:opacity-50 data-[highlighted]:bg-brand-50 data-[highlighted]:text-brand-700 dark:data-[highlighted]:bg-brand-500/15 dark:data-[highlighted]:text-brand-300"
          >
            <SelectItemText>{{ opt.label }}</SelectItemText>
            <SelectItemIndicator class="shrink-0">
              <Check :size="14" class="text-brand-600 dark:text-brand-400" />
            </SelectItemIndicator>
          </SelectItem>
          <div v-if="filteredOptions.length === 0" class="px-2.5 py-3 text-center text-xs text-ink-faint">
            无匹配选项
          </div>
        </SelectViewport>
      </SelectContent>
    </SelectPortal>
  </SelectRoot>
</template>

<script setup lang="ts">
import { computed, nextTick, ref, watch } from 'vue';
import {
  SelectContent,
  SelectIcon,
  SelectItem,
  SelectItemIndicator,
  SelectItemText,
  SelectPortal,
  SelectRoot,
  SelectTrigger,
  SelectValue,
  SelectViewport,
} from 'reka-ui';
import { Check, ChevronDown, Loader2, X } from '@lucide/vue';

import { useFormItem } from './form';
import type { SelectOption } from './types';

const props = withDefaults(
  defineProps<{
    modelValue?: string | number | Array<string | number> | undefined;
    options?: SelectOption[];
    multiple?: boolean;
    placeholder?: string;
    allowClear?: boolean;
    showSearch?: boolean;
    maxTagCount?: number;
    loading?: boolean;
    disabled?: boolean;
    size?: 'large' | 'middle' | 'small' | 'default';
    name?: string;
  }>(),
  { options: () => [], multiple: false, allowClear: false, showSearch: false, size: 'middle', disabled: false, loading: false },
);

const emit = defineEmits<{
  'update:modelValue': [value: unknown];
  change: [value: unknown];
}>();

const formItem = useFormItem();
const open = ref(false);
const search = ref('');
const searchRef = ref<HTMLInputElement | null>(null);

const model = computed({
  get(): string | number | Array<string | number> | undefined {
    return props.modelValue;
  },
  set(value: unknown) {
    emit('update:modelValue', value as string | number | Array<string | number> | undefined);
    emit('change', value);
    formItem?.clearError();
  },
});

const sizeClasses = computed(() => {
  if (props.size === 'large') return 'min-h-10 text-[15px]';
  if (props.size === 'small') return 'min-h-8 text-[13px]';
  return 'min-h-9';
});

const multipleValues = computed<Array<string | number>>(() =>
  Array.isArray(model.value) ? model.value : [],
);

const filteredOptions = computed(() => {
  if (props.showSearch === false || search.value.trim() === '') return props.options;
  const q = search.value.trim().toLowerCase();
  return props.options.filter((o) => o.label.toLowerCase().includes(q));
});

function labelOf(value: unknown): string {
  const found = props.options.find((o) => o.value === value);
  return found ? found.label : String(value ?? '');
}

/** 多选可见标签(maxTagCount 截断) */
const visibleTags = computed(() => {
  const values = Array.isArray(model.value) ? model.value : [];
  const limit = props.maxTagCount !== undefined ? props.maxTagCount : values.length;
  return values.slice(0, limit).map((v) => ({ value: v, label: labelOf(v) }));
});

const hiddenCount = computed(() => {
  const values = Array.isArray(model.value) ? model.value : [];
  const limit = props.maxTagCount !== undefined ? props.maxTagCount : values.length;
  return Math.max(0, values.length - limit);
});

function removeTag(value: string | number): void {
  const values = Array.isArray(model.value) ? [...model.value] : [];
  const index = values.indexOf(value);
  if (index >= 0) {
    values.splice(index, 1);
    model.value = values;
  }
}

function clearValue(): void {
  model.value = props.multiple ? [] : undefined;
}

function onOpenAutoFocus(): void {
  if (props.showSearch) {
    void nextTick(() => {
      searchRef.value?.focus();
    });
  }
}

watch(open, (isOpen) => {
  if (isOpen === false) search.value = '';
});
</script>
