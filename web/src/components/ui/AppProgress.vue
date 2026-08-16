<template>
  <div class="flex items-center gap-2">
    <div
      class="flex-1 overflow-hidden rounded-full bg-surface-strong"
      :style="{ height: strokeWidth + 'px' }"
    >
      <div
        class="h-full rounded-full transition-[width] duration-300 ease-out"
        :class="barClasses"
        :style="{ width: clamped + '%' }"
      />
    </div>
    <span v-if="showInfo" class="w-9 shrink-0 text-right text-xs text-ink-soft tabular-nums">
      {{ Math.round(clamped) }}%
    </span>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue';

const props = withDefaults(
  defineProps<{
    percent?: number;
    status?: 'normal' | 'exception' | 'active' | 'success';
    showInfo?: boolean;
    strokeWidth?: number;
  }>(),
  { percent: 0, status: 'normal', showInfo: true, strokeWidth: 8 },
);

const clamped = computed(() => Math.max(0, Math.min(100, props.percent)));

const barClasses = computed(() => {
  if (props.status === 'exception') return 'bg-err';
  if (props.status === 'success') return 'bg-ok';
  if (props.status === 'active') return 'progress-active';
  return 'bg-brand-600 dark:bg-brand-500';
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

@keyframes progress-stripes {
  from {
    background-position: 1rem 0;
  }
  to {
    background-position: 0 0;
  }
}
</style>
