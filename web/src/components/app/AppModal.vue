<template>
  <DialogRoot
    :open="open"
    :default-open="defaultOpen"
    @update:open="onOpenChange"
  >
    <DialogPortal>
      <DialogOverlay />
      <!-- p-0 / gap-0 抵消 ui/dialog-content 的默认内边距与栅格间距:
           弹窗的留白由 header / body / footer 三段各自承担 -->
      <DialogContent
        :class="
          cn(
            'flex w-[calc(100%-2rem)] flex-col gap-0 border-line bg-surface p-0 text-ink duration-150',
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
              <DialogTitle class="text-ink">
                <slot name="title">{{ title }}</slot>
              </DialogTitle>
              <DialogDescription
                v-if="description || $slots.description"
                class="text-ink-faint"
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
                class="app-field flex size-7 shrink-0 cursor-pointer items-center justify-center rounded-lg text-ink-faint transition-colors hover:bg-surface-strong hover:text-ink"
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
import { X } from '@lucide/vue';

import DialogRoot from '@/components/ui/dialog.vue';
import DialogPortal from '@/components/ui/dialog-portal.vue';
import DialogOverlay from '@/components/ui/dialog-overlay.vue';
import DialogContent from '@/components/ui/dialog-content.vue';
import DialogClose from '@/components/ui/dialog-close.vue';
import DialogTitle from '@/components/ui/dialog-title.vue';
import DialogDescription from '@/components/ui/dialog-description.vue';

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
