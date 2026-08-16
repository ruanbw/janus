<template>
  <div class="group inline-flex cursor-pointer items-center gap-2 disabled:cursor-not-allowed">
    <RadioGroupItem
      :value="value"
      :id="uid"
      :disabled="disabled"
      class="flex h-4 w-4 shrink-0 items-center justify-center rounded-full border border-line-strong bg-surface transition-colors group-data-[state=checked]:border-brand-600"
    >
      <RadioGroupIndicator>
        <span class="block h-2 w-2 rounded-full bg-brand-600" />
      </RadioGroupIndicator>
    </RadioGroupItem>
    <label :for="uid" class="cursor-pointer text-[13px] text-ink select-none"><slot /></label>
  </div>
</template>

<script setup lang="ts">
import { RadioGroupIndicator, RadioGroupItem } from 'reka-ui';

withDefaults(
  defineProps<{
    value: string | number;
    disabled?: boolean;
  }>(),
  { disabled: false },
);

const uid = 'radio-' + Math.random().toString(36).slice(2, 9);
</script>
