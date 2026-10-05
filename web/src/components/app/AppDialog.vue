<script lang="ts">
/**
 * 无头底座的对外部件名:视图层(与其它 app/ 组件)按 shadcn 的方式从这里取
 * DialogClose / DialogTitle / …,不必知道它们来自 reka-ui。
 * 这是 components/app/ 里唯一被门禁白名单放行的 reka 直连(见 issue 04 第 14 项)。
 */
export {
  DialogClose,
  DialogDescription,
  DialogOverlay,
  DialogPortal,
  DialogRoot,
  DialogTitle,
  DialogTrigger,
} from 'reka-ui';
</script>

<script setup lang="ts">
import { DialogRoot as DialogRootPrimitive } from '@/components/ui/dialog';
import { DialogPortal } from '@/components/ui/dialog';
import { DialogOverlay } from '@/components/ui/dialog';
import { DialogContent } from '@/components/ui/dialog';
import { DialogTitle } from '@/components/ui/dialog';
import { DialogDescription } from '@/components/ui/dialog';

import { cn } from '@/lib/utils';

/** 调用方未提供标题/描述时的兜底文案；reka 只校验对应 id 的节点存在 */
const DEFAULT_TITLE = '对话框';
const DEFAULT_DESCRIPTION = '对话框内容';

const props = withDefaults(
  defineProps<{
    open?: boolean;
    defaultOpen?: boolean;
    modal?: boolean;
    title?: string;
    description?: string;
    class?: any;
    overlayClass?: any;
  }>(),
  { modal: true },
);

const emit = defineEmits<{
  'update:open': [val: boolean];
}>();
</script>

<template>
  <DialogRootPrimitive
    :open="open"
    :default-open="defaultOpen"
    :modal="modal"
    @update:open="emit('update:open', $event)"
  >
    <DialogPortal>
      <DialogOverlay :class="cn(overlayClass)" />
      <DialogContent :class="cn('border-line bg-surface text-ink', props.class)">
        <!-- 无头底座不自带标题区，语义层无条件渲染（sr-only），否则 reka 必刷两条警告 -->
        <DialogTitle class="sr-only">
          <slot name="title">{{ title || DEFAULT_TITLE }}</slot>
        </DialogTitle>
        <DialogDescription class="sr-only">
          <slot name="description">{{ description || DEFAULT_DESCRIPTION }}</slot>
        </DialogDescription>
        <slot />
      </DialogContent>
    </DialogPortal>
  </DialogRootPrimitive>
</template>
