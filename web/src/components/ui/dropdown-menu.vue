<script lang="ts">
import type { DropdownMenuRootEmits, DropdownMenuRootProps } from 'reka-ui';

export { default as DropdownMenuTrigger } from './dropdown-menu-trigger.vue';
export { default as DropdownMenuPortal } from './dropdown-menu-portal.vue';
export { default as DropdownMenuContent } from './dropdown-menu-content.vue';
export { default as DropdownMenuItem } from './dropdown-menu-item.vue';
export { default as DropdownMenuLabel } from './dropdown-menu-label.vue';
export { default as DropdownMenuSeparator } from './dropdown-menu-separator.vue';

export type { DropdownMenuRootEmits, DropdownMenuRootProps };
</script>

<script setup lang="ts">
import { DropdownMenuRoot } from 'reka-ui';

import { cn } from '@/lib/utils';

/**
 * `open` / `modal` 必须保持三态（undefined / true / false）。
 * Vue 对 Boolean 类型的 prop 会在「未传入」时强制补成 false，
 * 若直接转发 `props.open`，reka 的 `passive: props.open === void 0`
 * 就会恒为 false —— 于是 defaultOpen 永远不生效、受控分支永远进不去。
 * withDefaults 显式声明 default: undefined，把三态还回去。
 *
 * 根上必须自己渲染一个元素来承接 class 与 fallthrough attrs：
 * DropdownMenuRoot → MenuRoot → PopperRoot 这条链路的终点是 PopperRoot，
 * 它自带 inheritAttrs: false 且只 return renderSlot($slots, 'default')，
 * 于是根上的 class 与所有 data-* 会被静默丢弃，连 warn 都不触发。
 * display:contents 让这个包裹层不生成盒子，布局与原先「无根元素」完全一致。
 *
 * 注意：这段说明必须留在 <script> 里。写成 <template> 里的 HTML 注释会让
 * Vue 把根编译成 Fragment（注释算根节点之一），$attrs 的自动继承随即失效，
 * 并触发 "Extraneous non-props attributes ... renders fragment" 告警。
 */
const props = withDefaults(defineProps<DropdownMenuRootProps & { class?: any }>(), {
  open: undefined,
  modal: undefined,
});
const emit = defineEmits<DropdownMenuRootEmits>();
defineOptions({ inheritAttrs: false });
</script>

<template>
  <div v-bind="$attrs" :style="{ display: 'contents' }" :class="cn('relative', props.class)">
  <DropdownMenuRoot
    :open="props.open"
    :default-open="props.defaultOpen"
    :dir="props.dir"
    :modal="props.modal"
    @update:open="emit('update:open', $event)">
    <slot />
  </DropdownMenuRoot>
</div>
</template>
