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
import DialogRootPrimitive from '@/components/ui/dialog.vue';
import DialogPortal from '@/components/ui/dialog-portal.vue';
import DialogOverlay from '@/components/ui/dialog-overlay.vue';
import DialogContent from '@/components/ui/dialog-content.vue';

import { cn } from '@/lib/utils';

const props = withDefaults(
  defineProps<{
    open?: boolean;
    defaultOpen?: boolean;
    modal?: boolean;
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
        <slot />
      </DialogContent>
    </DialogPortal>
  </DialogRootPrimitive>
</template>
