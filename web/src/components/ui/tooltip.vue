<script setup lang="ts">
import { TooltipProvider, TooltipRoot, type TooltipProviderProps, type TooltipRootProps } from 'reka-ui';

/**
 * open / disabled / ignoreNonKeyboardFocus 原先只声明不转发：TooltipRoot 只拿到
 * defaultOpen，于是 v-model:open 完全失效，disabled / ignoreNonKeyboardFocus 也形同虚设。
 * 这里全部透传，并重新派发 update:open 让 v-model:open 成立。
 *
 * 注意 attrs：本组件 → TooltipProvider → TooltipRoot → PopperRoot 一路都是
 * reka 的 inheritAttrs:false 无渲染根，整条链上不存在任何承载 DOM 的元素，
 * 所以未声明的 attr 无处可落（显式 v-bind="$attrs" 也一样，PopperRoot 会照丢）。
 * 需要控制外观的请走 class prop，由 AppTooltip 显式转给 UiTooltipContent。
 */
const props = defineProps<TooltipProviderProps & TooltipRootProps>();

const emit = defineEmits<{ 'update:open': [val: boolean] }>();
</script>

<template>
  <TooltipProvider
    :delay-duration="props.delayDuration"
    :skip-delay-duration="props.skipDelayDuration"
    :disable-hoverable-content="props.disableHoverableContent"
    :disable-closing-trigger="props.disableClosingTrigger"
    :disabled="props.disabled"
    :ignore-non-keyboard-focus="props.ignoreNonKeyboardFocus"
    :content="props.content"
  >
    <TooltipRoot
      :open="props.open"
      :default-open="props.defaultOpen"
      :delay-duration="props.delayDuration"
      :disable-hoverable-content="props.disableHoverableContent"
      :disable-closing-trigger="props.disableClosingTrigger"
      :disabled="props.disabled"
      :ignore-non-keyboard-focus="props.ignoreNonKeyboardFocus"
      @update:open="emit('update:open', $event)"
  >
      <slot />
    </TooltipRoot>
  </TooltipProvider>
</template>