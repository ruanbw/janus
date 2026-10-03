<template>
  <UiLabel
    class="inline-flex items-center gap-2 text-sm text-ink select-none"
    :class="disabled ? 'cursor-not-allowed opacity-50' : 'cursor-pointer'"
  >
    <UiCheckbox
      :id="uid"
      :aria-label="$props['aria-label']"
      :model-value="model"
      :disabled="disabled"
      :name="name"
      :class="checkboxClasses"
      @update:model-value="onCheckedChange"
    >
      <template #indicator>
        <Minus v-if="indeterminate" :size="12" stroke-width="3" />
        <Check v-else :size="12" stroke-width="3" />
      </template>
    </UiCheckbox>
    <span v-if="$slots.default" class="text-sm font-medium leading-none">
      <slot />
    </span>
  </UiLabel>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import { Check, Minus } from '@lucide/vue';

import UiCheckbox from '@/components/ui/checkbox.vue';
import UiLabel from '@/components/ui/label.vue';

import { cn } from '@/lib/utils';
import { useFormItem } from './form';

const props = withDefaults(
  defineProps<{
    modelValue?: boolean;
    checked?: boolean;
    name?: string;
    id?: string;
    disabled?: boolean;
    /**
     * 半选（表头「全选」在部分行选中时用）。ui/checkbox 走 reka 的 CheckboxRoot，
     * 原生支持 `modelValue: 'indeterminate'`，这里只是把它暴露成一个布尔 prop。
     */
    indeterminate?: boolean;
    /**
     * 无可见文案时的无障碍名（如表格行内的勾选框）。根节点是 <label>,
     * 直接写 aria-label 会落在 label 上而不是复选框上，所以单独透传。
     */
    'aria-label'?: string;
    class?: any;
  }>(),
  { disabled: false, indeterminate: false },
);

const emit = defineEmits<{
  'update:modelValue': [val: boolean];
  'update:checked': [val: boolean];
  change: [val: boolean];
}>();

// 优先级：显式 id > 所属 AppFormItem 的 id（让 <label for> 能命中）> 兜底随机 id。
const formItem = useFormItem();
const uid = props.id ?? formItem?.id ?? 'checkbox-' + Math.random().toString(36).slice(2, 9);

const model = computed<boolean | 'indeterminate'>(() => {
  if (props.indeterminate) return 'indeterminate';
  if (props.modelValue !== undefined) return props.modelValue;
  if (props.checked !== undefined) return props.checked;
  return false;
});

const checkboxClasses = computed(() =>
  // 未选中态用控件状态层的填充与描边（--control-track / --control-thumb-edge）；
  // 选中 / 半选由 ui/checkbox 自带的 data-[state=*] 变体接管，这里不重复声明。
  cn('border-control-thumb-edge bg-control-track', props.class),
);

function onCheckedChange(val: boolean | 'indeterminate'): void {
  const bool = val === true;
  emit('update:modelValue', bool);
  emit('update:checked', bool);
  emit('change', bool);
}
</script>
