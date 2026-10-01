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
    class?: any;
  }>(),
  { color: 'default' },
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

/**
 * 显式传 variant,不让 cva 落到 default(近黑底)。
 * 未指定 variant 时统一用 outline 作底,再由项目语义色类覆盖
 * 描边/底色/文字色(twMerge 会按后者取胜)。
 */
const badgeVariant = computed<TagVariant>(() => props.variant ?? 'outline');

const tagClasses = computed(() => {
  if (props.variant !== undefined) return props.class;
  // 自定义色(#rrggbb)由 customStyle 上色,不叠预设类
  if (isCustomHex.value) return props.class;
  return cn(PRESETS[props.color] ?? PRESETS.default, props.class);
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