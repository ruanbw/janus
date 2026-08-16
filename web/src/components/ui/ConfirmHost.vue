<template>
  <AlertDialogRoot :open="state?.open ?? false" @update:open="onOpenChange">
    <AlertDialogPortal>
      <AlertDialogOverlay class="fixed inset-0 z-[90] bg-black/45 backdrop-blur-[2px]" />
      <AlertDialogContent class="fixed top-1/2 left-1/2 z-[95] w-[92vw] max-w-md -translate-x-1/2 -translate-y-1/2 rounded-2xl border border-line bg-surface p-5 shadow-2xl">
        <div class="flex items-start gap-3">
          <span
            class="flex h-9 w-9 shrink-0 items-center justify-center rounded-full"
            :class="state?.danger ? 'bg-err/12 text-err' : 'bg-brand-500/12 text-brand-600 dark:text-brand-400'"
          >
            <TriangleAlert v-if="state?.danger" :size="17" />
            <ShieldQuestion v-else :size="17" />
          </span>
          <div class="min-w-0">
            <AlertDialogTitle class="text-[15px] font-semibold text-ink">{{ state?.title }}</AlertDialogTitle>
            <AlertDialogDescription v-if="state?.content" class="mt-1.5 text-[13px] leading-relaxed text-ink-soft">
              {{ state.content }}
            </AlertDialogDescription>
          </div>
        </div>
        <div class="mt-5 flex justify-end gap-2">
          <AlertDialogCancel as-child>
            <AppButton>{{ state?.cancelText ?? '取消' }}</AppButton>
          </AlertDialogCancel>
          <AlertDialogAction as-child>
            <AppButton type="primary" :danger="state?.danger ?? false" :loading="busy" @click="onOk">
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
