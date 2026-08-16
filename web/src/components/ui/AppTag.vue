<template>
  <span
    class="inline-flex items-center gap-1 rounded-md border px-2 py-0.5 text-xs font-medium leading-5 whitespace-nowrap"
    :class="presetClass"
    :style="customStyle"
  >
    <slot />
  </span>
</template>

<script setup lang="ts">
import { computed } from 'vue';

type TagColor =
  | 'default'
  | 'blue'
  | 'cyan'
  | 'green'
  | 'success'
  | 'orange'
  | 'warning'
  | 'red'
  | 'error'
  | 'purple'
  | 'gold'
  | 'geekblue';

const props = withDefaults(
  defineProps<{
    /** antd Tag 兼容的预设色名,或任意 hex 颜色 */
    color?: TagColor | string;
  }>(),
  { color: 'default' },
);

const PRESETS: Record<string, string> = {
  default:
    'border-line bg-surface-strong text-ink-soft dark:bg-surface-strong',
  blue: 'border-blue-200 bg-blue-50 text-blue-700 dark:border-blue-500/30 dark:bg-blue-500/10 dark:text-blue-300',
  cyan: 'border-cyan-200 bg-cyan-50 text-cyan-700 dark:border-cyan-500/30 dark:bg-cyan-500/10 dark:text-cyan-300',
  green:
    'border-green-200 bg-green-50 text-green-700 dark:border-green-500/30 dark:bg-green-500/10 dark:text-green-300',
  success:
    'border-green-200 bg-green-50 text-green-700 dark:border-green-500/30 dark:bg-green-500/10 dark:text-green-300',
  orange:
    'border-orange-200 bg-orange-50 text-orange-700 dark:border-orange-500/30 dark:bg-orange-500/10 dark:text-orange-300',
  warning:
    'border-orange-200 bg-orange-50 text-orange-700 dark:border-orange-500/30 dark:bg-orange-500/10 dark:text-orange-300',
  red: 'border-red-200 bg-red-50 text-red-700 dark:border-red-500/30 dark:bg-red-500/10 dark:text-red-300',
  error:
    'border-red-200 bg-red-50 text-red-700 dark:border-red-500/30 dark:bg-red-500/10 dark:text-red-300',
  purple:
    'border-purple-200 bg-purple-50 text-purple-700 dark:border-purple-500/30 dark:bg-purple-500/10 dark:text-purple-300',
  gold: 'border-amber-200 bg-amber-50 text-amber-700 dark:border-amber-500/30 dark:bg-amber-500/10 dark:text-amber-300',
  geekblue:
    'border-indigo-200 bg-indigo-50 text-indigo-700 dark:border-indigo-500/30 dark:bg-indigo-500/10 dark:text-indigo-300',
};

const presetClass = computed(() => PRESETS[props.color] ?? '');
const customStyle = computed(() => {
  if (PRESETS[props.color] !== undefined) return undefined;
  const hex = props.color;
  if (/^#[0-9a-fA-F]{6}$/.test(hex) === false) return undefined;
  return {
    color: hex,
    borderColor: hex + '55',
    backgroundColor: hex + '14',
  };
});
</script>
