<script lang="ts">
import { cva } from 'class-variance-authority';

/**
 * 单选标记（外圈 + 内点）。默认由 radio-group-item.vue 自动渲染；
 * 当「选项本身」是一张大卡片（AppRadioCard）时，卡片里显式放一个本组件，
 * 免得在项目层重新拼装 reka 的 indicator。
 */
export const radioGroupIndicatorVariants = cva(
  'flex size-4 shrink-0 items-center justify-center rounded-full border border-input text-primary shadow-xs transition-colors data-[state=checked]:border-primary data-[state=checked]:bg-primary',
);

export const radioGroupIndicatorDotVariants = cva(
  'size-1.5 rounded-full bg-primary-foreground',
);
</script>

<script setup lang="ts">
import { RadioGroupIndicator, type RadioGroupIndicatorProps } from 'reka-ui';

import { cn } from '@/lib/utils';

const props = defineProps<RadioGroupIndicatorProps & { class?: any }>();
</script>

<template>
  <RadioGroupIndicator :class="cn(radioGroupIndicatorVariants(), props.class)" :force-mount="props.forceMount" :as-child="props.asChild" :as="props.as">
    <span :class="radioGroupIndicatorDotVariants()" />
  </RadioGroupIndicator>
</template>
