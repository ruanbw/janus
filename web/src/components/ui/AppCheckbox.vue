<template>
  <CheckboxRoot
    v-model:checked="checked"
    :disabled="disabled"
    :name="name"
    class="group inline-flex cursor-pointer items-center gap-2 disabled:cursor-not-allowed"
  >
    <span
      class="flex h-4 w-4 shrink-0 items-center justify-center rounded border border-line-strong bg-surface transition-colors group-data-[state=checked]:border-brand-600 group-data-[state=checked]:bg-brand-600 group-data-[disabled]:opacity-50"
    >
      <CheckboxIndicator>
        <Check :size="12" stroke-width="3" class="text-white" />
      </CheckboxIndicator>
    </span>
    <span class="text-[13px] text-ink select-none"><slot /></span>
  </CheckboxRoot>
</template>

<script setup lang="ts">
import { CheckboxIndicator, CheckboxRoot } from 'reka-ui';
import { Check } from '@lucide/vue';

withDefaults(
  defineProps<{
    name?: string;
    disabled?: boolean;
  }>(),
  { disabled: false },
);

const checked = defineModel<boolean>({ default: false });
</script>
