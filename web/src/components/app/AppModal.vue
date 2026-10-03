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
        <!--
          无障碍语义层：DialogTitle / DialogDescription 无条件存在。

          reka-ui 在 onMounted 里查 document.getElementById(titleId / descriptionId)，
          查不到就各刷一条 console.warn。原先这两个节点只写在 header 插槽的 fallback
          分支中，调用方一旦传 <template #header>（三处预览弹窗）整段默认 header 不渲染，
          于是每次打开都刷警告；不传 description 时同样刷；`:closable="false"` 且不传
          title/description 时连 header 都不渲染，两个节点一起消失，警告翻倍。

          这里把它们从 header 的视觉结构里拆出来：需要可见呈现时（默认 header 且调用方
          确实给了 title / description）在 header 里可见渲染，其余情况一律在这里用
          sr-only 兜住 —— 自定义 header、无标题、无描述，全都只占住 id 不抢视觉。
          兜底文案不能是空串：aria-labelledby 指向空标题等于没有可访问名称。
        -->
        <DialogTitle
          v-if="needsSrOnlyTitle"
          class="sr-only"
        >
          <slot name="title">{{ title || DEFAULT_TITLE }}</slot>
        </DialogTitle>
        <DialogDescription
          v-if="needsSrOnlyDescription"
          class="sr-only"
        >
          <slot name="description">{{ description || DEFAULT_DESCRIPTION }}</slot>
        </DialogDescription>

        <!-- Header -->
        <slot name="header">
          <div
            v-if="rendersHeaderBlock"
            class="flex items-start justify-between gap-3 border-b border-line px-5 py-3.5 bg-surface-muted/30 rounded-t-xl"
          >
            <div class="min-w-0 space-y-1">
              <DialogTitle
                v-if="!needsSrOnlyTitle"
                class="text-ink"
              >
                <slot name="title">{{ title }}</slot>
              </DialogTitle>
              <DialogDescription
                v-if="!needsSrOnlyDescription"
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
import { computed, useSlots } from 'vue';
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

/** 调用方既没传 title 也没给 title 插槽时的兜底可访问名称 */
const DEFAULT_TITLE = '对话框';
/** 兜底描述：reka 只要求该 id 存在,内容不应为空以免读屏念出一句无意义的话 */
const DEFAULT_DESCRIPTION = '对话框内容';

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

const slots = useSlots();

const SIZE_CLASSES: Record<ModalSize, string> = {
  sm: 'max-w-sm',
  md: 'max-w-lg',
  lg: 'max-w-2xl',
  xl: 'max-w-4xl',
  '2xl': 'max-w-5xl',
  full: 'max-w-[94vw] h-[90vh]',
};

/** 调用方接管了 header 插槽时,默认 header 整段不渲染,语义层需要 sr-only 兜底 */
const hasCustomHeader = computed(() => Boolean(slots.header));

const hasTitleContent = computed(() => Boolean(props.title || slots.title));
const hasDescriptionContent = computed(() => Boolean(props.description || slots.description));

/** 默认 header 是否至少渲染出可见的标题（或只有关闭按钮） */
const rendersHeaderBlock = computed(
  () => hasTitleContent.value || hasDescriptionContent.value || props.closable,
);

/**
 * 需要 sr-only 语义兜底 = 默认 header 不会可见渲染出这个节点。
 * 调用方接管了 header，或压根没给标题 / 描述，都走这里。
 */
const needsSrOnlyTitle = computed(() => hasCustomHeader.value || !hasTitleContent.value);
const needsSrOnlyDescription = computed(
  () => hasCustomHeader.value || !hasDescriptionContent.value,
);

function onOpenChange(val: boolean): void {
  emit('update:open', val);
  if (!val) emit('close');
}
</script>
