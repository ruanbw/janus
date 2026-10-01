<script lang="ts">
import { cva } from 'class-variance-authority';

export const switchVariants = cva(
  'peer relative inline-flex shrink-0 cursor-pointer items-center rounded-full border border-transparent p-0.5 shadow-xs transition-colors outline-none focus-visible:border-ring focus-visible:ring-[3px] focus-visible:ring-ring/50 disabled:cursor-not-allowed disabled:opacity-50 data-[state=checked]:bg-primary data-[state=unchecked]:bg-input',
);

export const switchThumbVariants = cva(
  'pointer-events-none block size-4 translate-x-0 rounded-full bg-background shadow-sm ring-0 transition-transform duration-150 data-[state=checked]:translate-x-4',
);
</script>

<script setup lang="ts">
import { SwitchRoot, SwitchThumb, type SwitchRootProps } from 'reka-ui';

import { cn } from '@/lib/utils';

const props = defineProps<SwitchRootProps & { class?: any }>();
</script>

<template>
  <SwitchRoot
    :model-value="props.modelValue"
    :default-value="props.defaultValue"
    :disabled="props.disabled"
    :id="props.id"
    :name="props.name"
    :value="props.value"
    :true-value="props.trueValue"
    :false-value="props.falseValue"
    :as-child="props.asChild"
    :class="cn(switchVariants(), props.class)"
  >
    <SwitchThumb :class="switchThumbVariants()">
      <slot />
    </SwitchThumb>
  </SwitchRoot>
</template>