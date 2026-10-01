<template>
  <RouterLink
    to="/overview"
    aria-label="Janus 控制台首页"
    class="flex h-14 shrink-0 items-center gap-2.5 px-3.5 transition-opacity hover:opacity-80"
  >
    <BrandMark :size="28" class="shrink-0" />
    <span v-if="showLabels" class="flex flex-col leading-tight">
      <span class="text-sm font-bold tracking-tight text-sidebar-ink-active">Janus</span>
      <span class="font-mono text-2xs tracking-wider text-sidebar-ink uppercase">Janus Console</span>
    </span>
  </RouterLink>
</template>

<script setup lang="ts">
import { computed } from 'vue';

import BrandMark from '@/components/BrandMark.vue';

const props = withDefaults(defineProps<{ collapsed?: boolean }>(), { collapsed: false });

const showLabels = computed(() => !props.collapsed);
</script>
