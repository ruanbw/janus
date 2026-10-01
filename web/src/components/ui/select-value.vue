<script setup lang="ts">
import { SelectValue, type AcceptableValue, type SelectValueProps } from 'reka-ui';

const props = defineProps<SelectValueProps & { class?: any }>();

/**
 * 显式声明默认插槽的作用域：reka 的 SelectValue 把 { modelValue, selectedLabel }
 * 透给插槽，但本组件只是转发，不声明的话 vue-tsc 把插槽参数推成 {}，
 * 调用方解构 modelValue 就报错。
 */
defineSlots<{
  default?: (slotProps: {
    modelValue: AcceptableValue | AcceptableValue[] | undefined;
    selectedLabel: string[];
  }) => unknown;
}>();
</script>

<template>
  <SelectValue
    :placeholder="props.placeholder"
    :as="props.as"
    :as-child="props.asChild"
    :class="props.class"
  >
    <!-- v-slot 转发：reka 给插槽的 { modelValue, selectedLabel } 必须原样传下去，
         直接写 <slot /> 推不出参数，写死参数又会让标签永远拿不到值 -->
    <template #default="slotProps">
      <slot v-bind="slotProps" />
    </template>
  </SelectValue>
</template>