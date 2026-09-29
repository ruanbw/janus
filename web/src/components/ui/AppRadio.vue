<template>
  <label
    :for="uid"
    class="inline-flex items-center gap-2 text-sm text-ink select-none"
    :class="disabled ? 'cursor-not-allowed opacity-50' : 'cursor-pointer'"
  >
    <RadioGroupItem
      :id="uid"
      :value="value"
      :disabled="disabled"
      class="peer flex h-4 w-4 shrink-0 items-center justify-center rounded-full border border-line-strong bg-surface shadow-xs transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500 focus-visible:ring-offset-2 ring-offset-surface disabled:cursor-not-allowed disabled:opacity-50 data-[state=checked]:border-brand-600 dark:data-[state=checked]:border-brand-500"
      :class="props.class"
    >
      <RadioGroupIndicator class="flex items-center justify-center">
        <span class="h-2 w-2 rounded-full bg-brand-600 dark:bg-brand-500" />
      </RadioGroupIndicator>
    </RadioGroupItem>
    <span v-if="$slots.default" class="text-sm font-medium leading-none">
      <slot />
    </span>
  </label>
</template>

<script setup lang="ts">
import { RadioGroupIndicator, RadioGroupItem } from 'reka-ui';

const props = withDefaults(
  defineProps<{
    value: string | number;
    disabled?: boolean;
    id?: string;
    class?: any;
  }>(),
  { disabled: false },
);

const uid = props.id ?? 'radio-' + Math.random().toString(36).slice(2, 9);
</script>
