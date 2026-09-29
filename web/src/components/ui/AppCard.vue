<template>
  <div :class="cardClasses">
    <!-- 语法糖模式:提供了 title 或 title slot -->
    <template v-if="title || $slots.title">
      <div :class="[size === 'small' ? 'p-4 pb-3' : 'p-6 pb-4', 'flex flex-row items-center justify-between space-y-0']">
        <div class="space-y-1">
          <h3 class="text-base font-semibold leading-none tracking-tight text-ink">
            <slot name="title">{{ title }}</slot>
          </h3>
          <p v-if="description || $slots.description" class="text-xs text-ink-faint">
            <slot name="description">{{ description }}</slot>
          </p>
        </div>
        <div v-if="$slots.extra">
          <slot name="extra" />
        </div>
      </div>
      <div :class="[size === 'small' ? 'p-4 pt-0' : 'p-6 pt-0']">
        <slot />
      </div>
      <div v-if="$slots.footer" :class="[size === 'small' ? 'p-4 pt-0' : 'p-6 pt-0', 'flex items-center']">
        <slot name="footer" />
      </div>
    </template>
    <!-- 普通模式:若开启 padding 则包装内边距，否则直接透传插槽供子组件自由排版 -->
    <template v-else>
      <div v-if="padding" :class="size === 'small' ? 'p-4' : 'p-6'">
        <slot />
      </div>
      <slot v-else />
    </template>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue';

import { cn } from '@/lib/utils';
import CardHeader from './CardHeader.vue';
import CardTitle from './CardTitle.vue';
import CardDescription from './CardDescription.vue';
import CardContent from './CardContent.vue';
import CardFooter from './CardFooter.vue';

const props = withDefaults(
  defineProps<{
    title?: string;
    description?: string;
    bordered?: boolean;
    size?: 'default' | 'small';
    padding?: boolean;
    class?: any;
  }>(),
  { bordered: true, size: 'default', padding: true },
);

const cardClasses = computed(() => {
  return cn(
    'rounded-xl bg-surface text-ink shadow-xs transition-colors',
    props.bordered ? 'border border-line' : 'border border-transparent shadow-none',
    props.class,
  );
});

defineExpose({
  CardHeader,
  CardTitle,
  CardDescription,
  CardContent,
  CardFooter,
});
</script>
