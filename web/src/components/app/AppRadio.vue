<template>
  <label
    :for="uid"
    class="inline-flex select-none items-center gap-2 text-sm text-ink"
    :class="disabled ? 'cursor-not-allowed opacity-50' : 'cursor-pointer'"
  >
    <RadioGroupItem
      :id="uid"
      :value="value"
      :disabled="disabled"
      class="app-field"
      :class="props.class"
    />
    <span v-if="$slots.default" class="text-sm font-medium leading-none">
      <slot />
    </span>
  </label>
</template>

<script setup lang="ts">
import RadioGroupItem from '@/components/ui/radio-group-item.vue';

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