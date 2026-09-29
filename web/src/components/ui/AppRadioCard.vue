<template>
  <RadioGroupItem
    :value="value"
    :disabled="disabled"
    :class="cardClasses"
  >
    <div
      v-if="$slots.icon || icon"
      class="mt-0.5 shrink-0 text-ink-soft transition-colors group-data-[state=checked]:text-brand-600 dark:group-data-[state=checked]:text-brand-400"
    >
      <slot name="icon">
        <component :is="icon" :size="20" />
      </slot>
    </div>

    <div class="min-w-0 flex-1">
      <div class="flex items-center justify-between gap-2">
        <div class="text-sm font-semibold text-ink transition-colors group-data-[state=checked]:text-brand-700 dark:group-data-[state=checked]:text-brand-300">
          <slot name="title">{{ title }}</slot>
          <slot v-if="!title && !$slots.title" />
        </div>
        <slot name="extra" />
      </div>
      <p
        v-if="description || $slots.description"
        class="mt-1 text-xs leading-relaxed text-ink-faint transition-colors group-data-[state=checked]:text-ink-soft"
      >
        <slot name="description">{{ description }}</slot>
      </p>
    </div>

    <div class="mt-0.5 flex h-4 w-4 shrink-0 items-center justify-center rounded-full border border-line-strong transition-colors group-data-[state=checked]:border-brand-600 group-data-[state=checked]:bg-brand-600 dark:group-data-[state=checked]:border-brand-500 dark:group-data-[state=checked]:bg-brand-500">
      <RadioGroupIndicator class="flex items-center justify-center">
        <span class="h-1.5 w-1.5 rounded-full bg-white" />
      </RadioGroupIndicator>
    </div>
  </RadioGroupItem>
</template>

<script setup lang="ts">
import { computed, type Component } from 'vue';
import { RadioGroupIndicator, RadioGroupItem } from 'reka-ui';

import { cn } from '@/lib/utils';

const props = withDefaults(
  defineProps<{
    value: string | number;
    title?: string;
    description?: string;
    icon?: Component;
    disabled?: boolean;
    class?: any;
  }>(),
  { disabled: false },
);

const cardClasses = computed(() => {
  return cn(
    'group relative flex w-full cursor-pointer items-start gap-3 rounded-xl border border-line bg-surface/50 p-4 text-left shadow-xs transition-all hover:border-line-strong hover:bg-surface focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500 focus-visible:ring-offset-2 ring-offset-surface disabled:cursor-not-allowed disabled:opacity-50 data-[state=checked]:border-brand-600 data-[state=checked]:bg-brand-50/40 data-[state=checked]:shadow-xs dark:bg-surface-strong/20 dark:hover:bg-surface-strong/40 dark:data-[state=checked]:border-brand-500 dark:data-[state=checked]:bg-brand-500/10',
    props.disabled && 'pointer-events-none opacity-50',
    props.class,
  );
});
</script>
