<template>
  <div class="flex items-center gap-3" :style="dividerStyle">
    <span v-if="$slots.default" class="shrink-0 text-xs text-ink-faint"><slot /></span>
    <span class="h-px flex-1 bg-line" />
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue';

const props = defineProps<{
  /** 与 antd 一致的 style 透传(margin 等) */
  style?: Record<string, string | number>;
}>();

const dividerStyle = computed(() => props.style ?? {});
</script>
