<template>
  <div v-if="visible" class="quota-bar">
    <div v-if="linksMax !== undefined" class="quota-row">
      <span class="quota-label">短链配额</span>
      <a-progress
        class="quota-progress"
        :percent="linkPercent"
        :status="linkPercent >= 100 ? 'exception' : 'normal'"
        :show-info="false"
        :stroke-width="6"
      />
      <span class="quota-value" :class="{ danger: linkPercent >= 100 }">
        {{ linksUsed ?? 0 }}/{{ linksMax ?? '-' }}
      </span>
    </div>
    <div v-if="domainsMax !== undefined" class="quota-row">
      <span class="quota-label">域名配额</span>
      <a-progress
        class="quota-progress"
        :percent="domainPercent"
        :status="domainPercent >= 100 ? 'exception' : 'normal'"
        :show-info="false"
        :stroke-width="6"
      />
      <span class="quota-value" :class="{ danger: domainPercent >= 100 }">
        {{ domainsUsed ?? 0 }}/{{ domainsMax ?? '-' }}
      </span>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue';

const props = defineProps<{
  linksUsed?: number;
  linksMax?: number;
  domainsUsed?: number;
  domainsMax?: number;
}>();

const visible = computed(() => props.linksMax !== undefined || props.domainsMax !== undefined);

function percent(used: number | undefined, max: number | undefined): number {
  if (max === undefined || max <= 0) return 0;
  return Math.min(100, Math.round(((used ?? 0) / max) * 100));
}

const linkPercent = computed(() => percent(props.linksUsed, props.linksMax));
const domainPercent = computed(() => percent(props.domainsUsed, props.domainsMax));
</script>

<style scoped>
.quota-bar {
  display: grid;
  grid-template-columns: 1fr;
  gap: 14px;
  padding: 14px 16px;
  margin-bottom: 20px;
  background: #f8fafc;
  border: 1px solid #e6ebf1;
  border-radius: 10px;
}

@media (min-width: 768px) {
  .quota-bar {
    grid-template-columns: 1fr 1fr;
  }
}

/* 仅一行配额时占满整行,避免左侧留白 */
.quota-bar:has(> .quota-row:only-child) {
  grid-template-columns: 1fr;
}

.quota-row {
  display: flex;
  align-items: center;
  gap: 12px;
  min-width: 0;
}

.quota-label {
  flex-shrink: 0;
  width: 64px;
  font-size: 13px;
  color: #5b6b7c;
}

.quota-progress {
  flex: 1;
  min-width: 60px;
  margin: 0 !important;
}

.quota-value {
  flex-shrink: 0;
  min-width: 52px;
  text-align: right;
  font-size: 13px;
  font-weight: 600;
  color: #0f172a;
  font-variant-numeric: tabular-nums;
}

.quota-value.danger {
  color: #dc2626;
}
</style>
