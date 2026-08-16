<template>
  <div
    class="flex items-start gap-2.5 rounded-xl border px-3.5 py-3 text-[13px] leading-relaxed"
    :class="classes"
  >
    <span v-if="showIcon" class="mt-0.5 shrink-0">
      <component :is="icon" :size="15" />
    </span>
    <div class="min-w-0">
      <p class="font-medium">{{ message }}</p>
      <p v-if="description" class="mt-0.5 opacity-80">{{ description }}</p>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import { AlertTriangle, CheckCircle2, Info, XCircle } from '@lucide/vue';

const props = withDefaults(
  defineProps<{
    type?: 'info' | 'warning' | 'success' | 'error';
    showIcon?: boolean;
    message?: string;
    description?: string;
  }>(),
  { type: 'info', showIcon: true, message: '' },
);

const CLASSES: Record<string, string> = {
  info: 'border-info/30 bg-info/10 text-info',
  warning: 'border-warn/30 bg-warn/10 text-warn',
  success: 'border-ok/30 bg-ok/10 text-ok',
  error: 'border-err/30 bg-err/10 text-err',
};

const classes = computed(() => CLASSES[props.type]);

const icon = computed(() => {
  switch (props.type) {
    case 'warning':
      return AlertTriangle;
    case 'success':
      return CheckCircle2;
    case 'error':
      return XCircle;
    default:
      return Info;
  }
});
</script>
