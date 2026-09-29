<template>
  <label
    class="inline-flex items-center gap-2 text-sm text-ink select-none"
    :class="disabled ? 'cursor-not-allowed opacity-50' : 'cursor-pointer'"
  >
    <CheckboxRoot
      :id="uid"
      :checked="model"
      :disabled="disabled"
      :name="name"
      class="peer flex h-4 w-4 shrink-0 items-center justify-center rounded-[4px] border border-line-strong bg-surface shadow-xs transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500 focus-visible:ring-offset-2 ring-offset-surface disabled:cursor-not-allowed disabled:opacity-50 data-[state=checked]:border-brand-600 data-[state=checked]:bg-brand-600 data-[state=checked]:text-white dark:data-[state=checked]:border-brand-500 dark:data-[state=checked]:bg-brand-500"
      :class="props.class"
      @update:checked="onCheckedChange"
    >
      <CheckboxIndicator class="flex items-center justify-center text-current">
        <Check :size="12" stroke-width="3" />
      </CheckboxIndicator>
    </CheckboxRoot>
    <span v-if="$slots.default" class="text-sm font-medium leading-none">
      <slot />
    </span>
  </label>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import { CheckboxIndicator, CheckboxRoot } from 'reka-ui';
import { Check } from '@lucide/vue';

const props = withDefaults(
  defineProps<{
    modelValue?: boolean;
    checked?: boolean;
    name?: string;
    id?: string;
    disabled?: boolean;
    class?: any;
  }>(),
  { disabled: false },
);

const emit = defineEmits<{
  'update:modelValue': [val: boolean];
  'update:checked': [val: boolean];
  change: [val: boolean];
}>();

const uid = props.id ?? 'checkbox-' + Math.random().toString(36).slice(2, 9);

const model = computed(() => {
  if (props.modelValue !== undefined) return props.modelValue;
  if (props.checked !== undefined) return props.checked;
  return false;
});

function onCheckedChange(val: boolean): void {
  emit('update:modelValue', val);
  emit('update:checked', val);
  emit('change', val);
}
</script>
