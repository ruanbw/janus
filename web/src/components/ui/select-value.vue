<script setup lang="ts">
import { SelectValue, type AcceptableValue, type SelectValueProps } from 'reka-ui';

const props = defineProps<SelectValueProps & { class?: any }>();

/**
 * 显式声明默认插槽的作用域：reka 的 SelectValue 把 { modelValue, selectedLabel }
 * 透给插槽，但本组件只是转发，不声明的话 vue-tsc 把插槽参数推成 {}，
 * 调用方解构 modelValue 就报错。
 */
defineSlots<{
  default?: (slotProps: {
    modelValue: AcceptableValue | AcceptableValue[] | undefined;
    selectedLabel: string[];
  }) => unknown;
}>();
</script>

<template>
  <SelectValue
    :placeholder="props.placeholder"
    :as="props.as"
    :as-child="props.asChild"
    :class="props.class"
  >
    <!--
      v-slot 转发：reka 给插槽的 { modelValue, selectedLabel } 必须原样传下去，
      直接写 <slot /> 推不出参数，写死参数又会让标签永远拿不到值。

      只在消费方真的传了 default 插槽时才转发。reka 把 placeholder 文案放在默认插槽的
      **fallback** 里，而 Vue 只在「未提供 default 插槽」时走 fallback；本封装无条件
      提供 default 插槽就等于替消费方把这个决定做了。

      注意：实测下来（tests/b1-appkit/SelectValue.spec.ts 的“渲染为空的插槽”用例）
      仅含注释的插槽内容会被 Vue 的 renderSlot 丢弃并自动退回 fallback，所以这一行
      单独看并不改变行为。真正让单选触发器显示空白的是 AppSelect —— 它给
      labelOf('') 渲染出了一个真实的空 <span>。这里保留条件转发，是为了让本封装
      契约明确（转发与否只取决于消费方），不依赖 renderSlot 的内部实现细节。
    -->
    <template
      v-if="$slots.default"
      #default="slotProps"
    >
      <slot v-bind="slotProps" />
    </template>
  </SelectValue>
</template>