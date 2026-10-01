<template>
  <DialogRoot
    :open="open"
    :default-open="defaultOpen"
    @update:open="onOpenChange"
  >
    <DialogPortal>
      <DialogOverlay
        class="fixed inset-0 z-50 bg-black/60 backdrop-blur-xs data-[state=open]:animate-in data-[state=closed]:animate-out data-[state=closed]:fade-out-0 data-[state=open]:fade-in-0"
      />
      <DialogContent
        :class="
          cn(
            'fixed left-1/2 top-1/2 z-50 flex w-[calc(100%-2rem)] -translate-x-1/2 -translate-y-1/2 flex-col rounded-xl border border-line bg-surface shadow-2xl outline-none duration-150 data-[state=open]:animate-in data-[state=closed]:animate-out data-[state=closed]:fade-out-0 data-[state=open]:fade-in-0 data-[state=closed]:zoom-out-95 data-[state=open]:zoom-in-95 data-[state=closed]:slide-out-to-left-1/2 data-[state=closed]:slide-out-to-top-[48%] data-[state=open]:slide-in-from-left-1/2 data-[state=open]:slide-in-from-top-[48%]',
            SIZE_CLASSES[size],
            props.class,
          )
        "
      >
        <!-- Header -->
        <slot name="header">
          <div
            v-if="title || $slots.title || description || closable"
            class="flex items-start justify-between gap-3 border-b border-line px-5 py-3.5 bg-surface-muted/30 rounded-t-xl"
          >
            <div class="min-w-0 space-y-1">
              <DialogTitle class="text-base font-semibold text-ink leading-tight">
                <slot name="title">{{ title }}</slot>
              </DialogTitle>
              <DialogDescription
                v-if="description || $slots.description"
                class="text-xs text-ink-faint leading-relaxed"
              >
                <slot name="description">{{ description }}</slot>
              </DialogDescription>
            </div>
            <DialogClose
              v-if="closable"
              as-child
            >
              <button
                type="button"
                aria-label="关闭"
                class="flex size-7 shrink-0 items-center justify-center rounded-lg text-ink-faint hover:bg-surface-strong hover:text-ink transition-colors cursor-pointer outline-none focus-visible:ring-2 focus-visible:ring-ring"
              >
                <X :size="16" />
              </button>
            </DialogClose>
          </div>
        </slot>

        <!-- Body -->
        <div
          :class="
            cn(
              'min-h-0 flex-1 overflow-y-auto',
              padding ? 'p-5' : 'p-0',
              bodyClass,
            )
          "
        >
          <slot />
        </div>

        <!-- Footer -->
        <div
          v-if="$slots.footer"
          class="flex items-center justify-end gap-2 border-t border-line px-5 py-3 bg-surface-muted/30 rounded-b-xl"
        >
          <slot name="footer" />
        </div>
      </DialogContent>
    </DialogPortal>
  </DialogRoot>
</template>

<script setup lang="ts">
import {
  DialogClose,
  DialogContent,
  DialogDescription,
  DialogOverlay,
  DialogPortal,
  DialogRoot,
  DialogTitle,
} from 'reka-ui';
import { X } from '@lucide/vue';

import { cn } from '@/lib/utils';

export type ModalSize = 'sm' | 'md' | 'lg' | 'xl' | '2xl' | 'full';

const props = withDefaults(
  defineProps<{
    open?: boolean;
    defaultOpen?: boolean;
    title?: string;
    description?: string;
    size?: ModalSize;
    closable?: boolean;
    padding?: boolean;
    class?: any;
    bodyClass?: any;
  }>(),
  {
    open: false,
    size: 'md',
    closable: true,
    padding: true,
  },
);

const emit = defineEmits<{
  'update:open': [val: boolean];
  close: [];
}>();

const SIZE_CLASSES: Record<ModalSize, string> = {
  sm: 'max-w-sm',
  md: 'max-w-lg',
  lg: 'max-w-2xl',
  xl: 'max-w-4xl',
  '2xl': 'max-w-5xl',
  full: 'max-w-[94vw] h-[90vh]',
};

function onOpenChange(val: boolean): void {
  emit('update:open', val);
  if (!val) emit('close');
}
</script>
