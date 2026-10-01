<template>
  <div class="space-y-5 pb-10" data-od-id="overview-view">
    <!-- KPI 卡片网格 (真实数据驱动：3列自适应、高度统一) -->
    <section class="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-3 auto-rows-fr">
      <AppCard
        v-for="kpi in kpiList"
        :key="kpi.label"
        class="relative flex flex-col justify-between min-h-[118px] hover:z-20 focus-within:z-20 p-4"
      >
        <div>
          <div class="font-mono text-2xs uppercase tracking-wider text-ink-faint pr-7">
            {{ kpi.label }}
          </div>
          <div class="mt-1 flex items-baseline font-mono text-2xl font-semibold tracking-tight text-ink tabular-nums leading-none">
            <span>{{ kpi.value }}</span>
            <span v-if="kpi.unit" class="ml-1 text-sm font-normal text-ink-soft">{{ kpi.unit }}</span>
          </div>
        </div>
        <div class="mt-2 text-xs text-ink-soft">
          {{ kpi.sub }}
        </div>

        <!-- 说明提示气泡 -->
        <AppTooltip :title="kpi.tip">
          <AppButton
            variant="ghost"
            size="icon"
            class="absolute top-2.5 right-2.5 h-6 w-6 rounded-full text-ink-faint hover:text-ink"
            :aria-label="`指标说明：${kpi.label}`"
          >
            <CircleHelp :size="15" />
          </AppButton>
        </AppTooltip>
      </AppCard>
    </section>

    <!-- 世界地图：与其他分布图同源，但单独占满一整行 -->
    <WorldMapPanel v-if="visits.length > 0" :visits="visits" />

    <!--
      分析与分布卡片：宽屏一行三列 / 中屏两列 / 手机一列，用 Tailwind 断点工具类表达。
      卡片内部的条形行则按卡片自身宽度换挡，依赖 @container 容器查询。
    -->
    <section class="grid grid-cols-1 items-stretch gap-4 sm:grid-cols-2 xl:grid-cols-3">
      <!-- 暂无流量数据卡片 -->
      <AppCard v-if="totalVisits === 0" :padding="false" class="sm:col-span-2 xl:col-span-3">
        <div class="flex flex-wrap items-center justify-between gap-3 border-b border-line px-4 py-3">
          <div>
            <h2 class="text-base font-semibold tracking-tight text-ink">流量与转化分析</h2>
            <p class="mt-0.5 text-xs text-ink-soft">短链访问与落地页点击的实时汇总分析。</p>
          </div>
          <AppButton to="/links" size="sm" variant="outline">
            管理短链 →
          </AppButton>
        </div>
        <div class="py-14 px-4">
          <div class="mx-auto flex max-w-md flex-col items-center text-center">
            <div class="flex h-12 w-12 items-center justify-center rounded-xl bg-surface-strong text-ink-faint">
              <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round" class="h-6 w-6">
                <path d="M3 3v18h18" />
                <path d="m19 9-5 5-4-4-3 3" />
              </svg>
            </div>
            <h3 class="mt-4 text-base font-semibold text-ink">暂无流量访问数据</h3>
            <p class="mt-1.5 text-xs leading-relaxed text-ink-soft">
              暂无流量访问数据，投放或测试访问后将在此自动汇总。
            </p>
            <p class="mt-1 text-xs text-ink-faint">
              当前已配置 {{ links.length }} 条短链（{{ activeLinksCount }} 条已启用），{{ domains.length }} 个域名（{{ activeDomainsCount }} 个已激活）。
            </p>
            <div class="mt-6 flex flex-wrap items-center justify-center gap-3">
              <AppButton to="/links" size="sm" variant="default">
                {{ links.length === 0 ? '创建第一条短链' : '前往短链列表' }}
              </AppButton>
              <AppButton size="sm" variant="outline" :loading="loading" @click="loadData">
                <template #icon><RefreshCw :size="13" /></template>
                刷新数据
              </AppButton>
            </div>
          </div>
        </div>
      </AppCard>

      <!-- 热门短链访问排行 -->
      <AppCard v-if="totalVisits > 0" :padding="false" class="@container flex flex-col">
        <div class="flex flex-wrap items-center justify-between gap-3 border-b border-line px-4 py-3">
          <div>
            <h2 class="text-base font-semibold tracking-tight text-ink">热门短链访问排行</h2>
            <p class="mt-0.5 text-xs text-ink-soft">按访问量降序排列的短链流量表现。</p>
          </div>
          <AppButton to="/links" size="sm" variant="outline">
            全部短链 →
          </AppButton>
        </div>
        <div class="flex-1 p-4">
          <div class="flex flex-col gap-2.5">
            <div
              v-for="link in topLinks"
              :key="link.id"
              class="grid grid-cols-[124px_minmax(0,1fr)_116px] items-center gap-3 @max-[470px]:grid-cols-[minmax(72px,1fr)_minmax(48px,1.3fr)_max-content] @max-[340px]:grid-cols-2 @max-[340px]:gap-y-1.5"
            >
              <div class="flex items-center gap-1.5 min-w-0 @max-[340px]:col-span-1" :title="`/${link.code}`">
                <span class="truncate font-mono text-xs font-semibold text-ink">/{{ link.code }}</span>
                <AppTag :color="link.linkType === 'landing' ? 'info' : 'default'">
                  {{ link.linkType === 'landing' ? '落地页' : '跳转' }}
                </AppTag>
              </div>
              <div class="h-3.5 w-full overflow-hidden rounded bg-surface-strong @max-[340px]:col-span-2 @max-[340px]:row-start-2">
                <div
                  class="h-full min-w-[2px] rounded bg-brand-600 transition-all duration-300 dark:bg-brand-500"
                  :style="{ width: `${Math.min(100, Math.max(2, Math.round(((link.visits || 0) / totalVisits) * 100)))}%` }"
                />
              </div>
              <span class="whitespace-nowrap font-mono text-right text-xs text-ink-soft @max-[340px]:col-start-2 @max-[340px]:row-start-1">
                {{ (link.visits || 0).toLocaleString() }} · {{ (((link.visits || 0) / totalVisits) * 100).toFixed(1) }}%
              </span>
            </div>
          </div>
        </div>
        <div class="border-t border-line px-4 py-2.5 text-xs text-ink-faint">
          按单条短链累计访问量排序，反映流量在各投放短码上的集中度。
        </div>
      </AppCard>

      <!-- 短链类型与流量结构 -->
      <AppCard v-if="totalVisits > 0" :padding="false" class="@container flex flex-col">
        <div class="flex flex-wrap items-center justify-between gap-3 border-b border-line px-4 py-3">
          <div>
            <h2 class="text-base font-semibold tracking-tight text-ink">短链类型与流量结构</h2>
            <p class="mt-0.5 text-xs text-ink-soft">区分直接跳转与落地页承接，合计等于总访问量。</p>
          </div>
          <AppButton to="/links" size="sm" variant="outline">
            查看各短链的访问 →
          </AppButton>
        </div>
        <div class="flex-1 p-4">
          <div
            class="flex h-6.5 w-full overflow-hidden rounded-md border border-line"
            role="img"
            :aria-label="`跳转型 ${redirectVisits} 次占 ${redirectPercent}%，落地页型 ${landingVisits} 次占 ${landingPercent}%`"
          >
            <span class="h-full bg-brand-600 transition-all duration-300 dark:bg-brand-500" :style="{ width: `${redirectPercent}%` }"></span>
            <span class="h-full bg-info transition-all duration-300" :style="{ width: `${landingPercent}%` }"></span>
          </div>

          <div class="mt-3 flex flex-wrap items-center gap-3.5 text-xs text-ink-soft">
            <span class="flex items-center gap-1.5">
              <span class="h-2.5 w-2.5 shrink-0 rounded-xs bg-brand-600 dark:bg-brand-500"></span>
              跳转型
            </span>
            <span class="flex items-center gap-1.5">
              <span class="h-2.5 w-2.5 shrink-0 rounded-xs bg-info"></span>
              落地页型
            </span>
          </div>

          <div class="mt-4 flex flex-col gap-2.5">
            <div class="grid grid-cols-[124px_minmax(0,1fr)_116px] items-center gap-3 @max-[470px]:grid-cols-[minmax(72px,1fr)_minmax(48px,1.3fr)_max-content] @max-[340px]:grid-cols-2 @max-[340px]:gap-y-1.5">
              <span class="min-w-0 truncate text-xs text-ink @max-[340px]:col-span-1">跳转型访问</span>
              <div class="h-3.5 w-full overflow-hidden rounded bg-surface-strong @max-[340px]:col-span-2 @max-[340px]:row-start-2">
                <div class="h-full min-w-[2px] rounded bg-brand-600 transition-all duration-300 dark:bg-brand-500" :style="{ width: `${redirectPercent}%` }" />
              </div>
              <span class="whitespace-nowrap font-mono text-right text-xs text-ink-soft @max-[340px]:col-start-2 @max-[340px]:row-start-1">{{ redirectVisits.toLocaleString() }} · {{ redirectPercent }}%</span>
            </div>
            <div class="grid grid-cols-[124px_minmax(0,1fr)_116px] items-center gap-3 @max-[470px]:grid-cols-[minmax(72px,1fr)_minmax(48px,1.3fr)_max-content] @max-[340px]:grid-cols-2 @max-[340px]:gap-y-1.5">
              <span class="min-w-0 truncate text-xs text-ink @max-[340px]:col-span-1">落地页访问</span>
              <div class="h-3.5 w-full overflow-hidden rounded bg-surface-strong @max-[340px]:col-span-2 @max-[340px]:row-start-2">
                <div class="h-full min-w-[2px] rounded bg-info transition-all duration-300" :style="{ width: `${landingPercent}%` }" />
              </div>
              <span class="whitespace-nowrap font-mono text-right text-xs text-ink-soft @max-[340px]:col-start-2 @max-[340px]:row-start-1">{{ landingVisits.toLocaleString() }} · {{ landingPercent }}%</span>
            </div>
            <div class="grid grid-cols-[124px_minmax(0,1fr)_116px] items-center gap-3 @max-[470px]:grid-cols-[minmax(72px,1fr)_minmax(48px,1.3fr)_max-content] @max-[340px]:grid-cols-2 @max-[340px]:gap-y-1.5">
              <span class="min-w-0 truncate text-xs text-ink @max-[340px]:col-span-1">落地页点击</span>
              <div class="h-3.5 w-full overflow-hidden rounded bg-surface-strong @max-[340px]:col-span-2 @max-[340px]:row-start-2">
                <div class="h-full min-w-[2px] rounded bg-ok transition-all duration-300" :style="{ width: `${landingVisits > 0 ? Math.min(100, Math.round((totalClicks / landingVisits) * 100)) : 0}%` }" />
              </div>
              <span class="whitespace-nowrap font-mono text-right text-xs text-ink-soft @max-[340px]:col-start-2 @max-[340px]:row-start-1">{{ totalClicks.toLocaleString() }} · CTR {{ landingCTR }}</span>
            </div>
          </div>
        </div>
        <div class="border-t border-line px-4 py-2.5 text-xs text-ink-faint">
          落地页点击经平台 JS SDK 回传，整体转化率（CTR）反映落地页对目标 URL 的转化效率。
        </div>
      </AppCard>

      <!-- 流量来源分布 -->
      <AppCard v-if="visits.length > 0" :padding="false" class="@container flex flex-col">
        <div class="flex flex-wrap items-center justify-between gap-3 border-b border-line px-4 py-3">
          <div>
            <h2 class="text-base font-semibold tracking-tight text-ink">流量来源分布</h2>
            <p class="mt-0.5 text-xs text-ink-soft">根据请求 Referrer 与广告点击特征自动归类。</p>
          </div>
        </div>
        <div class="flex-1 p-4">
          <div class="flex flex-col gap-2.5">
            <div
              v-for="src in sourceBreakdown"
              :key="src.name"
              class="grid grid-cols-[124px_minmax(0,1fr)_116px] items-center gap-3 @max-[470px]:grid-cols-[minmax(72px,1fr)_minmax(48px,1.3fr)_max-content] @max-[340px]:grid-cols-2 @max-[340px]:gap-y-1.5"
            >
              <span class="min-w-0 truncate text-xs text-ink @max-[340px]:col-span-1">{{ src.name }}</span>
              <div class="h-3.5 w-full overflow-hidden rounded bg-surface-strong @max-[340px]:col-span-2 @max-[340px]:row-start-2">
                <div
                  class="h-full min-w-[2px] rounded transition-all duration-300"
                  :class="src.percent > 30 ? 'bg-brand-600 dark:bg-brand-500' : 'bg-ink/70 dark:bg-ink-soft'"
                  :style="{ width: `${src.percent}%` }"
                />
              </div>
              <span class="whitespace-nowrap font-mono text-right text-xs text-ink-soft @max-[340px]:col-start-2 @max-[340px]:row-start-1">{{ src.count }} 次 · {{ src.percent }}%</span>
            </div>
          </div>
        </div>
        <div class="border-t border-line px-4 py-2.5 text-xs text-ink-faint">
          优先解析主流广告渠道（Meta / TikTok / Google），未携带来源的流量归入直接访问。
        </div>
      </AppCard>

      <!-- 设备类型分布 -->
      <AppCard v-if="visits.length > 0" :padding="false" class="@container flex flex-col">
        <div class="flex flex-wrap items-center justify-between gap-3 border-b border-line px-4 py-3">
          <div>
            <h2 class="text-base font-semibold tracking-tight text-ink">设备类型分布</h2>
            <p class="mt-0.5 text-xs text-ink-soft">基于 User-Agent 特征与视口画像解析。</p>
          </div>
        </div>
        <div class="flex-1 p-4">
          <div class="flex flex-col gap-2.5">
            <div
              v-for="dev in deviceBreakdown"
              :key="dev.name"
              class="grid grid-cols-[124px_minmax(0,1fr)_116px] items-center gap-3 @max-[470px]:grid-cols-[minmax(72px,1fr)_minmax(48px,1.3fr)_max-content] @max-[340px]:grid-cols-2 @max-[340px]:gap-y-1.5"
            >
              <span class="min-w-0 truncate text-xs text-ink @max-[340px]:col-span-1">{{ dev.name }}</span>
              <div class="h-3.5 w-full overflow-hidden rounded bg-surface-strong @max-[340px]:col-span-2 @max-[340px]:row-start-2">
                <div
                  class="h-full min-w-[2px] rounded transition-all duration-300"
                  :class="dev.name === '移动端' ? 'bg-brand-600 dark:bg-brand-500' : 'bg-ink/70 dark:bg-ink-soft'"
                  :style="{ width: `${dev.percent}%` }"
                />
              </div>
              <span class="whitespace-nowrap font-mono text-right text-xs text-ink-soft @max-[340px]:col-start-2 @max-[340px]:row-start-1">{{ dev.count }} 次 · {{ dev.percent }}%</span>
            </div>
          </div>
        </div>
        <div class="border-t border-line px-4 py-2.5 text-xs text-ink-faint">
          斗篷准入规则可针对「移动端」进行放行，拦截「桌面端」审查机或爬虫环境。
        </div>
      </AppCard>

      <!-- 操作系统分布 -->
      <AppCard v-if="visits.length > 0" :padding="false" class="@container flex flex-col">
        <div class="flex flex-wrap items-center justify-between gap-3 border-b border-line px-4 py-3">
          <div>
            <h2 class="text-base font-semibold tracking-tight text-ink">操作系统分布</h2>
            <p class="mt-0.5 text-xs text-ink-soft">终端操作系统内核与主版本统计。</p>
          </div>
        </div>
        <div class="flex-1 p-4">
          <div class="flex flex-col gap-2.5">
            <div
              v-for="os in osBreakdown"
              :key="os.name"
              class="grid grid-cols-[124px_minmax(0,1fr)_116px] items-center gap-3 @max-[470px]:grid-cols-[minmax(72px,1fr)_minmax(48px,1.3fr)_max-content] @max-[340px]:grid-cols-2 @max-[340px]:gap-y-1.5"
            >
              <span class="min-w-0 truncate text-xs text-ink @max-[340px]:col-span-1">{{ os.name }}</span>
              <div class="h-3.5 w-full overflow-hidden rounded bg-surface-strong @max-[340px]:col-span-2 @max-[340px]:row-start-2">
                <div
                  class="h-full min-w-[2px] rounded transition-all duration-300"
                  :class="os.name.includes('iOS') ? 'bg-brand-600 dark:bg-brand-500' : 'bg-ink/70 dark:bg-ink-soft'"
                  :style="{ width: `${os.percent}%` }"
                />
              </div>
              <span class="whitespace-nowrap font-mono text-right text-xs text-ink-soft @max-[340px]:col-start-2 @max-[340px]:row-start-1">{{ os.count }} 次 · {{ os.percent }}%</span>
            </div>
          </div>
        </div>
        <div class="border-t border-line px-4 py-2.5 text-xs text-ink-faint">
          支持在规则引擎中配置 OS 白名单（如仅放行 iOS 或 Android）。
        </div>
      </AppCard>

      <!-- 浏览器分布 -->
      <AppCard v-if="visits.length > 0" :padding="false" class="@container flex flex-col">
        <div class="flex flex-wrap items-center justify-between gap-3 border-b border-line px-4 py-3">
          <div>
            <h2 class="text-base font-semibold tracking-tight text-ink">浏览器分布</h2>
            <p class="mt-0.5 text-xs text-ink-soft">独立浏览器与应用内嵌 WebView 占比。</p>
          </div>
        </div>
        <div class="flex-1 p-4">
          <div class="flex flex-col gap-2.5">
            <div
              v-for="br in browserBreakdown"
              :key="br.name"
              class="grid grid-cols-[124px_minmax(0,1fr)_116px] items-center gap-3 @max-[470px]:grid-cols-[minmax(72px,1fr)_minmax(48px,1.3fr)_max-content] @max-[340px]:grid-cols-2 @max-[340px]:gap-y-1.5"
            >
              <span class="min-w-0 truncate text-xs text-ink @max-[340px]:col-span-1">{{ br.name }}</span>
              <div class="h-3.5 w-full overflow-hidden rounded bg-surface-strong @max-[340px]:col-span-2 @max-[340px]:row-start-2">
                <div
                  class="h-full min-w-[2px] rounded bg-ink/70 transition-all duration-300 dark:bg-ink-soft"
                  :style="{ width: `${br.percent}%` }"
                />
              </div>
              <span class="whitespace-nowrap font-mono text-right text-xs text-ink-soft @max-[340px]:col-start-2 @max-[340px]:row-start-1">{{ br.count }} 次 · {{ br.percent }}%</span>
            </div>
          </div>
        </div>
        <div class="border-t border-line px-4 py-2.5 text-xs text-ink-faint">
          「应用内内置」指微信、抖音等 App 的 WebView，与独立浏览器分列以便看清各投放渠道的真实环境。
        </div>
      </AppCard>
    </section>

    <!-- 样本量提示 -->
    <p v-if="visits.length > 0" class="text-xs text-ink-faint">
      样本量：{{ sampleLinks.length }} 条有访问量的短链（按访问量降序取前 {{ SAMPLE_LINK_LIMIT }} 条）各
      {{ SAMPLE_PAGE_SIZE }} 条访问明细，共 <span class="font-mono text-ink">{{ visits.length }}</span> 条。地图与各分布图的占比按该样本计算，不等于全量访问结构。
    </p>

    <!-- 合规提示 -->
    <AppAlert type="info" show-icon title="合规提示">
      斗篷的本质是「对不同访问者返回不同内容」。用于绕过平台审核或对审核员定向展示白标内容，可能违反 TikTok / Meta / Google 的广告政策并导致账户封禁。本系统按「流量准入控制 + 品牌合规」的正向用途设计：拦截爬虫与无效流量、地域与语言适配、转化归因。落地生产前请确认业务场景合规性。
    </AppAlert>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue';
import { CircleHelp, RefreshCw } from '@lucide/vue';

import { listDomains } from '@/api/domains';
import { listLinks } from '@/api/links';
import { listVisits } from '@/api/visits';
import AppAlert from '@/components/ui/AppAlert.vue';
import AppButton from '@/components/ui/AppButton.vue';
import AppCard from '@/components/ui/AppCard.vue';
import AppTag from '@/components/ui/AppTag.vue';
import AppTooltip from '@/components/ui/AppTooltip.vue';
import { ApiError } from '@/types/api';
import type { Domain, Link, Visit } from '@/types/api';
import { message } from '@/utils/toast';

import {
  browserDistribution,
  deviceDistribution,
  osDistribution,
  sourceDistribution,
} from './trafficBreakdown';
import WorldMapPanel from './WorldMapPanel.vue';

/**
 * 分布图的样本上限：按访问量降序取前 10 条短链，每条取最近 50 条明细。
 *
 * 全量访问明细没有聚合接口，只能靠逐条短链拉取；这两个上限是为了让总览页的加载
 * 时间可接受，取值沿用原数据洞察页。
 */
const SAMPLE_LINK_LIMIT = 10;
const SAMPLE_PAGE_SIZE = 50;

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
    sub: `已启用 · 共 ${links.value.length} 条短链`,
    tip: '状态为「启用」的短链数量。已停用的短链不计入，不参与重定向与流量承接。',
  },
  {
    label: '承载域名',
    value: activeDomainsCount.value.toLocaleString(),
    unit: '个',
    sub: `已激活 · 共 ${domains.value.length} 个域名`,
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
    ]);
    links.value = linksRes?.items ?? [];
    domains.value = Array.isArray(domainsRes) ? domainsRes : [];

    // 分布图样本：只拉有访问量的短链，按访问量降序取前 N 条
    const items = linksRes?.items ?? [];
    sampleLinks.value = [...items]
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
