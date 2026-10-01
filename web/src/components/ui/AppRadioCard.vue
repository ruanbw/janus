<template>
  <RadioGroupItem
    :value="value"
    :disabled="disabled"
    :class="cardClasses"
  >
    <div
      v-if="$slots.icon || icon"
      class="mt-0.5 shrink-0 text-ink-soft transition-colors group-data-[state=checked]:text-primary"
    >
      <slot name="icon">
        <component :is="icon" :size="20" />
      </slot>
    </div>

    <div class="min-w-0 flex-1">
      <div class="flex items-center justify-between gap-2">
        <div class="text-sm font-semibold text-ink transition-colors group-data-[state=checked]:text-primary">
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

    <div class="mt-0.5 flex size-4 shrink-0 items-center justify-center rounded-full border border-control-thumb-edge bg-control-track transition-colors group-data-[state=checked]:border-primary group-data-[state=checked]:bg-primary">
      <RadioGroupIndicator class="flex items-center justify-center">
        <span class="size-1.5 rounded-full bg-primary-foreground" />
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
    'group relative flex w-full cursor-pointer items-start gap-3 rounded-xl border border-input bg-background p-4 text-left shadow-xs transition-all outline-none focus-visible:border-ring focus-visible:ring-[3px] focus-visible:ring-ring/50 hover:border-ring hover:bg-muted disabled:cursor-not-allowed disabled:opacity-50 data-[state=checked]:border-primary data-[state=checked]:bg-primary/10',
    props.disabled && 'pointer-events-none opacity-50',
    props.class,
  );
});
</script>
