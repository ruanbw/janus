<script lang="ts">
import { cva } from 'class-variance-authority';

export const radioGroupItemVariants = cva(
  'flex size-4 shrink-0 items-center justify-center rounded-full border border-input text-primary shadow-xs transition-colors outline-none focus-visible:border-ring focus-visible:ring-[3px] focus-visible:ring-ring/50 disabled:cursor-not-allowed disabled:opacity-50 data-[state=checked]:border-primary',
);
</script>

<script setup lang="ts">
import { RadioGroupIndicator, RadioGroupItem, type RadioGroupItemProps } from 'reka-ui';

import { cn } from '@/lib/utils';

const props = defineProps<RadioGroupItemProps & { class?: any }>();
</script>

<template>
  <RadioGroupItem
    :value="props.value"
    :id="props.id"
    :disabled="props.disabled"
    :name="props.name"
    :as-child="props.asChild"
    :class="cn(radioGroupItemVariants(), props.class)"
  >
    <!--
      裸用法（默认）：本组件自己就是那个圆，选中时在圆心点一个点。
      卡片用法（AppRadioCard）：传入默认插槽，整张卡片成为选项本体，
      圆点由调用方用 ui/radio-group-indicator.vue 放在卡片末尾。
    -->
    <slot>
      <RadioGroupIndicator class="size-1.5 rounded-full bg-primary" />
    </slot>
  </RadioGroupItem>
</template>
