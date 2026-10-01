<template>
  <RouterLink
    v-if="to !== undefined"
    :to="to"
    :class="btnClasses"
    :aria-disabled="inactive ? 'true' : undefined"
    :tabindex="inactive ? -1 : undefined"
    @click="onClick"
  >
    <Loader2 v-if="loading" :size="iconSize" class="animate-spin shrink-0" />
    <span v-else-if="$slots.icon" class="inline-flex shrink-0"><slot name="icon" /></span>
    <span v-if="$slots.default" class="inline-flex items-center gap-1.5"><slot /></span>
  </RouterLink>
  <button
    v-else
    :type="htmlType"
    :disabled="disabled || loading"
    :class="btnClasses"
    @click="onClick"
  >
    <Loader2 v-if="loading" :size="iconSize" class="animate-spin shrink-0" />
    <span v-else-if="$slots.icon" class="inline-flex shrink-0"><slot name="icon" /></span>
    <span v-if="$slots.default" class="inline-flex items-center gap-1.5"><slot /></span>
  </button>
</template>

<script lang="ts">
import { cva } from 'class-variance-authority';

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

export const buttonVariants = cva(
  'app-btn inline-flex select-none items-center justify-center gap-1.5 whitespace-nowrap rounded-md font-medium transition-all active:scale-[0.99] outline-none focus-visible:ring-[3px] focus-visible:ring-ring/50 disabled:pointer-events-none disabled:cursor-not-allowed disabled:opacity-50 [&_svg]:pointer-events-none [&_svg]:shrink-0',
  {
    variants: {
      variant: {
        default:
          'bg-primary text-primary-foreground shadow-xs hover:bg-primary/90',
        primary:
          'bg-primary text-primary-foreground shadow-xs hover:bg-primary/90',
        destructive:
          'border border-transparent bg-destructive text-destructive-foreground shadow-xs hover:bg-destructive/90',
        outline:
          'border border-input bg-background text-foreground shadow-xs hover:bg-accent hover:text-accent-foreground',
        secondary:
          'bg-secondary text-secondary-foreground shadow-xs hover:bg-secondary/80',
        ghost:
          'text-muted-foreground hover:bg-accent hover:text-accent-foreground',
        link:
          'text-primary underline-offset-4 hover:underline',
        dashed:
          'border border-dashed border-input bg-transparent text-muted-foreground hover:border-primary hover:text-primary',
      },
      size: {
        default: 'h-9 px-4 py-2 text-sm',
        sm: 'h-8 rounded-md px-3 text-xs',
        lg: 'h-10 rounded-md px-6 text-sm font-semibold',
        icon: 'h-9 w-9 p-0',
      },
    },
    defaultVariants: {
      variant: 'default',
      size: 'default',
    },
  },
);
</script>

<script setup lang="ts">
import { computed } from 'vue';
import { RouterLink, type RouteLocationRaw } from 'vue-router';
import { Loader2 } from '@lucide/vue';

import { cn } from '@/lib/utils';

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

const resolvedSize = computed<'default' | 'sm' | 'lg' | 'icon'>(() => {
  if (props.size === 'small' || props.size === 'sm') return 'sm';
  if (props.size === 'large' || props.size === 'lg') return 'lg';
  if (props.size === 'icon') return 'icon';
  return 'default';
});

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
