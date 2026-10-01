<script lang="ts">
import { cva } from 'class-variance-authority';

export const switchVariants = cva(
  // 轨道本体只有 36×20,作为表格行的唯一控件热区太小(低于 WCAG 2.2 的 24px 下限,
  // 也远低于触屏 44px 的舒适值)。用一条透明伪元素把可点区域撑到 44×32:
  // 横向只外扩 4px 免得吃掉相邻单元格的点击,纵向可以放心外扩 6px ——
  // 伪元素是根节点的子节点,点击照旧冒泡到 SwitchRoot,键盘与读屏语义完全不变。
  'peer relative inline-flex shrink-0 cursor-pointer items-center rounded-full border border-transparent p-0.5 shadow-xs transition-colors outline-none after:absolute after:-inset-x-1 after:-inset-y-1.5 after:content-[""] focus-visible:border-ring focus-visible:ring-[3px] focus-visible:ring-ring/50 disabled:cursor-not-allowed disabled:opacity-50 data-[state=checked]:bg-primary data-[state=unchecked]:bg-input',
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
