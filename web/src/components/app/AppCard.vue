<template>
  <Card :class="cardClasses">
    <!-- 语法糖模式:提供了 title 或 title slot -->
    <template v-if="title || $slots.title">
      <CardHeader :class="cn(headerSizeClass, 'flex-row items-center justify-between gap-0')">
        <div class="space-y-1">
          <CardTitle class="text-base text-ink">
            <slot name="title">{{ title }}</slot>
          </CardTitle>
          <CardDescription
            v-if="description || $slots.description"
            class="text-xs leading-normal text-ink-faint"
          >
            <slot name="description">{{ description }}</slot>
          </CardDescription>
        </div>
        <div v-if="$slots.extra">
          <slot name="extra" />
        </div>
      </CardHeader>
      <CardContent :class="blockSizeClass">
        <slot />
      </CardContent>
      <CardFooter v-if="$slots.footer" :class="blockSizeClass">
        <slot name="footer" />
      </CardFooter>
    </template>
    <!-- 普通模式:若开启 padding 则包装内边距，否则直接透传插槽供子组件自由排版 -->
    <template v-else>
      <div v-if="padding" :class="padSizeClass">
        <slot />
      </div>
      <slot v-else />
    </template>
  </Card>
</template>

<script setup lang="ts">
import { computed } from 'vue';

import { Card } from '@/components/ui/card';

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
    // 底色与前景色沿用项目层令牌(视图层消费的是同一组);
    // 圆角 rounded-lg(8px)与阴影 shadow-xs 由 ui/card 原语提供,这里不再重复声明。
    'bg-surface text-ink',
    props.bordered ? 'border-line' : 'border-transparent shadow-none',
    props.class,
  );
});

/** 语法糖模式表头:默认 p-6 pb-4,small 收成 p-4 pb-3 */
const headerSizeClass = computed(() => (props.size === 'small' ? 'p-4 pb-3' : 'p-6 pb-4'));

/** 语法糖模式正文/页脚:贴住表头,默认 p-6 pt-0,small 收成 p-4 pt-0 */
const blockSizeClass = computed(() => (props.size === 'small' ? 'p-4 pt-0' : 'p-6 pt-0'));

/** 普通模式的 padding 包装:四边等距,默认 p-6,small 收成 p-4 */
const padSizeClass = computed(() => (props.size === 'small' ? 'p-4' : 'p-6'));

defineExpose({
  CardHeader,
  CardTitle,
  CardDescription,
  CardContent,
  CardFooter,
});
</script>
