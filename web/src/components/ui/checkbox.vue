<script lang="ts">
import { cva } from 'class-variance-authority';

export const checkboxVariants = cva(
  // 16px 的方框直接当热区太小,尤其是表格里的全选/多选。透明伪元素把可点区域
  // 撑到 32×32(移动端舒适值),视觉尺寸不变;点击经伪元素冒泡回 CheckboxRoot,
  // 键盘可达性与 aria 语义都不受影响。
  // 未选中态悬停时描边转主色(Element 同款),是"可以点我"的第一层信号;
  // 已选中/半选的填充由 data-[state=*] 变体接管,悬停不再改变填充,只保持描边。
  'peer relative flex size-4 shrink-0 items-center justify-center rounded-[4px] border border-input shadow-xs transition-colors outline-none after:absolute after:-inset-2 after:content-[""] hover:border-primary focus-visible:border-ring focus-visible:ring-[3px] focus-visible:ring-ring/50 disabled:cursor-not-allowed disabled:opacity-50 disabled:hover:border-input data-[state=checked]:border-primary data-[state=checked]:bg-primary data-[state=checked]:text-primary-foreground data-[state=indeterminate]:border-primary data-[state=indeterminate]:bg-primary data-[state=indeterminate]:text-primary-foreground',
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
const emit = defineEmits<{ 'update:modelValue': [val: boolean | 'indeterminate'] }>();
</script>

<template>
  <CheckboxRoot
    :model-value="props.modelValue"
    @update:model-value="emit('update:modelValue', $event)"
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
