<template>
  <div class="pointer-events-none fixed top-4 left-1/2 z-[100] flex w-full max-w-md -translate-x-1/2 flex-col items-center gap-2 px-4">
    <TransitionGroup name="toast">
      <div
        v-for="t in toasts"
        :key="t.id"
        class="pointer-events-auto flex w-full items-start gap-2.5 rounded-xl border px-3.5 py-2.5 shadow-lg backdrop-blur"
        :class="toastClasses(t.type)"
      >
        <span class="mt-0.5 shrink-0">
          <component :is="iconOf(t.type)" :size="15" />
        </span>
        <p class="min-w-0 flex-1 break-words text-[13px] leading-relaxed">{{ t.content }}</p>
        <button
          type="button"
          class="shrink-0 opacity-60 transition-opacity hover:opacity-100"
          @click="dismiss(t.id)"
        >
          <X :size="13" />
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
      return 'border-ok/30 bg-ok/10 text-ink';
    case 'error':
      return 'border-err/30 bg-err/10 text-ink';
    case 'warning':
      return 'border-warn/30 bg-warn/10 text-ink';
    default:
      return 'border-info/30 bg-info/10 text-ink';
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
  transition:
    opacity 0.22s ease,
    transform 0.22s ease;
}

.toast-enter-from {
  opacity: 0;
  transform: translateY(-8px) scale(0.97);
}

.toast-leave-to {
  opacity: 0;
  transform: translateY(-6px) scale(0.98);
}
</style>
