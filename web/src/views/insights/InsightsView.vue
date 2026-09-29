<template>
  <div class="flex flex-col gap-5 pb-10" data-od-id="insights-view">
    <!-- ==================== 顶部选项卡 ==================== -->
    <section class="panel" data-od-id="ins-tabs">
      <div class="tabs" role="tablist" id="inTabs">
        <button
          role="tab"
          type="button"
          :aria-selected="currentTab === 'flow'"
          data-tab="flow"
          @click="switchTab('flow')"
        >
          流量结构
        </button>
        <button
          role="tab"
          type="button"
          :aria-selected="currentTab === 'rules'"
          data-tab="rules"
          @click="switchTab('rules')"
        >
          规则表现
        </button>
        <button
          role="tab"
          type="button"
          :aria-selected="currentTab === 'postback'"
          data-tab="postback"
          id="postback"
          @click="switchTab('postback')"
        >
          回传健康度
        </button>
      </div>
    </section>

    <!-- ==================== Tab 1: 流量结构 ==================== -->
    <section v-if="currentTab === 'flow'" data-tabpanel="flow" class="space-y-4">
      <!-- 4 个 KPI 卡片 -->
      <div class="kpi-grid">
        <div class="kpi">
          <div class="kpi-k">总访问量</div>
          <div class="kpi-v">{{ totalVisits.toLocaleString() }}</div>
          <div class="kpi-sub">
            覆盖 {{ links.length }} 条短链
          </div>
          <button
            type="button"
            class="kpi-info"
            aria-label="指标说明：总访问量"
          >
            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.9" stroke-linecap="round">
              <circle cx="12" cy="12" r="9" />
              <path d="M12 11.2v5.1M12 7.7h.01" />
            </svg>
            <span class="kpi-tip" role="tooltip">
              当前租户所有短链累计接收到的外部 HTTP 请求总次数。
            </span>
          </button>
        </div>

        <div class="kpi">
          <div class="kpi-k">落地页点击</div>
          <div class="kpi-v">{{ totalClicks.toLocaleString() }}</div>
          <div class="kpi-sub">
            <span class="up font-mono">{{ ctr }}</span> 点击转化率 (CTR)
          </div>
          <button
            type="button"
            class="kpi-info"
            aria-label="指标说明：落地页点击"
          >
            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.9" stroke-linecap="round">
              <circle cx="12" cy="12" r="9" />
              <path d="M12 11.2v5.1M12 7.7h.01" />
            </svg>
            <span class="kpi-tip" role="tooltip">
              落地页型短链中访客点击行动召唤（CTA）按钮并触发 SDK 回传的实际点击次数。
            </span>
          </button>
        </div>

        <div class="kpi">
          <div class="kpi-k">活跃短链</div>
          <div class="kpi-v">{{ activeLinksCount }}</div>
          <div class="kpi-sub">
            共计 {{ links.length }} 条短链已就绪
          </div>
          <button
            type="button"
            class="kpi-info"
            aria-label="指标说明：活跃短链"
          >
            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.9" stroke-linecap="round">
              <circle cx="12" cy="12" r="9" />
              <path d="M12 11.2v5.1M12 7.7h.01" />
            </svg>
            <span class="kpi-tip" role="tooltip">
              当前处于启用状态、可正常执行准入裁决与跳转的短链总数。
            </span>
          </button>
        </div>

        <div class="kpi">
          <div class="kpi-k">已分析样本</div>
          <div class="kpi-v">{{ visits.length }}</div>
          <div class="kpi-sub">
            用于设备与来源多维分析
          </div>
          <button
            type="button"
            class="kpi-info"
            aria-label="指标说明：已分析样本"
          >
            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.9" stroke-linecap="round">
              <circle cx="12" cy="12" r="9" />
              <path d="M12 11.2v5.1M12 7.7h.01" />
            </svg>
            <span class="kpi-tip" role="tooltip">
              从各短链拉取到的最近明细访问日志条数，用于计算实时设备分布与操作系统占比。
            </span>
          </button>
        </div>
      </div>

      <!-- 无访问数据时的空状态 -->
      <div v-if="totalVisits === 0" class="panel p-12 text-center">
        <AppEmpty description="暂无多维访问数据。在投放短链产生访问后，系统将自动汇总来源、操作系统、设备与转化数据。" />
        <div class="mt-4">
          <router-link to="/links" class="btn btn-primary btn-sm">
            前往短链管理
          </router-link>
        </div>
      </div>

      <!-- 有访问数据时的多维图表 -->
      <template v-else>
        <!-- 第一行: 流量来源 + 访问最多的短链 -->
        <div class="cols-2 items-stretch">
          <!-- 流量来源分布 -->
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
                  v-for="src in sourceDistribution"
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

          <!-- TOP 短链流量贡献 -->
          <div class="panel flex flex-col">
            <div class="panel-hd">
              <div>
                <h2>短链流量榜</h2>
                <p>接收访问量前 5 的短链与其转化贡献。</p>
              </div>
              <router-link to="/links" class="btn btn-sm">
                全部短链 →
              </router-link>
            </div>
            <div class="panel-bd flex-1">
              <div v-if="topLinks.length === 0" class="empty">暂无短链流量数据</div>
              <div v-else class="bars">
                <div
                  v-for="link in topLinks"
                  :key="link.id"
                  class="bar-row bar-row-lg"
                >
                  <span class="bar-lab font-mono font-medium text-ink">/{{ link.code }}</span>
                  <span class="bar-track">
                    <span
                      class="bar-fill t-accent"
                      :style="{ width: `${totalVisits > 0 ? Math.min(100, Math.round((link.visits / totalVisits) * 100)) : 0}%` }"
                    ></span>
                  </span>
                  <span class="bar-val">
                    {{ link.visits }} 访问
                    <span v-if="link.linkType === 'landing'" class="text-xs text-muted">({{ link.clicks }} 点击)</span>
                  </span>
                </div>
              </div>
            </div>
            <div class="panel-ft">
              落地页型短链同时计入访客浏览量与 CTA 按钮点击转化数。
            </div>
          </div>
        </div>

        <!-- 第二行: 设备类型 + 操作系统分布 -->
        <div class="cols-2 items-stretch">
          <!-- 设备类型分布 -->
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
                  v-for="dev in deviceDistribution"
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

          <!-- 操作系统分布 -->
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
                  v-for="os in osDistribution"
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
        </div>

        <!-- 第三行: 浏览器分布 -->
        <div class="panel">
          <div class="panel-hd">
            <div>
              <h2>访问浏览器分布</h2>
              <p>各应用内嵌 WebView 与独立浏览器占比。</p>
            </div>
          </div>
          <div class="panel-bd">
            <div class="grid grid-cols-2 gap-4 sm:grid-cols-4">
              <div
                v-for="br in browserDistribution"
                :key="br.name"
                class="rounded-lg border border-line bg-surface-muted p-3"
              >
                <div class="text-xs text-muted">{{ br.name }}</div>
                <div class="text-lg font-bold font-mono text-ink mt-1">{{ br.percent }}%</div>
                <div class="text-xs text-muted mt-0.5">{{ br.count }} 次访问</div>
              </div>
            </div>
          </div>
        </div>
      </template>
    </section>

    <!-- ==================== Tab 2: 规则表现 ==================== -->
    <section v-else-if="currentTab === 'rules'" data-tabpanel="rules" class="space-y-4">
      <div class="two-col">
        <!-- 各规则当前状态与命中情况 -->
        <div class="panel flex flex-col">
          <div class="panel-hd">
            <div>
              <h2>规则集求值表现</h2>
              <p>首条命中即裁决 (First-Match-Wins)。未命中任何规则时执行短链兜底动作。</p>
            </div>
            <router-link to="/rules" class="btn btn-sm">
              去规则引擎 →
            </router-link>
          </div>
          <div class="panel-bd flex-1">
            <div v-if="configuredRules.length === 0" class="empty py-6 text-center text-muted">
              暂无规则数据
            </div>
            <div v-else class="bars">
              <div
                v-for="rule in configuredRules"
                :key="rule.id"
                class="bar-row bar-row-lg"
              >
                <span class="bar-lab">
                  <span class="font-mono font-medium">{{ rule.id }}</span> {{ rule.name }}
                  <span
                    v-if="rule.orphan"
                    class="badge badge-warn"
                    title="spec D2:作用域为「指定短链」但未关联任何短链的规则不会退化成全局规则，它永远不命中"
                  >
                    未关联 · 不会命中
                  </span>
                </span>
                <span class="bar-track">
                  <span
                    class="bar-fill"
                    :class="{ 't-accent': rule.action === '放行', 't-danger': rule.action === '404' || rule.action === '限流 429' }"
                    :style="{ width: `${rule.hits > 0 ? Math.min(100, Math.round((rule.hits / Math.max(1, maxRuleHits)) * 100)) : 4}%` }"
                  ></span>
                </span>
                <span class="bar-val">
                  <span :class="rule.enabled ? 'badge badge-ok' : 'badge badge-neutral'">
                    {{ rule.enabled ? '生效中' : '已停用' }}
                  </span>
                </span>
              </div>
            </div>
          </div>
          <div class="panel-ft">
            柱长为各规则的 24h 命中次数（相对最大值归一化）。逐条求值推演请到规则引擎页的「规则模拟器」。
          </div>
        </div>

        <!-- 历史回放沙盘与健康检查 -->
        <div class="stack">
          <div class="panel">
            <div class="panel-hd">
              <div>
                <h2 class="text-[15px]">爬虫 UA 粗筛</h2>
                <p>仅按 UA 正则粗筛已有访问样本，不是规则求值结果。</p>
              </div>
            </div>
            <div class="panel-bd stack-sm">
              <p class="text-xs text-muted">
                当前内存中保有 <span class="font-mono font-semibold text-ink">{{ visits.length }}</span> 条真实访问日志。规则求值在服务端跳转链路上执行，本页拿不到逐条裁决结果。
              </p>
              <div v-if="replayResult" class="rounded-lg border border-line bg-surface-muted p-3 text-xs space-y-1 mt-2">
                <div class="text-ok font-semibold">✓ 粗筛完成</div>
                <div class="text-muted">已粗筛样本: {{ replayResult.total }} 条</div>
                <div class="text-muted">命中爬虫 UA 特征: {{ replayResult.blocked }} 条 | 未命中: {{ replayResult.passed }} 条</div>
              </div>
            </div>
            <div class="panel-ft">
              <button
                type="button"
                class="btn btn-sm btn-primary w-full"
                :disabled="replaying || visits.length === 0"
                @click="runReplay"
              >
                {{ replaying ? '粗筛中…' : '按爬虫 UA 粗筛访问样本' }}
              </button>
            </div>
          </div>

          <div class="panel">
            <div class="panel-hd">
              <div>
                <h2 class="text-[15px]">规则集健康度检查</h2>
              </div>
            </div>
            <div class="panel-bd stack-sm text-xs">
              <div class="row-between py-1 border-b border-line">
                <span class="text-muted">规则总数</span>
                <span class="font-mono font-semibold">{{ rules.length }} 条</span>
              </div>
              <div class="row-between py-1 border-b border-line">
                <span class="text-muted">已启用规则</span>
                <span class="font-mono font-semibold">{{ rules.filter((r) => r.enabled).length }} 条</span>
              </div>
              <div class="row-between py-1 border-b border-line">
                <span class="text-muted">未关联短链的规则</span>
                <span :class="orphanRuleCount > 0 ? 'badge badge-warn' : 'font-mono font-semibold'">
                  {{ orphanRuleCount > 0 ? orphanRuleCount + ' 条 · 不会命中' : '0 条' }}
                </span>
              </div>
              <div class="row-between py-1">
                <span class="text-muted">有效短链数量</span>
                <span class="font-mono font-semibold">{{ links.length }} 条</span>
              </div>
            </div>
          </div>
        </div>
      </div>
    </section>

    <!-- ==================== Tab 3: 回传健康度 ==================== -->
    <section v-else-if="currentTab === 'postback'" data-tabpanel="postback" class="space-y-4">
      <div class="kpi-grid">
        <div class="kpi">
          <div class="kpi-k">回传集成状态</div>
          <div class="kpi-v">就绪</div>
          <div class="kpi-sub">
            支持 TikTok / Meta / 自有 Webhook
          </div>
        </div>
        <div class="kpi">
          <div class="kpi-k">已配置通道</div>
          <div class="kpi-v">{{ postbackChannels.filter(c => c.configured).length }}</div>
          <div class="kpi-sub">
            共 {{ postbackChannels.length }} 个平台接口
          </div>
        </div>
        <div class="kpi">
          <div class="kpi-k">转化回传模式</div>
          <div class="kpi-v">Server2Server</div>
          <div class="kpi-sub">
            CAPI / Events API 双端保障
          </div>
        </div>
        <div class="kpi">
          <div class="kpi-k">数据脱敏</div>
          <div class="kpi-v">SHA-256</div>
          <div class="kpi-sub">
            合规匿名化参数处理
          </div>
        </div>
      </div>

      <!-- 回传通道状态表 -->
      <div class="panel">
        <div class="panel-hd">
          <div>
            <h2>转化回传通道</h2>
            <p>每个广告平台的事件回传通道、接入方式与运行状态。</p>
          </div>
        </div>
        <div class="tbl-wrap">
          <table class="tbl">
            <thead>
              <tr>
                <th>通道名称</th>
                <th>接入协议</th>
                <th>支持事件</th>
                <th>状态</th>
                <th>配置</th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="ch in postbackChannels" :key="ch.id">
                <td class="font-medium text-ink">{{ ch.name }}</td>
                <td class="font-mono text-xs text-muted">{{ ch.protocol }}</td>
                <td class="text-xs text-muted">{{ ch.events }}</td>
                <td>
                  <span :class="ch.configured ? 'badge badge-ok' : 'badge badge-neutral'">
                    {{ ch.configured ? '已配置' : '待配置' }}
                  </span>
                </td>
                <td class="shrink">
                  <button type="button" class="btn btn-sm" @click="openConfigModal(ch)">
                    查看说明
                  </button>
                </td>
              </tr>
            </tbody>
          </table>
        </div>
        <div class="panel-ft">
          在创建或编辑短链时，勾选对应平台的 Pixel ID 与 CAPI Token 即可为该投放链接开启自动回传。
        </div>
      </div>
    </section>

    <!-- 通道说明模态框 -->
    <div
      v-if="activeChannelModal"
      class="fixed inset-0 z-50 flex items-center justify-center bg-black/45 p-4"
      @click.self="activeChannelModal = null"
    >
      <div class="panel w-full max-w-lg shadow-2xl">
        <div class="panel-hd">
          <h2>{{ activeChannelModal.name }} 回传配置指引</h2>
          <button type="button" class="icon-btn" @click="activeChannelModal = null">
            <X :size="16" />
          </button>
        </div>
        <div class="panel-bd space-y-3 text-sm">
          <p class="text-muted leading-relaxed">
            CLOAK 提供了端到端的 Server-to-Server 转化回传机制。当通过落地页 SDK 或自定义跳转产生有效转化时，系统将在服务端异步将事件发送至 {{ activeChannelModal.name }}。
          </p>
          <div class="rounded-lg border border-line bg-surface-muted p-3 text-xs space-y-1 font-mono">
            <div>通道: {{ activeChannelModal.name }}</div>
            <div>协议: {{ activeChannelModal.protocol }}</div>
            <div>事件: {{ activeChannelModal.events }}</div>
          </div>
          <p class="text-xs text-muted">
            请前往「短链与目标」编辑指定短链，在「出站与回传」设置中启用并填写 Pixel ID 与访问凭证。
          </p>
        </div>
        <div class="panel-ft flex justify-end">
          <button type="button" class="btn btn-primary btn-sm" @click="activeChannelModal = null">
            我知道了
          </button>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue';
import { useRoute, useRouter } from 'vue-router';
import { X } from '@lucide/vue';
import { UAParser } from 'ua-parser-js';

import { listLinks } from '@/api/links';
import { listRules } from '@/api/rules';
import { listVisits } from '@/api/visits';
import AppEmpty from '@/components/ui/AppEmpty.vue';
import type { Link, Rule, RuleAction, Visit } from '@/types/api';
import { message } from '@/utils/toast';

export type InsightsTab = 'flow' | 'rules' | 'postback';

const TAB_STORAGE_KEY = 'cloak.in.tab';

const route = useRoute();
const router = useRouter();

const currentTab = ref<InsightsTab>('flow');
const loading = ref(false);
const links = ref<Link[]>([]);
const visits = ref<Visit[]>([]);

function resolveTabFromRoute(): InsightsTab | null {
  const hash = (route.hash || '').replace('#', '').trim();
  if (hash === 'flow' || hash === 'rules' || hash === 'postback') {
    return hash;
  }
  const queryTab = route.query.tab as string | undefined;
  if (queryTab === 'flow' || queryTab === 'rules' || queryTab === 'postback') {
    return queryTab;
  }
  return null;
}

function switchTab(tab: InsightsTab, updateRoute = true) {
  currentTab.value = tab;
  try {
    localStorage.setItem(TAB_STORAGE_KEY, tab);
  } catch {
    // 忽略异常
  }

  if (updateRoute) {
    router.replace({
      path: route.path,
      query: route.query,
      hash: `#${tab}`,
    }).catch(() => {});
  }
}

// ==================== 真实数据加载与聚合 ====================
async function fetchData() {
  loading.value = true;
  try {
    const res = await listLinks(1, 100);
    links.value = res.items || [];

    // 规则集表现（真实规则数据）
    listRules({ page: 1, pageSize: 100 })
      .then((r) => {
        rules.value = r.items;
      })
      .catch(() => {
        rules.value = [];
      });

    // 并发拉取有访问量的短链明细记录
    const linksWithVisits = links.value.filter((l) => l.visits > 0);
    const visitPromises = linksWithVisits.slice(0, 10).map((l) =>
      listVisits(l.id, { page: 1, pageSize: 50 }).catch(() => ({ items: [], total: 0 }))
    );

    const visitResults = await Promise.all(visitPromises);
    const mergedVisits: Visit[] = [];
    visitResults.forEach((r) => {
      if (r?.items) mergedVisits.push(...r.items);
    });

    visits.value = mergedVisits;
    message.success('数据分析已同步更新');
  } catch (err: unknown) {
    const msg = err instanceof Error ? err.message : '获取数据失败';
    message.error(msg);
  } finally {
    loading.value = false;
  }
}

onMounted(() => {
  const routeTab = resolveTabFromRoute();
  if (routeTab) {
    currentTab.value = routeTab;
  } else {
    try {
      const saved = localStorage.getItem(TAB_STORAGE_KEY) as InsightsTab | null;
      if (saved && (saved === 'flow' || saved === 'rules' || saved === 'postback')) {
        currentTab.value = saved;
      }
    } catch {
      // 忽略
    }
  }
  fetchData();
});

watch(
  () => [route.hash, route.query.tab],
  () => {
    const routeTab = resolveTabFromRoute();
    if (routeTab && routeTab !== currentTab.value) {
      switchTab(routeTab, false);
    }
  }
);

// ==================== 响应式计算指标 ====================
const totalVisits = computed(() => links.value.reduce((acc, l) => acc + (l.visits || 0), 0));
const totalClicks = computed(() => links.value.reduce((acc, l) => acc + (l.clicks || 0), 0));
const activeLinksCount = computed(() => links.value.filter((l) => l.status === 'enabled').length);

const ctr = computed(() => {
  if (totalVisits.value === 0) return '0%';
  return `${((totalClicks.value / totalVisits.value) * 100).toFixed(1)}%`;
});

const topLinks = computed(() => {
  return [...links.value].sort((a, b) => (b.visits || 0) - (a.visits || 0)).slice(0, 5);
});

// 解析真实访问记录的 UA 与 Referrer
interface BreakdownItem {
  name: string;
  count: number;
  percent: number;
}

const sourceDistribution = computed<BreakdownItem[]>(() => {
  if (visits.value.length === 0) return [];
  const map: Record<string, number> = {
    'TikTok Ads': 0,
    'Meta Ads': 0,
    'Google Ads': 0,
    '直接访问': 0,
    '其他来源': 0,
  };

  visits.value.forEach((v) => {
    const ref = (v.referer || '').toLowerCase();
    if (ref.includes('tiktok')) map['TikTok Ads']++;
    else if (ref.includes('facebook') || ref.includes('instagram') || ref.includes('meta')) map['Meta Ads']++;
    else if (ref.includes('google')) map['Google Ads']++;
    else if (!ref || ref === '-') map['直接访问']++;
    else map['其他来源']++;
  });

  const total = visits.value.length;
  return Object.entries(map).map(([name, count]) => ({
    name,
    count,
    percent: Math.round((count / total) * 100),
  })).sort((a, b) => b.count - a.count);
});

const deviceDistribution = computed<BreakdownItem[]>(() => {
  if (visits.value.length === 0) return [];
  const map: Record<string, number> = {
    '移动端': 0,
    '桌面端': 0,
    '平板': 0,
    '爬虫 / 机器人': 0,
  };

  visits.value.forEach((v) => {
    const parser = new UAParser(v.userAgent);
    const dev = parser.getDevice();
    const ua = (v.userAgent || '').toLowerCase();
    if (/bot|spider|crawl|curl|wget|python/i.test(ua)) {
      map['爬虫 / 机器人']++;
    } else if (dev.type === 'mobile' || /mobile|iphone|android/i.test(ua)) {
      map['移动端']++;
    } else if (dev.type === 'tablet' || /ipad/i.test(ua)) {
      map['平板']++;
    } else {
      map['桌面端']++;
    }
  });

  const total = visits.value.length;
  return Object.entries(map).map(([name, count]) => ({
    name,
    count,
    percent: Math.round((count / total) * 100),
  })).sort((a, b) => b.count - a.count);
});

const osDistribution = computed<BreakdownItem[]>(() => {
  if (visits.value.length === 0) return [];
  const map: Record<string, number> = {
    iOS: 0,
    Android: 0,
    Windows: 0,
    macOS: 0,
    Linux: 0,
    其他: 0,
  };

  visits.value.forEach((v) => {
    const parser = new UAParser(v.userAgent);
    const osName = parser.getOS().name || '';
    if (/ios/i.test(osName)) map.iOS++;
    else if (/android/i.test(osName)) map.Android++;
    else if (/windows/i.test(osName)) map.Windows++;
    else if (/mac/i.test(osName)) map.macOS++;
    else if (/linux/i.test(osName)) map.Linux++;
    else map['其他']++;
  });

  const total = visits.value.length;
  return Object.entries(map).map(([name, count]) => ({
    name,
    count,
    percent: Math.round((count / total) * 100),
  })).sort((a, b) => b.count - a.count);
});

const browserDistribution = computed<BreakdownItem[]>(() => {
  if (visits.value.length === 0) return [];
  const map: Record<string, number> = {
    'Chrome / WebKit': 0,
    'Safari': 0,
    'Firefox': 0,
    '应用内内置': 0,
  };

  visits.value.forEach((v) => {
    const parser = new UAParser(v.userAgent);
    const brName = parser.getBrowser().name || '';
    if (/chrome|chromium/i.test(brName)) map['Chrome / WebKit']++;
    else if (/safari/i.test(brName)) map['Safari']++;
    else if (/firefox/i.test(brName)) map['Firefox']++;
    else map['应用内内置']++;
  });

  const total = visits.value.length;
  return Object.entries(map).map(([name, count]) => ({
    name,
    count,
    percent: Math.round((count / total) * 100),
  })).sort((a, b) => b.count - a.count);
});

// ==================== Tab 2: 规则表现与沙盘 ====================
interface RuleDisplayItem {
  id: number;
  name: string;
  action: string;
  enabled: boolean;
  hits: number;
  /** scope=links 且未关联短链的规则永远不命中(spec D2) */
  orphan: boolean;
}

const RULE_ACTION_LABEL: Record<RuleAction, string> = {
  pass: '放行',
  redirect: '重定向',
  notfound: '404',
  throttle: '限流 429',
};

/** 真实规则数据（GET /api/rules），不再用占位规则编号凑图表 */
const rules = ref<Rule[]>([]);

const configuredRules = computed<RuleDisplayItem[]>(() =>
  rules.value.map((r) => ({
    id: r.id,
    name: r.name,
    action: RULE_ACTION_LABEL[r.action] ?? r.action,
    enabled: r.enabled,
    hits: r.hits24h || 0,
    orphan: r.scope === 'links' && (r.linkCount || 0) === 0,
  })),
);

/** 零关联的 scoped 规则：永远不命中，规则引擎页同样会显式告警 */
const orphanRuleCount = computed(() => configuredRules.value.filter((r) => r.orphan).length);

/** 柱长归一化基准：各规则 24h 命中次数的最大值 */
const maxRuleHits = computed(() =>
  configuredRules.value.reduce((acc, r) => Math.max(acc, r.hits), 0),
);

const replaying = ref(false);
const replayResult = ref<{ total: number; passed: number; blocked: number } | null>(null);

/**
 * 爬虫 UA 粗筛：只按 UA 正则粗筛访问样本，**不是**规则求值结果
 * （规则求值在服务端跳转链路上执行，本页拿不到逐条裁决结果）。
 */
function runReplay() {
  if (visits.value.length === 0) {
    message.info('当前暂无访问样本，请先产生访问记录');
    return;
  }
  replaying.value = true;
  setTimeout(() => {
    replaying.value = false;
    const total = visits.value.length;
    const blocked = visits.value.filter((v) => /bot|spider|crawl/i.test(v.userAgent)).length;
    replayResult.value = {
      total,
      passed: total - blocked,
      blocked,
    };
    message.success(`已粗筛 ${total} 条历史访问记录`);
  }, 500);
}

// ==================== Tab 3: 回传通道 ====================
interface PostbackChannel {
  id: string;
  name: string;
  protocol: string;
  events: string;
  configured: boolean;
}

const postbackChannels: PostbackChannel[] = [
  { id: 'tiktok', name: 'TikTok Events API', protocol: 'HTTP POST / CAPI', events: 'ViewContent, Click, Purchase', configured: true },
  { id: 'meta', name: 'Meta Conversions API', protocol: 'Graph API CAPI', events: 'ViewContent, Purchase', configured: true },
  { id: 'google', name: 'Google Ads Enhanced Conversions', protocol: 'Google Ads API', events: 'Conversion, Click', configured: false },
  { id: 'custom', name: '自有服务器 Webhook 回调', protocol: 'RESTful Webhook', events: '全量事件实时透传', configured: true },
];

const activeChannelModal = ref<PostbackChannel | null>(null);

function openConfigModal(ch: PostbackChannel) {
  activeChannelModal.value = ch;
}
</script>
