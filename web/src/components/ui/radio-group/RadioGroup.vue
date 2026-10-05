<script setup lang="ts">
import { RadioGroupRoot, type RadioGroupRootEmits, type RadioGroupRootProps } from 'reka-ui';

const props = defineProps<RadioGroupRootProps>();

/**
 * 必须显式声明并转抛：只声明 props 的话，调用方的 `onUpdate:modelValue` 会进入 $attrs，
 * 再经 fallthrough 落到根 vnode —— 而 Vue 的 filterModelListeners() 会把凡是
 * `update:xxx`（xxx 已是本组件声明过的 prop）的键从 fallthrough 里剔除，
 * 假设「组件自己会处理」。本组件并不处理，于是监听器被静默丢弃，
 * AppRadioGroup 的 v-model 与 change 全部失效。参见 ui/checkbox.vue 的同款写法。
 */
const emit = defineEmits<RadioGroupRootEmits>();
</script>

<template>
  <RadioGroupRoot
    :model-value="props.modelValue"
    @update:model-value="emit('update:modelValue', $event)"
    :default-value="props.defaultValue"
    :disabled="props.disabled"
    :name="props.name"
    :orientation="props.orientation"
    :loop="props.loop"
    :dir="props.dir"
    :as-child="props.asChild"
    :as="props.as"
    :required="props.required"
  >
    <slot />
  </RadioGroupRoot>
</template>