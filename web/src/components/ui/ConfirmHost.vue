<template>
  <AlertDialogRoot :open="state?.open ?? false" @update:open="onOpenChange">
    <AlertDialogPortal>
      <AlertDialogOverlay
        class="fixed inset-0 z-[90] bg-black/60 backdrop-blur-xs data-[state=open]:animate-in data-[state=closed]:animate-out data-[state=closed]:fade-out-0 data-[state=open]:fade-in-0"
      />
      <AlertDialogContent
        class="fixed top-1/2 left-1/2 z-[95] w-[92vw] max-w-md -translate-x-1/2 -translate-y-1/2 rounded-xl border border-line bg-surface p-6 shadow-2xl outline-none duration-200 data-[state=open]:animate-in data-[state=closed]:animate-out data-[state=closed]:fade-out-0 data-[state=open]:fade-in-0 data-[state=closed]:zoom-out-95 data-[state=open]:zoom-in-95 data-[state=closed]:slide-out-to-left-1/2 data-[state=closed]:slide-out-to-top-[48%] data-[state=open]:slide-in-from-left-1/2 data-[state=open]:slide-in-from-top-[48%]"
      >
        <div class="flex items-start gap-3.5">
          <span
            class="flex h-10 w-10 shrink-0 items-center justify-center rounded-full"
            :class="state?.danger ? 'bg-err/12 text-err' : 'bg-brand-500/12 text-brand-600 dark:text-brand-400'"
          >
            <TriangleAlert v-if="state?.danger" :size="19" />
            <ShieldQuestion v-else :size="19" />
          </span>
          <div class="min-w-0 flex-1">
            <AlertDialogTitle class="text-base font-semibold leading-none tracking-tight text-ink">
              {{ state?.title }}
            </AlertDialogTitle>
            <AlertDialogDescription v-if="state?.content" class="mt-2 text-sm leading-relaxed text-ink-soft">
              {{ state.content }}
            </AlertDialogDescription>
          </div>
        </div>
        <div class="mt-6 flex justify-end gap-2.5">
          <AlertDialogCancel as-child>
            <AppButton>{{ state?.cancelText ?? '取消' }}</AppButton>
          </AlertDialogCancel>
          <AlertDialogAction as-child>
            <AppButton
              type="primary"
              :danger="state?.danger ?? false"
              :loading="busy"
              @click="onOk"
            >
              {{ state?.okText ?? '确定' }}
            </AppButton>
          </AlertDialogAction>
        </div>
      </AlertDialogContent>
    </AlertDialogPortal>
  </AlertDialogRoot>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue';
import {
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogOverlay,
  AlertDialogPortal,
  AlertDialogRoot,
  AlertDialogTitle,
} from 'reka-ui';
import { ShieldQuestion, TriangleAlert } from '@lucide/vue';

import AppButton from './AppButton.vue';
import { closeConfirm, confirmState } from './confirm';

const busy = ref(false);
const state = computed(() => confirmState.current);

function onOpenChange(open: boolean): void {
  if (open === false) {
    closeConfirm(false);
  }
}

async function onOk(): Promise<void> {
  const current = confirmState.current;
  if (current === null) return;
  busy.value = true;
  try {
    if (current.onOk) {
      await current.onOk();
    }
  } finally {
    busy.value = false;
    closeConfirm(true);
  }
}
</script>
