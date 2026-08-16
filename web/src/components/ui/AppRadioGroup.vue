<template>
  <RadioGroupRoot v-model="model" :disabled="disabled" class="flex flex-wrap items-center gap-x-5 gap-y-2">
    <slot />
  </RadioGroupRoot>
</template>

<script setup lang="ts">
import { RadioGroupRoot } from 'reka-ui';

withDefaults(
  defineProps<{
    disabled?: boolean;
    name?: string;
  }>(),
  { disabled: false },
);

const model = defineModel<string | number | undefined>();
</script>
