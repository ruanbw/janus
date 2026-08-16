<template>
  <button
    :type="htmlType"
    :disabled="disabled || loading"
    :class="btnClasses"
    class="app-btn inline-flex select-none items-center justify-center gap-1.5 font-medium transition-colors disabled:cursor-not-allowed disabled:opacity-55"
  >
    <Loader2 v-if="loading" :size="iconSize" class="animate-spin" />
    <span v-else-if="$slots.icon" class="inline-flex"><slot name="icon" /></span>
    <span class="inline-flex items-center gap-1.5"><slot /></span>
  </button>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import { Loader2 } from '@lucide/vue';

type ButtonType = 'primary' | 'default' | 'text' | 'dashed' | 'ghost';
type ButtonSize = 'small' | 'middle' | 'large';

const props = withDefaults(
  defineProps<{
    type?: ButtonType;
    size?: ButtonSize | 'default';
    danger?: boolean;
    block?: boolean;
    loading?: boolean;
    disabled?: boolean;
    htmlType?: 'button' | 'submit' | 'reset';
  }>(),
  { type: 'default', size: 'middle', danger: false, block: false, loading: false, disabled: false, htmlType: 'button' },
);

const iconSize = computed(() => (props.size === 'small' ? 13 : props.size === 'large' ? 17 : 15));

const btnClasses = computed(() => {
  const list: string[] = [];
  // 尺寸
  if (props.size === 'small') list.push('h-7 rounded-md px-2.5 text-xs');
  else if (props.size === 'large') list.push('h-10 rounded-lg px-5 text-[15px]');
  else list.push('h-8 rounded-lg px-3.5 text-[13px]');

  if (props.block) list.push('w-full');

  // 危险色优先(antd 语义:danger 修饰 primary/text/default)
  if (props.danger) {
    if (props.type === 'primary') {
      list.push('border border-err bg-err text-white hover:bg-err/90');
    } else if (props.type === 'text') {
      list.push('border border-transparent bg-transparent text-err hover:bg-err/10');
    } else {
      list.push('border border-err/45 bg-transparent text-err hover:bg-err/10');
    }
    return list;
  }

  switch (props.type) {
    case 'primary':
      list.push(
        'border border-transparent bg-brand-600 text-white shadow-sm hover:bg-brand-500 dark:bg-brand-500 dark:hover:bg-brand-400',
      );
      break;
    case 'text':
      list.push('border border-transparent bg-transparent text-ink-soft hover:bg-brand-50 hover:text-brand-600 dark:hover:bg-brand-500/10');
      break;
    case 'dashed':
      list.push('border border-dashed border-line-strong bg-transparent text-ink-soft hover:border-brand-400 hover:text-brand-600');
      break;
    case 'ghost':
      list.push('border border-brand-600/40 bg-transparent text-brand-600 hover:bg-brand-50 dark:text-brand-400 dark:hover:bg-brand-500/10');
      break;
    default:
      list.push(
        'border border-line bg-surface text-ink shadow-xs hover:border-brand-400 hover:text-brand-600 dark:bg-surface-strong',
      );
  }
  return list;
});
</script>
