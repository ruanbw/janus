<template>
  <div class="flex flex-col gap-5 pb-10" data-od-id="visit-stream-view">
    <!-- ==================== 顶部页头与系统说明 ==================== -->
    <header class="panel" data-od-id="topbar-visit-stream">
      <div class="panel-hd">
        <div>
          <div class="eyebrow">CLOAK / 访问决策流</div>
          <h1 class="text-xl font-bold tracking-tight text-ink md:text-2xl mt-0.5">访问决策流</h1>
          <p class="topbar-sub">
            每一次访问都留下完整决策链。被误拦时，这里能直接回答「哪条规则、哪个字段、为什么」。
          </p>
        </div>
        <div class="btn-row">
          <span class="badge badge-neutral" title="本原型内所有数值均为演示数据，不代表真实流量">
            <span class="dot" style="background: var(--muted)"></span>
            原型演示数据
          </span>
        </div>
      </div>
    </header>

    <!-- ==================== 顶部 4 个实时 KPI 卡片 ==================== -->
    <section class="kpi-grid" data-od-id="live-kpi">
      <!-- KPI 1: 当前在线访客 -->
      <div
        class="kpi"
        data-tip="最近 5 分钟内发起过请求、且连接尚未结束的去重 IP 数。窗口取 5 分钟是为了避开 NAT 把多个真实用户合并成一个 IP 的误差。"
      >
        <div class="kpi-k">当前在线访客</div>
        <div class="kpi-v" id="liveCount">{{ liveCount }}</div>
        <div class="kpi-sub">
          <span class="dot dot-live" style="display: inline-block; color: var(--accent)"></span>
          实时
        </div>
        <button
          type="button"
          class="kpi-info"
          aria-label="指标说明：当前在线访客"
          aria-describedby="kpi-tip-1"
        >
          <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.9" stroke-linecap="round">
            <circle cx="12" cy="12" r="9" />
            <path d="M12 11.2v5.1M12 7.7h.01" />
          </svg>
          <span class="kpi-tip" id="kpi-tip-1" role="tooltip">
            最近 5 分钟内发起过请求、且连接尚未结束的去重 IP 数。窗口取 5 分钟是为了避开 NAT 把多个真实用户合并成一个 IP 的误差。
          </span>
        </button>
      </div>

      <!-- KPI 2: 今日决策次数 -->
      <div
        class="kpi"
        data-tip="今天执行过的规则裁决总次数。同一个访客每次访问都算一次，所以它不等于访客人数；一次请求只会裁决一次，命中即停。"
      >
        <div class="kpi-k">今日决策次数</div>
        <div class="kpi-v">{{ todayDecisionsFormatted }}</div>
        <div class="kpi-sub">P50 11ms · P99 46ms</div>
        <button
          type="button"
          class="kpi-info"
          aria-label="指标说明：今日决策次数"
          aria-describedby="kpi-tip-2"
        >
          <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.9" stroke-linecap="round">
            <circle cx="12" cy="12" r="9" />
            <path d="M12 11.2v5.1M12 7.7h.01" />
          </svg>
          <span class="kpi-tip" id="kpi-tip-2" role="tooltip">
            今天执行过的规则裁决总次数。同一个访客每次访问都算一次，所以它不等于访客人数；一次请求只会裁决一次，命中即停。
          </span>
        </button>
      </div>

      <!-- KPI 3: 拦截总量与结构 -->
      <div
        class="kpi"
        data-tip="被规则拒绝的访问数，按触发拦截的条件分类。爬虫来源通常是平台审查员、竞品抓取和自动化工具，不区分它们会把投放预算浪费在不可交互的流量上。"
      >
        <div class="kpi-k">拦截</div>
        <div class="kpi-v">{{ blockedCountFormatted }}</div>
        <div class="kpi-sub">爬虫 59% · 代理 20% · 频次 16%</div>
        <button
          type="button"
          class="kpi-info"
          aria-label="指标说明：拦截"
          aria-describedby="kpi-tip-3"
        >
          <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.9" stroke-linecap="round">
            <circle cx="12" cy="12" r="9" />
            <path d="M12 11.2v5.1M12 7.7h.01" />
          </svg>
          <span class="kpi-tip" id="kpi-tip-3" role="tooltip">
            被规则拒绝的访问数，按触发拦截的条件分类。爬虫来源通常是平台审查员、竞品抓取和自动化工具，不区分它们会把投放预算浪费在不可交互的流量上。
          </span>
        </button>
      </div>

      <!-- KPI 4: 异常告警 -->
      <div
        class="kpi"
        data-tip="系统自动检出、需要人工确认的异常：规则长时间零命中、放行率突变、单条规则命中量异常升高、或全部域名同时不可用。"
      >
        <div class="kpi-k">异常告警</div>
        <div class="kpi-v">3</div>
        <div class="kpi-sub">1 条规则 30 分钟内零命中</div>
        <button
          type="button"
          class="kpi-info"
          aria-label="指标说明：异常告警"
          aria-describedby="kpi-tip-4"
        >
          <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.9" stroke-linecap="round">
            <circle cx="12" cy="12" r="9" />
            <path d="M12 11.2v5.1M12 7.7h.01" />
          </svg>
          <span class="kpi-tip" id="kpi-tip-4" role="tooltip">
            系统自动检出、需要人工确认的异常：规则长时间零命中、放行率突变、单条规则命中量异常升高、或全部域名同时不可用。
          </span>
        </button>
      </div>
    </section>

    <!-- ==================== 两栏主体布局 ==================== -->
    <section class="two-col" data-od-id="stream-detail">
      <!-- 左栏：访问决策流列表与控制 -->
      <div class="panel">
        <div class="panel-hd">
          <div>
            <h2>访问决策流</h2>
            <p>每次请求的裁决结果。点任意一行查看完整决策链。</p>
          </div>
          <div class="row" style="gap: 8px">
            <label class="row tiny muted" style="gap: 6px">
              <span class="switch">
                <input type="checkbox" v-model="autoScroll" id="autoScroll" />
                <i></i>
              </span>
              自动滚动
            </label>
            <button
              type="button"
              :class="isPaused ? 'btn btn-sm btn-primary' : 'btn btn-sm'"
              id="pauseBtn"
              @click="togglePause"
            >
              {{ isPaused ? '继续' : '暂停' }}
            </button>
          </div>
        </div>

        <div class="panel-bd" style="padding-bottom: 12px">
          <!-- 筛选工具栏 -->
          <div class="toolbar">
            <!-- 裁决动作分段器 -->
            <div class="seg-filter" id="vAct" role="group" aria-label="按动作过滤">
              <button
                v-for="act in actionFilters"
                :key="act.key"
                type="button"
                :aria-pressed="activeActionFilter === act.key"
                :data-f="act.key"
                @click="activeActionFilter = act.key"
              >
                {{ act.label }}
              </button>
            </div>

            <!-- 国家下拉过滤 -->
            <select
              class="select"
              id="vCountry"
              v-model="selectedCountry"
              aria-label="按国家过滤"
            >
              <option value="all">全部国家</option>
              <option value="BR">BR (巴西)</option>
              <option value="US">US (美国)</option>
              <option value="DE">DE (德国)</option>
              <option value="NL">NL (荷兰)</option>
              <option value="JP">JP (日本)</option>
              <option value="GB">GB (英国)</option>
            </select>

            <!-- 搜索框 -->
            <input
              v-model="searchQuery"
              class="input grow"
              id="vSearch"
              placeholder="搜索 IP、设备或来源…"
              aria-label="搜索访问"
            />
          </div>
        </div>

        <!-- 高密表头 -->
        <div class="feed-hd">
          <span>时间</span>
          <span>来访 IP</span>
          <span>国家</span>
          <span>访客画像</span>
          <span>命中规则</span>
          <span>动作</span>
          <span style="text-align: right">延迟</span>
        </div>

        <!-- 决策流列表 -->
        <div class="feed" id="feed" ref="feedRef">
          <button
            v-for="item in filteredVisits"
            :key="item.id"
            type="button"
            class="feed-row"
            :class="{ 'new-in': item.isNew }"
            :data-visit="item.id"
            :data-ip="item.ip"
            :data-act="item.action"
            :data-country="item.country"
            :data-rule="item.ruleId"
            :aria-selected="selectedVisit?.id === item.id"
            @click="selectVisit(item)"
          >
            <span class="t">{{ item.time }}</span>
            <span class="ip">{{ item.ip }}</span>
            <span class="mono tiny">{{ item.country }}</span>
            <span class="dev">
              {{ item.device }} · <span class="mono">{{ item.lang }}</span> · 来自 {{ item.referrer }}
            </span>
            <span>
              <span class="badge badge-neutral mono">{{ item.ruleId }}</span>
            </span>
            <span>
              <span :class="['badge', getActionBadgeClass(item.action)]">
                {{ formatActionText(item) }}
              </span>
            </span>
            <span class="lat">{{ item.latency }}ms</span>
          </button>

          <!-- 空状态 -->
          <div v-if="filteredVisits.length === 0" class="empty">
            没有匹配的访问决策记录
          </div>
        </div>

        <!-- 底部说明与统计 -->
        <div class="panel-ft row-between">
          <span>显示最近 {{ filteredVisits.length }} 条，共 {{ todayDecisionsFormatted }} 条 · 保留 90 天</span>
          <span class="mono">日志脱敏：IPv4 保留前三段，IPv6 保留 /48</span>
        </div>
      </div>

      <!-- 右栏：决策链回放（sticky）与拦截构成 -->
      <div class="stack" style="position: sticky; top: 80px">
        <!-- 决策链回放面板 -->
        <div class="panel" id="detailPanel" v-if="selectedVisit">
          <div class="panel-hd">
            <div>
              <h2 style="font-size: 15px">决策链回放</h2>
              <p>解释这一次访问为什么得到这个结果。</p>
            </div>
            <span :class="['badge mono', getActionBadgeClass(selectedVisit.action)]" id="dVerdict">
              {{ verdictBadgeText }}
            </span>
          </div>

          <div class="panel-bd stack">
            <!-- 访问基本画像 KV -->
            <dl class="kv">
              <dt>来访 IP</dt>
              <dd id="dIp">{{ selectedVisit.ip }}</dd>
              <dt>时间</dt>
              <dd id="dTime">{{ selectedVisit.timeWithMs }}</dd>
              <dt>短链</dt>
              <dd>{{ selectedVisit.shortlink }}</dd>
              <dt>Referrer</dt>
              <dd id="dRef">{{ selectedVisit.referrer }}</dd>
              <dt>参数</dt>
              <dd>{{ selectedVisit.params }}</dd>
            </dl>

            <hr class="rule" style="border: 0; border-top: 1px solid var(--border)" />

            <!-- 逐条规则求值追踪链路 -->
            <div>
              <div class="mono micro muted" style="margin-bottom: 8px">逐条求值</div>
              <div class="trace" id="dTrace">
                <div
                  v-for="(step, sIdx) in selectedVisit.traceSteps"
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
              <div class="tiny muted" v-html="selectedVisit.outbound.primary"></div>
              <div class="tiny muted">{{ selectedVisit.outbound.secondary }}</div>
            </div>
          </div>

          <!-- 底部操作按钮 -->
          <div class="panel-ft row" style="gap: 8px">
            <button
              type="button"
              class="btn btn-sm btn-danger"
              id="banBtn"
              :disabled="isCurrentIpBanned"
              @click="handleBanIp"
            >
              {{ isCurrentIpBanned ? '已加入 L-04 风控名单' : '加入风控名单' }}
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

        <!-- 拦截构成图表卡片 -->
        <div class="panel" data-od-id="intercept-breakdown">
          <div class="panel-hd">
            <div>
              <h2 style="font-size: 15px">拦截构成</h2>
              <p>今日 {{ blockedCountFormatted }} 次拦截的去向分布。</p>
            </div>
          </div>
          <div class="panel-bd">
            <div class="bars">
              <div class="bar-row">
                <span class="bar-lab">爬虫 UA</span>
                <span class="bar-track">
                  <span class="bar-fill t-danger" style="width: 58.8%"></span>
                </span>
                <span class="bar-val">4,406</span>
              </div>
              <div class="bar-row">
                <span class="bar-lab">代理 / 机房</span>
                <span class="bar-track">
                  <span class="bar-fill" style="width: 19.5%"></span>
                </span>
                <span class="bar-val">1,464</span>
              </div>
              <div class="bar-row">
                <span class="bar-lab">高频访问</span>
                <span class="bar-track">
                  <span class="bar-fill" style="width: 15.7%"></span>
                </span>
                <span class="bar-val">1,178</span>
              </div>
              <div class="bar-row">
                <span class="bar-lab">名单直接命中</span>
                <span class="bar-track">
                  <span class="bar-fill" style="width: 5.9%"></span>
                </span>
                <span class="bar-val">444</span>
              </div>
            </div>

            <!-- 告警提示卡片 -->
            <div class="note" style="margin-top: 14px">
              <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round">
                <path d="M12 9v4M12 17h.01M10.3 3.9 1.8 18a2 2 0 0 0 1.7 3h17a2 2 0 0 0 1.7-3L13.7 3.9a2 2 0 0 0-3.4 0z" />
              </svg>
              <div>
                <b>R-005 已停用 30 分钟，零命中。</b>它的条件被 R-001 完全覆盖。可能是停用误操作，建议在规则引擎里做一次冲突扫描。
              </div>
            </div>
          </div>
        </div>
      </div>
    </section>
  </div>
</template>

<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref } from 'vue';
import { useRouter } from 'vue-router';

const router = useRouter();

// ==================== 类型定义 ====================
export interface TraceFact {
  text: string;
  type?: 'hit' | 'miss' | 'neutral';
}

export interface TraceStep {
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

export interface VisitItem {
  id: string;
  time: string;
  timeWithMs: string;
  ip: string;
  country: string;
  device: string;
  lang: string;
  referrer: string;
  rawUa: string;
  ruleId: string;
  action: '放行' | '404' | '限流' | '白标';
  destPool?: string;
  latency: number;
  isNew?: boolean;
  shortlink: string;
  params: string;
  traceSteps: TraceStep[];
  outbound: {
    primary: string;
    secondary: string;
  };
}

// ==================== 顶部 KPI 响应式状态 ====================
const liveCount = ref(312);
const todayDecisions = ref(128406);
const blockedCount = ref(7492);

const todayDecisionsFormatted = computed(() => todayDecisions.value.toLocaleString());
const blockedCountFormatted = computed(() => blockedCount.value.toLocaleString());

// ==================== 访问流控制项 ====================
const autoScroll = ref(true);
const isPaused = ref(false);
const feedRef = ref<HTMLElement | null>(null);

function togglePause() {
  isPaused.value = !isPaused.value;
}

// ==================== 筛选状态 ====================
const activeActionFilter = ref('all');
const selectedCountry = ref('all');
const searchQuery = ref('');

const actionFilters = [
  { key: 'all', label: '全部' },
  { key: '放行', label: '放行' },
  { key: '404', label: '404' },
  { key: '限流', label: '限流' },
  { key: '白标', label: '白标' },
];

// ==================== 初始访客数据集 ====================
const initialVisits: VisitItem[] = [
  {
    id: 'v-1',
    time: '14:22:07',
    timeWithMs: '14:22:07.412',
    ip: '202.159.44.7',
    country: 'BR',
    device: 'iPhone 15 · iOS 18.1',
    lang: 'pt-BR',
    referrer: 'facebook.com',
    rawUa:
      'Mozilla/5.0 (iPhone; CPU iPhone OS 18_1 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/18.1 Mobile/15E148 Safari/604.1',
    ruleId: 'R-004',
    action: '放行',
    destPool: 'B',
    latency: 38,
    shortlink: 'go.northwind-media.com / vip-access',
    params: 'ttclid=9d1f2a7c',
    traceSteps: [
      {
        status: 'skip',
        name: 'R-006 内部测试强制放行',
        badgeText: '未命中',
        badgeClass: 'badge-neutral mono',
        facts: [{ text: 'IP 202.159.44.7', type: 'miss' }],
        whyText: '不在 L-01 内部测试网段',
      },
      {
        status: 'skip',
        name: 'R-001 目标市场 · 移动端放行',
        badgeText: '未命中',
        badgeClass: 'badge-neutral mono',
        facts: [
          { text: '设备类型 = 移动', type: 'hit' },
          { text: '国家 BR', type: 'miss' },
        ],
        whyText: '不在 L-05 目标放行国家',
      },
      {
        status: 'hit',
        name: 'R-004 语言分流 · 葡语市场',
        badgeText: '命中 · 裁决',
        badgeClass: 'badge-ok mono',
        facts: [
          { text: '语言 = pt-BR', type: 'hit' },
          { text: '设备 ≠ 爬虫', type: 'hit' },
        ],
        actionText: '放行',
        destText: '目标池 B（权重 1:1 选出 <span class="mono">acesso-vip.com.br</span>）',
      },
      {
        status: 'skip',
        name: 'R-005 / R-007 / R-008',
        skipReason: '首条命中即裁决，剩余 3 条未求值（节省 0.4ms）',
      },
    ],
    outbound: {
      primary: '302 → <span class="mono" style="color:var(--fg)">https://acesso-vip.com.br/entrada</span>',
      secondary: 'Referer 已剥离 · 随机延迟 214ms · 无 Cookie',
    },
  },
  {
    id: 'v-2',
    time: '14:21:58',
    timeWithMs: '14:21:58.820',
    ip: '104.28.64.7',
    country: 'US',
    device: 'Pixel 8 · Android 14',
    lang: 'en-US',
    referrer: 'googleads',
    rawUa:
      'Mozilla/5.0 (Linux; Android 14; Pixel 8) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.6778.85 Mobile Safari/537.36',
    ruleId: 'R-001',
    action: '放行',
    destPool: 'A',
    latency: 41,
    shortlink: 'go.northwind-media.com / vip-access',
    params: 'gclid=CjwKCAiA_x1',
    traceSteps: [
      {
        status: 'skip',
        name: 'R-006 内部测试强制放行',
        badgeText: '未命中',
        badgeClass: 'badge-neutral mono',
        facts: [{ text: 'IP 104.28.64.7', type: 'miss' }],
        whyText: '不在 L-01 内部测试网段',
      },
      {
        status: 'hit',
        name: 'R-001 目标市场 · 移动端放行',
        badgeText: '命中 · 裁决',
        badgeClass: 'badge-ok mono',
        facts: [
          { text: '设备类型 = 移动', type: 'hit' },
          { text: '国家 = US', type: 'hit' },
          { text: '网络 = 住宅IP', type: 'hit' },
        ],
        actionText: '放行',
        destText: '目标池 A（轮询选出 <span class="mono">target-landing-1.com</span>）',
      },
      {
        status: 'skip',
        name: 'R-004 / R-005 / R-007 / R-008',
        skipReason: '首条命中即裁决，剩余 4 条未求值（节省 0.5ms）',
      },
    ],
    outbound: {
      primary: '302 → <span class="mono" style="color:var(--fg)">https://target-landing-1.com/offer</span>',
      secondary: 'Referer 已剥离 · 随机延迟 182ms · 无 Cookie',
    },
  },
  {
    id: 'v-3',
    time: '14:21:44',
    timeWithMs: '14:21:44.204',
    ip: '157.240.1.35',
    country: 'US',
    device: '— (无 UA)',
    lang: 'en-US',
    referrer: 'facebookexternalhit',
    rawUa: 'facebookexternalhit/1.1 (+http://www.facebook.com/externalhit_uatext.php)',
    ruleId: 'R-002',
    action: '404',
    latency: 3,
    shortlink: 'go.northwind-media.com / vip-access',
    params: 'fbclid=IwAR27abc',
    traceSteps: [
      {
        status: 'skip',
        name: 'R-006 内部测试强制放行',
        badgeText: '未命中',
        badgeClass: 'badge-neutral mono',
        facts: [{ text: 'IP 157.240.1.35', type: 'miss' }],
        whyText: '不在 L-01 内部测试网段',
      },
      {
        status: 'block',
        name: 'R-002 爬虫防护 · 平台审查员拦截',
        badgeText: '命中 · 阻断',
        badgeClass: 'badge-danger mono',
        facts: [
          { text: 'UA 匹配 facebookexternalhit', type: 'hit' },
          { text: '来源 = L-02 审查爬虫段', type: 'hit' },
        ],
        actionText: '阻断 (404)',
        destText: '丢弃连接并返回 404 Not Found',
      },
      {
        status: 'skip',
        name: 'R-001 / R-003 / R-004 / R-005',
        skipReason: '阻断规则生效，剩余 5 条未求值（节省 0.8ms）',
      },
    ],
    outbound: {
      primary: 'HTTP 404 Not Found · 立即中断连接',
      secondary: '响应体 0 字节 · 无重定向 · 延迟 1ms',
    },
  },
  {
    id: 'v-4',
    time: '14:21:31',
    timeWithMs: '14:21:31.902',
    ip: '45.83.12.190',
    country: 'US',
    device: 'Desktop · Windows 11',
    lang: 'en-US',
    referrer: 'l.facebook.com',
    rawUa:
      'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36',
    ruleId: 'R-008',
    action: '白标',
    latency: 22,
    shortlink: 'go.northwind-media.com / vip-access',
    params: 'fbclid=IwAR31xyz',
    traceSteps: [
      {
        status: 'skip',
        name: 'R-006 内部测试强制放行',
        badgeText: '未命中',
        badgeClass: 'badge-neutral mono',
        facts: [{ text: 'IP 45.83.12.190', type: 'miss' }],
        whyText: '不在 L-01 内部测试网段',
      },
      {
        status: 'skip',
        name: 'R-001 目标市场 · 移动端放行',
        badgeText: '未命中',
        badgeClass: 'badge-neutral mono',
        facts: [{ text: '设备类型 = 桌面', type: 'miss' }],
        whyText: '未匹配移动端放行规则',
      },
      {
        status: 'hit',
        name: 'R-008 审查员白标页伪装',
        badgeText: '命中 · 裁决',
        badgeClass: 'badge-warn mono',
        facts: [
          { text: '来源 = l.facebook.com', type: 'hit' },
          { text: '机房 IP 标记', type: 'hit' },
        ],
        actionText: '白标',
        destText: '展示合规白标内容页（安全文章静态模板）',
      },
      {
        status: 'skip',
        name: 'R-003 / R-004 / R-005',
        skipReason: '伪装规则生效，阻断真实投放目标',
      },
    ],
    outbound: {
      primary: '200 OK · 返回伪装白标落地页 (blog-template-safe)',
      secondary: '安全内容伪装 · 剥离追踪像素 · 正常渲染',
    },
  },
  {
    id: 'v-5',
    time: '14:21:19',
    timeWithMs: '14:21:19.450',
    ip: '198.51.100.22',
    country: 'DE',
    device: 'Desktop · Chrome 131',
    lang: 'de-DE',
    referrer: '直接访问',
    rawUa:
      'Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36',
    ruleId: 'R-005',
    action: '放行',
    destPool: 'A',
    latency: 29,
    shortlink: 'go.northwind-media.com / vip-access',
    params: '—',
    traceSteps: [
      {
        status: 'skip',
        name: 'R-006 内部测试强制放行',
        badgeText: '未命中',
        badgeClass: 'badge-neutral mono',
        facts: [{ text: 'IP 198.51.100.22', type: 'miss' }],
        whyText: '不在 L-01 内部测试网段',
      },
      {
        status: 'skip',
        name: 'R-001 目标市场 · 移动端放行',
        badgeText: '未命中',
        badgeClass: 'badge-neutral mono',
        facts: [{ text: '设备类型 = 桌面', type: 'miss' }],
        whyText: '非移动端流量',
      },
      {
        status: 'hit',
        name: 'R-005 欧洲桌面流量放行',
        badgeText: '命中 · 裁决',
        badgeClass: 'badge-ok mono',
        facts: [
          { text: '国家 = DE', type: 'hit' },
          { text: '系统语言 = de-DE', type: 'hit' },
        ],
        actionText: '放行',
        destText: '目标池 A（轮询选出 <span class="mono">target-landing-1.com</span>）',
      },
      {
        status: 'skip',
        name: 'R-007 / R-008',
        skipReason: '首条命中即裁决，剩余规则跳过',
      },
    ],
    outbound: {
      primary: '302 → <span class="mono" style="color:var(--fg)">https://target-landing-1.com/offer</span>',
      secondary: 'Referer 已剥离 · 随机延迟 190ms · 无 Cookie',
    },
  },
  {
    id: 'v-6',
    time: '14:21:07',
    timeWithMs: '14:21:07.110',
    ip: '91.243.44.10',
    country: 'NL',
    device: '服务器 · 无浏览器',
    lang: '—',
    referrer: '直接访问',
    rawUa: 'curl/8.4.0',
    ruleId: 'R-003',
    action: '限流',
    latency: 2,
    shortlink: 'go.northwind-media.com / vip-access',
    params: '—',
    traceSteps: [
      {
        status: 'skip',
        name: 'R-006 内部测试强制放行',
        badgeText: '未命中',
        badgeClass: 'badge-neutral mono',
        facts: [{ text: 'IP 91.243.44.10', type: 'miss' }],
        whyText: '不在内部测试网段',
      },
      {
        status: 'skip',
        name: 'R-002 爬虫防护',
        badgeText: '未命中',
        badgeClass: 'badge-neutral mono',
        facts: [{ text: '无已知黑名单签名', type: 'miss' }],
        whyText: '未命中黑名单 UA',
      },
      {
        status: 'block',
        name: 'R-003 频次限制 · 单 IP 高频防护',
        badgeText: '命中 · 限流',
        badgeClass: 'badge-warn mono',
        facts: [
          { text: '近 60s 频次 58 次/分', type: 'hit' },
          { text: '阈值 > 30', type: 'hit' },
        ],
        actionText: '限流',
        destText: '响应 HTTP 429 Too Many Requests（封禁 5 分钟）',
      },
      {
        status: 'skip',
        name: 'R-001 / R-004 / R-005',
        skipReason: '频控规则生效，剩余路由跳过',
      },
    ],
    outbound: {
      primary: 'HTTP 429 Too Many Requests · Retry-After: 300s',
      secondary: '触发频控封禁 · 来源 IP 限速 5 分钟',
    },
  },
  {
    id: 'v-7',
    time: '14:20:58',
    timeWithMs: '14:20:58.601',
    ip: '177.54.144.9',
    country: 'BR',
    device: 'Galaxy S23 · Android 14',
    lang: 'pt-BR',
    referrer: 'instagram.com',
    rawUa:
      'Mozilla/5.0 (Linux; Android 14; SM-S911B) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/130.0.0.0 Mobile Safari/537.36',
    ruleId: 'R-001',
    action: '放行',
    destPool: 'A',
    latency: 44,
    shortlink: 'go.northwind-media.com / vip-access',
    params: 'igshid=MzRlODBiNWFlZA==',
    traceSteps: [
      {
        status: 'skip',
        name: 'R-006 内部测试强制放行',
        badgeText: '未命中',
        badgeClass: 'badge-neutral mono',
        facts: [{ text: 'IP 177.54.144.9', type: 'miss' }],
        whyText: '不在内部测试网段',
      },
      {
        status: 'hit',
        name: 'R-001 目标市场 · 移动端放行',
        badgeText: '命中 · 裁决',
        badgeClass: 'badge-ok mono',
        facts: [
          { text: '设备类型 = 移动', type: 'hit' },
          { text: '国家 = BR', type: 'hit' },
          { text: '真人环境验证', type: 'hit' },
        ],
        actionText: '放行',
        destText: '目标池 A（轮询选出 <span class="mono">target-landing-1.com</span>）',
      },
      {
        status: 'skip',
        name: 'R-004 / R-005 / R-007',
        skipReason: '首条命中即裁决，剩余规则跳过',
      },
    ],
    outbound: {
      primary: '302 → <span class="mono" style="color:var(--fg)">https://target-landing-1.com/offer</span>',
      secondary: 'Referer 已剥离 · 随机延迟 176ms · 无 Cookie',
    },
  },
  {
    id: 'v-8',
    time: '14:20:41',
    timeWithMs: '14:20:41.332',
    ip: '52.95.245.14',
    country: 'US',
    device: 'Desktop · Chrome 131',
    lang: 'en-US',
    referrer: '直接访问',
    rawUa:
      'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36',
    ruleId: 'R-003',
    action: '限流',
    latency: 2,
    shortlink: 'go.northwind-media.com / vip-access',
    params: '—',
    traceSteps: [
      {
        status: 'skip',
        name: 'R-006 内部测试强制放行',
        badgeText: '未命中',
        badgeClass: 'badge-neutral mono',
        facts: [{ text: 'IP 52.95.245.14', type: 'miss' }],
        whyText: '不在内部测试网段',
      },
      {
        status: 'skip',
        name: 'R-002 爬虫防护',
        badgeText: '未命中',
        badgeClass: 'badge-neutral mono',
        facts: [{ text: '普通浏览器 UA', type: 'miss' }],
        whyText: '未匹配自动化爬虫签名',
      },
      {
        status: 'block',
        name: 'R-003 频次限制 · 单 IP 高频防护',
        badgeText: '命中 · 限流',
        badgeClass: 'badge-warn mono',
        facts: [
          { text: '近 60s 频次 42 次/分', type: 'hit' },
          { text: '单 IP 速率上限 30 次/分', type: 'hit' },
        ],
        actionText: '限流',
        destText: '响应 HTTP 429 Too Many Requests（封禁 5 分钟）',
      },
      {
        status: 'skip',
        name: 'R-001 / R-004 / R-005',
        skipReason: '频控规则生效，剩余路由跳过',
      },
    ],
    outbound: {
      primary: 'HTTP 429 Too Many Requests · Retry-After: 300s',
      secondary: '触发频控封禁 · 来源 IP 限速 5 分钟',
    },
  },
  {
    id: 'v-9',
    time: '14:20:29',
    timeWithMs: '14:20:29.871',
    ip: '189.45.71.13',
    country: 'BR',
    device: 'iPhone 14 · iOS 17.6',
    lang: 'pt-BR',
    referrer: 'tiktok.com',
    rawUa:
      'Mozilla/5.0 (iPhone; CPU iPhone OS 17_6 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.6 Mobile/15E148 Safari/604.1',
    ruleId: 'R-004',
    action: '放行',
    destPool: 'B',
    latency: 47,
    shortlink: 'go.northwind-media.com / vip-access',
    params: 'ttclid=7a8b9c0d',
    traceSteps: [
      {
        status: 'skip',
        name: 'R-006 内部测试强制放行',
        badgeText: '未命中',
        badgeClass: 'badge-neutral mono',
        facts: [{ text: 'IP 189.45.71.13', type: 'miss' }],
        whyText: '不在内部测试网段',
      },
      {
        status: 'skip',
        name: 'R-001 目标市场 · 移动端放行',
        badgeText: '未命中',
        badgeClass: 'badge-neutral mono',
        facts: [{ text: '国家 BR', type: 'miss' }],
        whyText: '不在 L-05 默认放行国家',
      },
      {
        status: 'hit',
        name: 'R-004 语言分流 · 葡语市场',
        badgeText: '命中 · 裁决',
        badgeClass: 'badge-ok mono',
        facts: [
          { text: '语言 = pt-BR', type: 'hit' },
          { text: '来源 = tiktok.com', type: 'hit' },
        ],
        actionText: '放行',
        destText: '目标池 B（权重 1:1 选出 <span class="mono">acesso-vip.com.br</span>）',
      },
      {
        status: 'skip',
        name: 'R-005 / R-007 / R-008',
        skipReason: '首条命中即裁决，剩余规则跳过',
      },
    ],
    outbound: {
      primary: '302 → <span class="mono" style="color:var(--fg)">https://acesso-vip.com.br/entrada</span>',
      secondary: 'Referer 已剥离 · 随机延迟 210ms · 无 Cookie',
    },
  },
  {
    id: 'v-10',
    time: '14:20:12',
    timeWithMs: '14:20:12.540',
    ip: '89.187.164.3',
    country: 'DE',
    device: 'Galaxy S23 · Android 13',
    lang: 'de-DE',
    referrer: 'googleads',
    rawUa:
      'Mozilla/5.0 (Linux; Android 13; SM-S911B) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/128.0.0.0 Mobile Safari/537.36',
    ruleId: 'R-001',
    action: '放行',
    destPool: 'A',
    latency: 36,
    shortlink: 'go.northwind-media.com / vip-access',
    params: 'gclid=EAIaIQobChMI',
    traceSteps: [
      {
        status: 'skip',
        name: 'R-006 内部测试强制放行',
        badgeText: '未命中',
        badgeClass: 'badge-neutral mono',
        facts: [{ text: 'IP 89.187.164.3', type: 'miss' }],
        whyText: '不在内部测试网段',
      },
      {
        status: 'hit',
        name: 'R-001 目标市场 · 移动端放行',
        badgeText: '命中 · 裁决',
        badgeClass: 'badge-ok mono',
        facts: [
          { text: '设备类型 = 移动', type: 'hit' },
          { text: '国家 = DE', type: 'hit' },
        ],
        actionText: '放行',
        destText: '目标池 A（轮询选出 <span class="mono">target-landing-1.com</span>）',
      },
      {
        status: 'skip',
        name: 'R-004 / R-005 / R-007',
        skipReason: '首条命中即裁决，剩余规则跳过',
      },
    ],
    outbound: {
      primary: '302 → <span class="mono" style="color:var(--fg)">https://target-landing-1.com/offer</span>',
      secondary: 'Referer 已剥离 · 随机延迟 195ms · 无 Cookie',
    },
  },
  {
    id: 'v-11',
    time: '14:19:58',
    timeWithMs: '14:19:58.118',
    ip: '185.220.101.9',
    country: 'DE',
    device: '— (无 UA)',
    lang: '—',
    referrer: '爬虫',
    rawUa: 'Python-urllib/3.10',
    ruleId: 'R-002',
    action: '404',
    latency: 1,
    shortlink: 'go.northwind-media.com / vip-access',
    params: '—',
    traceSteps: [
      {
        status: 'skip',
        name: 'R-006 内部测试强制放行',
        badgeText: '未命中',
        badgeClass: 'badge-neutral mono',
        facts: [{ text: 'IP 185.220.101.9', type: 'miss' }],
        whyText: '不在内部测试网段',
      },
      {
        status: 'block',
        name: 'R-002 爬虫防护 · 空 UA 与审查员拦截',
        badgeText: '命中 · 阻断',
        badgeClass: 'badge-danger mono',
        facts: [
          { text: 'UA = Python-urllib', type: 'hit' },
          { text: 'Tor 出口节点', type: 'hit' },
        ],
        actionText: '阻断 (404)',
        destText: '丢弃连接并返回 404 Not Found',
      },
      {
        status: 'skip',
        name: 'R-001 / R-003 / R-004',
        skipReason: '阻断规则生效，剩余规则跳过',
      },
    ],
    outbound: {
      primary: 'HTTP 404 Not Found · 立即中断连接',
      secondary: '响应体 0 字节 · 无重定向 · 延迟 1ms',
    },
  },
  {
    id: 'v-12',
    time: '14:19:44',
    timeWithMs: '14:19:44.752',
    ip: '203.0.113.45',
    country: 'US',
    device: 'MacBook · Safari 18',
    lang: 'en-US',
    referrer: '直接访问',
    rawUa:
      'Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/18.0 Safari/605.1.15',
    ruleId: 'R-006',
    action: '放行',
    destPool: 'A',
    latency: 18,
    shortlink: 'go.northwind-media.com / vip-access',
    params: 'test=1&debug=true',
    traceSteps: [
      {
        status: 'hit',
        name: 'R-006 内部测试强制放行',
        badgeText: '命中 · 裁决',
        badgeClass: 'badge-ok mono',
        facts: [
          { text: 'IP 203.0.113.45', type: 'hit' },
          { text: '属于 L-01 内部测试网段', type: 'hit' },
        ],
        actionText: '放行',
        destText: '目标池 A（白名单内部直达）',
      },
      {
        status: 'skip',
        name: 'R-001 / R-002 / R-003 / R-004',
        skipReason: '首条命中即裁决，跳过全部风控校验',
      },
    ],
    outbound: {
      primary: '302 → <span class="mono" style="color:var(--fg)">https://target-landing-1.com/offer</span>',
      secondary: '测试通道直达 · Referer 保留 · 延迟 18ms',
    },
  },
];

const visits = ref<VisitItem[]>([...initialVisits]);
const selectedVisit = ref<VisitItem | null>(visits.value[0]);

// ==================== 过滤计算属性 ====================
const filteredVisits = computed(() => {
  const act = activeActionFilter.value;
  const cc = selectedCountry.value;
  const q = searchQuery.value.trim().toLowerCase();

  return visits.value.filter((r) => {
    const okA = act === 'all' || r.action === act;
    const okC = cc === 'all' || r.country === cc;
    if (!okA || !okC) return false;
    if (!q) return true;

    return (
      r.ip.toLowerCase().includes(q) ||
      r.device.toLowerCase().includes(q) ||
      r.referrer.toLowerCase().includes(q) ||
      r.ruleId.toLowerCase().includes(q) ||
      r.country.toLowerCase().includes(q)
    );
  });
});

// ==================== 行选择与联动 ====================
function selectVisit(item: VisitItem) {
  selectedVisit.value = item;
}

// 最终裁决 Badge 文本
const verdictBadgeText = computed(() => {
  if (!selectedVisit.value) return '—';
  const { action, ruleId, destPool } = selectedVisit.value;
  if (action === '放行') {
    return destPool ? `放行 → ${destPool} · ${ruleId}` : `放行 · ${ruleId}`;
  }
  return `${action} · ${ruleId}`;
});

function getActionBadgeClass(action: string) {
  switch (action) {
    case '放行':
      return 'badge-ok';
    case '404':
      return 'badge-danger';
    case '限流':
    case '白标':
    default:
      return 'badge-warn';
  }
}

function formatActionText(item: VisitItem) {
  if (item.action === '放行' && item.destPool) {
    return `放行 → ${item.destPool}`;
  }
  return item.action;
}

// ==================== 风控名单操作 ====================
const bannedIps = ref<Set<string>>(new Set());

const isCurrentIpBanned = computed(() => {
  return selectedVisit.value ? bannedIps.value.has(selectedVisit.value.ip) : false;
});

function handleBanIp() {
  if (!selectedVisit.value) return;
  bannedIps.value.add(selectedVisit.value.ip);
}

// ==================== 模拟器联动跳转 ====================
function openInSimulator() {
  if (!selectedVisit.value) return;
  router.push({
    path: '/rules',
    query: {
      tab: 'simulator',
      ip: selectedVisit.value.ip,
      ua: selectedVisit.value.rawUa || selectedVisit.value.device,
      lang: selectedVisit.value.lang !== '—' ? selectedVisit.value.lang : 'en-US',
    },
  });
}

// ==================== 实时动态流量生成 ====================
let timer: ReturnType<typeof setInterval> | null = null;

const GENERATOR_TEMPLATES = [
  {
    device: 'iPhone 15 · iOS 18.1',
    lang: 'pt-BR',
    referrer: 'facebook.com',
    country: 'BR',
    ruleId: 'R-004',
    action: '放行' as const,
    destPool: 'B',
    rawUa:
      'Mozilla/5.0 (iPhone; CPU iPhone OS 18_1 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/18.1 Mobile/15E148 Safari/604.1',
    shortlink: 'go.northwind-media.com / vip-access',
    params: 'ttclid=9d1f2a7c',
    destUrl: 'https://acesso-vip.com.br/entrada',
  },
  {
    device: 'Galaxy S23 · Android 14',
    lang: 'en-US',
    referrer: 'tiktok.com',
    country: 'US',
    ruleId: 'R-001',
    action: '放行' as const,
    destPool: 'A',
    rawUa:
      'Mozilla/5.0 (Linux; Android 14; SM-S911B) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/130.0.0.0 Mobile Safari/537.36',
    shortlink: 'go.northwind-media.com / vip-access',
    params: 'ttclid=3f8a9e1b',
    destUrl: 'https://target-landing-1.com/offer',
  },
  {
    device: 'Pixel 8 · Android 14',
    lang: 'en-US',
    referrer: 'googleads',
    country: 'US',
    ruleId: 'R-001',
    action: '放行' as const,
    destPool: 'A',
    rawUa:
      'Mozilla/5.0 (Linux; Android 14; Pixel 8) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.6778.85 Mobile Safari/537.36',
    shortlink: 'go.northwind-media.com / vip-access',
    params: 'gclid=EAIaIQobChMI',
    destUrl: 'https://target-landing-1.com/offer',
  },
  {
    device: 'Desktop · Chrome 131',
    lang: 'de-DE',
    referrer: 'googleads',
    country: 'DE',
    ruleId: 'R-004',
    action: '放行' as const,
    destPool: 'B',
    rawUa:
      'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36',
    shortlink: 'go.northwind-media.com / vip-access',
    params: 'gclid=CjwKCAiA_x1',
    destUrl: 'https://acesso-vip.com.br/entrada',
  },
  {
    device: 'iPad Air · iPadOS 18',
    lang: 'es-ES',
    referrer: 'instagram.com',
    country: 'BR',
    ruleId: 'R-001',
    action: '放行' as const,
    destPool: 'A',
    rawUa:
      'Mozilla/5.0 (iPad; CPU OS 18_0 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/18.0 Mobile/15E148 Safari/604.1',
    shortlink: 'go.northwind-media.com / vip-access',
    params: 'igshid=MzRlODBiNWFlZA==',
    destUrl: 'https://target-landing-1.com/offer',
  },
  {
    device: '— (无 UA)',
    lang: 'en-US',
    referrer: 'facebookexternalhit',
    country: 'US',
    ruleId: 'R-002',
    action: '404' as const,
    destPool: undefined,
    rawUa: 'facebookexternalhit/1.1 (+http://www.facebook.com/externalhit_uatext.php)',
    shortlink: 'go.northwind-media.com / vip-access',
    params: 'fbclid=IwAR27abc',
    destUrl: '',
  },
  {
    device: '服务器 · 无浏览器',
    lang: '—',
    referrer: '直接访问',
    country: 'NL',
    ruleId: 'R-003',
    action: '限流' as const,
    destPool: undefined,
    rawUa: 'curl/8.4.0',
    shortlink: 'go.northwind-media.com / vip-access',
    params: '—',
    destUrl: '',
  },
  {
    device: 'Desktop · Windows 11',
    lang: 'en-US',
    referrer: 'l.facebook.com',
    country: 'US',
    ruleId: 'R-008',
    action: '白标' as const,
    destPool: undefined,
    rawUa:
      'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36',
    shortlink: 'go.northwind-media.com / vip-access',
    params: 'fbclid=IwAR31xyz',
    destUrl: '',
  },
  {
    device: 'iPhone 16 · iOS 18.2',
    lang: 'ja-JP',
    referrer: 'twitter.com',
    country: 'JP',
    ruleId: 'R-001',
    action: '放行' as const,
    destPool: 'A',
    rawUa:
      'Mozilla/5.0 (iPhone; CPU iPhone OS 18_2 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/18.2 Mobile/15E148 Safari/604.1',
    shortlink: 'go.northwind-media.com / vip-access',
    params: 'twclid=8c7b6a',
    destUrl: 'https://target-landing-1.com/offer',
  },
  {
    device: 'Desktop · Firefox 133',
    lang: 'en-GB',
    referrer: '直接访问',
    country: 'GB',
    ruleId: 'R-005',
    action: '放行' as const,
    destPool: 'A',
    rawUa: 'Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:133.0) Gecko/20100101 Firefox/133.0',
    shortlink: 'go.northwind-media.com / vip-access',
    params: '—',
    destUrl: 'https://target-landing-1.com/offer',
  },
];

function generateRandomIp(country: string): string {
  switch (country) {
    case 'BR':
      return `${Math.floor(Math.random() * 20 + 177)}.${Math.floor(Math.random() * 250)}.${Math.floor(Math.random() * 250)}.${Math.floor(Math.random() * 250 + 2)}`;
    case 'US':
      return `${Math.floor(Math.random() * 150 + 50)}.${Math.floor(Math.random() * 250)}.${Math.floor(Math.random() * 250)}.${Math.floor(Math.random() * 250 + 2)}`;
    case 'DE':
      return `89.${Math.floor(Math.random() * 100 + 100)}.${Math.floor(Math.random() * 250)}.${Math.floor(Math.random() * 250 + 2)}`;
    case 'NL':
      return `91.${Math.floor(Math.random() * 100 + 140)}.${Math.floor(Math.random() * 250)}.${Math.floor(Math.random() * 250 + 2)}`;
    case 'JP':
      return `133.${Math.floor(Math.random() * 100 + 140)}.${Math.floor(Math.random() * 250)}.${Math.floor(Math.random() * 250 + 2)}`;
    case 'GB':
      return `82.${Math.floor(Math.random() * 100 + 100)}.${Math.floor(Math.random() * 250)}.${Math.floor(Math.random() * 250 + 2)}`;
    default:
      return `${Math.floor(Math.random() * 200 + 20)}.${Math.floor(Math.random() * 250)}.${Math.floor(Math.random() * 250)}.${Math.floor(Math.random() * 250 + 2)}`;
  }
}

function buildTraceStepsForTemplate(
  tmpl: (typeof GENERATOR_TEMPLATES)[0],
  ip: string,
): { steps: TraceStep[]; outbound: { primary: string; secondary: string } } {
  if (tmpl.action === '404') {
    return {
      steps: [
        {
          status: 'skip',
          name: 'R-006 内部测试强制放行',
          badgeText: '未命中',
          badgeClass: 'badge-neutral mono',
          facts: [{ text: `IP ${ip}`, type: 'miss' }],
          whyText: '不在 L-01 内部测试网段',
        },
        {
          status: 'block',
          name: 'R-002 爬虫防护 · 自动化爬虫拦截',
          badgeText: '命中 · 阻断',
          badgeClass: 'badge-danger mono',
          facts: [
            { text: `特征匹配 ${tmpl.referrer}`, type: 'hit' },
            { text: '无真实交互能力', type: 'hit' },
          ],
          actionText: '阻断 (404)',
          destText: '丢弃连接并返回 404 Not Found',
        },
        {
          status: 'skip',
          name: 'R-001 / R-003 / R-004 / R-005',
          skipReason: '阻断规则生效，剩余 5 条未求值（节省 0.8ms）',
        },
      ],
      outbound: {
        primary: 'HTTP 404 Not Found · 立即中断连接',
        secondary: '响应体 0 字节 · 无重定向 · 延迟 1ms',
      },
    };
  }

  if (tmpl.action === '限流') {
    return {
      steps: [
        {
          status: 'skip',
          name: 'R-006 内部测试强制放行',
          badgeText: '未命中',
          badgeClass: 'badge-neutral mono',
          facts: [{ text: `IP ${ip}`, type: 'miss' }],
          whyText: '不在 L-01 内部测试网段',
        },
        {
          status: 'skip',
          name: 'R-002 爬虫防护',
          badgeText: '未命中',
          badgeClass: 'badge-neutral mono',
          facts: [{ text: '无已知黑名单签名', type: 'miss' }],
          whyText: '未命中已知黑名单 UA',
        },
        {
          status: 'block',
          name: 'R-003 频次限制 · 单 IP 高频防护',
          badgeText: '命中 · 限流',
          badgeClass: 'badge-warn mono',
          facts: [
            { text: '近 60s 频次 46 次/分', type: 'hit' },
            { text: '阈值 > 30', type: 'hit' },
          ],
          actionText: '限流',
          destText: '响应 HTTP 429 Too Many Requests（封禁 5 分钟）',
        },
        {
          status: 'skip',
          name: 'R-001 / R-004 / R-005',
          skipReason: '频控规则生效，剩余路由跳过',
        },
      ],
      outbound: {
        primary: 'HTTP 429 Too Many Requests · Retry-After: 300s',
        secondary: '触发频控封禁 · 来源 IP 限速 5 分钟',
      },
    };
  }

  if (tmpl.action === '白标') {
    return {
      steps: [
        {
          status: 'skip',
          name: 'R-006 内部测试强制放行',
          badgeText: '未命中',
          badgeClass: 'badge-neutral mono',
          facts: [{ text: `IP ${ip}`, type: 'miss' }],
          whyText: '不在 L-01 内部测试网段',
        },
        {
          status: 'skip',
          name: 'R-001 目标市场 · 移动端放行',
          badgeText: '未命中',
          badgeClass: 'badge-neutral mono',
          facts: [{ text: '设备类型 = 桌面', type: 'miss' }],
          whyText: '未匹配移动端放行规则',
        },
        {
          status: 'hit',
          name: 'R-008 审查员白标页伪装',
          badgeText: '命中 · 裁决',
          badgeClass: 'badge-warn mono',
          facts: [
            { text: `来源 = ${tmpl.referrer}`, type: 'hit' },
            { text: '机房代理标记', type: 'hit' },
          ],
          actionText: '白标',
          destText: '展示合规白标内容页（安全文章静态模板）',
        },
        {
          status: 'skip',
          name: 'R-003 / R-004 / R-005',
          skipReason: '伪装规则生效，阻断真实投放目标',
        },
      ],
      outbound: {
        primary: '200 OK · 返回伪装白标落地页 (blog-template-safe)',
        secondary: '安全内容伪装 · 剥离追踪像素 · 正常渲染',
      },
    };
  }

  // 放行
  return {
    steps: [
      {
        status: 'skip',
        name: 'R-006 内部测试强制放行',
        badgeText: '未命中',
        badgeClass: 'badge-neutral mono',
        facts: [{ text: `IP ${ip}`, type: 'miss' }],
        whyText: '不在 L-01 内部测试网段',
      },
      {
        status: 'hit',
        name: `${tmpl.ruleId} 目标市场准入规则`,
        badgeText: '命中 · 裁决',
        badgeClass: 'badge-ok mono',
        facts: [
          { text: `国家 = ${tmpl.country}`, type: 'hit' },
          { text: `语言 = ${tmpl.lang}`, type: 'hit' },
        ],
        actionText: '放行',
        destText: `目标池 ${tmpl.destPool || 'A'}（权重分配选出 <span class="mono">${tmpl.destUrl.replace('https://', '')}</span>）`,
      },
      {
        status: 'skip',
        name: 'R-005 / R-007 / R-008',
        skipReason: '首条命中即裁决，剩余 3 条未求值（节省 0.4ms）',
      },
    ],
    outbound: {
      primary: `302 → <span class="mono" style="color:var(--fg)">${tmpl.destUrl}</span>`,
      secondary: 'Referer 已剥离 · 随机延迟 198ms · 无 Cookie',
    },
  };
}

function spawnLiveVisit() {
  if (isPaused.value) return;

  const tmpl = GENERATOR_TEMPLATES[Math.floor(Math.random() * GENERATOR_TEMPLATES.length)];
  const now = new Date();
  const time = [now.getHours(), now.getMinutes(), now.getSeconds()]
    .map((x) => String(x).padStart(2, '0'))
    .join(':');
  const ms = String(now.getMilliseconds()).padStart(3, '0');
  const timeWithMs = `${time}.${ms}`;
  const ip = generateRandomIp(tmpl.country);
  const latency = Math.floor(Math.random() * 24 + (tmpl.action === '404' || tmpl.action === '限流' ? 1 : 28));

  const { steps, outbound } = buildTraceStepsForTemplate(tmpl, ip);

  const newVisit: VisitItem = {
    id: `v-live-${Date.now()}-${Math.floor(Math.random() * 1000)}`,
    time,
    timeWithMs,
    ip,
    country: tmpl.country,
    device: tmpl.device,
    lang: tmpl.lang,
    referrer: tmpl.referrer,
    rawUa: tmpl.rawUa,
    ruleId: tmpl.ruleId,
    action: tmpl.action,
    destPool: tmpl.destPool,
    latency,
    isNew: true,
    shortlink: tmpl.shortlink,
    params: tmpl.params,
    traceSteps: steps,
    outbound,
  };

  // 插入顶部
  visits.value.unshift(newVisit);

  // 维持上限 50 条
  if (visits.value.length > 50) {
    visits.value.pop();
  }

  // 动态更新指标计数
  todayDecisions.value++;
  if (tmpl.action === '404' || tmpl.action === '限流') {
    blockedCount.value++;
  }
  // 在线访客微幅抖动
  liveCount.value = Math.max(300, Math.min(330, liveCount.value + (Math.random() > 0.48 ? 1 : -1)));

  // 动画结束后清除 isNew 标记
  setTimeout(() => {
    newVisit.isNew = false;
  }, 1000);

  // 自动滚动响应
  if (autoScroll.value && feedRef.value) {
    nextTick(() => {
      const el = feedRef.value;
      if (el) {
        const rect = el.getBoundingClientRect();
        if (rect.top < 70) {
          el.scrollIntoView({ behavior: 'smooth', block: 'nearest' });
        }
      }
    });
  }
}

// ==================== 生命周期 ====================
onMounted(() => {
  timer = setInterval(spawnLiveVisit, 3800);
});

onBeforeUnmount(() => {
  if (timer) {
    clearInterval(timer);
    timer = null;
  }
});
</script>
