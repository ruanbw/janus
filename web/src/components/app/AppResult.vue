<template>
  <div class="flex flex-col items-center px-6 py-14 text-center">
    <span class="mb-4 flex h-16 w-16 items-center justify-center rounded-full" :class="iconWrapClasses">
      <component :is="icon" :size="30" stroke-width="1.8" />
    </span>
    <h3 class="text-lg font-semibold text-ink">{{ title }}</h3>
    <p v-if="subTitle" class="mt-1.5 max-w-md text-xs leading-relaxed text-ink-soft">{{ subTitle }}</p>
    <div v-if="$slots.extra" class="mt-5"><slot name="extra" /></div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import { CheckCircle2, Info, TriangleAlert, XCircle } from '@lucide/vue';

const props = withDefaults(
  defineProps<{
    status?: 'success' | 'error' | 'info' | 'warning';
    title?: string;
    subTitle?: string;
  }>(),
  { status: 'info', title: '' },
);

const ICONS = {
  success: CheckCircle2,
  error: XCircle,
  info: Info,
  warning: TriangleAlert,
};

const icon = computed(() => ICONS[props.status as keyof typeof ICONS]);

const iconWrapClasses = computed(() => {
  switch (props.status) {
    case 'success':
      return 'bg-ok/12 text-ok';
    case 'error':
      return 'bg-err/12 text-err';
    case 'warning':
      return 'bg-warn/12 text-warn';
    default:
      return 'bg-info/12 text-info';
  }
});
</script>
