<script lang="ts">
import { cva } from 'class-variance-authority';

export const radioGroupItemVariants = cva(
  // 未选中态悬停描边转主色(Element 同款)。AppRadioCard 会用 class 覆盖掉
  // size-4 / rounded-full 变成整张卡片,圆点本身不再单独 hover。
  'flex size-4 shrink-0 items-center justify-center rounded-full border border-input text-primary shadow-xs transition-colors outline-none hover:border-primary focus-visible:border-ring focus-visible:ring-[3px] focus-visible:ring-ring/50 disabled:cursor-not-allowed disabled:opacity-50 disabled:hover:border-input data-[state=checked]:border-primary',
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
    :required="props.required"
    :as-child="props.asChild"
    :as="props.as"
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
