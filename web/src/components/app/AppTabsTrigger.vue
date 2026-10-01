<template>
  <UiTabsTrigger
    :value="value"
    :disabled="disabled"
    :class="
      variant === 'line'
        ? cn(
            LINE_VARIANTS,
            'rounded-none border-b-2 border-transparent px-4 py-2.5 text-muted-foreground hover:text-foreground',
            'data-[state=active]:border-primary data-[state=active]:bg-transparent data-[state=active]:text-foreground data-[state=active]:shadow-none',
          )
        : undefined
    "
  >
    <slot />
  </UiTabsTrigger>
</template>

<script lang="ts">
import { cn } from '@/lib/utils';

/** line 变体：下划线型页签。药丸型直接用 ui/tabs-trigger 的默认外观。 */
const LINE_VARIANTS =
  'flex items-center gap-2 rounded-none bg-transparent shadow-none';
</script>

<script setup lang="ts">
import UiTabsTrigger from '@/components/ui/tabs-trigger.vue';

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
