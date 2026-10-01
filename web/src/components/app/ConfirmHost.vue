<template>
  <AlertDialog :open="state?.open ?? false" @update:open="onOpenChange">
    <AlertDialogPortal>
      <AlertDialogOverlay class="z-[90]" />
      <AlertDialogContent
        class="z-[95] w-[92vw] max-w-md gap-0 border-line bg-surface p-6 text-ink duration-200"
      >
        <div class="flex items-start gap-3.5">
          <span
            class="flex h-10 w-10 shrink-0 items-center justify-center rounded-full"
            :class="state?.danger ? 'bg-err/12 text-err' : 'bg-primary/12 text-primary'"
          >
            <TriangleAlert v-if="state?.danger" :size="19" />
            <ShieldQuestion v-else :size="19" />
          </span>
          <div class="min-w-0 flex-1">
            <AlertDialogTitle class="text-ink leading-none tracking-tight">
              {{ state?.title }}
            </AlertDialogTitle>
            <AlertDialogDescription
              v-if="state?.content"
              class="mt-2 text-sm leading-relaxed text-ink-soft"
            >
              {{ state.content }}
            </AlertDialogDescription>
          </div>
        </div>
        <div class="mt-6 flex justify-end gap-2.5">
          <!-- 按钮用自己的 AppButton 外观,不走 ui/alert-dialog-action / -cancel
               (那两个原语自带按钮外观,会与 AppButton 的 variants 撞车)。

               也不套 ui/dialog-close as-child:AppButton 的根是 UiButton(渲染 <button>),
               没有把 asChild 透传下去,外层的 as-child 会被吞掉 ——
               表现为「点确认按钮完全没反应」,onOk 永不执行,所有破坏性操作变成空操作。
               开关本就由 confirmState 驱动(AlertDialog 的 :open),直接点按钮调
               closeConfirm 即可,不需要 DialogClose 这层。 -->
          <AppButton @click="onCancel">{{ state?.cancelText ?? '取消' }}</AppButton>
          <AppButton
            type="primary"
            :danger="state?.danger ?? false"
            :loading="busy"
            @click="onOk"
          >
            {{ state?.okText ?? '确定' }}
          </AppButton>
        </div>
      </AlertDialogContent>
    </AlertDialogPortal>
  </AlertDialog>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue';
import { ShieldQuestion, TriangleAlert } from '@lucide/vue';

import AlertDialog from '@/components/ui/alert-dialog.vue';
import AlertDialogPortal from '@/components/ui/alert-dialog-portal.vue';
import AlertDialogOverlay from '@/components/ui/alert-dialog-overlay.vue';
import AlertDialogContent from '@/components/ui/alert-dialog-content.vue';
import AlertDialogTitle from '@/components/ui/alert-dialog-title.vue';
import AlertDialogDescription from '@/components/ui/alert-dialog-description.vue';

import AppButton from './AppButton.vue';
import { closeConfirm, confirmState } from './confirm';

const busy = ref(false);
const state = computed(() => confirmState.current);

function onOpenChange(open: boolean): void {
  if (open === false) {
    closeConfirm(false);
  }
}

function onCancel(): void {
  closeConfirm(false);
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
