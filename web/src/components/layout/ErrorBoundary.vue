<template>
  <slot v-if="error === null" :attempt="attempt" />

  <div v-else class="mx-auto max-w-xl py-12">
    <div class="rounded-xl border border-line bg-surface p-8 text-center shadow-xs">
      <div class="mx-auto mb-3.5 flex h-12 w-12 items-center justify-center rounded-full bg-err/10 text-err">
        <AlertTriangle :size="24" />
      </div>
      <h3 class="text-base font-semibold text-ink">页面加载或渲染出错</h3>
      <p class="mt-1.5 text-sm leading-relaxed text-ink-muted">
        {{ error.message || '子组件渲染时发生未捕获异常，请尝试重试或刷新页面' }}
      </p>
      <div class="mt-6 flex items-center justify-center gap-3">
        <AppButton type="primary" @click="retry">
          <template #icon><RefreshCw :size="14" /></template>
          重新加载
        </AppButton>
        <AppButton @click="reloadPage">刷新整页</AppButton>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { onErrorCaptured, ref, watch } from 'vue';
import { AlertTriangle, RefreshCw } from '@lucide/vue';

import AppButton from '@/components/ui/AppButton.vue';

/**
 * 渲染错误边界。只包住它自己的插槽内容,因此拦到的是「页面渲染出错」,
 * 而不会把顶栏 / 侧边栏的异常一起吞掉 —— 外壳坏掉时应当照常显示,
 * 由用户看到白屏之外的真实故障现场,而不是一块降级卡片。
 *
 * 插槽参数 `attempt` 是递增的挂载序号:调用方把它拼进组件 key,
 * 「重新加载」时即可强制重建整棵子树(而不只是再渲染一次)。
 */
const props = defineProps<{
  /** 变化时清空错误状态:切换路由后新页面不该继承上一页的降级卡片 */
  resetKey?: string;
}>();

const error = ref<Error | null>(null);
const attempt = ref(0);

onErrorCaptured((err: unknown) => {
  console.error('[ErrorBoundary]', err);
  error.value = err instanceof Error ? err : new Error(String(err));
  return false;
});

watch(
  () => props.resetKey,
  () => {
    error.value = null;
  },
);

function retry(): void {
  error.value = null;
  attempt.value += 1;
}

function reloadPage(): void {
  window.location.reload();
}
</script>
