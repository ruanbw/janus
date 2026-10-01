<template>
  <SelectRoot
    v-model="model"
    :multiple="multiple"
    :disabled="disabled || loading"
    v-model:open="open"
    :name="name"
  >
    <!-- 触发器外观来自 ui/select.vue(原语层只管外观) -->
    <SelectTrigger
      :id="fieldId"
      :class="triggerClasses"
      :aria-invalid="isInvalid ? 'true' : 'false'"
    >
      <!-- 多选:值标签 + 溢出计数 -->
      <div v-if="multiple" class="flex min-w-0 flex-1 flex-wrap items-center gap-1">
        <template v-for="tag in visibleTags" :key="String(tag.value)">
          <span
            class="inline-flex max-w-[160px] items-center gap-1 truncate rounded-md border border-line bg-surface-strong px-1.5 py-0.5 text-xs font-medium text-ink-soft"
          >
            <span class="truncate">{{ tag.label }}</span>
            <button
              type="button"
              tabindex="-1"
              class="relative -mr-0.5 flex size-4 shrink-0 items-center justify-center rounded-xs text-ink-faint transition-colors after:absolute after:-inset-1 after:content-[''] hover:bg-surface-strong hover:text-ink focus-visible:outline-none"
              :aria-label="'移除 ' + tag.label"
              @click.stop="removeTag(tag.value)"
            >
              <X :size="12" />
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

      <div class="flex shrink-0 items-center gap-1.5 text-ink-faint">
        <Loader2 v-if="loading" :size="14" class="animate-spin" />
        <button
          v-else-if="allowClear && hasValue && !disabled"
          type="button"
          tabindex="-1"
          class="flex size-5 items-center justify-center rounded-md transition-colors hover:bg-surface-strong hover:text-ink focus-visible:outline-none"
          aria-label="清除选择"
          @click.stop="clearValue"
        >
          <X :size="12" />
        </button>
        <SelectIcon as-child>
          <ChevronDown :size="14" class="opacity-70 transition-transform duration-200" />
        </SelectIcon>
      </div>
    </SelectTrigger>

    <!-- 内容区(搜索框 / 多选标签 / 溢出计数)是项目逻辑,留在 app/ 层直连 reka-ui -->
    <SelectPortal>
      <SelectContent
        :side-offset="4"
        position="popper"
        class="z-50 min-w-[var(--reka-select-trigger-width)] max-h-72 overflow-hidden rounded-md border border-border bg-popover p-1 text-popover-foreground shadow-md data-[state=open]:animate-in data-[state=closed]:animate-out data-[state=closed]:fade-out-0 data-[state=open]:fade-in-0 data-[state=closed]:zoom-out-95 data-[state=open]:zoom-in-95 data-[side=bottom]:slide-in-from-top-2 data-[side=left]:slide-in-from-right-2 data-[side=right]:slide-in-from-left-2 data-[side=top]:slide-in-from-bottom-2"
        @open-auto-focus="onOpenAutoFocus"
      >
        <div v-if="showSearch" class="mb-1 flex items-center border-b border-line px-2.5 py-1.5">
          <Search :size="13" class="mr-2 shrink-0 text-ink-faint" />
          <input
            ref="searchRef"
            v-model="search"
            type="text"
            placeholder="搜索选项…"
            class="w-full bg-transparent text-xs text-ink placeholder:text-ink-faint focus:outline-none"
            @click.stop
            @keydown.stop
          />
          <button
            v-if="search"
            type="button"
            class="shrink-0 text-ink-faint hover:text-ink"
            @click.stop="search = ''"
          >
            <X :size="12" />
          </button>
        </div>
        <SelectViewport class="max-h-60 overflow-y-auto p-0.5">
          <SelectItem
            v-for="opt in filteredOptions"
            :key="String(opt.value)"
            :value="opt.value"
            :disabled="opt.disabled"
            class="relative flex w-full cursor-pointer select-none items-center rounded-md py-1.5 pl-8 pr-2 text-xs outline-none transition-colors data-[disabled]:pointer-events-none data-[disabled]:opacity-50 data-[highlighted]:bg-accent data-[highlighted]:text-accent-foreground"
          >
            <span class="absolute left-2 flex size-3.5 items-center justify-center">
              <SelectItemIndicator>
                <Check :size="14" class="text-brand" />
              </SelectItemIndicator>
            </span>
            <SelectItemText>{{ opt.label }}</SelectItemText>
          </SelectItem>
          <div v-if="filteredOptions.length === 0" class="py-6 text-center text-xs text-ink-faint">
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
  SelectViewport,
} from 'reka-ui';
import { Check, ChevronDown, Loader2, Search, X } from '@lucide/vue';

import SelectTrigger from '@/components/ui/select.vue';
import SelectValue from '@/components/ui/select-value.vue';
import { cn } from '@/lib/utils';
import { useFormItem } from './form';
import type { SelectOption } from './types';

const props = withDefaults(
  defineProps<{
    modelValue?: any;
    options?: SelectOption[];
    multiple?: boolean;
    placeholder?: string;
    allowClear?: boolean;
    showSearch?: boolean;
    maxTagCount?: number;
    loading?: boolean;
    disabled?: boolean;
    size?: 'large' | 'middle' | 'small' | 'default' | 'sm' | 'lg';
    name?: string;
    invalid?: boolean;
    /** 显式 id；缺省时自动跟随外层 AppFormItem */
    id?: string;
    class?: any;
  }>(),
  {
    options: () => [],
    multiple: false,
    allowClear: false,
    showSearch: false,
    size: 'middle',
    disabled: false,
    loading: false,
  },
);

const emit = defineEmits<{
  'update:modelValue': [value: any];
  change: [value: any];
}>();

const formItem = useFormItem();

/** 显式 id 优先；否则跟随所属 AppFormItem */
const fieldId = computed(() => props.id ?? formItem?.id);
const open = ref(false);
const search = ref('');
const searchRef = ref<HTMLInputElement | null>(null);

const model = computed({
  get(): any {
    return props.modelValue;
  },
  set(value: any) {
    emit('update:modelValue', value);
    emit('change', value);
    formItem?.clearError();
  },
});

const isInvalid = computed(() => Boolean(props.invalid || formItem?.invalid.value));

const triggerClasses = computed(() => {
  const isSm = props.size === 'small' || props.size === 'sm';
  const isLg = props.size === 'large' || props.size === 'lg';
  // 多选的值标签会换行,触发器按 min-height 生长;单选固定为控件高度。
  const height = isSm
    ? props.multiple
      ? 'h-auto min-h-control-sm'
      : 'h-control-sm'
    : isLg
      ? props.multiple
        ? 'h-auto min-h-control-lg'
        : 'h-control-lg'
      : props.multiple
        ? 'h-auto min-h-control-md'
        : 'h-control-md';
  const padding = isSm ? 'py-1 text-xs' : isLg ? 'py-2 text-base' : 'py-1.5';

  return cn(
    'app-field w-full items-center justify-between gap-2 rounded-md border border-input bg-control-bg px-3 text-sm transition-[color,background-color,border-color,box-shadow] placeholder:text-muted-foreground disabled:cursor-not-allowed disabled:opacity-50',
    height,
    padding,
    isInvalid.value && 'border-err',
    props.class,
  );
});

const multipleValues = computed<Array<string | number>>(() =>
  Array.isArray(model.value) ? model.value : [],
);

const hasValue = computed(() => {
  if (props.multiple) {
    return multipleValues.value.length > 0;
  }
  return model.value !== undefined && model.value !== null && model.value !== '';
});

const filteredOptions = computed(() => {
  if (!props.showSearch || search.value.trim() === '') return props.options;
  const q = search.value.trim().toLowerCase();
  return props.options.filter((o) => o.label.toLowerCase().includes(q));
});

function labelOf(value: unknown): string {
  const found = props.options.find((o) => o.value === value);
  return found ? found.label : String(value ?? '');
}

/** 多选可见标签(maxTagCount 截断) */
const visibleTags = computed(() => {
  const values = multipleValues.value;
  const limit = props.maxTagCount !== undefined ? props.maxTagCount : values.length;
  return values.slice(0, limit).map((v) => ({ value: v, label: labelOf(v) }));
});

const hiddenCount = computed(() => {
  const values = multipleValues.value;
  const limit = props.maxTagCount !== undefined ? props.maxTagCount : values.length;
  return Math.max(0, values.length - limit);
});

function removeTag(value: string | number): void {
  const values = [...multipleValues.value];
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
  if (!isOpen) search.value = '';
});
</script>
