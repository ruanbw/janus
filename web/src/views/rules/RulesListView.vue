<script setup lang="ts">
/**
 * 规则列表：只做一件事——让运营看清「有哪些规则、作用在哪、现在有没有在跑」，
 * 然后决定是改、停还是删。编辑与模拟都不在这里发生：
 * 编辑是表单操作，走 /rules/new 与 /rules/:id/edit；模拟走 /rules/simulator。
 */
import { computed, onMounted, ref } from 'vue';
import { useRouter } from 'vue-router';
import { FlaskConical, Link2, Pencil, Plus, RefreshCw, Search, Trash2 } from '@lucide/vue';

import { deleteRule, listRules, updateRule } from '@/api/rules';
import PageHeader from '@/components/PageHeader.vue';
import { confirm } from '@/components/ui/confirm';
import type { TableColumn, TablePaginationConfig } from '@/components/ui/types';
import type { Rule } from '@/types/api';
import { message } from '@/utils/toast';
import { actionLabel, actionTagColor, conditionSummary, logicLabel } from './ruleMeta';

const router = useRouter();

const rules = ref<Rule[]>([]);
const total = ref(0);
const page = ref(1);
const pageSize = ref(20);
const loading = ref(false);
const togglingId = ref<number | null>(null);

const actionFilter = ref<string>('all');
const statusFilter = ref<string>('all');
const keyword = ref('');

const ACTION_FILTERS = [
  { value: 'all', label: '全部动作' },
  { value: 'pass', label: '放行' },
  { value: 'redirect', label: '重定向' },
  { value: 'notfound', label: '404' },
  { value: 'throttle', label: '限流' },
];
const STATUS_FILTERS = [
  { value: 'all', label: '全部状态' },
  { value: 'enabled', label: '已启用' },
  { value: 'disabled', label: '已停用' },
];

/** 列表接口不带 conditions，过滤只能作用在当前页的内存副本上 */
const filteredRules = computed(() => {
  const kw = keyword.value.trim().toLowerCase();
  return rules.value.filter((rule) => {
    if (actionFilter.value !== 'all' && rule.action !== actionFilter.value) return false;
    if (statusFilter.value === 'enabled' && !rule.enabled) return false;
    if (statusFilter.value === 'disabled' && rule.enabled) return false;
    if (!kw) return true;
    return (
      rule.name.toLowerCase().includes(kw) ||
      (rule.description || '').toLowerCase().includes(kw) ||
      String(rule.id) === kw
    );
  });
});

const hasFilter = computed(
  () => actionFilter.value !== 'all' || statusFilter.value !== 'all' || keyword.value.trim() !== '',
);

const pagination = computed<TablePaginationConfig>(() => ({
  current: page.value,
  pageSize: pageSize.value,
  total: total.value,
  showSizeChanger: true,
  showTotal: (t: number) => `共 ${t} 条规则`,
}));

const columns: TableColumn[] = [
  { title: '编号', key: 'id', width: 80 },
  { title: '规则', key: 'name', minWidth: 240 },
  { title: '作用域', key: 'scope', minWidth: 200 },
  { title: '条件', key: 'conditions', minWidth: 260, ellipsis: true },
  { title: '动作', key: 'action', width: 104 },
  { title: '去向', key: 'destination', minWidth: 170, ellipsis: true },
  { title: '24h 命中', key: 'hits24h', width: 104, align: 'right' },
  { title: '状态', key: 'enabled', width: 80 },
  { title: '操作', key: 'actions', width: 128, align: 'right', nowrap: true },
];

function toRule(record: Record<string, unknown>): Rule {
  return record as unknown as Rule;
}

const tableData = computed(() => filteredRules.value as unknown as Record<string, unknown>[]);

async function loadRules() {
  loading.value = true;
  try {
    const res = await listRules({ page: page.value, pageSize: pageSize.value });
    rules.value = res.items;
    total.value = res.total;
    // 删到本页空了就回退一页，避免停在没有数据的页码上
    if (res.items.length === 0 && page.value > 1) {
      page.value -= 1;
      await loadRules();
    }
  } catch (error) {
    message.error(error instanceof Error ? error.message : '规则列表加载失败');
  } finally {
    loading.value = false;
  }
}

function onTableChange(p: { current?: number; pageSize?: number }) {
  if (p.pageSize) pageSize.value = p.pageSize;
  page.value = p.current ?? 1;
  loadRules();
}

function resetFilters() {
  actionFilter.value = 'all';
  statusFilter.value = 'all';
  keyword.value = '';
}

function openCreate() {
  router.push({ name: 'rule-create' });
}

function openEdit(rule: Rule) {
  router.push({ name: 'rule-edit', params: { id: String(rule.id) } });
}

function openSimulator() {
  router.push({ name: 'rule-simulator' });
}

async function onToggleRule(rule: Rule) {
  togglingId.value = rule.id;
  try {
    await updateRule(rule.id, { enabled: !rule.enabled });
    rule.enabled = !rule.enabled;
    message.success(rule.enabled ? `已启用「${rule.name}」` : `已停用「${rule.name}」`);
  } catch (error) {
    message.error(error instanceof Error ? error.message : '切换启用状态失败');
  } finally {
    togglingId.value = null;
  }
}

function handleDeleteRule(rule: Rule) {
  confirm({
    title: `删除规则「${rule.name}」？`,
    content: '删除后不可恢复，正在命中这条规则的访问会立刻回落到后续规则或短链自身流程。',
    okText: '删除',
    danger: true,
    onOk: async () => {
      try {
        await deleteRule(rule.id);
        message.success('规则已删除');
        await loadRules();
      } catch (error) {
        message.error(error instanceof Error ? error.message : '规则删除失败');
      }
    },
  });
}

onMounted(loadRules);
</script>

<template>
  <!-- 内容型页面与短链列表、总览一致：占满内容区，不居中限宽 -->
  <div>
    <PageHeader
      title="规则引擎"
      description="规则在短链可用性之后裁决：按优先级升序逐条求值，首条命中即定案。零关联短链的规则永远不会命中。"
    >
      <template #actions>
        <AppButton @click="openSimulator">
          <template #icon><FlaskConical :size="15" /></template>
          规则模拟器
        </AppButton>
        <AppButton @click="loadRules">
          <template #icon><RefreshCw :size="15" /></template>
          刷新
        </AppButton>
        <AppButton type="primary" @click="openCreate">
          <template #icon><Plus :size="15" /></template>
          新建规则
        </AppButton>
      </template>
    </PageHeader>

    <div class="mb-4 grid gap-3 sm:grid-cols-2 lg:grid-cols-[minmax(0,1fr)_180px_180px_auto]">
      <AppInput v-model="keyword" placeholder="搜索规则名称、描述或编号" allow-clear>
        <template #prefix><Search :size="15" class="text-ink-faint" /></template>
      </AppInput>
      <AppSelect v-model="actionFilter" :options="ACTION_FILTERS" />
      <AppSelect v-model="statusFilter" :options="STATUS_FILTERS" />
      <AppButton v-if="hasFilter" @click="resetFilters">清空过滤</AppButton>
    </div>

    <AppTable
      :columns="columns"
      :data-source="tableData"
      :loading="loading"
      :pagination="pagination"
      :scroll="{ x: 1500 }"
      row-key="id"
      @change="onTableChange"
    >
      <template #cell="{ column, record }">
        <template v-if="column.key === 'id'">
          <span class="mono text-[13px] text-ink-faint">#{{ record.id }}</span>
        </template>

        <template v-else-if="column.key === 'name'">
          <div class="min-w-0">
            <div class="truncate text-[13px] font-medium text-ink">{{ record.name }}</div>
            <div class="mt-0.5 flex items-center gap-2 text-xs text-ink-faint">
              <span>优先级 {{ record.priority }}</span>
              <span>·</span>
              <span>{{ logicLabel(record.logic as Rule['logic']) }}</span>
            </div>
          </div>
        </template>

        <template v-else-if="column.key === 'scope'">
          <div class="flex flex-col items-start gap-1">
            <AppTag v-if="record.scope === 'global'" color="blue">全局</AppTag>
            <div v-else class="flex items-center gap-1.5 text-[13px] text-ink">
              <Link2 :size="14" class="text-ink-faint" />
              <span>{{ record.linkCount || 0 }} 条短链</span>
            </div>
            <AppTag v-if="record.scope === 'links' && (record.linkCount || 0) === 0" color="warning">
              未关联短链 · 不会命中
            </AppTag>
          </div>
        </template>

        <template v-else-if="column.key === 'conditions'">
          <span class="mono text-xs text-ink-soft" :title="conditionSummary(toRule(record))">
            {{ conditionSummary(toRule(record)) }}
          </span>
        </template>

        <template v-else-if="column.key === 'action'">
          <AppTag :color="actionTagColor(record.action as Rule['action'])">
            {{ actionLabel(record.action as Rule['action']) }}
          </AppTag>
        </template>

        <template v-else-if="column.key === 'destination'">
          <span v-if="record.action === 'redirect' && record.destination" class="mono text-xs text-ink-soft">
            {{ record.destination }}
          </span>
          <span v-else class="text-xs text-ink-faint">—</span>
        </template>

        <template v-else-if="column.key === 'hits24h'">
          <span class="mono text-[13px] text-ink-soft">{{ record.hits24h ?? 0 }}</span>
        </template>

        <template v-else-if="column.key === 'enabled'">
          <button
            type="button"
            role="switch"
            :aria-checked="record.enabled === true"
            :aria-label="record.enabled ? '点击停用' : '点击启用'"
            :disabled="togglingId === record.id"
            class="relative h-5 w-9 shrink-0 rounded-full transition-colors disabled:cursor-not-allowed disabled:opacity-50"
            :class="record.enabled ? 'bg-brand-500' : 'bg-line-strong'"
            @click="onToggleRule(toRule(record))"
          >
            <span
              class="absolute top-0.5 h-4 w-4 rounded-full bg-white shadow transition-all"
              :class="record.enabled ? 'left-[18px]' : 'left-0.5'"
            />
          </button>
        </template>

        <template v-else-if="column.key === 'actions'">
          <div class="flex items-center justify-end gap-1">
            <AppButton size="small" type="text" @click="openEdit(toRule(record))">
              <template #icon><Pencil :size="14" /></template>
              编辑
            </AppButton>
            <AppButton size="small" type="text" danger @click="handleDeleteRule(toRule(record))">
              <template #icon><Trash2 :size="14" /></template>
            </AppButton>
          </div>
        </template>
      </template>
    </AppTable>
  </div>
</template>
