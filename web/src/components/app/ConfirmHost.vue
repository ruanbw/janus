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
            <!--
              AlertDialogDescription 不能被 v-if 门控：ConfirmOptions.content 是可选的，
              调用方不传 content 时描述节点消失，reka 的 useWarning 查不到
              descriptionId 对应的元素，每次弹窗刷一条 Missing 警告。
              无 content 时退到 sr-only，只占住 aria-describedby。
            -->
            <AlertDialogDescription
              :class="state?.content ? 'mt-2 text-sm leading-relaxed text-ink-soft' : 'sr-only'"
            >
              {{ state?.content || DEFAULT_CONFIRM_DESCRIPTION }}
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

import { AlertDialog } from '@/components/ui/alert-dialog';
import { AlertDialogPortal } from '@/components/ui/alert-dialog';
import { AlertDialogOverlay } from '@/components/ui/alert-dialog';
import { AlertDialogContent } from '@/components/ui/alert-dialog';
import { AlertDialogTitle } from '@/components/ui/alert-dialog';
import { AlertDialogDescription } from '@/components/ui/alert-dialog';

import AppButton from './AppButton.vue';
import { closeConfirm, confirmState } from './confirm';

const busy = ref(false);
const state = computed(() => confirmState.current);

/** 调用方未传 ConfirmOptions.content 时的兜底描述 */
const DEFAULT_CONFIRM_DESCRIPTION = '请确认是否执行该操作';

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
