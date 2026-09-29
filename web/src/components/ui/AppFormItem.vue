<template>
  <div :class="cn('app-form-item mb-5', props.class)">
    <div
      v-if="label || $slots.label"
      class="mb-1.5 flex items-center gap-1 text-sm font-medium leading-none text-ink select-none"
    >
      <label :for="props.name">
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
      class="mt-1.5 whitespace-pre-line text-[13px] leading-relaxed text-ink-faint"
    >
      <slot name="extra">{{ extra }}</slot>
    </p>
  </div>
</template>

<script setup lang="ts">
import { computed, inject, onBeforeUnmount, onMounted, provide, ref } from 'vue';
import { CircleAlert } from '@lucide/vue';

import { cn } from '@/lib/utils';
import { formContextKey, formItemKey, validateRules } from './form';
import type { FormItemContext } from './form';
import type { FormRule } from './types';

const props = defineProps<{
  label?: string;
  name?: string;
  extra?: string;
  required?: boolean;
  class?: any;
}>();

const form = inject(formContextKey);
const invalid = ref(false);
const errorMessage = ref('');

const rules = computed<FormRule[]>(() => {
  if (props.name === undefined || form === undefined) return [];
  return form.rules[props.name] ?? [];
});

const isRequired = computed(() => {
  if (props.required !== undefined) return props.required;
  return rules.value.some((r) => r.required === true);
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
