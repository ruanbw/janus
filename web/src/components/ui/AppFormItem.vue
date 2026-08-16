<template>
  <div class="app-form-item mb-5">
    <label v-if="label" class="mb-1.5 block text-[13px] font-medium text-ink">{{ label }}</label>
    <div :data-invalid="invalid ? 'true' : 'false'">
      <slot />
    </div>
    <p v-if="extra && !errorMessage" class="mt-1.5 whitespace-pre-line text-xs leading-relaxed text-ink-faint">{{ extra }}</p>
    <p v-if="errorMessage" class="mt-1.5 flex items-center gap-1 text-xs text-err">
      <CircleAlert :size="12" />
      {{ errorMessage }}
    </p>
  </div>
</template>

<script setup lang="ts">
import { computed, inject, onBeforeUnmount, onMounted, provide, ref } from 'vue';
import { CircleAlert } from '@lucide/vue';

import { formContextKey, formItemKey, validateRules } from './form';
import type { FormItemContext } from './form';
import type { FormRule } from './types';

const props = defineProps<{
  label?: string;
  /** 与 form.model 的键对应;缺省时不参与校验 */
  name?: string;
  /** 字段说明(extra) */
  extra?: string;
}>();

const form = inject(formContextKey);
const invalid = ref(false);
const errorMessage = ref('');

const rules = computed<FormRule[]>(() => {
  if (props.name === undefined || form === undefined) return [];
  return form.rules[props.name] ?? [];
});

async function validate(): Promise<boolean> {
  if (props.name === undefined || form === undefined) {
    return true;
  }
  const value = form.model[props.name];
  const error = await validateRules(rules.value, value);
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

const ctx: FormItemContext = {
  get name() {
    return props.name ?? '';
  },
  invalid,
  errorMessage,
  validate,
  clearError,
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
</script>
