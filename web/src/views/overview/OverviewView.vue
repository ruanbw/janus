<template>
  <div class="space-y-5 pb-10">
    <!-- KPI 卡片网格 (真实数据驱动) -->
    <section class="grid grid-cols-2 gap-3 sm:grid-cols-3 lg:grid-cols-5">
      <div
        v-for="(kpi, idx) in kpiList"
        :key="kpi.label"
        class="kpi"
        :class="{ 'col-span-2 sm:col-span-1': idx === 4 }"
      >
        <div class="kpi-k">{{ kpi.label }}</div>
        <div class="kpi-v">
          {{ kpi.value }}<span v-if="kpi.unit" class="text-[14px] text-muted font-normal ml-1">{{ kpi.unit }}</span>
        </div>
        <div class="kpi-sub">
          {{ kpi.sub }}
        </div>

        <!-- 说明提示气泡 -->
        <button
          type="button"
          class="kpi-info"
          :aria-label="`指标说明：${kpi.label}`"
        >
          <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.9" stroke-linecap="round">
            <circle cx="12" cy="12" r="9" />
            <path d="M12 11.2v5.1M12 7.7h.01" />
          </svg>
          <span class="kpi-tip" role="tooltip">
            {{ kpi.tip }}
          </span>
        </button>
      </div>
    </section>

    <!-- 流量与转化分析 (真实数据驱动 / 优雅空状态) -->
    <section>
      <!-- 空状态：当访问量为 0 或无短链时 -->
      <div v-if="totalVisits === 0" class="panel">
        <div class="panel-hd">
          <div>
            <h2>流量与转化分析</h2>
            <p>短链访问与落地页点击的实时汇总分析。</p>
          </div>
          <router-link to="/links" class="btn btn-sm">
            管理短链 →
          </router-link>
        </div>
        <div class="panel-bd py-14">
          <div class="mx-auto flex max-w-md flex-col items-center text-center">
            <div class="flex h-12 w-12 items-center justify-center rounded-xl bg-surface-strong text-ink-faint">
              <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round" class="h-6 w-6">
                <path d="M3 3v18h18" />
                <path d="m19 9-5 5-4-4-3 3" />
              </svg>
            </div>
            <h3 class="mt-4 text-[16px] font-semibold text-ink">暂无流量访问数据</h3>
            <p class="mt-1.5 text-[13px] leading-relaxed text-ink-soft">
              暂无流量访问数据，投放或测试访问后将在此自动汇总。
            </p>
            <p class="mt-1 text-[12px] text-ink-faint">
              当前已配置 {{ links.length }} 条短链（{{ activeLinksCount }} 条已启用），{{ domains.length }} 个域名（{{ activeDomainsCount }} 个已激活）。
            </p>
            <div class="mt-6 flex flex-wrap items-center justify-center gap-3">
              <router-link to="/links" class="btn btn-primary btn-sm">
                {{ links.length === 0 ? '创建第一条短链' : '前往短链列表' }}
              </router-link>
              <AppButton size="sm" variant="outline" :loading="loading" @click="loadData">
                <template #icon><RefreshCw :size="13" /></template>
                刷新数据
              </AppButton>
            </div>
          </div>
        </div>
      </div>

      <!-- 真实数据构成：当有访问量时展示 -->
      <div v-else class="cols-2 items-stretch">
        <!-- 热门短链访问排行 -->
        <div class="panel flex flex-col">
          <div class="panel-hd">
            <div>
              <h2>热门短链访问排行</h2>
              <p>按访问量降序排列的短链流量表现。</p>
            </div>
            <router-link to="/links" class="btn btn-sm">
              全部短链 →
            </router-link>
          </div>
          <div class="panel-bd flex-1">
            <div class="bars">
              <div
                v-for="link in topLinks"
                :key="link.id"
                class="bar-row bar-row-lg"
              >
                <div class="bar-lab flex items-center gap-1.5 min-w-0" :title="`/${link.code}`">
                  <span class="mono font-semibold truncate text-[13px]">/{{ link.code }}</span>
                  <span
                    class="text-[10px] px-1.5 py-0.5 rounded shrink-0 font-medium leading-none"
                    :class="link.linkType === 'landing' ? 'bg-cyan-50 text-cyan-700 dark:bg-cyan-950 dark:text-cyan-300' : 'bg-slate-100 text-slate-700 dark:bg-slate-800 dark:text-slate-300'"
                  >
                    {{ link.linkType === 'landing' ? '落地页' : '跳转' }}
                  </span>
                </div>
                <span class="bar-track">
                  <span
                    class="bar-fill"
                    :style="{ width: `${Math.min(100, Math.max(2, Math.round(((link.visits || 0) / totalVisits) * 100)))}%` }"
                  ></span>
                </span>
                <span class="bar-val font-mono text-[12px]">
                  {{ (link.visits || 0).toLocaleString() }} · {{ (((link.visits || 0) / totalVisits) * 100).toFixed(1) }}%
                </span>
              </div>
            </div>
          </div>
          <div class="panel-ft">
            按单条短链累计访问量排序，反映流量在各投放短码上的集中度。
          </div>
        </div>

        <!-- 短链类型与流量结构 -->
        <div class="panel flex flex-col">
          <div class="panel-hd">
            <div>
              <h2>短链类型与流量结构</h2>
              <p>区分直接跳转与落地页承接，合计等于总访问量。</p>
            </div>
            <!-- 原来指向已删的「统计」页。访问明细只有一条路:短链列表点那一行的访问列,
                 所以这里就指到列表页,别再给同一个能力开第二个入口。 -->
            <router-link to="/links" class="btn btn-sm">
              查看各短链的访问 →
            </router-link>
          </div>
          <div class="panel-bd flex-1">
            <div
              class="stackbar"
              role="img"
              :aria-label="`跳转型 ${redirectVisits} 次占 ${redirectPercent}%，落地页型 ${landingVisits} 次占 ${landingPercent}%`"
            >
              <span class="bg-brand-600" :style="{ width: `${redirectPercent}%` }"></span>
              <span class="bg-cyan-500" :style="{ width: `${landingPercent}%` }"></span>
            </div>

            <div class="legend">
              <span class="legend-item">
                <span class="legend-key bg-brand-600"></span>
                跳转型
              </span>
              <span class="legend-item">
                <span class="legend-key bg-cyan-500"></span>
                落地页型
              </span>
            </div>

            <div class="bars mt-4">
              <div class="bar-row bar-row-lg">
                <span class="bar-lab">跳转型访问</span>
                <span class="bar-track">
                  <span class="bar-fill bg-brand-600" :style="{ width: `${redirectPercent}%` }"></span>
                </span>
                <span class="bar-val font-mono text-[12px]">{{ redirectVisits.toLocaleString() }} · {{ redirectPercent }}%</span>
              </div>
              <div class="bar-row bar-row-lg">
                <span class="bar-lab">落地页访问</span>
                <span class="bar-track">
                  <span class="bar-fill bg-cyan-500" :style="{ width: `${landingPercent}%` }"></span>
                </span>
                <span class="bar-val font-mono text-[12px]">{{ landingVisits.toLocaleString() }} · {{ landingPercent }}%</span>
              </div>
              <div class="bar-row bar-row-lg">
                <span class="bar-lab">落地页点击</span>
                <span class="bar-track">
                  <span class="bar-fill bg-emerald-500" :style="{ width: `${landingVisits > 0 ? Math.min(100, Math.round((totalClicks / landingVisits) * 100)) : 0}%` }"></span>
                </span>
                <span class="bar-val font-mono text-[12px]">{{ totalClicks.toLocaleString() }} · CTR {{ landingCTR }}</span>
              </div>
            </div>
          </div>
          <div class="panel-ft">
            落地页点击经平台 JS SDK 回传，整体转化率（CTR）反映落地页对目标 URL 的转化效率。
          </div>
        </div>
      </div>
    </section>

    <!-- 流量结构分布：来源 / 设备 / 操作系统 / 浏览器（四块都来自同一份访问明细样本） -->
    <template v-if="visits.length > 0">
      <div class="cols-2 items-stretch">
        <div class="panel flex flex-col">
          <div class="panel-hd">
            <div>
              <h2>流量来源分布</h2>
              <p>根据请求 Referrer 与广告点击特征自动归类。</p>
            </div>
          </div>
          <div class="panel-bd flex-1">
            <div class="bars">
              <div
                v-for="src in sourceBreakdown"
                :key="src.name"
                class="bar-row bar-row-lg"
              >
                <span class="bar-lab">{{ src.name }}</span>
                <span class="bar-track">
                  <span
                    class="bar-fill"
                    :class="{ 't-accent': src.percent > 30 }"
                    :style="{ width: `${src.percent}%` }"
                  ></span>
                </span>
                <span class="bar-val">{{ src.count }} 次 · {{ src.percent }}%</span>
              </div>
            </div>
          </div>
          <div class="panel-ft">
            优先解析主流广告渠道（Meta / TikTok / Google），未携带来源的流量归入直接访问。
          </div>
        </div>

        <div class="panel flex flex-col">
          <div class="panel-hd">
            <div>
              <h2>设备类型分布</h2>
              <p>基于 User-Agent 特征与视口画像解析。</p>
            </div>
          </div>
          <div class="panel-bd flex-1">
            <div class="bars">
              <div
                v-for="dev in deviceBreakdown"
                :key="dev.name"
                class="bar-row bar-row-lg"
              >
                <span class="bar-lab">{{ dev.name }}</span>
                <span class="bar-track">
                  <span
                    class="bar-fill"
                    :class="{ 't-accent': dev.name === '移动端' }"
                    :style="{ width: `${dev.percent}%` }"
                  ></span>
                </span>
                <span class="bar-val">{{ dev.count }} 次 · {{ dev.percent }}%</span>
              </div>
            </div>
          </div>
          <div class="panel-ft">
            斗篷准入规则可针对「移动端」进行放行，拦截「桌面端」审查机或爬虫环境。
          </div>
        </div>
      </div>

      <div class="cols-2 items-stretch">
        <div class="panel flex flex-col">
          <div class="panel-hd">
            <div>
              <h2>操作系统分布</h2>
              <p>终端操作系统内核与主版本统计。</p>
            </div>
          </div>
          <div class="panel-bd flex-1">
            <div class="bars">
              <div
                v-for="os in osBreakdown"
                :key="os.name"
                class="bar-row bar-row-lg"
              >
                <span class="bar-lab">{{ os.name }}</span>
                <span class="bar-track">
                  <span
                    class="bar-fill"
                    :class="{ 't-accent': os.name.includes('iOS') }"
                    :style="{ width: `${os.percent}%` }"
                  ></span>
                </span>
                <span class="bar-val">{{ os.count }} 次 · {{ os.percent }}%</span>
              </div>
            </div>
          </div>
          <div class="panel-ft">
            支持在规则引擎中配置 OS 白名单（如仅放行 iOS 或 Android）。
          </div>
        </div>

        <div class="panel flex flex-col">
          <div class="panel-hd">
            <div>
              <h2>浏览器分布</h2>
              <p>独立浏览器与应用内嵌 WebView 占比。</p>
            </div>
          </div>
          <div class="panel-bd flex-1">
            <div class="bars">
              <div
                v-for="br in browserBreakdown"
                :key="br.name"
                class="bar-row bar-row-lg"
              >
                <span class="bar-lab">{{ br.name }}</span>
                <span class="bar-track">
                  <span class="bar-fill" :style="{ width: `${br.percent}%` }"></span>
                </span>
                <span class="bar-val">{{ br.count }} 次 · {{ br.percent }}%</span>
              </div>
            </div>
          </div>
          <div class="panel-ft">
            「应用内内置」指微信、抖音等 App 的 WebView，与独立浏览器分列以便看清各投放渠道的真实环境。
          </div>
        </div>
      </div>

      <!-- 样本量必须写出来：这四张图算的是样本占比，不是全量访问结构 -->
      <p class="text-[12px] text-muted">
        样本量：{{ sampleLinks.length }} 条有访问量的短链（按访问量降序取前 {{ SAMPLE_LINK_LIMIT }} 条）各
        {{ SAMPLE_PAGE_SIZE }} 条访问明细，共 <span class="font-mono text-ink">{{ visits.length }}</span> 条。上方占比按该样本计算，不等于全量访问结构。
      </p>
    </template>

    <!-- 合规提示 -->
    <section class="panel">
      <div class="panel-bd">
        <div class="note">
          <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round">
            <circle cx="12" cy="12" r="9" />
            <path d="M12 8h.01M11 12h1v4h1" />
          </svg>
          <div>
            <b>合规提示。</b>斗篷的本质是「对不同访问者返回不同内容」。用于绕过平台审核或对审核员定向展示白标内容，可能违反 TikTok / Meta / Google 的广告政策并导致账户封禁。本系统按「流量准入控制 + 品牌合规」的正向用途设计：拦截爬虫与无效流量、地域与语言适配、转化归因。落地生产前请确认业务场景合规性。
          </div>
        </div>
      </div>
    </section>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue';
import { RefreshCw } from '@lucide/vue';

import { listDomains } from '@/api/domains';
import { listLinks } from '@/api/links';
import { listVisits } from '@/api/visits';
import AppButton from '@/components/ui/AppButton.vue';
import { useAuthStore } from '@/stores/auth';
import { ApiError } from '@/types/api';
import type { Domain, Link, Visit } from '@/types/api';
import { message } from '@/utils/toast';

import {
  browserDistribution,
  deviceDistribution,
  osDistribution,
  sourceDistribution,
} from './trafficBreakdown';

/**
 * 分布图的样本上限：按访问量降序取前 10 条短链，每条取最近 50 条明细。
 *
 * 全量访问明细没有聚合接口，只能靠逐条短链拉取；这两个上限是为了让总览页的加载
 * 时间可接受，取值沿用原数据洞察页。
 */
const SAMPLE_LINK_LIMIT = 10;
const SAMPLE_PAGE_SIZE = 50;

const auth = useAuthStore();

const loading = ref(false);
const links = ref<Link[]>([]);
const domains = ref<Domain[]>([]);
/** 分布图的访问明细样本（不是全量，见 SAMPLE_LINK_LIMIT 注释） */
const visits = ref<Visit[]>([]);
const sampleLinks = ref<Link[]>([]);

const sourceBreakdown = computed(() => sourceDistribution(visits.value));
const deviceBreakdown = computed(() => deviceDistribution(visits.value));
const osBreakdown = computed(() => osDistribution(visits.value));
const browserBreakdown = computed(() => browserDistribution(visits.value));

const quotaUsage = computed(() => auth.tenant?.usage);

// 活跃短链数：真实已启用的短链数量
const activeLinksCount = computed(() => {
  return links.value.filter((link) => link.status === 'enabled').length;
});

// 承载域名数：真实激活的域名数量
const activeDomainsCount = computed(() => {
  return domains.value.filter((domain) => domain.status === 'active').length;
});

// 总访问数：累加所有短链的真实访问量（visits）
const totalVisits = computed(() => {
  return links.value.reduce((sum, link) => sum + (link.visits || 0), 0);
});

// 落地页短链数量
const landingLinksCount = computed(() => {
  return links.value.filter((link) => link.linkType === 'landing').length;
});

// 落地页点击数：累加落地页型短链的真实点击量（clicks）
const totalClicks = computed(() => {
  return links.value
    .filter((link) => link.linkType === 'landing')
    .reduce((sum, link) => sum + (link.clicks || 0), 0);
});

// 整体转化率（CTR）：点击数 / 访问数（当访问数为 0 时显示 0%）
const ctr = computed(() => {
  if (totalVisits.value === 0) return 0;
  return (totalClicks.value / totalVisits.value) * 100;
});

const ctrText = computed(() => {
  if (totalVisits.value === 0) return '0%';
  return `${ctr.value.toFixed(1)}%`;
});

// 跳转型访问统计
const redirectVisits = computed(() => {
  return links.value
    .filter((link) => link.linkType === 'redirect')
    .reduce((sum, link) => sum + (link.visits || 0), 0);
});

// 落地页型访问统计
const landingVisits = computed(() => {
  return links.value
    .filter((link) => link.linkType === 'landing')
    .reduce((sum, link) => sum + (link.visits || 0), 0);
});

const redirectPercent = computed(() => {
  if (totalVisits.value === 0) return 0;
  return Math.round((redirectVisits.value / totalVisits.value) * 1000) / 10;
});

const landingPercent = computed(() => {
  if (totalVisits.value === 0) return 0;
  return Math.round((landingVisits.value / totalVisits.value) * 1000) / 10;
});

const landingCTR = computed(() => {
  if (landingVisits.value === 0) return '0%';
  return `${((totalClicks.value / landingVisits.value) * 100).toFixed(1)}%`;
});

// 热门短链排行 (TOP 5)
const topLinks = computed(() => {
  return [...links.value]
    .sort((a, b) => (b.visits || 0) - (a.visits || 0))
    .slice(0, 5);
});

interface KPIItem {
  label: string;
  value: string;
  unit?: string;
  sub: string;
  tip: string;
}

const kpiList = computed<KPIItem[]>(() => [
  {
    label: '活跃短链',
    value: activeLinksCount.value.toLocaleString(),
    unit: '条',
    sub: quotaUsage.value?.maxLinks
      ? `已启用 · 配额 ${quotaUsage.value.links}/${quotaUsage.value.maxLinks}`
      : `已启用 · 共 ${links.value.length} 条短链`,
    tip: '状态为「启用」的短链数量。已停用的短链不计入，不参与重定向与流量承接。',
  },
  {
    label: '承载域名',
    value: activeDomainsCount.value.toLocaleString(),
    unit: '个',
    sub: quotaUsage.value?.maxDomains
      ? `已激活 · 配额 ${quotaUsage.value.domains}/${quotaUsage.value.maxDomains}`
      : `已激活 · 共 ${domains.value.length} 个域名`,
    tip: 'DNS 解析已指向本服务器且状态为「已激活」的域名数量，可正常签发证书并承载短链跳转。',
  },
  {
    label: '总访问数',
    value: totalVisits.value.toLocaleString(),
    unit: '次',
    sub: totalVisits.value > 0 ? '所有短链累计访问总量' : '暂无访问记录',
    tip: '所有短链收到的访问请求累计总数。跳转型短链重定向即记录一次，落地页型短链落地页展现即记录一次。',
  },
  {
    label: '落地页点击数',
    value: totalClicks.value.toLocaleString(),
    unit: '次',
    sub: landingLinksCount.value > 0
      ? `来自 ${landingLinksCount.value} 条落地页型短链`
      : '暂无落地页短链',
    tip: '落地页上按钮经 SDK 触发回传的累计有效点击次数。仅落地页型短链拥有点击统计。',
  },
  {
    label: '整体转化率 (CTR)',
    value: ctrText.value,
    sub: totalVisits.value > 0
      ? `点击 ${totalClicks.value.toLocaleString()} / 访问 ${totalVisits.value.toLocaleString()}`
      : '访问量为 0 时显示 0%',
    tip: '整体点击转化率（CTR）= 落地页点击数 / 总访问数。当总访问数为 0 时显示 0%。',
  },
]);

async function loadData() {
  loading.value = true;
  try {
    const [linksRes, domainsRes] = await Promise.all([
      listLinks(1, 100),
      listDomains(),
      auth.fetchMe(),
    ]);
    links.value = linksRes.items;
    domains.value = domainsRes;

    // 分布图样本：只拉有访问量的短链，按访问量降序取前 N 条
    sampleLinks.value = [...linksRes.items]
      .filter((l) => (l.visits || 0) > 0)
      .sort((a, b) => (b.visits || 0) - (a.visits || 0))
      .slice(0, SAMPLE_LINK_LIMIT);
    const visitResults = await Promise.all(
      sampleLinks.value.map((l) =>
        listVisits(l.id, { page: 1, pageSize: SAMPLE_PAGE_SIZE }).catch(() => ({ items: [], total: 0 })),
      ),
    );
    visits.value = visitResults.flatMap((r) => r?.items ?? []);
  } catch (error) {
    if (error instanceof ApiError && error.status !== 401) {
      message.error(error.message);
    }
  } finally {
    loading.value = false;
  }
}

onMounted(() => {
  loadData();
});
</script>
