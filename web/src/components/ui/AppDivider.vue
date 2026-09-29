<template>
  <!-- 纵向分割线 -->
  <Separator
    v-if="orientation === 'vertical'"
    orientation="vertical"
    :decorative="decorative"
    :class="cn('inline-block h-4 w-px bg-line shrink-0 align-middle mx-2', props.class)"
    :style="style"
  />

  <!-- 横向带文字分割线 -->
  <div
    v-else-if="$slots.default"
    :class="cn('flex items-center gap-3 my-4', props.class)"
    :style="style"
  >
    <Separator
      :decorative="decorative"
      class="h-px bg-line"
      :class="orientationText === 'left' ? 'w-6 shrink-0' : 'flex-1'"
    />
    <span class="shrink-0 text-xs font-medium text-ink-faint select-none">
      <slot />
    </span>
    <Separator
      :decorative="decorative"
      class="h-px bg-line"
      :class="orientationText === 'right' ? 'w-6 shrink-0' : 'flex-1'"
    />
  </div>

  <!-- 横向纯分割线 -->
  <Separator
    v-else
    orientation="horizontal"
    :decorative="decorative"
    :class="cn('h-px w-full bg-line my-4', props.class)"
    :style="style"
  />
</template>

<script setup lang="ts">
import { Separator } from 'reka-ui';

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
