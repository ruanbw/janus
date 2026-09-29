<template>
  <div
    role="alert"
    :class="alertClasses"
  >
    <span v-if="showIcon" class="mt-0.5 shrink-0 text-current">
      <slot name="icon">
        <component :is="icon" :size="16" />
      </slot>
    </span>
    <div class="min-w-0 flex-1">
      <h5
        v-if="headingText || $slots.title"
        class="font-medium leading-none tracking-tight"
        :class="(description || $slots.description || $slots.default) ? 'mb-1.5' : ''"
      >
        <slot name="title">{{ headingText }}</slot>
      </h5>
      <div
        v-if="description || $slots.description || $slots.default"
        class="text-xs leading-relaxed opacity-90 [&_p]:leading-relaxed"
      >
        <slot name="description">
          <slot>{{ description }}</slot>
        </slot>
      </div>
    </div>
  </div>
</template>

<script lang="ts">
export type AlertType = 'info' | 'warning' | 'success' | 'error';
export type AlertVariant = 'default' | 'destructive' | 'info' | 'warning' | 'success';
</script>

<script setup lang="ts">
import { computed } from 'vue';
import { CheckCircle2, Info, TriangleAlert, XCircle } from '@lucide/vue';

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

const headingText = computed(() => props.title || props.message || '');

const resolvedVariant = computed<string>(() => {
  if (props.variant !== undefined) {
    if (props.variant === 'destructive') return 'error';
    return props.variant;
  }
  return props.type;
});

const VARIANT_CLASSES: Record<string, string> = {
  default: 'border-line bg-surface text-ink',
  info: 'border-info/30 bg-info/10 text-info dark:border-info/40 dark:bg-info/15',
  warning: 'border-warn/30 bg-warn/10 text-warn dark:border-warn/40 dark:bg-warn/15',
  success: 'border-ok/30 bg-ok/10 text-ok dark:border-ok/40 dark:bg-ok/15',
  error: 'border-err/30 bg-err/10 text-err dark:border-err/40 dark:bg-err/15',
};

const alertClasses = computed(() => {
  return cn(
    'relative flex w-full items-start gap-3 rounded-lg border px-4 py-3.5 text-sm shadow-2xs transition-colors',
    VARIANT_CLASSES[resolvedVariant.value] ?? VARIANT_CLASSES.info,
    props.class,
  );
});

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
