<template>
  <div :class="cn('flex w-full items-center gap-3', props.class)">
    <ProgressRoot
      v-model="clamped"
      :max="100"
      class="relative flex-1 overflow-hidden rounded-full bg-control-track/60"
      :style="{ height: strokeWidth + 'px' }"
    >
      <ProgressIndicator
        class="h-full w-full flex-1 rounded-full transition-all duration-300 ease-in-out"
        :class="barClasses"
        :style="{ transform: `translateX(-${100 - clamped}%)` }"
      />
    </ProgressRoot>
    <span
      v-if="showInfo"
      class="w-10 shrink-0 text-right text-xs font-medium text-ink-soft tabular-nums select-none"
    >
      {{ Math.round(clamped) }}%
    </span>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import { ProgressIndicator, ProgressRoot } from 'reka-ui';

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
.progress-active {
  background-image: linear-gradient(
    45deg,
    rgba(255, 255, 255, 0.22) 25%,
    transparent 25%,
    transparent 50%,
    rgba(255, 255, 255, 0.22) 50%,
    rgba(255, 255, 255, 0.22) 75%,
    transparent 75%,
    transparent
  );
  background-size: 1rem 1rem;
  animation: progress-stripes 1s linear infinite;
}

/* 深色主题下 --primary 变浅,白纹叠上去看不见,换深色斜纹 */
:global(.dark) .progress-active {
  background-image: linear-gradient(
    45deg,
    rgba(2, 6, 23, 0.2) 25%,
    transparent 25%,
    transparent 50%,
    rgba(2, 6, 23, 0.2) 50%,
    rgba(2, 6, 23, 0.2) 75%,
    transparent 75%,
    transparent
  );
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
