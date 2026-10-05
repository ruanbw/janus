<template>
  <UiPopover v-model:open="open">
    <UiPopoverTrigger as-child><slot /></UiPopoverTrigger>
    <UiPopoverPortal>
      <UiPopoverContent :side-offset="sideOffset" :class="contentClasses">
        <div class="flex items-start gap-2.5">
          <span class="mt-0.5 shrink-0 text-warn"><Info :size="16" /></span>
          <div class="min-w-0 flex-1">
            <p class="text-sm font-medium leading-snug text-ink">{{ title }}</p>
            <p v-if="description" class="mt-1 text-xs leading-relaxed text-ink-faint">{{ description }}</p>
          </div>
        </div>
        <div class="mt-3.5 flex justify-end gap-2">
          <AppButton size="small" @click="open = false">{{ cancelText }}</AppButton>
          <AppButton size="small" type="primary" :danger="danger" @click="onConfirm">
            {{ okText }}
          </AppButton>
        </div>
        <UiPopoverArrow class="fill-surface stroke-line" :width="10" :height="5" />
      </UiPopoverContent>
    </UiPopoverPortal>
  </UiPopover>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue';
import { Info } from '@lucide/vue';

import { Popover as UiPopover } from '@/components/ui/popover';
import { PopoverArrow as UiPopoverArrow } from '@/components/ui/popover';
import { PopoverContent as UiPopoverContent } from '@/components/ui/popover';
import { PopoverPortal as UiPopoverPortal } from '@/components/ui/popover';
import { PopoverTrigger as UiPopoverTrigger } from '@/components/ui/popover';

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
    'z-[85] w-72 rounded-lg border border-line bg-surface p-3.5 text-ink shadow-lg',
    props.class,
  );
});

function onConfirm(): void {
  open.value = false;
  emit('confirm');
}
</script>
