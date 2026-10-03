<template>
  <div :class="cn('app-form-item mb-5', props.class)">
    <div
      v-if="label || $slots.label"
      class="mb-1.5 flex items-center gap-1 text-sm font-medium leading-none text-ink select-none"
    >
      <label :id="labelId" :for="fieldId">
        <slot name="label">{{ label }}</slot>
      </label>
      <span v-if="isRequired" class="text-xs font-bold text-err" aria-hidden="true">*</span>
    </div>
    <div :data-invalid="invalid ? 'true' : 'false'">
      <slot />
    </div>
    <p
      v-if="errorMessage"
      class="mt-1.5 flex items-center gap-1.5 text-xs font-medium text-err animate-in fade-in-50"
    >
      <CircleAlert :size="13" class="shrink-0" />
      <span>{{ errorMessage }}</span>
    </p>
    <p
      v-else-if="extra || $slots.extra"
      class="mt-1.5 whitespace-pre-line text-xs leading-relaxed text-ink-faint"
    >
      <slot name="extra">{{ extra }}</slot>
    </p>
  </div>
</template>

<script setup lang="ts">
import { computed, inject, onBeforeUnmount, onMounted, provide, ref, useId } from 'vue';
import { CircleAlert } from '@lucide/vue';

import { cn } from '@/lib/utils';
import { formContextKey, formItemKey, isBlank, isRequiredSchema, requiredMessageOf, validateWithSchema } from './form';
import type { FormItemContext } from './form';

const props = defineProps<{
  label?: string;
  name?: string;
  extra?: string;
  required?: boolean;
  class?: any;
}>();

const form = inject(formContextKey, undefined);
const invalid = ref(false);
const errorMessage = ref('');

/**
 * label 的 for 必须指向控件的真实 DOM id。useId 由 Vue 生成、同一渲染树内稳定，
 * 因此 label ↔ 控件是一对一绑定的，不会像「拿模型字段名当 id」那样指向空气。
 * 有 name 时用它做前缀，纯装饰性调用(无 name)也能拿到唯一 id。
 */
const fieldId = `app-field-${props.name ?? 'x'}-${useId()}`;
const labelId = `app-label-${props.name ?? 'x'}-${useId()}`;

const fieldSchema = computed(() => {
  if (props.name === undefined || form === undefined) return undefined;
  return form.schema[props.name];
});

const isRequired = computed(() => {
  if (props.required !== undefined) return props.required;
  return isRequiredSchema(fieldSchema.value);
});

/** 必填未填时的文案：取 schema 对空串的第一个错误 message，否则按 label 拼一句 */
const requiredMessage = computed(() => requiredMessageOf(fieldSchema.value, props.label));

async function validate(): Promise<boolean> {
  if (props.name === undefined || form === undefined) {
    return true;
  }
  const value = form.model[props.name];

  // required 此前只是个"显示红星"的开关：标了星号却拦不住空值提交。
  // 这里把它变成唯一真源——星号与拦截由同一个 isRequired 推导，两者不可能再打架。
  // 放在自定义规则之前，让"必填"永远先于"格式"报错（先说没填，再谈填错没有）。
  if (isRequired.value && isBlank(value)) {
    errorMessage.value = requiredMessage.value;
    invalid.value = true;
    return false;
  }

  const error = await validateWithSchema(fieldSchema.value, value);
  if (error !== null) {
    errorMessage.value = error;
    invalid.value = true;
    return false;
  }
  errorMessage.value = '';
  invalid.value = false;
  return true;
}

function clearError(): void {
  errorMessage.value = '';
  invalid.value = false;
}

function setError(message: string): void {
  errorMessage.value = message;
  invalid.value = true;
}

const ctx: FormItemContext = {
  get name() {
    return props.name ?? '';
  },
  id: fieldId,
  labelId,
  invalid,
  errorMessage,
  validate,
  clearError,
  setError,
};

provide(formItemKey, ctx);

onMounted(() => {
  if (props.name !== undefined && form !== undefined) {
    form.registerItem(ctx);
  }
});

onBeforeUnmount(() => {
  if (props.name !== undefined && form !== undefined) {
    form.unregisterItem(ctx);
  }
});

defineExpose({ setError });
</script>
