<template>
  <div v-if="$slots.default" class="relative">
    <div v-if="spinning" class="absolute inset-0 z-10 flex items-center justify-center rounded-lg bg-surface/60 backdrop-blur-[1px]">
      <Loader2 :size="spinSize" class="animate-spin text-brand-600 dark:text-brand-400" />
    </div>
    <div :class="spinning ? 'pointer-events-none opacity-60' : ''"><slot /></div>
  </div>
  <div v-else class="flex flex-col items-center gap-3 py-8">
    <Loader2 :size="spinSize" class="animate-spin text-brand-600 dark:text-brand-400" />
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import { Loader2 } from '@lucide/vue';

const props = withDefaults(
  defineProps<{
    spinning?: boolean;
    size?: 'default' | 'large' | 'small';
  }>(),
  { spinning: true, size: 'default' },
);

const spinSize = computed(() => (props.size === 'large' ? 32 : props.size === 'small' ? 16 : 22));
</script>
