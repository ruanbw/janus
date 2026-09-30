<template>
  <div v-if="fatalError" class="min-h-screen flex items-center justify-center bg-surface-muted p-6">
    <div class="w-full max-w-md rounded-2xl border border-line bg-surface p-8 text-center shadow-lg">
      <div class="mx-auto mb-4 flex h-14 w-14 items-center justify-center rounded-full bg-err/10 text-err">
        <AlertTriangle :size="28" />
      </div>
      <h2 class="text-lg font-bold text-ink">系统暂时无法加载</h2>
      <p class="mt-2 text-sm text-ink-muted leading-relaxed">
        {{ fatalError.message || '运行遇到未知异常，请尝试刷新' }}
      </p>
      <div class="mt-6 flex justify-center gap-3">
        <button
          type="button"
          class="btn btn-primary btn-sm"
          @click="reloadPage"
        >
          刷新重试
        </button>
      </div>
    </div>
  </div>
  <template v-else>
    <router-view />
    <Toaster />
    <ConfirmHost />
  </template>
</template>

<script setup lang="ts">
import { onErrorCaptured, ref } from 'vue';
import { AlertTriangle } from '@lucide/vue';
import ConfirmHost from '@/components/ui/ConfirmHost.vue';
import Toaster from '@/components/ui/Toaster.vue';

const fatalError = ref<Error | null>(null);

onErrorCaptured((err: unknown) => {
  console.error('[App Fatal ErrorCaptured]', err);
  fatalError.value = err instanceof Error ? err : new Error(String(err));
  return false;
});

function reloadPage(): void {
  window.location.reload();
}
</script>
