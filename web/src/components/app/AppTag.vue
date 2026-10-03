<template>
  <Badge
    :variant="badgeVariant"
    :class="tagClasses"
    :style="customStyle"
  >
    <slot />
  </Badge>
</template>

<script lang="ts">
import { badgeVariants, type BadgeVariant } from '@/components/ui/badge.vue';

export type TagVariant = BadgeVariant;
export type TagSize = 'default' | 'small';
export type TagColor =
  | 'default'
  | 'brand'
  | 'info'
  | 'ok'
  | 'warn'
  | 'err'
  // 历史别名兼容
  | 'success'
  | 'warning'
  | 'error'
  | 'green'
  | 'orange'
  | 'red'
  | 'blue'
  | 'cyan'
  | 'purple'
  | 'geekblue'
  | 'gold';

/** 原语层的 variants 直接复用,项目层不再手搓一份 */
export { badgeVariants };
</script>

<script setup lang="ts">
import { computed } from 'vue';

import Badge from '@/components/ui/badge.vue';
import { cn } from '@/lib/utils';

const props = withDefaults(
  defineProps<{
    variant?: TagVariant;
    color?: TagColor | string;
    /**
     * 标签尺寸。原实现从未声明它，`size="small"` 作为 fallthrough attr
     * 直接漏到 Badge 渲染的 <span> 上 —— 不产生任何样式，只是给标记塞了个
     * 无效属性。这里显式声明并落实为真实的字号 / 内边距。
     */
    size?: TagSize;
    class?: any;
  }>(),
  { color: 'default', size: 'default' },
);

/**
 * 项目语义色预设。语义色一律走 --ok/--warn/--err/--info 令牌;
 * brand 走 nova 新增的 --brand 令牌(靛蓝),不再用 --primary——
 * nova 的 primary 是近黑色,用它会让 brand 标签变成灰的。
 */
const PRESETS: Record<string, string> = {
  default: 'border-line bg-surface-strong text-ink-soft',
  brand: 'border-brand/30 bg-brand/10 text-brand',
  info: 'border-info/30 bg-info/10 text-info',
  ok: 'border-ok/30 bg-ok/10 text-ok',
  warn: 'border-warn/30 bg-warn/10 text-warn',
  err: 'border-err/30 bg-err/10 text-err',
  // 兼容别名映射
  success: 'border-ok/30 bg-ok/10 text-ok',
  warning: 'border-warn/30 bg-warn/10 text-warn',
  error: 'border-err/30 bg-err/10 text-err',
  green: 'border-ok/30 bg-ok/10 text-ok',
  orange: 'border-warn/30 bg-warn/10 text-warn',
  red: 'border-err/30 bg-err/10 text-err',
  blue: 'border-info/30 bg-info/10 text-info',
  cyan: 'border-info/30 bg-info/10 text-info',
  purple: 'border-brand/30 bg-brand/10 text-brand',
  geekblue: 'border-brand/30 bg-brand/10 text-brand',
  gold: 'border-warn/30 bg-warn/10 text-warn',
};

const isCustomHex = computed(() => /^#[0-9a-fA-F]{6}$/.test(props.color ?? ''));

/** 原语层 badgeVariants 固定 text-xs / px-2.5 / py-0.5，small 档靠 twMerge 覆盖 */
const SIZE_CLASSES: Record<TagSize, string> = {
  default: '',
  small: 'px-1.5 py-px text-2xs',
};

/**
 * 显式传 variant,不让 cva 落到 default(近黑底)。
 * 未指定 variant 时统一用 outline 作底,再由项目语义色类覆盖
 * 描边/底色/文字色(twMerge 会按后者取胜)。
 */
const badgeVariant = computed<TagVariant>(() => props.variant ?? 'outline');

const tagClasses = computed(() => {
  const sizeClass = SIZE_CLASSES[props.size] ?? SIZE_CLASSES.default;
  if (props.variant !== undefined) return cn(sizeClass, props.class);
  // 自定义色(#rrggbb)由 customStyle 上色，不叠预设类
  if (isCustomHex.value) return cn(sizeClass, props.class);
  return cn(PRESETS[props.color] ?? PRESETS.default, sizeClass, props.class);
});

const customStyle = computed(() => {
  if (props.variant !== undefined || !isCustomHex.value) return undefined;
  const hex = props.color as string;
  return {
    color: hex,
    borderColor: hex + '55',
    backgroundColor: hex + '14',
  };
});
</script>