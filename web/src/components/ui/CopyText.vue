<template>
  <span
    class="group inline-flex max-w-full cursor-pointer items-center gap-1 align-middle"
    :title="title"
    @click="copy"
  >
    <span class="min-w-0 truncate"><slot /></span>
    <span class="shrink-0 text-ink-faint transition-colors group-hover:text-brand-600">
      <Check v-if="copied" :size="13" class="text-ok" />
      <Copy v-else :size="13" />
    </span>
  </span>
</template>

<script setup lang="ts">
import { ref } from 'vue';
import { Check, Copy } from '@lucide/vue';

import { message } from './toast';

const props = defineProps<{
  text: string;
}>();

const copied = ref(false);
const title = '点击复制';

async function copy(): Promise<void> {
  try {
    await navigator.clipboard.writeText(props.text);
    copied.value = true;
    window.setTimeout(() => {
      copied.value = false;
    }, 1500);
  } catch {
    message.error('复制失败,请手动选择复制');
  }
}
</script>
