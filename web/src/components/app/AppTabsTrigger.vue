<template>
  <UiTabsTrigger
    :value="value"
    :disabled="disabled"
    :class="
      variant === 'line'
        ? cn(
            LINE_VARIANTS,
            'rounded-none border-b-2 border-transparent px-4 py-2.5 text-muted-foreground hover:bg-transparent hover:text-foreground',
            'data-[state=active]:border-primary data-[state=active]:bg-transparent data-[state=active]:text-foreground data-[state=active]:shadow-none',
            props.class,
          )
        : props.class
    "
  >
    <slot />
  </UiTabsTrigger>
</template>

<script lang="ts">
import { cn } from '@/lib/utils';

/**
 * line 变体：下划线型页签。药丸型直接用 ui/tabs-trigger 的默认外观。
 *
 * 模板里的 `hover:bg-transparent` 是用来抵消原语层药丸型的 `hover:bg-accent`：
 * 线型页签只有一条下划线，悬停时不该浮出一块底色，只让文字变深即可。
 * cn() 走 tailwind-merge，后写的 hover:bg-transparent 胜出。
 */
const LINE_VARIANTS =
  'flex items-center gap-2 rounded-none bg-transparent shadow-none';
</script>

<script setup lang="ts">
import { TabsTrigger as UiTabsTrigger } from '@/components/ui/tabs';

const props = withDefaults(
  defineProps<{
    value: string | number;
    disabled?: boolean;
    variant?: 'line' | 'pill';
    class?: any;
  }>(),
  {
    disabled: false,
    variant: 'line',
  },
);
</script>
