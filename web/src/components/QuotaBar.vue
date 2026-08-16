<template>
  <div
    v-if="visible"
    class="mb-5 grid grid-cols-1 gap-3.5 rounded-xl border border-line bg-surface p-4 md:grid-cols-2"
    :class="single ? 'md:grid-cols-1' : ''"
  >
    <div v-if="linksMax !== undefined" class="flex min-w-0 items-center gap-3">
      <span class="w-16 shrink-0 text-[13px] text-ink-soft">短链配额</span>
      <AppProgress
        :percent="linkPercent"
        :status="linkPercent >= 100 ? 'exception' : 'normal'"
        :show-info="false"
        :stroke-width="6"
      />
      <span class="shrink-0 text-right text-[13px] font-semibold tabular-nums" :class="linkPercent >= 100 ? 'text-err' : 'text-ink'">
        {{ linksUsed ?? 0 }}/{{ linksMax ?? '-' }}
      </span>
    </div>
    <div v-if="domainsMax !== undefined" class="flex min-w-0 items-center gap-3">
      <span class="w-16 shrink-0 text-[13px] text-ink-soft">域名配额</span>
      <AppProgress
        :percent="domainPercent"
        :status="domainPercent >= 100 ? 'exception' : 'normal'"
        :show-info="false"
        :stroke-width="6"
      />
      <span class="shrink-0 text-right text-[13px] font-semibold tabular-nums" :class="domainPercent >= 100 ? 'text-err' : 'text-ink'">
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
const single = computed(() => (props.linksMax !== undefined) !== (props.domainsMax !== undefined));

function percent(used: number | undefined, max: number | undefined): number {
  if (max === undefined || max <= 0) return 0;
  return Math.min(100, Math.round(((used ?? 0) / max) * 100));
}

const linkPercent = computed(() => percent(props.linksUsed, props.linksMax));
const domainPercent = computed(() => percent(props.domainsUsed, props.domainsMax));
</script>
