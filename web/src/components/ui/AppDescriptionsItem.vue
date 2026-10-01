<template>
  <div class="flex min-w-0 items-baseline gap-3">
    <dt class="w-24 shrink-0 text-xs text-ink-faint">{{ label }}</dt>
    <dd class="min-w-0 flex-1 text-xs text-ink"><slot /></dd>
  </div>
</template>

<script setup lang="ts">
defineProps<{
  label: string;
}>();
</script>
