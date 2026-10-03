<template>
  <template v-if="fatalError">
    <div class="min-h-screen flex items-center justify-center bg-surface-muted p-6">
      <div class="w-full max-w-md rounded-2xl border border-line bg-surface p-8 text-center shadow-lg">
        <div class="mx-auto mb-4 flex h-14 w-14 items-center justify-center rounded-full bg-err/10 text-err">
          <AlertTriangle :size="28" />
        </div>
        <h2 class="text-lg font-bold text-ink">系统暂时无法加载</h2>
        <p class="mt-2 text-sm text-ink-muted leading-relaxed">
          {{ fatalError.message || '运行遇到未知异常，请尝试重试' }}
        </p>
        <div class="mt-6 flex justify-center gap-3">
          <AppButton
            size="sm"
            @click="retry"
          >
            重试
          </AppButton>
          <AppButton
            size="sm"
            variant="ghost"
            @click="reloadPage"
          >
            刷新重试
          </AppButton>
        </div>
      </div>
    </div>
  </template>
  <!-- attempt 拼进 key:重试时强制重建整棵子树,而不只是再渲染一次 -->
  <router-view
    v-else
    :key="attempt"
  />
  <!--
    Toaster / ConfirmHost 不受 fatalError 影响地常驻:
    放在 v-if 分支里会在崩溃时被卸载 —— ConfirmHost 一旦被卸载,打开中的
    confirmAsync(...) 的 resolving 永不结算,调用方 await 永久挂起
    (表现为「点了确认按钮后整个操作无响应、无任何报错」)。
  -->
  <Toaster />
  <ConfirmHost />
</template>

<script setup lang="ts">
import { onErrorCaptured, ref } from 'vue';
import { AlertTriangle } from '@lucide/vue';
import AppButton from '@/components/app/AppButton.vue';
import ConfirmHost from '@/components/app/ConfirmHost.vue';
import Toaster from '@/components/app/Toaster.vue';

const fatalError = ref<Error | null>(null);
/** 递增的挂载序号:retry() 自增,配合模板里的 :key 重建子树 */
const attempt = ref(0);

onErrorCaptured((err: unknown) => {
  if (import.meta.env.DEV) console.error('[App Fatal ErrorCaptured]', err);
  fatalError.value = err instanceof Error ? err : new Error(String(err));
  // 不 return false:继续冒泡到 app.config.errorHandler,统一由全局处理器上报。
  // 边界只负责「就地降级 + 可恢复」,上报口径只有一处。
});

/** 不刷新页面的恢复:清错 + 递增 attempt 强制重建子树 */
function retry(): void {
  fatalError.value = null;
  attempt.value += 1;
}

function reloadPage(): void {
  window.location.reload();
}
</script>