<script setup lang="ts">
import { RadioGroupRoot, type RadioGroupRootProps } from 'reka-ui';

const props = defineProps<RadioGroupRootProps>();
</script>

<template>
  <RadioGroupRoot
    :model-value="props.modelValue"
    :default-value="props.defaultValue"
    :disabled="props.disabled"
    :name="props.name"
    :orientation="props.orientation"
    :loop="props.loop"
    :dir="props.dir"
    :as-child="props.asChild"
  >
    <slot />
  </RadioGroupRoot>
</template>