<script setup lang="ts">
import { cn } from '@/lib/utils';

const props = defineProps<{ class?: any }>();
// 本组件有两层：外层是滚动容器，内层是真正承载语义的 <table>。
// 若不关掉自动继承，class 会落到 <table>、而其余 fallthrough 属性落到外层 div，
// 属性被拆散到两个元素上。这里显式关掉继承，把全部属性统一绑到 <table>。
defineOptions({ inheritAttrs: false });
</script>

<template>
  <div class="relative w-full overflow-x-auto">
    <table v-bind="$attrs" :class="cn('w-full caption-bottom text-sm', props.class)">
      <slot />
    </table>
  </div>
</template>