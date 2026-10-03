<template>
  <!--
    卡片本体是一个 <button role="radio">，其内容模型只允许 phrasing content。
    块级元素（div / p）放进去是非法的，浏览器的 foster parenting 会把它们挪出按钮，
    焦点环与 aria-labelledby 的关系随之失效。所以这里的布局容器一律用 span + display 类。
  -->
  <UiRadioGroupItem
    :value="value"
    :disabled="disabled"
    :class="cardClasses"
  >
    <span
      v-if="$slots.icon || icon"
      class="mt-0.5 block shrink-0 text-ink-soft transition-colors group-data-[state=checked]:text-brand"
    >
      <slot name="icon">
        <component :is="icon" :size="20" />
      </slot>
    </span>

    <span class="block min-w-0 flex-1">
      <span class="flex items-center justify-between gap-2">
        <span class="text-sm font-semibold text-ink transition-colors group-data-[state=checked]:text-brand">
          <slot name="title">{{ title }}</slot>
          <slot v-if="!title && !$slots.title" />
        </span>
        <slot name="extra" />
      </span>
      <span
        v-if="description || $slots.description"
        class="mt-1 block text-xs leading-relaxed text-ink-faint transition-colors group-data-[state=checked]:text-ink-soft"
      >
        <slot name="description">{{ description }}</slot>
      </span>
    </span>

    <UiRadioGroupIndicator class="mt-0.5" />
  </UiRadioGroupItem>
</template>

<script setup lang="ts">
import { computed, type Component } from 'vue';

import UiRadioGroupIndicator from '@/components/ui/radio-group-indicator.vue';
import UiRadioGroupItem from '@/components/ui/radio-group-item.vue';

import { cn } from '@/lib/utils';

const props = withDefaults(
  defineProps<{
    value: string | number;
    title?: string;
    description?: string;
    icon?: Component;
    disabled?: boolean;
    class?: any;
  }>(),
  { disabled: false },
);

/**
 * 原语层的 radio-group-item 默认是「一个圆点」；这里把整张卡片变成选项本体，
 * 所以要把它的圆形外观拆掉（size-4 / rounded-full / border-input / bg 由下划线类覆盖）。
 */
const cardClasses = computed(() => {
  return cn(
    'group relative flex h-auto w-full cursor-pointer items-start gap-3 rounded-lg border bg-background p-4 text-left text-ink shadow-xs transition-all outline-none focus-visible:border-ring focus-visible:ring-[3px] focus-visible:ring-ring/50 hover:border-ring hover:bg-muted disabled:cursor-not-allowed disabled:opacity-50 data-[state=checked]:border-primary data-[state=checked]:bg-primary/10',
    props.disabled && 'pointer-events-none opacity-50',
    props.class,
  );
});
</script>
