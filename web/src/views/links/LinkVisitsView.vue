<template>
  <div class="flex flex-col gap-5 pb-10" data-od-id="link-visits-view">
    <!-- ==================== 页头:短码 / 承载域名 / 类型 + 返回 ==================== -->
    <PageHeader title="短链访问明细" :description="headerDescription">
      <template #actions>
        <span v-if="link" class="badge badge-neutral">{{ linkTypeLabel }}</span>
        <button
          type="button"
          class="btn btn-sm"
          :disabled="loading"
          title="刷新访问明细"
          @click="loadData"
        >
          <RefreshCw :size="13" :class="loading ? 'animate-spin' : ''" />
          刷新
        </button>
        <button type="button" class="btn btn-sm" @click="goBack">
          <ArrowLeft :size="13" />
          返回列表
        </button>
      </template>
    </PageHeader>

    <!-- ==================== 短链加载失败(如已被彻底删除) ==================== -->
    <section v-if="loadError" class="panel" data-od-id="link-visits-error">
      <AppResult
        status="error"
        title="无法查看该短链的访问明细"
        :sub-title="loadError"
      >
        <template #extra>
          <button type="button" class="btn btn-sm btn-primary" @click="goBack">
            <ArrowLeft :size="13" />
            返回短链列表
          </button>
        </template>
      </AppResult>
    </section>

    <!-- ==================== 首次加载 ==================== -->
    <div v-else-if="loading && !loaded" class="panel py-16 flex flex-col items-center justify-center">
      <AppSpin size="large" />
      <p class="tiny muted mt-3">正在拉取该短链的访问明细…</p>
    </div>

    <template v-else>
      <!-- ==================== 摘要条:访问 / 点击 / CTR ==================== -->
      <section
        class="kpi-grid"
        style="grid-template-columns: repeat(3, minmax(0, 1fr))"
        data-od-id="link-visits-summary"
      >
        <div class="kpi">
          <div class="kpi-k">访问次数</div>
          <div class="kpi-v">{{ visitsCount.toLocaleString() }}<span class="text-[14px] text-ink-soft font-normal"> 次</span></div>
          <div class="kpi-sub">跳转 / 落地页视图次数,点击行不计入</div>
        </div>
        <div class="kpi">
          <div class="kpi-k">点击次数</div>
          <div class="kpi-v">{{ clicksCount.toLocaleString() }}<span class="text-[14px] text-ink-soft font-normal"> 次</span></div>
          <div class="kpi-sub">落地页按钮经 SDK 回传的点击次数</div>
        </div>
        <div class="kpi">
          <div class="kpi-k">点击转化率 (CTR)</div>
          <div class="kpi-v">{{ ctr }}</div>
          <div class="kpi-sub">{{ isLanding ? '点击 / 访问,仅落地页型有意义' : '非落地页型短链不统计点击' }}</div>
        </div>
      </section>

      <!-- ==================== 访问明细主面板 ==================== -->
      <section class="panel" data-od-id="link-visits-list">
        <div class="panel-hd">
          <div>
            <h2>访问明细</h2>
            <p>逐条记录该短链每次跳转、落地页视图与按钮点击的动作与结果。</p>
          </div>
          <div class="btn-row">
            <span class="badge badge-neutral mono">共 {{ total }} 条</span>
          </div>
        </div>

        <!-- 工具栏:动作筛选 / 只看失败 / 关键词(后两项仅在当前页数据内过滤) -->
        <div class="panel-bd">
          <div class="toolbar">
            <div class="seg-filter" role="group" aria-label="按动作筛选">
              <button
                v-for="opt in ACTION_OPTIONS"
                :key="opt.value"
                type="button"
                :aria-pressed="actionFilter === opt.value"
                @click="setAction(opt.value)"
              >
                {{ opt.label }}
              </button>
            </div>

            <div class="row" style="gap: 6px">
              <label class="switch" title="仅显示失败的动作(前端过滤,不影响总数)">
                <input v-model="onlyFailed" type="checkbox" aria-label="只看失败" />
                <i></i>
              </label>
              <span class="tiny">只看失败</span>
            </div>

            <div class="relative grow min-w-[220px]">
              <input
                v-model="keyword"
                class="input input-icon"
                id="visitSearch"
                placeholder="搜索 IP、User-Agent、来源或目标 URL…"
                aria-label="搜索访问明细"
              />
              <Search
                :size="14"
                class="absolute left-2.5 top-1/2 -translate-y-1/2 text-muted pointer-events-none"
              />
            </div>

            <button
              v-if="hasLocalFilter"
              type="button"
              class="btn btn-sm btn-ghost text-muted hover:text-fg"
              @click="resetLocalFilters"
            >
              <X :size="13" />
              清空过滤
            </button>

            <!--
              搜索与「只看失败」是页内过滤(动作筛选才走后端),
              这里常驻一行说明,避免用户把页内命中当成全量统计。
            -->
            <span v-if="hasLocalFilter" class="tiny muted" data-od-id="visit-filter-scope-hint">
              搜索与「只看失败」仅筛选当前页,切换分页后请重新确认
            </span>
          </div>
        </div>

        <AppSpin :spinning="loading">
          <div class="tbl-wrap">
            <table class="tbl" id="linkVisitTable">
              <thead>
                <tr>
                  <th class="shrink" title="服务端记录的访问时间">时间</th>
                  <th class="shrink" title="来访 IP:X-Forwarded-For 优先,回退 RemoteAddr">IP</th>
                  <th title="地理数据源待接入(0008 已预留字段):国家 / 数据中心 / 语言">地理位置</th>
                  <th title="由 User-Agent 解析:设备型号 · 操作系统 · 浏览器">设备型号</th>
                  <th class="shrink" title="本次触发的动作与结果;失败的动作不计入访问次数">动作</th>
                  <th title="本次动作最终抵达的地址,缺省时回退显示来源页">目标 / 来源</th>
                </tr>
              </thead>
              <tbody>
                <!-- 加载态 -->
                <tr v-if="loading && rows.length === 0">
                  <td colspan="6" class="empty">
                    <div class="flex items-center justify-center gap-2 py-6">
                      <RefreshCw class="animate-spin" :size="16" />
                      正在加载访问明细...
                    </div>
                  </td>
                </tr>

                <!-- 该短链从未被访问 -->
                <tr v-else-if="rows.length === 0">
                  <td colspan="6" class="py-8">
                    <AppEmpty description="该短链暂无访问记录,短链被访问后明细将在此处逐条呈现" />
                  </td>
                </tr>

                <!-- 当前页经本地过滤后无命中 -->
                <tr v-else-if="filteredRows.length === 0">
                  <td colspan="6" class="py-8">
                    <AppEmpty description="当前页没有符合筛选条件的记录(搜索与「只看失败」仅在当前页数据内过滤)" />
                    <div class="flex justify-center">
                      <button type="button" class="btn btn-sm" @click="resetLocalFilters">
                        重置本地过滤
                      </button>
                    </div>
                  </td>
                </tr>

                <!-- 明细行 -->
                <tr
                  v-for="row in filteredRows"
                  :key="row.visit.id"
                  :data-outcome="row.visit.outcome"
                >
                  <!-- 时间 -->
                  <td class="shrink">
                    <span class="mono tiny">{{ formatDateTime(row.visit.createdAt) }}</span>
                  </td>

                  <!-- 来访 IP -->
                  <td class="shrink">
                    <span class="mono tiny" :title="row.visit.ip || '未知 IP'">
                      {{ row.visit.ip || '—' }}
                    </span>
                  </td>

                  <!-- 地理位置:国家 / 数据中心 / 语言(数据源待接入) -->
                  <td>
                    <div class="stack" style="gap: 2px; min-width: 0">
                      <span class="tiny truncate" :title="row.country || '地理数据源待接入(0008 已预留字段)'">
                        国家:{{ row.country }}
                      </span>
                      <span>
                        <span
                          :class="['badge', row.network.badge]"
                          :title="row.network.title"
                        >
                          {{ row.network.text }}
                        </span>
                      </span>
                      <span class="tiny muted truncate" :title="row.lang || '未携带 Accept-Language'">
                        语言:{{ row.lang }}
                      </span>
                    </div>
                  </td>

                  <!-- 设备型号 -->
                  <td>
                    <div v-if="row.hasUa" class="stack" style="gap: 2px; min-width: 0">
                      <div class="row" style="gap: 5px; flex-wrap: nowrap; min-width: 0">
                        <span :class="['badge', getDeviceBadgeClass(row.parsedUa.deviceType)]">
                          {{ row.parsedUa.deviceType }}
                        </span>
                        <span class="tiny truncate" :title="row.parsedUa.deviceModel">
                          {{ row.parsedUa.deviceModel }}
                        </span>
                      </div>
                      <span
                        class="tiny muted truncate"
                        :title="row.parsedUa.os + ' · ' + row.parsedUa.browser"
                      >
                        {{ row.parsedUa.os }} · {{ row.parsedUa.browser }}
                      </span>
                    </div>
                    <span v-else class="tiny muted">未知设备</span>
                  </td>

                  <!-- 动作与结果 -->
                  <td class="shrink">
                    <div class="stack" style="gap: 3px">
                      <div class="row" style="gap: 4px; flex-wrap: nowrap">
                        <span :class="['badge', row.action.badge]">{{ row.action.text }}</span>
                        <span
                          :class="[
                            'badge',
                            row.visit.outcome === 'failed' ? 'badge-danger' : 'badge-ok',
                          ]"
                        >
                          {{ row.visit.outcome === 'failed' ? '✗ 失败' : '✓ 成功' }}
                        </span>
                      </div>
                      <span v-if="row.reasonText" class="tiny" style="color: var(--danger)">
                        {{ row.reasonText }}
                      </span>
                    </div>
                  </td>

                  <!-- 目标 / 来源 -->
                  <td>
                    <div class="stack" style="gap: 2px; min-width: 0; max-width: 420px">
                      <span
                        v-if="row.visit.targetUrl"
                        class="mono tiny truncate"
                        :title="row.visit.targetUrl"
                      >
                        {{ row.visit.targetUrl }}
                      </span>
                      <span
                        v-else-if="row.visit.referer"
                        class="tiny truncate"
                        :title="row.visit.referer"
                      >
                        {{ row.visit.referer }}
                      </span>
                      <span v-else class="tiny muted">直接访问</span>
                      <span
                        v-if="row.visit.targetUrl && row.visit.referer"
                        class="tiny muted truncate"
                        :title="'来源页:' + row.visit.referer"
                      >
                        来自 {{ row.visit.referer }}
                      </span>
                    </div>
                  </td>
                </tr>
              </tbody>
            </table>
          </div>
        </AppSpin>

        <!-- 底栏与真实分页 -->
        <div class="panel-ft row-between flex-wrap gap-3">
          <div class="row tiny muted" style="gap: 12px">
            <span>共 <strong class="text-ink font-mono">{{ total }}</strong> 条访问明细</span>
            <span v-if="hasLocalFilter">
              当前页命中 <strong class="text-ink font-mono">{{ filteredRows.length }}</strong> / {{ rows.length }} 条
            </span>
          </div>
          <div class="row" style="gap: 10px">
            <div class="row tiny muted" style="gap: 6px">
              <span>每页</span>
              <select
                v-model.number="pageSize"
                class="select"
                style="min-height: 28px; padding: 2px 20px 2px 8px; font-size: 12px"
                aria-label="每页条数"
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
    </template>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue';
import { useRoute, useRouter } from 'vue-router';
import { ArrowLeft, RefreshCw, Search, X } from '@lucide/vue';

import { getLink } from '@/api/links';
import { getLinkStats, listVisits } from '@/api/visits';
import PageHeader from '@/components/PageHeader.vue';
import AppEmpty from '@/components/ui/AppEmpty.vue';
import AppResult from '@/components/ui/AppResult.vue';
import AppSpin from '@/components/ui/AppSpin.vue';
import { ApiError } from '@/types/api';
import type { Link, Visit, VisitAction, VisitReason } from '@/types/api';
import { formatDateTime } from '@/utils/format';
import { message } from '@/utils/toast';
import { getDeviceBadgeClass, parseUserAgent } from '@/utils/userAgent';
import type { ParsedUA } from '@/utils/userAgent';

const route = useRoute();
const router = useRouter();

// ==================== 展示文案映射 ====================
/** 动作 → 徽标文案/配色(后端枚举与前端一一对应,未知值兜底展示原值) */
const ACTION_META: Record<VisitAction, { text: string; badge: string }> = {
  redirect: { text: '跳转', badge: 'badge-neutral' },
  landing_view: { text: '落地页', badge: 'badge-warn' },
  click: { text: '点击', badge: 'badge-ok' },
};

/** 动作筛选项:全部 = 不传 action,由后端返回该短链的全部动作 */
const ACTION_OPTIONS: { value: 'all' | VisitAction; label: string }[] = [
  { value: 'all', label: '全部' },
  { value: 'redirect', label: '跳转' },
  { value: 'landing_view', label: '落地页' },
  { value: 'click', label: '点击' },
];

/** 失败原因 → 中文文案(仅 outcome=failed 时展示;后端新增枚举值时回退展示原值) */
const REASON_TEXT: Record<VisitReason, string> = {
  link_disabled: '短链已停用',
  link_deleted: '短链已删除',
  no_target: '无可用目标',
  landing_missing: '落地页文件缺失',
};

/** 表格行:在访问记录之上补齐解析结果与展示文案 */
interface VisitRow {
  visit: Visit;
  hasUa: boolean;
  parsedUa: ParsedUA;
  country: string;
  lang: string;
  network: { text: string; badge: string; title: string };
  action: { text: string; badge: string };
  reasonText: string;
}

// ==================== 响应式状态 ====================
const link = ref<Link | null>(null);
const stats = ref<{ visits: number; clicks: number } | null>(null);
const visits = ref<Visit[]>([]);
const total = ref(0);
const page = ref(1);
const pageSize = ref(20);

const actionFilter = ref<'all' | VisitAction>('all');
/** 只看失败:后端 action 过滤已够用,失败筛选在前端当前页内完成,保持接口契约不变 */
const onlyFailed = ref(false);
const keyword = ref('');

const loading = ref(false);
/** 是否已成功发起过首次加载(用于区分「首次骨架」与「翻页刷新」) */
const loaded = ref(false);
const loadError = ref<string | null>(null);

const linkId = computed(() => Number(route.params.id));
const isLanding = computed(() => link.value?.linkType === 'landing');

const linkTypeLabel = computed(() => {
  if (!link.value) return '';
  return link.value.linkType === 'landing' ? '落地页型' : `跳转型 · ${link.value.redirectStatus || '302'}`;
});

const headerDescription = computed(() => {
  if (!link.value) return '加载该短链的访问明细:逐条查看来访 IP、地理、设备与动作结果。';
  const domain = link.value.domains?.[0] || '未关联域名';
  return `承载域名 ${domain} · 短码 ${link.value.code} · 逐条查看来访 IP、地理、设备与动作结果。`;
});

// ==================== 数据加载 ====================
/** 拉取短链基本信息与摘要统计;失败(多为 404:短链已被硬删)时给出明确提示而非空白页 */
async function loadLink() {
  if (!Number.isInteger(linkId.value) || linkId.value <= 0) {
    loadError.value = '链接参数不合法:缺少短链 ID。';
    return;
  }
  try {
    link.value = await getLink(linkId.value);
  } catch (error) {
    loadError.value =
      error instanceof ApiError && error.status === 404
        ? '短链不存在或已被彻底删除,其访问明细也一并被移除。'
        : error instanceof ApiError
          ? error.message
          : '加载短链信息失败,请稍后重试。';
    return;
  }

  // 摘要统计:接口异常时回退到短链自带的计数,不影响明细列表展示
  try {
    stats.value = await getLinkStats(linkId.value);
  } catch {
    stats.value = { visits: link.value.visits || 0, clicks: link.value.clicks || 0 };
  }
}

/** 拉取当前页访问明细(action 过滤交给后端) */
async function loadVisits() {
  if (loadError.value) return;
  try {
    const res = await listVisits(linkId.value, {
      page: page.value,
      pageSize: pageSize.value,
      action: actionFilter.value === 'all' ? undefined : actionFilter.value,
    });
    visits.value = res.items;
    total.value = res.total;
  } catch (error) {
    if (error instanceof ApiError) message.error(error.message);
    else message.error('加载访问明细失败,请稍后重试');
    visits.value = [];
    total.value = 0;
  }
}

async function loadData() {
  loading.value = true;
  try {
    await loadLink();
    await loadVisits();
  } finally {
    loading.value = false;
    loaded.value = true;
  }
}

// ==================== 行数据组装 ====================
function toRow(visit: Visit): VisitRow {
  const hasUa = Boolean(visit.userAgent && visit.userAgent.trim());
  const meta = ACTION_META[visit.action] ?? { text: visit.action || '未知动作', badge: 'badge-neutral' };

  return {
    visit,
    hasUa,
    // 解析结果带缓存,重复 UA 不会重复构造解析器
    parsedUa: parseUserAgent(visit.userAgent || ''),
    country: visit.country || '—',
    lang: visit.lang || '—',
    network: visit.isDatacenter
      ? { text: '数据中心', badge: 'badge-warn', title: visit.asn ? `ASN ${visit.asn}` : '数据中心出口' }
      : {
          text: '住宅/未知',
          badge: 'badge-neutral',
          title: visit.asn ? `ASN ${visit.asn}` : '地理数据源待接入(0008 已预留字段)',
        },
    action: meta,
    reasonText:
      visit.outcome === 'failed'
        ? REASON_TEXT[visit.reason as VisitReason] || visit.reason || '未知原因'
        : '',
  };
}

const rows = computed<VisitRow[]>(() => visits.value.map(toRow));

/** 本地过滤:只看失败 + IP / UA / 来源 / 目标 关键词(与访问决策流一致,仅限当前页) */
const filteredRows = computed<VisitRow[]>(() => {
  const q = keyword.value.trim().toLowerCase();
  return rows.value.filter((row) => {
    if (onlyFailed.value && row.visit.outcome !== 'failed') return false;
    if (!q) return true;
    return (
      (row.visit.ip || '').toLowerCase().includes(q) ||
      (row.visit.userAgent || '').toLowerCase().includes(q) ||
      (row.visit.referer || '').toLowerCase().includes(q) ||
      (row.visit.targetUrl || '').toLowerCase().includes(q)
    );
  });
});

const hasLocalFilter = computed(() => onlyFailed.value || keyword.value.trim() !== '');

function resetLocalFilters() {
  keyword.value = '';
  onlyFailed.value = false;
}

// ==================== 摘要指标 ====================
const visitsCount = computed(() => stats.value?.visits ?? link.value?.visits ?? 0);
const clicksCount = computed(() => stats.value?.clicks ?? link.value?.clicks ?? 0);

/** CTR 口径与短链列表一致:非落地页型或访问为 0 时不展示百分比 */
const ctr = computed(() => {
  if (!isLanding.value) return '—';
  if (!visitsCount.value) return '—';
  if (!clicksCount.value) return '0.00%';
  return `${((clicksCount.value / visitsCount.value) * 100).toFixed(2)}%`;
});

// ==================== 分页与筛选交互 ====================
const totalPages = computed(() => Math.max(1, Math.ceil(total.value / pageSize.value)));

/** 切换动作筛选:动作过滤走后端,需回到第一页重取 */
function setAction(value: 'all' | VisitAction) {
  if (actionFilter.value === value) return;
  actionFilter.value = value;
  page.value = 1;
  loadVisits();
}

function goToPage(p: number) {
  if (p < 1 || p > totalPages.value || p === page.value) return;
  page.value = p;
  loadVisits();
}

function onPageSizeChange() {
  page.value = 1;
  loadVisits();
}

function goBack() {
  router.push({ name: 'links' });
}

onMounted(() => {
  loadData();
});

// 同一组件实例内切换短链(如浏览器前进/后退):清空旧数据后重拉
watch(linkId, () => {
  link.value = null;
  stats.value = null;
  visits.value = [];
  total.value = 0;
  page.value = 1;
  loadError.value = null;
  loaded.value = false;
  loadData();
});
</script>

<style scoped>
/* 失败动作整行淡红底:直接给行加语义,方便一眼扫出「没跳出去」的访问 */
.tbl tbody tr[data-outcome='failed'] {
  background: var(--danger-soft);
}

.tbl tbody tr[data-outcome='failed']:hover {
  background: color-mix(in srgb, var(--danger) 18%, var(--surface));
}
</style>
