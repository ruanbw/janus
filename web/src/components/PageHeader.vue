<template>
  <div class="page-header">
    <div class="page-header-text">
      <h2 class="page-header-title">{{ title }}</h2>
      <p v-if="description" class="page-header-desc">{{ description }}</p>
    </div>
    <div v-if="$slots.actions" class="page-header-actions">
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

<style scoped>
.page-header {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 16px;
  flex-wrap: wrap;
  margin-bottom: 20px;
}

.page-header-title {
  margin: 0;
  font-size: 20px;
  font-weight: 700;
  line-height: 1.4;
  color: #0f172a;
}

.page-header-desc {
  margin: 6px 0 0;
  font-size: 13px;
  line-height: 1.7;
  color: #8b98a5;
  max-width: 640px;
}

.page-header-actions {
  display: flex;
  align-items: center;
  gap: 8px;
}
</style>
