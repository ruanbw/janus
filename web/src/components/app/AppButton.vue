<template>
  <UiButton
    :as="isLink ? RouterLink : 'button'"
    :to="to"
    :type="isLink ? undefined : htmlType"
    :disabled="isLink ? undefined : disabled || loading"
    :variant="uiVariant"
    :size="uiSize"
    :aria-disabled="isLink && inactive ? 'true' : undefined"
    :tabindex="isLink && inactive ? -1 : undefined"
    :class="btnClasses"
    @click="onClick"
  >
    <Loader2 v-if="loading" :size="iconSize" class="animate-spin shrink-0" />
    <span v-else-if="$slots.icon" class="inline-flex shrink-0"><slot name="icon" /></span>
    <span v-if="$slots.default" class="inline-flex items-center gap-1.5"><slot /></span>
  </UiButton>
</template>

<script lang="ts">
import {
  buttonVariants as uiButtonVariants,
  type ButtonSize as UiButtonSize,
  type ButtonVariant as UiButtonVariant,
} from '@/components/ui/button';

import { cn } from '@/lib/utils';

export type ButtonVariant =
  | 'default'
  | 'primary'
  | 'destructive'
  | 'outline'
  | 'secondary'
  | 'ghost'
  | 'link'
  | 'dashed';

export type ButtonType = 'primary' | 'default' | 'text' | 'dashed' | 'ghost';
export type ButtonSize = 'default' | 'small' | 'middle' | 'large' | 'sm' | 'lg' | 'icon';

/** 项目层的 variant/size 词表比原语层宽，这里做一次投影（`primary` → 原语的 `default`）。 */
const UI_VARIANT: Record<ButtonVariant, UiButtonVariant> = {
  default: 'default',
  primary: 'default',
  secondary: 'secondary',
  outline: 'outline',
  ghost: 'ghost',
  link: 'link',
  destructive: 'destructive',
  // 原语层没有「虚线描边」这一档，落在 outline 上再由下方覆盖边框样式
  dashed: 'outline',
};

const UI_SIZE: Record<ButtonSize, UiButtonSize> = {
  sm: 'sm',
  small: 'sm',
  default: 'default',
  middle: 'default',
  lg: 'lg',
  large: 'lg',
  icon: 'icon',
};

/** lg 档项目层要求加粗，原语层没有这一档 */
const UI_SIZE_EXTRA: Partial<Record<ButtonSize, string>> = {
  lg: 'font-semibold',
};

/**
 * 外观完全委托给 ui/button 的变体表，本层只负责两件事：
 * 1) 虚线描边（项目历史别名，原语层没有）；
 * 2) 按压反馈（原语层只做颜色过渡，按下那一帧的缩放由项目层补）。
 */
export const buttonVariants = (options: {
  variant?: ButtonVariant;
  size?: ButtonSize;
  class?: any;
} = {}): string => {
  const variant = options.variant ?? 'default';
  const size = options.size ?? 'default';

  return cn(
    uiButtonVariants({ variant: UI_VARIANT[variant], size: UI_SIZE[size] }),
    UI_SIZE_EXTRA[size],
    variant === 'dashed' &&
      'border-dashed border-input bg-transparent text-muted-foreground shadow-none hover:border-primary hover:bg-transparent hover:text-primary',
    'transition-all active:scale-[0.99]',
    options.class,
  );
};
</script>

<script setup lang="ts">
import { computed } from 'vue';
import { RouterLink, type RouteLocationRaw } from 'vue-router';
import { Loader2 } from '@lucide/vue';

import { Button as UiButton } from '@/components/ui/button';

const emit = defineEmits<{ click: [event: MouseEvent] }>();

const props = withDefaults(
  defineProps<{
    variant?: ButtonVariant;
    type?: ButtonType;
    size?: ButtonSize;
    danger?: boolean;
    block?: boolean;
    loading?: boolean;
    disabled?: boolean;
    htmlType?: 'button' | 'submit' | 'reset';
    /** 传入后渲染为 RouterLink,保留链接语义(middle-click / 新标签页 / 右键菜单) */
    to?: string | RouteLocationRaw;
    class?: any;
  }>(),
  {
    type: 'default',
    danger: false,
    block: false,
    loading: false,
    disabled: false,
    htmlType: 'button',
  },
);

const resolvedVariant = computed<ButtonVariant>(() => {
  if (props.variant !== undefined) {
    return props.variant;
  }
  if (props.danger) {
    return 'destructive';
  }
  if (props.type === 'primary') {
    return 'primary';
  }
  if (props.type === 'text') {
    return 'ghost';
  }
  if (props.type === 'dashed') {
    return 'dashed';
  }
  if (props.type === 'ghost') {
    return 'ghost';
  }
  return 'outline';
});

const resolvedSize = computed<ButtonSize>(() => {
  if (props.size === 'small' || props.size === 'sm') return 'sm';
  if (props.size === 'large' || props.size === 'lg') return 'lg';
  if (props.size === 'icon') return 'icon';
  return 'default';
});

const uiVariant = computed(() => UI_VARIANT[resolvedVariant.value]);
const uiSize = computed(() => UI_SIZE[resolvedSize.value]);

const iconSize = computed(() => {
  if (resolvedSize.value === 'sm') return 14;
  if (resolvedSize.value === 'lg') return 18;
  return 15;
});

/** 链接模式下 disabled/loading 不能靠原生 disabled 属性生效,改用视觉与交互抑制 */
const inactive = computed(() => props.disabled || props.loading);

const isLink = computed(() => props.to !== undefined);

const btnClasses = computed(() => {
  return cn(
    buttonVariants({ variant: resolvedVariant.value, size: resolvedSize.value }),
    props.block && 'w-full',
    isLink.value && inactive.value && 'pointer-events-none cursor-not-allowed opacity-50',
    props.class,
  );
});

function onClick(event: MouseEvent): void {
  if (isLink.value && inactive.value) {
    event.preventDefault();
    return;
  }
  emit('click', event);
}
</script>