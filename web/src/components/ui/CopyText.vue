<template>
  <span
    class="group inline-flex max-w-full cursor-pointer items-center gap-1 align-middle"
    :title="title"
    @click="handleCopy"
  >
    <span class="min-w-0 truncate"><slot /></span>
    <span class="shrink-0 text-ink-faint transition-colors group-hover:text-brand-600">
      <Check v-if="copied" :size="13" class="text-ok" />
      <Copy v-else :size="13" />
    </span>
  </span>
</template>

<script setup lang="ts">
import { Check, Copy } from '@lucide/vue';
import { useClipboard } from '@vueuse/core';

import { message } from './toast';

const props = defineProps<{
  text: string;
}>();

const { copy, copied, isSupported } = useClipboard({ copiedDuring: 1500 });
const title = '点击复制';

async function handleCopy(): Promise<void> {
  if (!isSupported.value) {
    message.error('当前浏览器不支持剪贴板操作,请手动选择复制');
    return;
  }
  try {
    await copy(props.text);
  } catch {
    message.error('复制失败,请手动选择复制');
  }
}
</script>
