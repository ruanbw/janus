<template>
  <UiSeparator
    v-if="orientation === 'vertical'"
    orientation="vertical"
    :decorative="decorative"
    :class="cn('mx-2 inline-block h-4 w-px shrink-0 align-middle bg-line', props.class)"
    :style="style"
  />
  <div
    v-else-if="$slots.default"
    :class="cn('my-4 flex items-center gap-3', props.class)"
    :style="style"
  >
    <UiSeparator
      :decorative="decorative"
      :class="cn('h-px bg-line', orientationText === 'left' ? 'w-6 shrink-0' : 'flex-1')"
    />
    <span class="shrink-0 select-none text-xs font-medium text-ink-faint">
      <slot />
    </span>
    <UiSeparator
      :decorative="decorative"
      :class="cn('h-px bg-line', orientationText === 'right' ? 'w-6 shrink-0' : 'flex-1')"
    />
  </div>
  <UiSeparator
    v-else
    orientation="horizontal"
    :decorative="decorative"
    :class="cn('my-4 h-px w-full bg-line', props.class)"
    :style="style"
  />
</template>

<script setup lang="ts">
import UiSeparator from '@/components/ui/separator.vue';

import { cn } from '@/lib/utils';

const props = withDefaults(
  defineProps<{
    orientation?: 'horizontal' | 'vertical';
    orientationText?: 'left' | 'center' | 'right';
    decorative?: boolean;
    style?: Record<string, string | number>;
    class?: any;
  }>(),
  {
    orientation: 'horizontal',
    orientationText: 'center',
    decorative: true,
  },
);
</script>
