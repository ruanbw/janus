<template>
  <div class="mb-5 flex flex-wrap items-start justify-between gap-x-4 gap-y-3">
    <div class="min-w-0">
      <h2 class="text-xl font-bold leading-snug text-ink">{{ title }}</h2>
      <p v-if="description" class="mt-1.5 max-w-2xl text-sm leading-relaxed text-ink-soft">
        {{ description }}
      </p>
    </div>
    <div v-if="$slots.actions" class="flex shrink-0 flex-wrap items-center gap-2">
      <slot name="actions" />
    </div>
  </div>
</template>

<script setup lang="ts">
defineProps<{
  title: string;
  description?: string;
}>();
</script>
