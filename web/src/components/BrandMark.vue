<template>
  <svg :width="size" :height="size" viewBox="0 0 64 64" aria-hidden="true">
    <defs>
      <linearGradient id="cloak-brand-g" x1="0" y1="0" x2="1" y2="1">
        <stop offset="0" stop-color="#6366f1" />
        <stop offset="1" stop-color="#22d3ee" />
      </linearGradient>
    </defs>
    <rect width="64" height="64" rx="15" fill="url(#cloak-brand-g)" />
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
withDefaults(
  defineProps<{
    /** 边长(px) */
    size?: number;
  }>(),
  { size: 32 },
);
</script>
