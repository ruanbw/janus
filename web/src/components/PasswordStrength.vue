<template>
  <div v-if="password.length > 0" class="mt-1.5">
    <div class="flex gap-1">
      <span
        v-for="i in 4"
        :key="i"
        class="h-1 flex-1 rounded-full transition-colors"
        :class="i <= level ? barColor : 'bg-surface-strong'"
      />
    </div>
    <p class="mt-1 text-xs text-ink-faint">
      密码强度:<span :class="labelColor">{{ label }}</span>
      <span v-if="hint" class="text-ink-faint">· {{ hint }}</span>
    </p>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue';

const props = defineProps<{ password: string }>();
const password = computed(() => props.password);

const score = computed(() => {
  const p = props.password;
  let s = 0;
  if (p.length >= 8) s++;
  if (p.length >= 12) s++;
  if (/[0-9]/.test(p)) s++;
  if (/[a-z]/.test(p) && /[A-Z]/.test(p)) s++;
  if (/[^A-Za-z0-9]/.test(p)) s++;
  return s;
});

const level = computed(() => {
  if (score.value <= 1) return 1;
  if (score.value === 2) return 2;
  if (score.value === 3) return 3;
  return 4;
});

const label = computed(() => ['', '弱', '一般', '较强', '强'][level.value]);
const barColor = computed(() => ['', 'bg-err', 'bg-warn', 'bg-info', 'bg-ok'][level.value]);
const labelColor = computed(() => ['', 'text-err', 'text-warn', 'text-info', 'text-ok'][level.value]);
const hint = computed(() => {
  const p = props.password;
  if (p.length > 0 && p.length < 8) return '至少 8 位';
  if (level.value <= 2) return '混合大小写、数字与符号可提升强度';
  return '';
});
</script>
