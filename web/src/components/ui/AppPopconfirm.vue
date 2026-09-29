<template>
  <PopoverRoot v-model:open="open">
    <PopoverTrigger as-child><slot /></PopoverTrigger>
    <PopoverPortal>
      <PopoverContent
        :side-offset="sideOffset"
        :class="contentClasses"
      >
        <div class="flex items-start gap-2.5">
          <span class="mt-0.5 shrink-0 text-warn"><Info :size="16" /></span>
          <div class="min-w-0 flex-1">
            <p class="text-sm font-medium leading-snug text-ink">{{ title }}</p>
            <p v-if="description" class="mt-1 text-xs text-ink-faint leading-relaxed">{{ description }}</p>
          </div>
        </div>
        <div class="mt-3.5 flex justify-end gap-2">
          <AppButton size="small" @click="open = false">{{ cancelText }}</AppButton>
          <AppButton size="small" type="primary" :danger="danger" @click="onConfirm">
            {{ okText }}
          </AppButton>
        </div>
        <PopoverArrow class="fill-surface stroke-line" :width="10" :height="5" />
      </PopoverContent>
    </PopoverPortal>
  </PopoverRoot>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue';
import { PopoverArrow, PopoverContent, PopoverPortal, PopoverRoot, PopoverTrigger } from 'reka-ui';
import { Info } from '@lucide/vue';

import { cn } from '@/lib/utils';
import AppButton from './AppButton.vue';

const props = withDefaults(
  defineProps<{
    title?: string;
    description?: string;
    okText?: string;
    cancelText?: string;
    danger?: boolean;
    sideOffset?: number;
    class?: any;
  }>(),
  { title: '', okText: '确定', cancelText: '取消', danger: false, sideOffset: 6 },
);

const emit = defineEmits<{ confirm: [] }>();

const open = ref(false);

const contentClasses = computed(() => {
  return cn(
    'z-[85] w-72 rounded-xl border border-line bg-surface p-3.5 shadow-xl outline-none',
    'data-[state=open]:animate-in data-[state=closed]:animate-out data-[state=closed]:fade-out-0 data-[state=open]:fade-in-0 data-[state=closed]:zoom-out-95 data-[state=open]:zoom-in-95',
    'data-[side=bottom]:slide-in-from-top-2 data-[side=left]:slide-in-from-right-2 data-[side=right]:slide-in-from-left-2 data-[side=top]:slide-in-from-bottom-2',
    props.class,
  );
});

function onConfirm(): void {
  open.value = false;
  emit('confirm');
}
</script>
