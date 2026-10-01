<template>
  <label
    class="inline-flex items-center gap-2 text-sm text-ink select-none"
    :class="disabled ? 'cursor-not-allowed opacity-50' : 'cursor-pointer'"
  >
    <CheckboxRoot
      :id="uid"
      :aria-label="$props['aria-label']"
      :checked="model"
      :disabled="disabled"
      :name="name"
      class="peer flex size-4 shrink-0 items-center justify-center rounded-[4px] border border-control-thumb-edge bg-control-track shadow-xs transition-colors outline-none focus-visible:border-ring focus-visible:ring-[3px] focus-visible:ring-ring/50 disabled:cursor-not-allowed disabled:opacity-50 data-[state=checked]:border-primary data-[state=checked]:bg-primary data-[state=checked]:text-primary-foreground data-[state=indeterminate]:border-primary data-[state=indeterminate]:bg-primary data-[state=indeterminate]:text-primary-foreground"
      :class="props.class"
      @update:checked="onCheckedChange"
    >
      <CheckboxIndicator class="flex items-center justify-center text-current">
        <Minus v-if="indeterminate" :size="12" stroke-width="3" />
        <Check v-else :size="12" stroke-width="3" />
      </CheckboxIndicator>
    </CheckboxRoot>
    <span v-if="$slots.default" class="text-sm font-medium leading-none">
      <slot />
    </span>
  </label>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import { CheckboxIndicator, CheckboxRoot } from 'reka-ui';
import { Check, Minus } from '@lucide/vue';

const props = withDefaults(
  defineProps<{
    modelValue?: boolean;
    checked?: boolean;
    name?: string;
    id?: string;
    disabled?: boolean;
    /**
     * 半选（表头「全选」在部分行选中时用）。Reka 的 CheckboxRoot 原生支持
     * `modelValue: 'indeterminate'`，这里只是把它暴露成一个布尔 prop。
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

const uid = props.id ?? 'checkbox-' + Math.random().toString(36).slice(2, 9);

const model = computed<boolean | 'indeterminate'>(() => {
  if (props.indeterminate) return 'indeterminate';
  if (props.modelValue !== undefined) return props.modelValue;
  if (props.checked !== undefined) return props.checked;
  return false;
});

function onCheckedChange(val: boolean): void {
  emit('update:modelValue', val);
  emit('update:checked', val);
  emit('change', val);
}
</script>
