<template>
  <Alert
    role="alert"
    :variant="alertVariant"
    :class="alertClasses"
  >
    <span v-if="showIcon" class="mt-0.5 shrink-0 text-current">
      <slot name="icon">
        <component :is="icon" :size="16" />
      </slot>
    </span>
    <div class="min-w-0 flex-1">
      <h5
        v-if="$slots.title || title"
        :class="cn(alertTitleVariants(), hasBody ? 'mb-1.5' : '')"
      >
        <slot name="title">{{ title }}</slot>
      </h5>
      <div v-if="hasBody" :class="alertDescriptionVariants()">
        <slot name="description">
          <slot>{{ description || message }}</slot>
        </slot>
      </div>
    </div>
  </Alert>
</template>

<script lang="ts">
export type AlertType = 'info' | 'warning' | 'success' | 'error';
export type AlertVariant = 'default' | 'destructive' | 'info' | 'warning' | 'success';
</script>

<script setup lang="ts">
import { computed, useSlots } from 'vue';
import { CheckCircle2, Info, TriangleAlert, XCircle } from '@lucide/vue';

import {
  alertDescriptionVariants,
  alertTitleVariants,
  type AlertVariant as PrimitiveAlertVariant,
} from '@/components/ui/alert';
import { Alert } from '@/components/ui/alert';
import { cn } from '@/lib/utils';

const props = withDefaults(
  defineProps<{
    type?: AlertType;
    variant?: AlertVariant;
    showIcon?: boolean;
    title?: string;
    message?: string;
    description?: string;
    class?: any;
  }>(),
  { type: 'info', showIcon: true },
);

const emitUnused = null;
const slots = useSlots();

/**
 * 标题只认 title（或 title 插槽）；message 是正文，不参与标题。
 * 原先 headingText = title || message 且 hasBody 不看 message，
 * 于是「title + message」时 message 全文被当标题吃掉且不进正文块。
 */
const hasBody = computed(
  () =>
    Boolean(props.description) ||
    Boolean(props.message) ||
    Boolean(slots.description) ||
    Boolean(slots.default),
);

/** variant 优先于 type;destructive 归一到 error */
const resolvedVariant = computed<string>(() => {
  if (props.variant !== undefined) {
    if (props.variant === 'destructive') return 'error';
    return props.variant;
  }
  return props.type;
});

/** 原语层只认 default / destructive,项目语义色由下面的类表叠加 */
const alertVariant = computed<PrimitiveAlertVariant>(() =>
  resolvedVariant.value === 'error' ? 'destructive' : 'default',
);

const TONE_CLASSES: Record<string, string> = {
  default: 'border-line bg-surface text-ink',
  info: 'border-info/30 bg-info/10 text-info',
  warning: 'border-warn/30 bg-warn/10 text-warn',
  success: 'border-ok/30 bg-ok/10 text-ok',
  error: 'border-err/30 bg-err/10 text-err',
};

const alertClasses = computed(() =>
  cn(
    'flex items-start gap-3 px-4 py-3.5 text-sm shadow-2xs transition-colors',
    TONE_CLASSES[resolvedVariant.value] ?? TONE_CLASSES.info,
    props.class,
  ),
);

const icon = computed(() => {
  switch (resolvedVariant.value) {
    case 'warning':
      return TriangleAlert;
    case 'success':
      return CheckCircle2;
    case 'error':
      return XCircle;
    default:
      return Info;
  }
});
</script>