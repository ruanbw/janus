<script lang="ts">
import { cva } from 'class-variance-authority';

export const checkboxVariants = cva(
  'peer flex size-4 shrink-0 items-center justify-center rounded-[4px] border border-input shadow-xs transition-colors outline-none focus-visible:border-ring focus-visible:ring-[3px] focus-visible:ring-ring/50 disabled:cursor-not-allowed disabled:opacity-50 data-[state=checked]:border-primary data-[state=checked]:bg-primary data-[state=checked]:text-primary-foreground data-[state=indeterminate]:border-primary data-[state=indeterminate]:bg-primary data-[state=indeterminate]:text-primary-foreground',
);

export const checkboxIndicatorVariants = cva(
  'flex items-center justify-center text-current',
);
</script>

<script setup lang="ts">
import { CheckboxIndicator, CheckboxRoot, type CheckboxRootProps } from 'reka-ui';
import { Check } from '@lucide/vue';

import { cn } from '@/lib/utils';

const props = defineProps<CheckboxRootProps & { class?: any }>();
</script>

<template>
  <CheckboxRoot
    :model-value="props.modelValue"
    :default-value="props.defaultValue"
    :disabled="props.disabled"
    :id="props.id"
    :name="props.name"
    :value="props.value"
    :true-value="props.trueValue"
    :false-value="props.falseValue"
    :as-child="props.asChild"
    :class="cn(checkboxVariants(), props.class)"
  >
    <CheckboxIndicator :class="checkboxIndicatorVariants()">
      <slot name="indicator">
        <Check :size="12" :stroke-width="3" />
      </slot>
    </CheckboxIndicator>
  </CheckboxRoot>
</template>