<template>
  <div class="pointer-events-none fixed top-4 left-1/2 z-[100] flex w-full max-w-md -translate-x-1/2 flex-col items-center gap-2 px-4">
    <TransitionGroup name="toast">
      <div
        v-for="t in toasts"
        :key="t.id"
        class="pointer-events-auto flex w-full items-start gap-3 rounded-xl border bg-surface/95 p-3.5 shadow-lg backdrop-blur-md transition-all dark:bg-surface-strong/95"
        :class="toastClasses(t.type)"
      >
        <span class="mt-0.5 shrink-0" :class="iconColorClass(t.type)">
          <component :is="iconOf(t.type)" :size="16" />
        </span>
        <p class="min-w-0 flex-1 break-words text-xs font-medium leading-relaxed text-ink">{{ t.content }}</p>
        <button
          type="button"
          tabindex="-1"
          class="shrink-0 rounded-xs p-0.5 text-ink-faint opacity-60 transition-opacity hover:opacity-100 hover:text-ink focus-visible:outline-none"
          @click="dismiss(t.id)"
        >
          <X :size="14" />
        </button>
      </div>
    </TransitionGroup>
  </div>
</template>

<script setup lang="ts">
import { CheckCircle2, Info, TriangleAlert, X, XCircle } from '@lucide/vue';

import { dismiss, toasts } from './toast';
import type { ToastType } from './toast';

function toastClasses(type: ToastType): string {
  switch (type) {
    case 'success':
      return 'border-ok/30 text-ink';
    case 'error':
      return 'border-err/30 text-ink';
    case 'warning':
      return 'border-warn/30 text-ink';
    default:
      return 'border-info/30 text-ink';
  }
}

function iconColorClass(type: ToastType): string {
  switch (type) {
    case 'success':
      return 'text-ok';
    case 'error':
      return 'text-err';
    case 'warning':
      return 'text-warn';
    default:
      return 'text-info';
  }
}

function iconOf(type: ToastType) {
  switch (type) {
    case 'success':
      return CheckCircle2;
    case 'error':
      return XCircle;
    case 'warning':
      return TriangleAlert;
    default:
      return Info;
  }
}
</script>

<style scoped>
.toast-enter-active,
.toast-leave-active {
  transition: all 0.2s cubic-bezier(0.16, 1, 0.3, 1);
}

.toast-enter-from {
  opacity: 0;
  transform: translateY(-12px) scale(0.95);
}

.toast-leave-to {
  opacity: 0;
  transform: translateY(-8px) scale(0.96);
}
</style>
