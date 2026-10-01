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
      class="peer flex size-4 shrink-0 items-center justify-center rounded-full border border-control-thumb-edge bg-control-track shadow-xs transition-colors outline-none focus-visible:border-ring focus-visible:ring-[3px] focus-visible:ring-ring/50 disabled:cursor-not-allowed disabled:opacity-50 data-[state=checked]:border-primary data-[state=checked]:bg-primary"
      :class="props.class"
    >
      <RadioGroupIndicator class="flex items-center justify-center">
        <span class="size-1.5 rounded-full bg-primary-foreground" />
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
