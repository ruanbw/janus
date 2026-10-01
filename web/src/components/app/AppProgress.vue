<template>
  <div :class="cn('flex w-full items-center gap-3', props.class)">
    <UiProgress
      :model-value="clamped"
      :max="100"
      :class="'flex-1 bg-control-track/60'"
      :style="{ height: strokeWidth + 'px' }"
      :indicator-class="barClasses"
    />
    <span
      v-if="showInfo"
      class="w-10 shrink-0 select-none text-right text-xs font-medium tabular-nums text-ink-soft"
    >
      {{ Math.round(clamped) }}%
    </span>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue';

import UiProgress from '@/components/ui/progress.vue';

import { cn } from '@/lib/utils';

const props = withDefaults(
  defineProps<{
    percent?: number;
    status?: 'normal' | 'exception' | 'active' | 'success';
    showInfo?: boolean;
    strokeWidth?: number;
    class?: any;
  }>(),
  { percent: 0, status: 'normal', showInfo: true, strokeWidth: 8 },
);

const clamped = computed(() => Math.max(0, Math.min(100, props.percent)));

const barClasses = computed(() => {
  if (props.status === 'exception') return 'bg-err';
  if (props.status === 'success') return 'bg-ok';
  if (props.status === 'active') return 'progress-active bg-primary';
  return 'bg-primary';
});
</script>

<style scoped>
/*
 * 斜纹用前景令牌做半透明：浅色下 --primary 是近黑、深色下是近白，
 * 同一份 color-mix 在两套主题下都可见，不需要 dark: 补丁。
 */
.progress-active {
  background-image: linear-gradient(
    45deg,
    color-mix(in srgb, var(--primary-foreground) 22%, transparent) 25%,
    transparent 25%,
    transparent 50%,
    color-mix(in srgb, var(--primary-foreground) 22%, transparent) 50%,
    color-mix(in srgb, var(--primary-foreground) 22%, transparent) 75%,
    transparent 75%,
    transparent
  );
  background-size: 1rem 1rem;
  animation: progress-stripes 1s linear infinite;
}

@keyframes progress-stripes {
  from {
    background-position: 1rem 0;
  }
  to {
    background-position: 0 0;
  }
}
</style>
