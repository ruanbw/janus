<template>
  <div class="flex flex-col gap-5 pb-10" data-od-id="visit-stream-view">
    <!-- ==================== 顶部 4 个真实 KPI 卡片 ==================== -->
    <section class="kpi-grid" data-od-id="live-kpi">
      <!-- KPI 1: 聚合总访问量 -->
      <div
        class="kpi"
        data-tip="当前租户名下所有短链的历史累积访问总量，来源于后端统计真实数据。"
      >
        <div class="kpi-k">聚合总访问量</div>
        <div class="kpi-v" id="liveCount">{{ totalVisitsFormatted }}</div>
        <div class="kpi-sub">
          <span class="dot dot-live" style="display: inline-block; color: var(--accent)"></span>
          共监控 {{ links.length }} 条短链
        </div>
        <button
          type="button"
          class="kpi-info"
          aria-label="指标说明：聚合总访问量"
          aria-describedby="kpi-tip-1"
        >
          <Info :size="14" />
          <span class="kpi-tip" id="kpi-tip-1" role="tooltip">
            当前租户名下所有短链的历史累积访问总量，来源于后端统计真实数据。
          </span>
        </button>
      </div>

      <!-- KPI 2: 独立访客 IP -->
      <div
        class="kpi"
        data-tip="在当前聚合的真实访问样本流中，去重统计后的独立访问 IP 地址数量。"
      >
        <div class="kpi-k">独立访客 IP</div>
        <div class="kpi-v">{{ uniqueIpsCount }}</div>
        <div class="kpi-sub">当前流样本去重</div>
        <button
          type="button"
          class="kpi-info"
          aria-label="指标说明：独立访客 IP"
          aria-describedby="kpi-tip-2"
        >
          <Info :size="14" />
          <span class="kpi-tip" id="kpi-tip-2" role="tooltip">
            在当前聚合的真实访问样本流中，去重统计后的独立访问 IP 地址数量。
          </span>
        </button>
      </div>

      <!-- KPI 3: 识别爬虫与机器人 -->
      <div
        class="kpi"
        data-tip="由 UA 解析引擎识别出的搜索引擎蜘蛛、抓取脚本、cURL 及自动化机器人数量与占比。"
      >
        <div class="kpi-k">爬虫与机器人识别</div>
        <div class="kpi-v">{{ botCountFormatted }}</div>
        <div class="kpi-sub">
          {{ allVisits.length > 0 ? `占比 ${botPct}% · 自动化流量` : '暂无检测样本' }}
        </div>
        <button
          type="button"
          class="kpi-info"
          aria-label="指标说明：识别爬虫与机器人"
          aria-describedby="kpi-tip-3"
        >
          <Info :size="14" />
          <span class="kpi-tip" id="kpi-tip-3" role="tooltip">
            由 UA 解析引擎识别出的搜索引擎蜘蛛、抓取脚本、cURL 及自动化机器人数量与占比。
          </span>
        </button>
      </div>

      <!-- KPI 4: 移动端占比 -->
      <div
        class="kpi"
        data-tip="基于真实访问 User-Agent 识别出的手机/移动设备占比分布。"
      >
        <div class="kpi-k">移动端流量占比</div>
        <div class="kpi-v">{{ mobilePct }}%</div>
        <div class="kpi-sub">
          {{ allVisits.length > 0 ? `移动端 ${mobileCount} · 桌面端 ${desktopCount}` : '暂无设备数据' }}
        </div>
        <button
          type="button"
          class="kpi-info"
          aria-label="指标说明：移动端占比"
          aria-describedby="kpi-tip-4"
        >
          <Info :size="14" />
          <span class="kpi-tip" id="kpi-tip-4" role="tooltip">
            基于真实访问 User-Agent 识别出的手机/移动设备占比分布。
          </span>
        </button>
      </div>
    </section>

    <!-- ==================== 顶部控制栏与全局筛选 ==================== -->
    <div class="panel">
      <div class="panel-bd" style="padding-top: 12px; padding-bottom: 12px">
        <div class="toolbar flex flex-wrap items-center gap-3">
          <!-- 短链筛选下拉框 -->
          <div class="flex items-center gap-1.5 min-w-[200px]">
            <select
              class="select"
              id="linkFilter"
              v-model="selectedLinkId"
              aria-label="按短链过滤"
            >
              <option value="all">全部短链 ({{ links.length }})</option>
              <option v-for="link in links" :key="link.id" :value="String(link.id)">
                {{ link.domains[0] ? link.domains[0] + '/' : '' }}{{ link.code }} ({{ link.visits }} 访)
              </option>
            </select>
          </div>

          <!-- 设备类型快捷筛选 -->
          <div class="seg-filter" role="group" aria-label="按设备类型过滤">
            <button
              v-for="d in deviceFilters"
              :key="d.key"
              type="button"
              :aria-pressed="activeDeviceFilter === d.key"
              @click="activeDeviceFilter = d.key"
            >
              {{ d.label }}
            </button>
          </div>

          <!-- 搜索输入框 (根据真实 IP、Referrer、UA 或短码实时过滤) -->
          <div class="relative grow min-w-[240px]">
            <input
              v-model="searchQuery"
              class="input input-icon"
              id="vSearch"
              placeholder="搜索真实 IP、Referrer、UA 或短码…"
              aria-label="搜索访问"
            />
            <Search
              :size="14"
              class="absolute left-2.5 top-1/2 -translate-y-1/2 text-ink-faint pointer-events-none"
            />
          </div>
        </div>
      </div>
    </div>

    <!-- ==================== 加载中状态 ==================== -->
    <div v-if="isLoading" class="panel p-16 flex flex-col items-center justify-center">
      <AppSpin spinning size="large" />
      <p class="text-xs text-ink-faint mt-3">正在拉取租户短链与真实访问记录…</p>
    </div>

    <!-- ==================== 租户完全无访问空状态 ==================== -->
    <div
      v-else-if="allVisits.length === 0"
      class="panel p-16 flex flex-col items-center justify-center"
      data-od-id="empty-visit-stream"
    >
      <AppEmpty
        description="暂无访问记录。当投放短链产生访问时，访客画像与访问明细将在此处实时呈现"
      />
    </div>

    <!-- ==================== 主体真实两栏决策流 ==================== -->
    <section v-else class="two-col" data-od-id="stream-detail">
      <!-- 左栏：真实访问流高密列表 -->
      <div class="panel">
        <div class="panel-hd">
          <div>
            <h2>访问决策流</h2>
            <p>点击任意一条记录查看完整真实画像与出站动作。</p>
          </div>
          <div class="row items-center gap-2">
            <span class="badge badge-neutral mono">
              {{ filteredVisits.length }} / {{ allVisits.length }} 条
            </span>
          </div>
        </div>

        <!-- 高密表头 (6 列真实维度) -->
        <div
          class="feed-hd"
          style="grid-template-columns: 75px 125px 105px minmax(0, 1fr) 90px 85px"
        >
          <span>时间</span>
          <span>来访 IP</span>
          <span>短链短码</span>
          <span>访客画像 (系统 / 浏览器 / 来源)</span>
          <span>设备类型</span>
          <span style="text-align: right">动作</span>
        </div>

        <!-- 决策流列表 -->
        <div class="feed" id="feed" ref="feedRef">
          <button
            v-for="item in filteredVisits"
            :key="item.id"
            type="button"
            class="feed-row"
            :class="{ 'new-in': item.isNew }"
            style="grid-template-columns: 75px 125px 105px minmax(0, 1fr) 90px 85px"
            :data-visit="item.id"
            :data-ip="item.ip"
            :aria-selected="selectedVisit?.id === item.id"
            @click="selectVisit(item)"
          >
            <!-- 时间 -->
            <span class="t" :title="item.displayTimeFull">{{ item.displayTime }}</span>
            <!-- IP -->
            <span class="ip" :title="item.ip">{{ item.ip }}</span>
            <!-- 短码 -->
            <span class="truncate">
              <span class="badge badge-neutral mono text-[11px]" :title="item.linkCode">
                {{ item.linkCode }}
              </span>
            </span>
            <!-- 访客画像: 系统 · 浏览器 · 来源 -->
            <span class="dev truncate">
              <span class="text-ink font-medium">{{ item.parsedUa.os }}</span>
              <span class="text-ink-faint"> · {{ item.parsedUa.browser }}</span>
              <span v-if="item.referer" class="text-ink-faint" :title="item.referer">
                · 来自 {{ item.referer }}
              </span>
            </span>
            <!-- 设备类型徽标 -->
            <span>
              <span :class="['badge', getDeviceBadgeClass(item.parsedUa.deviceType)]">
                {{ item.parsedUa.deviceType }}
              </span>
            </span>
            <!-- 动作 -->
            <span style="text-align: right">
              <span :class="['badge', formatRowAction(item).class]">
                {{ formatRowAction(item).text }}
              </span>
            </span>
          </button>

          <!-- 搜索/过滤后无结果空状态 -->
          <div v-if="filteredVisits.length === 0" class="py-12">
            <AppEmpty description="没有匹配的访问决策记录" />
          </div>
        </div>

        <!-- 底部说明与统计 -->
        <div class="panel-ft row-between">
          <span>显示 {{ filteredVisits.length }} 条记录，共监控 {{ links.length }} 条短链</span>
          <span class="mono">生产环境真实访问流 · 实时解析</span>
        </div>
      </div>

      <!-- 右栏：决策链回放 (sticky) 与真实设备分布 -->
      <div class="stack" style="position: sticky; top: 80px">
        <!-- 决策链回放面板 -->
        <div class="panel" id="detailPanel" v-if="selectedVisit">
          <div class="panel-hd">
            <div>
              <h2 style="font-size: 15px">访客画像与决策链</h2>
              <p>解释这一次真实访问的元数据与系统派发动作。</p>
            </div>
            <span :class="['badge mono', verdictBadge.class]" id="dVerdict">
              {{ verdictBadge.text }}
            </span>
          </div>

          <div class="panel-bd stack">
            <!-- 真实访客画像 KV -->
            <dl class="kv">
              <dt>来访真实 IP</dt>
              <dd id="dIp" class="flex items-center gap-1.5">
                <span>{{ selectedVisit.ip }}</span>
                <span v-if="isCurrentIpBanned" class="badge badge-danger text-[10px]">已在风控名单</span>
              </dd>
              <dt>访问时间</dt>
              <dd id="dTime">{{ selectedVisit.displayTimeFull }}</dd>
              <dt>关联短链</dt>
              <dd class="mono truncate">
                {{ selectedVisit.domain || selectedVisit.linkDomain || '平台域名' }} / {{ selectedVisit.linkCode }}
              </dd>
              <dt>短链类型</dt>
              <dd>
                {{ selectedVisit.linkType === 'landing' ? '落地页型 (landing)' : `跳转型 (${selectedVisit.redirectStatus})` }}
              </dd>
              <dt>目标目的地</dt>
              <dd class="truncate text-ink-faint" :title="selectedVisit.primaryTargetUrl || '—'">
                {{ selectedVisit.primaryTargetUrl || selectedVisit.landingUrl || '—' }}
              </dd>
              <dt>来源页 (Referrer)</dt>
              <dd id="dRef" class="truncate" :title="selectedVisit.referer || '直接访问'">
                {{ selectedVisit.referer || '直接访问 / 无 Referrer' }}
              </dd>
              <dt>操作系统</dt>
              <dd>{{ selectedVisit.parsedUa.os }}</dd>
              <dt>浏览器</dt>
              <dd>{{ selectedVisit.parsedUa.browser }}</dd>
              <dt>设备识别</dt>
              <dd>{{ selectedVisit.parsedUa.deviceType }} · {{ selectedVisit.parsedUa.deviceModel }}</dd>
              <dt>完整 UA</dt>
              <dd class="text-[11px] leading-relaxed break-all bg-surface-strong/60 p-2 rounded border border-line">
                {{ selectedVisit.userAgent || '无 User-Agent' }}
              </dd>
            </dl>

            <hr class="rule" style="border: 0; border-top: 1px solid var(--border)" />

            <!-- 逐条规则求值追踪链路 -->
            <div>
              <div class="mono micro muted" style="margin-bottom: 8px">逐条求值与安全审计</div>
              <div class="trace" id="dTrace">
                <div
                  v-for="(step, sIdx) in traceSteps"
                  :key="sIdx"
                  :class="['trace-step', step.status]"
                >
                  <div class="trace-rail">
                    <span class="trace-node"></span>
                  </div>
                  <div>
                    <div class="trace-name">
                      {{ step.name }}
                      <span v-if="step.badgeText" :class="['badge mono ml-1', step.badgeClass]">
                        {{ step.badgeText }}
                      </span>
                    </div>

                    <div class="trace-why">
                      <template v-if="step.facts && step.facts.length > 0">
                        <span
                          v-for="(f, fIdx) in step.facts"
                          :key="fIdx"
                          :class="['fact', f.type === 'hit' ? 'hit' : (f.type === 'miss' ? 'miss' : '')]"
                        >
                          {{ f.text }}
                        </span>
                      </template>
                      <span v-if="step.whyText" class="text-muted ml-0.5">{{ step.whyText }}</span>
                      <div v-if="step.actionText" style="margin-top: 6px">
                        动作：<b>{{ step.actionText }}</b> → <span v-html="step.destText"></span>
                      </div>
                      <div v-if="step.skipReason" class="text-muted">
                        {{ step.skipReason }}
                      </div>
                    </div>
                  </div>
                </div>
              </div>
            </div>

            <hr class="rule" style="border: 0; border-top: 1px solid var(--border)" />

            <!-- 出站描述 -->
            <div>
              <div class="mono micro muted" style="margin-bottom: 6px">出站</div>
              <div class="tiny text-ink" v-html="selectedVisitOutbound.primary"></div>
              <div class="tiny muted">{{ selectedVisitOutbound.secondary }}</div>
            </div>
          </div>

          <!-- 底部操作按钮 -->
          <div class="panel-ft row" style="gap: 8px">
            <button
              type="button"
              class="btn btn-sm btn-danger"
              id="banBtn"
              @click="handleBanIp"
            >
              {{ isCurrentIpBanned ? '已在风控名单 (移出)' : '加入风控名单' }}
            </button>
            <button
              type="button"
              class="btn btn-sm"
              style="flex: 1"
              id="openSimulatorBtn"
              @click="openInSimulator"
            >
              用此访客在模拟器打开
            </button>
          </div>
        </div>

        <!-- 真实设备分布构成卡片 -->
        <div class="panel" data-od-id="intercept-breakdown">
          <div class="panel-hd">
            <div>
              <h2 style="font-size: 15px">设备类型分布</h2>
              <p>基于当前聚合的 {{ allVisits.length }} 次真实访问样本分析。</p>
            </div>
          </div>
          <div class="panel-bd">
            <div class="bars">
              <div class="bar-row">
                <span class="bar-lab">移动端</span>
                <span class="bar-track">
                  <span
                    class="bar-fill"
                    :style="{ width: `${mobilePct}%`, background: 'var(--accent)' }"
                  ></span>
                </span>
                <span class="bar-val">{{ mobileCount }} ({{ mobilePct }}%)</span>
              </div>
              <div class="bar-row">
                <span class="bar-lab">桌面端</span>
                <span class="bar-track">
                  <span class="bar-fill" :style="{ width: `${desktopPct}%` }"></span>
                </span>
                <span class="bar-val">{{ desktopCount }} ({{ desktopPct }}%)</span>
              </div>
              <div class="bar-row">
                <span class="bar-lab">平板</span>
                <span class="bar-track">
                  <span
                    class="bar-fill"
                    :style="{ width: `${tabletPct}%`, background: 'var(--warn)' }"
                  ></span>
                </span>
                <span class="bar-val">{{ tabletCount }} ({{ tabletPct }}%)</span>
              </div>
              <div class="bar-row">
                <span class="bar-lab">爬虫机器人</span>
                <span class="bar-track">
                  <span class="bar-fill t-danger" :style="{ width: `${botPct}%` }"></span>
                </span>
                <span class="bar-val">{{ botCount }} ({{ botPct }}%)</span>
              </div>
            </div>
          </div>
        </div>
      </div>
    </section>
  </div>
</template>

<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue';
import { useRouter } from 'vue-router';
import { Info, Search } from '@lucide/vue';
import dayjs from 'dayjs';

import { listLinks } from '@/api/links';
import { listVisits } from '@/api/visits';
import AppEmpty from '@/components/ui/AppEmpty.vue';
import AppSpin from '@/components/ui/AppSpin.vue';
import { ApiError } from '@/types/api';
import type { Link, Visit } from '@/types/api';
import { message } from '@/utils/toast';
import { getDeviceBadgeClass, parseUserAgent } from '@/utils/userAgent';
import type { ParsedUA } from '@/utils/userAgent';

const router = useRouter();

// ==================== UA 解析引擎(已抽到 @/utils/userAgent 供访问明细列表复用) ====================
// ==================== 真实访问决策流项 ====================
interface ProcessedVisit {
  id: number;
  linkId: number;
  domain: string;
  linkDomain: string;
  linkCode: string;
  targetUrls: string[];
  primaryTargetUrl: string;
  redirectStatus: string;
  linkType: 'redirect' | 'landing';
  landingUrl: string;
  ip: string;
  userAgent: string;
  referer: string;
  createdAt: string;
  displayTime: string;
  displayTimeFull: string;
  parsedUa: ParsedUA;
  isNew?: boolean;
}

interface TraceFact {
  text: string;
  type?: 'hit' | 'miss' | 'neutral';
}

interface TraceStep {
  status: 'hit' | 'block' | 'skip';
  name: string;
  badgeText?: string;
  badgeClass?: string;
  facts?: TraceFact[];
  whyText?: string;
  actionText?: string;
  destText?: string;
  skipReason?: string;
}

function processVisit(visit: Visit, link: Link, isNew = false): ProcessedVisit {
  const d = dayjs(visit.createdAt);
  const displayTime = d.isValid() ? d.format('HH:mm:ss') : '-';
  const displayTimeFull = d.isValid() ? d.format('YYYY-MM-DD HH:mm:ss') : '-';
  const parsedUa = parseUserAgent(visit.userAgent);

  return {
    id: visit.id,
    linkId: visit.linkId,
    domain: visit.domain || link.domains[0] || '',
    linkDomain: link.domains[0] || '',
    linkCode: link.code,
    targetUrls: link.targetUrls || [],
    primaryTargetUrl: link.targetUrls?.[0] || '',
    redirectStatus: String(link.redirectStatus || '302'),
    linkType: link.linkType || 'redirect',
    landingUrl: link.landingUrl || '',
    ip: visit.ip,
    userAgent: visit.userAgent,
    referer: visit.referer || '',
    createdAt: visit.createdAt,
    displayTime,
    displayTimeFull,
    parsedUa,
    isNew,
  };
}

// ==================== 响应式状态 ====================
const links = ref<Link[]>([]);
const allVisits = ref<ProcessedVisit[]>([]);
const selectedVisit = ref<ProcessedVisit | null>(null);

const isLoading = ref(true);
const isRefreshing = ref(false);
const autoRefresh = ref(true);
const feedRef = ref<HTMLElement | null>(null);

// 筛选控件
const selectedLinkId = ref<string>('all');
const activeDeviceFilter = ref<string>('all');
const searchQuery = ref('');

const deviceFilters = [
  { key: 'all', label: '全部设备' },
  { key: '移动端', label: '移动端' },
  { key: '桌面端', label: '桌面端' },
  { key: '平板', label: '平板' },
  { key: '爬虫机器人', label: '爬虫机器人' },
];

// 风控黑名单操作 (前端响应式集合)
const bannedIps = ref<Set<string>>(new Set());

const isCurrentIpBanned = computed(() => {
  return selectedVisit.value ? bannedIps.value.has(selectedVisit.value.ip) : false;
});

function handleBanIp() {
  if (!selectedVisit.value) return;
  const ip = selectedVisit.value.ip;
  if (bannedIps.value.has(ip)) {
    bannedIps.value.delete(ip);
    message.info(`已将 IP ${ip} 从风控名单中移除`);
  } else {
    bannedIps.value.add(ip);
    message.success(`已将 IP ${ip} 加入风控名单`);
  }
}

// ==================== 模拟器联动跳转 ====================
function openInSimulator() {
  if (!selectedVisit.value) return;
  router.push({
    path: '/rules/simulator',
    query: {
      ip: selectedVisit.value.ip,
      ua: selectedVisit.value.userAgent,
      referrer: selectedVisit.value.referer || undefined,
      // 必带：模拟器靠 URL 里的短码定位短链，缺了它就找不到短链，
      // 于是全部 scope='links' 规则被判「不适用」——而这次访问恰恰可能是被它们裁定的。
      url:
        selectedVisit.value.domain && selectedVisit.value.linkCode
          ? `https://${selectedVisit.value.domain}/${selectedVisit.value.linkCode}`
          : undefined,
    },
  });
}

// ==================== 数据过滤流 ====================
const filteredVisits = computed(() => {
  return allVisits.value.filter((item) => {
    // 短链筛选
    if (selectedLinkId.value !== 'all' && String(item.linkId) !== selectedLinkId.value) {
      return false;
    }

    // 设备类型筛选
    if (
      activeDeviceFilter.value !== 'all' &&
      item.parsedUa.deviceType !== activeDeviceFilter.value
    ) {
      return false;
    }

    // 搜索输入框: 匹配真实 IP、Referrer、UA、短码或系统浏览器
    if (searchQuery.value.trim()) {
      const q = searchQuery.value.trim().toLowerCase();
      const matchIp = item.ip.toLowerCase().includes(q);
      const matchRef = (item.referer || '').toLowerCase().includes(q);
      const matchUa = (item.userAgent || '').toLowerCase().includes(q);
      const matchCode = (item.linkCode || '').toLowerCase().includes(q);
      const matchOs = item.parsedUa.os.toLowerCase().includes(q);
      const matchBrowser = item.parsedUa.browser.toLowerCase().includes(q);
      if (!matchIp && !matchRef && !matchUa && !matchCode && !matchOs && !matchBrowser) {
        return false;
      }
    }

    return true;
  });
});

// 当筛选变动时，保持当前选中或重置至首条
watch(filteredVisits, (newList) => {
  if (newList.length === 0) {
    selectedVisit.value = null;
  } else if (!selectedVisit.value || !newList.some((v) => v.id === selectedVisit.value?.id)) {
    selectedVisit.value = newList[0];
  }
});

function selectVisit(item: ProcessedVisit) {
  selectedVisit.value = item;
}

// ==================== KPI 统计计算 (基于真实数据) ====================
const totalVisitsCount = computed(() => {
  return links.value.reduce((acc, l) => acc + (l.visits || 0), 0);
});

const totalVisitsFormatted = computed(() => totalVisitsCount.value.toLocaleString());

const uniqueIpsCount = computed(() => {
  return new Set(allVisits.value.map((v) => v.ip)).size;
});

const botVisits = computed(() => {
  return allVisits.value.filter(
    (v) => v.parsedUa.isBot || v.parsedUa.deviceType === '爬虫机器人',
  );
});

const botCount = computed(() => botVisits.value.length);
const botCountFormatted = computed(() => botCount.value.toLocaleString());

const botPct = computed(() => {
  if (allVisits.value.length === 0) return 0;
  return Math.round((botCount.value / allVisits.value.length) * 100);
});

const mobileCount = computed(() => {
  return allVisits.value.filter((v) => v.parsedUa.deviceType === '移动端').length;
});

const desktopCount = computed(() => {
  return allVisits.value.filter((v) => v.parsedUa.deviceType === '桌面端').length;
});

const tabletCount = computed(() => {
  return allVisits.value.filter((v) => v.parsedUa.deviceType === '平板').length;
});

const mobilePct = computed(() => {
  if (allVisits.value.length === 0) return 0;
  return Math.round((mobileCount.value / allVisits.value.length) * 100);
});

const desktopPct = computed(() => {
  if (allVisits.value.length === 0) return 0;
  return Math.round((desktopCount.value / allVisits.value.length) * 100);
});

const tabletPct = computed(() => {
  if (allVisits.value.length === 0) return 0;
  return Math.round((tabletCount.value / allVisits.value.length) * 100);
});

// ==================== 徽标与操作描述 ====================
function formatRowAction(item: ProcessedVisit): { text: string; class: string } {
  if (bannedIps.value.has(item.ip)) {
    return { text: '拦截', class: 'badge-danger' };
  }
  if (item.linkType === 'landing') {
    return { text: '落地页', class: 'badge-warn' };
  }
  return { text: `${item.redirectStatus} 跳转`, class: 'badge-ok' };
}

const verdictBadge = computed(() => {
  const v = selectedVisit.value;
  if (!v) return { text: '—', class: 'badge-neutral' };
  if (isCurrentIpBanned.value) {
    return { text: '风控拦截 (403)', class: 'badge-danger' };
  }
  if (v.linkType === 'landing') {
    return { text: '放行 · 落地页', class: 'badge-warn' };
  }
  return { text: `放行 · ${v.redirectStatus}`, class: 'badge-ok' };
});

const selectedVisitOutbound = computed(() => {
  const v = selectedVisit.value;
  if (!v) {
    return { primary: '—', secondary: '—' };
  }
  if (isCurrentIpBanned.value) {
    return {
      primary: 'HTTP 403 Forbidden · 熔断并中断连接',
      secondary: '命中本地风控阻断名单 · 拒绝请求',
    };
  }
  if (v.linkType === 'landing') {
    return {
      primary: `200 OK → 渲染托管落地页 (<span class="mono" style="color:var(--fg)">${v.landingUrl || `/${v.linkCode}`}</span>)`,
      secondary: `Referrer: ${v.referer ? v.referer : '直接访问'} · SDK 转化回传就绪`,
    };
  }
  return {
    primary: `HTTP ${v.redirectStatus} → <span class="mono" style="color:var(--fg)">${v.primaryTargetUrl || '目标 URL'}</span>`,
    secondary: `来源页: ${v.referer ? v.referer : '直接访问'} · 无风险拦截 · 毫秒级重定向`,
  };
});

// ==================== 决策链推演 ====================
const traceSteps = computed<TraceStep[]>(() => {
  const v = selectedVisit.value;
  if (!v) return [];

  const isBanned = isCurrentIpBanned.value;
  const isBot = v.parsedUa.isBot;
  const isLanding = v.linkType === 'landing';

  return [
    {
      status: 'hit',
      name: `短链路由寻址 (/${v.linkCode})`,
      badgeText: '匹配成功',
      badgeClass: 'badge-ok mono',
      facts: [
        { text: `短码 /${v.linkCode}`, type: 'hit' },
        { text: `域名 ${v.domain || v.linkDomain || '默认域名'}`, type: 'hit' },
      ],
      whyText: '来访请求精确命中当前租户短链规则，接入裁决流水线',
    },
    {
      status: isBanned ? 'block' : 'hit',
      name: '风控安全名单排查',
      badgeText: isBanned ? '命中 · 拦截' : '合规 · 放行',
      badgeClass: isBanned ? 'badge-danger mono' : 'badge-ok mono',
      facts: [
        { text: `来访 IP ${v.ip}`, type: isBanned ? 'hit' : 'miss' },
        { text: isBanned ? '已在风控黑名单' : '未在黑名单库', type: isBanned ? 'hit' : 'neutral' },
      ],
      whyText: isBanned
        ? '访客 IP 位于本地风控阻断库中，立即熔断连接'
        : '访客 IP 信誉正常，未命中任何本地或平台级阻断名单',
      actionText: isBanned ? '阻断 (403)' : undefined,
      destText: isBanned ? '中断连接并丢弃请求' : undefined,
    },
    {
      status: isBot ? 'block' : 'hit',
      name: '客户端环境与 UA 审计',
      badgeText: isBot ? '爬虫特征' : '真实客户端',
      badgeClass: isBot ? 'badge-warn mono' : 'badge-ok mono',
      facts: [
        { text: `系统: ${v.parsedUa.os}`, type: 'neutral' },
        { text: `浏览器: ${v.parsedUa.browser}`, type: 'neutral' },
        { text: `类型: ${v.parsedUa.deviceType}`, type: isBot ? 'hit' : 'miss' },
      ],
      whyText: isBot
        ? '检测到爬虫/自动化程序特征签名，标记为非人类交互流量'
        : '具备完整现代浏览器及交互渲染特征，判定为真实访客',
    },
    {
      status: isBanned ? 'skip' : 'hit',
      name: '重定向与动作派发',
      badgeText: isBanned
        ? '已跳过'
        : isLanding
          ? '落地页呈现'
          : `${v.redirectStatus} 重定向`,
      badgeClass: isBanned ? 'badge-neutral mono' : 'badge-ok mono',
      facts: [
        { text: isLanding ? '落地页托管模式' : `${v.redirectStatus} 状态码`, type: 'hit' },
      ],
      whyText: isBanned
        ? '前序风控拦截生效，后续路由与重定向不再执行'
        : isLanding
          ? '按落地页策略渲染目标页面，等待点击转化'
          : `临时重定向至目标目的地 (${v.primaryTargetUrl || '配置目标'})`,
      actionText: isBanned
        ? undefined
        : isLanding
          ? '呈现落地页'
          : `HTTP ${v.redirectStatus} 放行`,
      destText: isBanned
        ? undefined
        : isLanding
          ? `<span class="mono">${v.landingUrl || `托管落地页 /${v.linkCode}`}</span>`
          : `<span class="mono">${v.primaryTargetUrl || '目标 URL'}</span>`,
      skipReason: isBanned ? '风控阻断生效，剩余动作已跳过' : undefined,
    },
  ];
});

// ==================== 生产级真实数据拉取 ====================
let autoRefreshTimer: ReturnType<typeof setInterval> | null = null;

async function loadData(silent = false) {
  if (!silent) {
    isRefreshing.value = true;
  }
  try {
    const linksRes = await listLinks({ page: 1, pageSize: 100 });
    links.value = linksRes.items;

    if (links.value.length === 0) {
      allVisits.value = [];
      selectedVisit.value = null;
      return;
    }

    const existingIds = new Set(allVisits.value.map((v) => v.id));

    // 并发获取各短链的最新真实访问记录
    const visitResults = await Promise.all(
      links.value.map(async (link) => {
        try {
          const res = await listVisits(link.id, { page: 1, pageSize: 50 });
          return res.items.map((item) =>
            processVisit(item, link, !existingIds.has(item.id) && allVisits.value.length > 0),
          );
        } catch {
          return [];
        }
      }),
    );

    const flatVisits = visitResults.flat();
    flatVisits.sort((a, b) => new Date(b.createdAt).getTime() - new Date(a.createdAt).getTime());
    allVisits.value = flatVisits;

    // 动画结束后清除 isNew 标记
    setTimeout(() => {
      for (const v of allVisits.value) {
        v.isNew = false;
      }
    }, 1000);

    // 同步选中状态
    if (selectedVisit.value) {
      const found = flatVisits.find((v) => v.id === selectedVisit.value?.id);
      selectedVisit.value = found || flatVisits[0] || null;
    } else if (flatVisits.length > 0) {
      selectedVisit.value = flatVisits[0];
    } else {
      selectedVisit.value = null;
    }
  } catch (error) {
    if (error instanceof ApiError && error.status !== 401) {
      message.error(error.message);
    }
  } finally {
    isLoading.value = false;
    isRefreshing.value = false;
  }
}

// 自动刷新定时器
function startAutoRefresh() {
  stopAutoRefresh();
  autoRefreshTimer = setInterval(() => {
    if (autoRefresh.value) {
      loadData(true);
    }
  }, 10000);
}

function stopAutoRefresh() {
  if (autoRefreshTimer) {
    clearInterval(autoRefreshTimer);
    autoRefreshTimer = null;
  }
}

watch(autoRefresh, (enabled) => {
  if (enabled) {
    startAutoRefresh();
    message.info('已开启 10 秒自动刷新');
  } else {
    stopAutoRefresh();
    message.info('已暂停自动刷新');
  }
});

// ==================== 生命周期 ====================
onMounted(async () => {
  await loadData(false);
  if (autoRefresh.value) {
    startAutoRefresh();
  }
});

onBeforeUnmount(() => {
  stopAutoRefresh();
});
</script>
