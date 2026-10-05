<script lang="ts">
import { cva } from 'class-variance-authority';

export const progressVariants = cva(
  'relative h-2 w-full overflow-hidden rounded-full bg-muted',
);

export const progressIndicatorVariants = cva(
  'h-full w-full flex-1 rounded-full bg-primary transition-transform duration-300 ease-in-out',
);
</script>

<script setup lang="ts">
import { computed } from 'vue';
import { ProgressIndicator, ProgressRoot, type ProgressRootEmits, type ProgressRootProps } from 'reka-ui';

import { cn } from '@/lib/utils';

const props = defineProps<ProgressRootProps & { class?: any; indicatorClass?: any }>();

/**
 * 必须显式声明并转抛：只声明 props 的话，调用方的 `onUpdate:modelValue` / `onUpdate:max`
 * 会进入 $attrs，再经 fallthrough 落到根 vnode —— 而 Vue 的 filterModelListeners()
 * 会把凡是 `update:xxx`（xxx 已是本组件声明过的 prop）的键从 fallthrough 里剔除，
 * 假设「组件自己会处理」。本组件并不处理，于是监听器被静默丢弃。
 * 参见 ui/checkbox.vue 的同款写法。
 */
const emit = defineEmits<ProgressRootEmits>();

/** reka 的默认值，与 ProgressRoot 的 DEFAULT_MAX 保持一致 */
const DEFAULT_MAX = 100;

/**
 * 数值净化：原语层对非法 modelValue / max 只 console.error 然后自归 null，
 * 噪音大且进度条会掉回“未知进度”。这里在进原语之前就挡掉。
 */
const safeMax = computed(() => {
  const n = typeof props.max === 'number' ? props.max : Number(props.max);
  return Number.isFinite(n) && n > 0 ? n : DEFAULT_MAX;
});

const safeValue = computed(() => {
  const raw = props.modelValue;
  // null / undefined 在 reka 语义里就是“不确定进度”，不是错误，原样透传
  if (raw === null || raw === undefined) return null;
  const n = typeof raw === 'number' ? raw : Number(raw);
  if (Number.isNaN(n)) return 0;
  return Math.min(safeMax.value, Math.max(0, n));
});
</script>

<template>
  <ProgressRoot
    :model-value="safeValue"
    @update:model-value="emit('update:modelValue', $event)"
    @update:max="emit('update:max', $event)"
    :max="safeMax"
    :get-value-label="props.getValueLabel"
    :get-value-text="props.getValueText"
    :as-child="props.asChild"
    :as="props.as"
    :class="cn(progressVariants(), props.class)"
  >
    <ProgressIndicator :class="cn(progressIndicatorVariants(), props.indicatorClass)">
      <slot />
    </ProgressIndicator>
  </ProgressRoot>
</template>