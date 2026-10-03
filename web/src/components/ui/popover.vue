<script setup lang="ts">
import { PopoverRoot, type PopoverRootEmits, type PopoverRootProps } from 'reka-ui';

/**
 * `open` / `modal` 必须保持三态（undefined / true / false）。
 * Vue 对 Boolean 类型的 prop 会在「未传入」时强制补成 false，
 * 若直接转发 props.open，reka 的 `passive: props.open === void 0`
 * 就会恒为 false —— defaultOpen 永远不生效、受控分支永远进不去。
 * withDefaults 显式声明 default: undefined，把三态还回去。
 *
 * 根上必须自己渲染一个元素来承接 class 与 fallthrough attrs：
 * PopoverRoot → PopperRoot 这条链路的终点是 PopperRoot，
 * 它自带 inheritAttrs: false 且只 return renderSlot($slots, 'default')，
 * 于是根上的 class 与所有 data-* 会被静默丢弃，连 warn 都不触发。
 * display:contents 让这个包裹层不生成盒子，布局与原先「无根元素」完全一致。
 *
 * 注意：这段说明必须留在 <script> 里。写成 <template> 里的 HTML 注释会让
 * Vue 把根编译成 Fragment（注释也算根节点之一），$attrs 的自动继承随即失效，
 * 并触发 "Extraneous non-props attributes ... renders fragment" 告警。
 */
const props = withDefaults(defineProps<PopoverRootProps & { class?: any }>(), {
  open: undefined,
  modal: undefined,
});
const emit = defineEmits<PopoverRootEmits>();
defineOptions({ inheritAttrs: false });
</script>

<template>
  <div v-bind="$attrs" :style="{ display: 'contents' }" :class="props.class">
    <PopoverRoot
      :open="props.open"
      :default-open="props.defaultOpen"
      :modal="props.modal"
      @update:open="emit('update:open', $event)">
      <slot />
    </PopoverRoot>
  </div>
</template>