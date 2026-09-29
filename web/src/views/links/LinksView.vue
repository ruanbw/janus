<template>
  <div class="flex flex-col gap-5 pb-10" data-od-id="links-view">
    <!-- ==================== 4 个 KPI 指标 ==================== -->
    <section class="kpi-grid" data-od-id="links-kpi">
      <!-- KPI 1: 短链配额 -->
      <div class="kpi">
        <div class="kpi-k">短链配额使用</div>
        <div class="kpi-v">
          {{ usage?.links ?? total }}
          <span class="text-[14px] text-muted font-normal">/ {{ usage?.maxLinks ?? '不限' }}</span>
        </div>
        <div class="kpi-sub">
          已用 {{ quotaPercent }}% · 剩余 {{ remainingQuota }}
        </div>
      </div>

      <!-- KPI 2: 活跃短链 -->
      <div class="kpi">
        <div class="kpi-k">活跃服务中</div>
        <div class="kpi-v">
          {{ activeLinksCount }}<span class="text-[14px] text-muted font-normal"> 条</span>
        </div>
        <div class="kpi-sub">
          <span class="dot dot-live" style="display: inline-block; color: var(--accent)"></span>
          已启用短链正常对外重定向
        </div>
      </div>

      <!-- KPI 3: 累计访问 -->
      <div class="kpi">
        <div class="kpi-k">本页累计访问量</div>
        <div class="kpi-v">
          {{ totalVisits.toLocaleString() }}<span class="text-[14px] text-muted font-normal"> 次</span>
        </div>
        <div class="kpi-sub">包含跳转型与落地页访问统计</div>
      </div>

      <!-- KPI 4: 落地页转化与 CTR -->
      <div class="kpi">
        <div class="kpi-k">落地页点击转化</div>
        <div class="kpi-v">
          {{ totalClicks.toLocaleString() }}<span class="text-[14px] text-muted font-normal"> 次</span>
        </div>
        <div class="kpi-sub">落地页综合转化率 {{ overallCtr }}</div>
      </div>
    </section>

    <!-- ==================== 短链列表主面板 ==================== -->
    <section class="panel" data-od-id="link-list">
      <div class="panel-hd">
        <div>
          <h2>短链列表</h2>
          <p>管理租户名下的短链，支持类型过滤、状态切换与出口多目标轮询配置。</p>
        </div>
        <div class="btn-row">
          <button
            type="button"
            class="btn btn-sm"
            :disabled="loading"
            title="刷新短链列表"
            @click="loadData"
          >
            <RefreshCw :size="13" :class="loading ? 'animate-spin' : ''" />
            刷新
          </button>
          <button
            type="button"
            class="btn btn-sm"
            @click="openBatchModal"
          >
            <Upload :size="13" />
            批量导入
          </button>
          <button
            type="button"
            class="btn btn-sm"
            :disabled="links.length === 0"
            @click="exportCsv"
          >
            <FileDown :size="13" />
            导出 CSV
          </button>
          <button
            type="button"
            class="btn btn-sm btn-primary"
            id="newLink"
            @click="goCreate"
          >
            <Plus :size="14" />
            新建短链
          </button>
        </div>
      </div>

      <!-- 搜索与筛选工具栏 -->
      <div class="panel-bd">
        <div class="toolbar">
          <div class="relative grow min-w-[200px]">
            <input
              v-model="keyword"
              class="input input-icon"
              id="linkSearch"
              placeholder="搜索短码、域名或目标 URL…"
              aria-label="搜索短链"
            />
            <Search
              :size="14"
              class="absolute left-2.5 top-1/2 -translate-y-1/2 text-muted pointer-events-none"
            />
          </div>
          <select
            v-model="typeFilter"
            class="select"
            id="typeFilter"
            aria-label="按类型过滤"
          >
            <option value="all">全部类型</option>
            <option value="redirect">跳转型 (redirect)</option>
            <option value="landing">落地页型 (landing)</option>
          </select>
          <select
            v-model="statusFilter"
            class="select"
            id="linkStatus"
            aria-label="按状态过滤"
          >
            <option value="all">全部状态</option>
            <option value="enabled">已启用 (enabled)</option>
            <option value="disabled">已停用 (disabled)</option>
          </select>
          <button
            v-if="hasActiveFilter"
            type="button"
            class="btn btn-sm btn-ghost text-muted hover:text-fg"
            @click="resetFilters"
          >
            <X :size="13" />
            清空过滤
          </button>
        </div>
      </div>

      <!-- 表格数据区 -->
      <div class="tbl-wrap">
        <table class="tbl" id="linkTable">
          <thead>
            <tr>
              <th class="shrink">短码</th>
              <th class="shrink">承载域名</th>
              <th class="shrink">类型</th>
              <th>出口目标 URL</th>
              <th class="num">24h 访问</th>
              <th class="num">转化点击</th>
              <th class="num">CTR</th>
              <th class="shrink">状态</th>
              <th class="shrink">启用</th>
              <th class="shrink">操作</th>
            </tr>
          </thead>
          <tbody>
            <!-- 加载态 -->
            <tr v-if="loading && links.length === 0">
              <td colspan="10" class="empty">
                <div class="flex items-center justify-center gap-2 text-muted py-6">
                  <RefreshCw class="animate-spin" :size="16" />
                  正在加载短链数据...
                </div>
              </td>
            </tr>

            <!-- 空状态 -->
            <tr v-else-if="filteredLinks.length === 0">
              <td colspan="10" class="py-8">
                <AppEmpty
                  :description="links.length === 0 ? '暂无短链记录，请点击下方按钮创建第一条短链' : '未找到符合当前筛选条件的短链记录'"
                />
                <div class="flex justify-center mt-3">
                  <button
                    v-if="links.length === 0"
                    type="button"
                    class="btn btn-sm btn-primary"
                    @click="goCreate"
                  >
                    <Plus :size="14" />
                    新建短链
                  </button>
                  <button
                    v-else
                    type="button"
                    class="btn btn-sm"
                    @click="resetFilters"
                  >
                    重置过滤条件
                  </button>
                </div>
              </td>
            </tr>

            <!-- 列表行 -->
            <tr
              v-for="link in filteredLinks"
              :key="link.id"
              :data-status="link.status === 'enabled' ? 'on' : 'off'"
            >
              <!-- 短码 -->
              <td class="shrink">
                <div class="row items-center" style="gap: 6px; flex-wrap: nowrap">
                  <button
                    type="button"
                    class="linkish mono font-semibold text-left"
                    :title="'点击编辑 ' + link.code"
                    @click="goEdit(link)"
                  >
                    {{ link.code }}
                  </button>
                  <button
                    type="button"
                    class="icon-btn text-muted hover:text-fg"
                    style="width: 22px; height: 22px; border: none; background: transparent; padding: 0"
                    title="复制完整短链 URL"
                    @click.stop="copyLinkUrl(link)"
                  >
                    <Copy :size="12" />
                  </button>
                  <a
                    :href="'https://' + getPrimaryDomain(link) + '/' + link.code"
                    target="_blank"
                    rel="noopener noreferrer"
                    class="icon-btn text-muted hover:text-fg inline-flex items-center justify-center"
                    style="width: 22px; height: 22px; border: none; background: transparent; padding: 0"
                    title="在新标签页测试访问短链"
                    @click.stop
                  >
                    <ExternalLink :size="12" />
                  </a>
                </div>
              </td>

              <!-- 承载域名 -->
              <td class="shrink mono tiny muted">
                <div class="flex items-center gap-1.5 flex-wrap">
                  <span>{{ getPrimaryDomain(link) }}</span>
                  <span
                    v-if="link.domains && link.domains.length > 1"
                    class="badge badge-neutral micro"
                    :title="link.domains.join(', ')"
                  >
                    +{{ link.domains.length - 1 }}
                  </span>
                </div>
              </td>

              <!-- 类型 -->
              <td class="shrink">
                <span v-if="link.linkType === 'landing'" class="badge badge-neutral">
                  落地页型
                </span>
                <span v-else class="badge badge-neutral">
                  跳转型 · {{ link.redirectStatus || '302' }}
                </span>
              </td>

              <!-- 目标 URL -->
              <td>
                <div class="flex items-center gap-1.5 min-w-0 max-w-md">
                  <span class="mono tiny truncate" :title="link.targetUrls?.[0] || '—'">
                    {{ formatTargetDisplay(link.targetUrls) }}
                  </span>
                  <span
                    v-if="link.targetUrls && link.targetUrls.length > 1"
                    class="badge badge-neutral micro shrink-0"
                    :title="`多目标轮询 (${link.targetUrls.length} 个出口):\n` + link.targetUrls.join('\n')"
                  >
                    +{{ link.targetUrls.length - 1 }} 轮询
                  </span>
                </div>
              </td>

              <!-- 24h 访问 -->
              <td class="num">
                {{ (link.visits || 0).toLocaleString() }}
              </td>

              <!-- 转化点击 -->
              <td class="num">
                {{ link.linkType === 'landing' ? (link.clicks || 0).toLocaleString() : '—' }}
              </td>

              <!-- CTR -->
              <td class="num">
                {{ getLinkCtr(link) }}
              </td>

              <!-- 状态 -->
              <td class="shrink">
                <span :class="link.status === 'enabled' ? 'badge badge-ok' : 'badge badge-neutral'">
                  {{ link.status === 'enabled' ? '已启用' : '已停用' }}
                </span>
              </td>

              <!-- 启用 Switch -->
              <td class="shrink">
                <label class="switch">
                  <input
                    type="checkbox"
                    :checked="link.status === 'enabled'"
                    :disabled="statusUpdatingId === link.id"
                    :aria-label="'启用短链 ' + link.code"
                    @change="onToggleLinkStatus(link)"
                  />
                  <i></i>
                </label>
              </td>

              <!-- 操作 -->
              <td class="shrink">
                <div class="row" style="gap: 4px; flex-wrap: nowrap">
                  <button
                    type="button"
                    class="btn btn-sm btn-ghost"
                    title="编辑短链"
                    @click="goEdit(link)"
                  >
                    <Pencil :size="12" />
                    编辑
                  </button>
                  <button
                    type="button"
                    class="btn btn-sm btn-ghost text-red-500 hover:text-red-600 hover:bg-red-50 dark:hover:bg-red-950/30"
                    title="逻辑删除（保留记录与历史数据）"
                    @click="handleDeleteLink(link)"
                  >
                    <Trash2 :size="12" />
                    删除
                  </button>
                  <button
                    type="button"
                    class="btn btn-sm btn-ghost text-xs text-red-600 hover:text-red-700 opacity-60 hover:opacity-100"
                    title="彻底清除（物理删除全部数据）"
                    @click="handlePurgeLink(link)"
                  >
                    彻底删除
                  </button>
                </div>
              </td>
            </tr>
          </tbody>
        </table>
      </div>

      <!-- 底栏与真实分页 -->
      <div class="panel-ft row-between flex-wrap gap-3">
        <div class="row tiny muted" style="gap: 12px">
          <span>共 <strong class="text-ink font-mono">{{ total }}</strong> 条短链</span>
          <span>配额使用 <strong class="text-ink font-mono">{{ usage?.links ?? total }}</strong> / {{ usage?.maxLinks ?? '不限' }}</span>
        </div>
        <div class="row" style="gap: 10px">
          <div class="row tiny muted" style="gap: 6px">
            <span>每页</span>
            <select
              v-model.number="pageSize"
              class="select"
              style="min-height: 28px; padding: 2px 20px 2px 8px; font-size: 12px"
              @change="onPageSizeChange"
            >
              <option :value="10">10</option>
              <option :value="20">20</option>
              <option :value="50">50</option>
              <option :value="100">100</option>
            </select>
            <span>条</span>
          </div>
          <div class="row" style="gap: 6px">
            <button
              type="button"
              class="btn btn-sm"
              :disabled="page <= 1 || loading"
              @click="goToPage(page - 1)"
            >
              上一页
            </button>
            <span class="mono tiny muted self-center px-1">
              {{ page }} / {{ totalPages }}
            </span>
            <button
              type="button"
              class="btn btn-sm"
              :disabled="page >= totalPages || loading"
              @click="goToPage(page + 1)"
            >
              下一页
            </button>
          </div>
        </div>
      </div>
    </section>

    <!-- ==================== 模态框: 批量导入短链 ==================== -->
    <div
      v-if="showBatchModal"
      class="fixed inset-0 z-50 flex items-center justify-center bg-black/45 backdrop-blur-xs p-4"
    >
      <div class="panel w-full max-w-lg shadow-2xl">
        <div class="panel-hd">
          <div>
            <h2 class="text-base font-bold">批量导入短链</h2>
            <p>每行一条短链，支持「短码 目标URL」或仅「目标URL」自动生成短码。</p>
          </div>
          <button
            type="button"
            class="icon-btn text-muted hover:text-fg"
            @click="showBatchModal = false"
          >
            <X :size="15" />
          </button>
        </div>
        <div class="panel-bd stack">
          <div class="field">
            <label for="batchDomain">指定承载域名</label>
            <select v-model="batchDomainId" class="select" id="batchDomain">
              <option
                v-for="d in domains"
                :key="d.id"
                :value="d.id"
                :disabled="d.status !== 'active'"
              >
                {{ d.fqdn }} ({{ d.origin === 'self' ? '自有' : '默认' }}){{ d.status !== 'active' ? ' - ' + (DOMAIN_STATUS_NOTE[d.status] || '未激活') : '' }}
              </option>
            </select>
          </div>
          <div class="field">
            <label for="batchInput">短链行列表</label>
            <textarea
              v-model="batchText"
              class="textarea mono"
              id="batchInput"
              rows="6"
              placeholder="deal-a https://example.com/target-a&#10;deal-b https://example.com/target-b&#10;https://example.com/target-c"
            ></textarea>
            <span class="hint">每行一条，短码与 URL 用空格分隔；若只有 URL 则由后端自动生成短码</span>
          </div>
        </div>
        <div class="panel-ft row-between">
          <span class="tiny text-muted">有效行数：{{ parsedBatchLinesCount }}</span>
          <div class="row" style="gap: 8px">
            <button
              type="button"
              class="btn btn-sm"
              :disabled="batchImporting"
              @click="showBatchModal = false"
            >
              取消
            </button>
            <button
              type="button"
              class="btn btn-sm btn-primary"
              :disabled="batchImporting || parsedBatchLinesCount === 0"
              @click="executeBatchImport"
            >
              {{ batchImporting ? '导入中...' : '开始导入' }}
            </button>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, onUnmounted, reactive, ref } from 'vue';
import { useRoute, useRouter } from 'vue-router';
import dayjs from 'dayjs';
import {
  ArrowDown,
  ArrowUp,
  Copy,
  ExternalLink,
  FileDown,
  Pencil,
  Plus,
  RefreshCw,
  Search,
  Trash2,
  Upload,
  X,
} from '@lucide/vue';

import { listDomains } from '@/api/domains';
import {
  createLink,
  deleteLink,
  getLink,
  listLinks,
  purgeLink,
  updateLink,
  uploadLanding,
} from '@/api/links';
import AppEmpty from '@/components/ui/AppEmpty.vue';
import { confirm } from '@/components/ui/confirm';
import { useAuthStore } from '@/stores/auth';
import { ApiError } from '@/types/api';
import type {
  Domain,
  LandingSource,
  Link,
  LinkStatus,
  LinkType,
  RedirectStatus,
} from '@/types/api';
import { formatDateTime } from '@/utils/format';
import { message } from '@/utils/toast';

const route = useRoute();
const router = useRouter();
const auth = useAuthStore();

const DOMAIN_STATUS_NOTE: Record<string, string> = {
  pending: 'DNS 待验证',
  active: '已激活',
  failed: '校验失败',
  stopped: '已停用',
};

// ==================== 响应式基础数据 ====================
const links = ref<Link[]>([]);
const domains = ref<Domain[]>([]);
const loading = ref(false);
const total = ref(0);
const page = ref(1);
const pageSize = ref(20);

const keyword = ref('');
const typeFilter = ref<string>('all');
const statusFilter = ref<string>('all');
const statusUpdatingId = ref<number | null>(null);

// ==================== 计算用量与 KPI ====================
const usage = computed(() => auth.config?.usage);

const quotaPercent = computed(() => {
  const max = usage.value?.maxLinks;
  if (!max || max <= 0) return 0;
  const used = usage.value?.links ?? total.value;
  return Math.min(100, Math.round((used / max) * 100));
});

const remainingQuota = computed(() => {
  const max = usage.value?.maxLinks;
  if (!max) return '不限';
  const rem = max - (usage.value?.links ?? total.value);
  return rem > 0 ? `${rem} 条` : '已耗尽';
});

const activeLinksCount = computed(() => {
  return links.value.filter((l) => l.status === 'enabled').length;
});

const totalVisits = computed(() => {
  return links.value.reduce((acc, l) => acc + (l.visits || 0), 0);
});

const totalClicks = computed(() => {
  return links.value.reduce((acc, l) => acc + (l.linkType === 'landing' ? l.clicks || 0 : 0), 0);
});

const overallCtr = computed(() => {
  const landingVisits = links.value
    .filter((l) => l.linkType === 'landing')
    .reduce((acc, l) => acc + (l.visits || 0), 0);
  if (landingVisits === 0 || totalClicks.value === 0) return '—';
  return `${((totalClicks.value / landingVisits) * 100).toFixed(2)}%`;
});

const totalPages = computed(() => Math.max(1, Math.ceil(total.value / pageSize.value)));

const activeDomains = computed(() => {
  return domains.value.filter((d) => d.status === 'active');
});

const hasActiveFilter = computed(() => {
  return keyword.value.trim() !== '' || typeFilter.value !== 'all' || statusFilter.value !== 'all';
});

const filteredLinks = computed(() => {
  const q = keyword.value.trim().toLowerCase();
  const tf = typeFilter.value;
  const sf = statusFilter.value;

  return links.value.filter((l) => {
    const codeMatch = l.code.toLowerCase().includes(q);
    const domainMatch = l.domains?.some((d) => d.toLowerCase().includes(q)) ?? false;
    const targetMatch = l.targetUrls?.some((u) => u.toLowerCase().includes(q)) ?? false;
    const okKeyword = !q || codeMatch || domainMatch || targetMatch;

    let okType = true;
    if (tf === 'redirect') okType = l.linkType === 'redirect';
    else if (tf === 'landing') okType = l.linkType === 'landing';

    let okStatus = true;
    if (sf === 'enabled') okStatus = l.status === 'enabled';
    else if (sf === 'disabled') okStatus = l.status === 'disabled';

    return okKeyword && okType && okStatus;
  });
});

// ==================== 模态框: 批量导入 ====================
const showBatchModal = ref(false);
const batchText = ref('');
const batchDomainId = ref<number | undefined>(undefined);
const batchImporting = ref(false);

const parsedBatchLinesCount = computed(() => {
  return batchText.value
    .split('\n')
    .map((s) => s.trim())
    .filter(Boolean).length;
});

// ==================== 数据加载 ====================
async function loadData() {
  loading.value = true;
  try {
    const [linksRes, domainsRes] = await Promise.all([
      listLinks({ page: page.value, pageSize: pageSize.value }),
      listDomains(),
    ]);

    links.value = linksRes.items;
    total.value = linksRes.total;
    domains.value = domainsRes;

    if (domainsRes.length > 0 && !batchDomainId.value) {
      const active = domainsRes.find((d) => d.status === 'active');
      batchDomainId.value = active ? active.id : domainsRes[0].id;
    }
  } catch (error) {
    if (error instanceof ApiError) {
      message.error(error.message);
    } else {
      message.error('加载短链数据失败');
    }
    links.value = [];
    total.value = 0;
  } finally {
    loading.value = false;
  }
}

// ==================== 列表与表格操作 ====================
function getPrimaryDomain(link: Link): string {
  if (link.domains && link.domains.length > 0) {
    return link.domains[0];
  }
  return domains.value[0]?.fqdn || '—';
}

function formatTargetDisplay(urls: string[] | undefined): string {
  if (!urls || urls.length === 0) return '—';
  try {
    const u = new URL(urls[0]);
    return `${u.hostname}${u.pathname !== '/' ? u.pathname : ''}`;
  } catch {
    return urls[0];
  }
}

function getLinkCtr(link: Link): string {
  if (link.linkType !== 'landing') return '—';
  if (!link.visits || link.visits === 0) return '—';
  if (!link.clicks || link.clicks === 0) return '0.00%';
  return `${((link.clicks / link.visits) * 100).toFixed(2)}%`;
}

async function copyLinkUrl(link: Link) {
  const fqdn = getPrimaryDomain(link);
  const url = `https://${fqdn}/${link.code}`;
  try {
    await navigator.clipboard.writeText(url);
    message.success('已复制短链: ' + url);
  } catch {
    message.error('复制失败，请手动复制');
  }
}

async function onToggleLinkStatus(link: Link) {
  if (statusUpdatingId.value === link.id) return;
  const nextStatus: LinkStatus = link.status === 'enabled' ? 'disabled' : 'enabled';
  const label = nextStatus === 'disabled' ? '停用' : '启用';

  statusUpdatingId.value = link.id;
  try {
    const updated = await updateLink(link.id, { status: nextStatus });
    link.status = updated.status;
    message.success(`短链「${link.code}」已${label}`);
  } catch (error) {
    if (error instanceof ApiError) {
      message.error(error.message);
    } else {
      message.error(`${label}失败，请稍后重试`);
    }
  } finally {
    statusUpdatingId.value = null;
  }
}

function handleDeleteLink(link: Link) {
  confirm({
    title: `逻辑删除短链「${link.code}」?`,
    content: '删除为逻辑删除：短链记录与历史访问明细保留，但「域名/短码」将不再对外重定向。',
    okText: '确认逻辑删除',
    cancelText: '取消',
    danger: true,
    onOk: async () => {
      try {
        await deleteLink(link.id);
        message.success(`短链「${link.code}」已逻辑删除`);
        await loadData();
        await auth.fetchMe();
      } catch (err) {
        if (err instanceof ApiError) message.error(err.message);
        else message.error('删除短链失败');
      }
    },
  });
}

function handlePurgeLink(link: Link) {
  confirm({
    title: `彻底清除短链「${link.code}」?`,
    content: '警告：彻底清除为物理删除！将永久移除该短链及全部历史访问明细，此操作不可撤销！',
    okText: '彻底物理删除',
    cancelText: '取消',
    danger: true,
    onOk: async () => {
      try {
        await purgeLink(link.id);
        message.success(`短链「${link.code}」已彻底清除`);
        await loadData();
        await auth.fetchMe();
      } catch (err) {
        if (err instanceof ApiError) message.error(err.message);
        else message.error('彻底删除短链失败');
      }
    },
  });
}

function exportCsv() {
  const headers = [
    '短码',
    '承载域名',
    '类型',
    '重定向状态码',
    '目标URL',
    '24h访问',
    '转化点击',
    'CTR',
    '状态',
    '创建时间',
  ];
  const rows = filteredLinks.value.map((l) => [
    l.code,
    (l.domains || []).join('; '),
    l.linkType === 'landing' ? '落地页型' : '跳转型',
    l.redirectStatus || '302',
    (l.targetUrls || []).join('; '),
    l.visits || 0,
    l.linkType === 'landing' ? l.clicks || 0 : 0,
    getLinkCtr(l),
    l.status === 'enabled' ? '启用' : '停用',
    formatDateTime(l.createdAt),
  ]);

  const csvContent = [
    headers.join(','),
    ...rows.map((r) => r.map((c) => `"${String(c).replace(/"/g, '""')}"`).join(',')),
  ].join('\n');

  const blob = new Blob(['\uFEFF' + csvContent], { type: 'text/csv;charset=utf-8;' });
  const url = URL.createObjectURL(blob);
  const a = document.createElement('a');
  a.href = url;
  a.download = `links-export-${dayjs().format('YYYYMMDD-HHmmss')}.csv`;
  a.click();
  URL.revokeObjectURL(a.href);
  message.success(`已导出 ${rows.length} 条短链记录`);
}

function resetFilters() {
  keyword.value = '';
  typeFilter.value = 'all';
  statusFilter.value = 'all';
}

function goToPage(p: number) {
  if (p < 1 || p > totalPages.value || p === page.value) return;
  page.value = p;
  loadData();
}

function onPageSizeChange() {
  page.value = 1;
  loadData();
}

// ==================== 独立页面路由导航 ====================
function goCreate() {
  router.push('/links/new');
}

function goEdit(link: Link) {
  router.push(`/links/${link.id}/edit`);
}

// ==================== 模态框操作: 批量导入 ====================
function openBatchModal() {
  batchText.value = '';
  const firstActive = activeDomains.value[0] || domains.value[0];
  if (firstActive) {
    batchDomainId.value = firstActive.id;
  }
  showBatchModal.value = true;
}

async function executeBatchImport() {
  const lines = batchText.value
    .split('\n')
    .map((l) => l.trim())
    .filter(Boolean);

  if (lines.length === 0) return;
  if (!batchDomainId.value) {
    message.error('请选择承载域名');
    return;
  }

  batchImporting.value = true;
  let successCount = 0;
  let failCount = 0;
  const errors: string[] = [];

  for (const line of lines) {
    const parts = line.split(/[\s,]+/).filter(Boolean);
    let code: string | undefined = undefined;
    let url = '';
    if (parts.length >= 2) {
      code = parts[0];
      url = parts[1];
    } else if (parts.length === 1) {
      url = parts[0];
    }

    if (!url.startsWith('http://') && !url.startsWith('https://')) {
      url = 'https://' + url;
    }

    try {
      new URL(url);
    } catch {
      failCount++;
      errors.push(`URL 非法: ${url}`);
      continue;
    }

    try {
      await createLink({
        code: code || undefined,
        targetUrls: [url],
        domainIds: [batchDomainId.value],
        redirectStatus: '302',
        linkType: 'redirect',
      });
      successCount++;
    } catch (err) {
      failCount++;
      if (err instanceof ApiError) {
        errors.push(`${code || url}: ${err.message}`);
      }
    }
  }

  batchImporting.value = false;
  showBatchModal.value = false;
  batchText.value = '';

  if (successCount > 0) {
    message.success(`成功导入 ${successCount} 条短链${failCount > 0 ? `，失败 ${failCount} 条` : ''}`);
    await loadData();
    await auth.fetchMe();
  } else {
    message.error(`导入失败：全部 ${failCount} 条均未成功 (${errors[0] || '未知错误'})`);
  }
}

// ==================== 键盘事件与路由监听 ====================
function onKeydown(e: KeyboardEvent) {
  if (e.key === 'Escape') {
    if (showBatchModal.value) {
      showBatchModal.value = false;
    }
  }
}

onMounted(async () => {
  window.addEventListener('keydown', onKeydown);
  await loadData();

  if (route.query.new === '1') {
    goCreate();
  } else if (route.query.edit) {
    const editIdOrCode = String(route.query.edit);
    const found = links.value.find(
      (l) => String(l.id) === editIdOrCode || l.code === editIdOrCode,
    );
    if (found) {
      goEdit(found);
    } else {
      const numId = Number(editIdOrCode);
      if (!isNaN(numId) && numId > 0) {
        router.push(`/links/${numId}/edit`);
      }
    }
  }
});

onUnmounted(() => {
  window.removeEventListener('keydown', onKeydown);
});
</script>

<style scoped>
</style>
