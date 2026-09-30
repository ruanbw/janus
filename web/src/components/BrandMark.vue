<template>
  <svg :width="size" :height="size" viewBox="0 0 64 64" aria-hidden="true">
    <defs>
      <linearGradient :id="gradientId" x1="0" y1="0" x2="1" y2="1">
        <stop offset="0" stop-color="#6366f1" />
        <stop offset="1" stop-color="#22d3ee" />
      </linearGradient>
    </defs>
    <rect width="64" height="64" rx="15" :fill="`url(#${gradientId})`" />
    <!-- 斗篷钩形:大 C + 斜切,呼应短链“截断/改写”语义 -->
    <path
      d="M21 23c9.5 0 17 7.5 17 17v3"
      fill="none"
      stroke="#fff"
      stroke-width="5"
      stroke-linecap="round"
    />
    <path
      d="M34 17l13 24"
      fill="none"
      stroke="#fff"
      stroke-width="5"
      stroke-linecap="round"
      opacity="0.92"
    />
  </svg>
</template>

<script setup lang="ts">
import { useId } from 'vue';

// 渐变 id 必须逐实例唯一:桌面侧边栏与移动抽屉会同时挂载本组件,
// 固定 id 会在文档里产生重复 ID,浏览器只会命中第一个。
const gradientId = `cloak-brand-g-${useId()}`;

withDefaults(
  defineProps<{
    /** 边长(px) */
    size?: number;
  }>(),
  { size: 32 },
);
</script>
