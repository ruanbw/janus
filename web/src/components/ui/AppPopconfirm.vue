<template>
  <PopoverRoot v-model:open="open">
    <PopoverTrigger as-child><slot /></PopoverTrigger>
    <PopoverPortal>
      <PopoverContent
        :side-offset="6"
        class="z-[85] w-64 rounded-xl border border-line bg-surface p-3 shadow-xl"
      >
        <div class="flex items-start gap-2.5">
          <span class="mt-0.5 shrink-0 text-warn"><Info :size="15" /></span>
          <p class="text-[13px] leading-relaxed text-ink">{{ title }}</p>
        </div>
        <div class="mt-3.5 flex justify-end gap-2">
          <AppButton size="small" @click="open = false">{{ cancelText }}</AppButton>
          <AppButton size="small" type="primary" :danger="danger" @click="onConfirm">
            {{ okText }}
          </AppButton>
        </div>
        <PopoverArrow class="fill-line" :width="10" :height="5" />
      </PopoverContent>
    </PopoverPortal>
  </PopoverRoot>
</template>

<script setup lang="ts">
import { ref } from 'vue';
import { PopoverArrow, PopoverContent, PopoverPortal, PopoverRoot, PopoverTrigger } from 'reka-ui';
import { Info } from '@lucide/vue';

import AppButton from './AppButton.vue';

withDefaults(
  defineProps<{
    title?: string;
    okText?: string;
    cancelText?: string;
    danger?: boolean;
  }>(),
  { title: '', okText: '确定', cancelText: '取消', danger: false },
);

const emit = defineEmits<{ confirm: [] }>();

const open = ref(false);

function onConfirm(): void {
  open.value = false;
  emit('confirm');
}
</script>
