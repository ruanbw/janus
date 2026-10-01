<script setup lang="ts">
import { cn } from '@/lib/utils';

const props = defineProps<{ class?: any }>();

/**
 * `hover:border-ring`：悬停时描边转主色（Element 的 `.el-input__wrapper:hover` 同款）。
 * 键盘不可达的场景里，鼠标悬停是「这个框能点」最直接的一层信号；
 * 失焦后描边退回 --input，鼠标一进来就重新亮起来，两者交替给出状态变化。
 */
</script>

<template>
  <input
    :class="
      cn(
        'flex h-9 w-full min-w-0 rounded-md border border-input bg-background px-3 py-1 text-sm shadow-xs transition-colors',
        'file:inline-flex file:h-7 file:border-0 file:bg-transparent file:text-sm file:font-medium',
        'placeholder:text-muted-foreground',
        'selection:bg-primary selection:text-primary-foreground',
        'hover:border-ring',
        'outline-none focus-visible:border-ring focus-visible:ring-[3px] focus-visible:ring-ring/50',
        'disabled:cursor-not-allowed disabled:opacity-50',
        props.class,
      )
    "
  />
</template>
