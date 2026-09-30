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
      <!-- ==================== 访问明细主面板 ==================== -->
      <!--
        这里原本挂着一排「访问次数 / 点击次数 / CTR」卡片,已移除:这三个数就是短链列表里
        那一行的两列,用户是点着它进来的,再摆一遍只占首屏。点击与访问的对比在列表页看;
        要按动作拆,下方表格的「动作与结果」列里有。
      -->
      <section class="panel" data-od-id="link-visits-list">
        <div class="panel-hd">
          <div>
            <h2>访问明细</h2>
            <p>逐条记录该短链每次{{ isLanding ? '落地页视图与按钮点击' : '跳转' }}的动作与结果，点行展开决策链。</p>
          </div>
          <div class="btn-row">
            <span class="badge badge-neutral mono">共 {{ total }} 条</span>
          </div>
        </div>

        <!-- 工具栏:设备类型筛选 / 只看失败 / 关键词(均仅在当前页数据内过滤) -->
        <div class="panel-bd">
          <div class="toolbar">
            <div class="seg-filter" role="group" aria-label="按设备类型筛选">
              <button
                v-for="opt in DEVICE_OPTIONS"
                :key="opt.value"
                type="button"
                :aria-pressed="deviceFilter === opt.value"
                @click="deviceFilter = opt.value"
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
              设备、搜索与「只看失败」都是页内过滤，这里常驻一行说明，
              避免用户把页内命中当成全量统计。
            -->
            <span v-if="hasLocalFilter" class="tiny muted" data-od-id="visit-filter-scope-hint">
              设备、搜索与「只看失败」仅筛选当前页，切换分页后请重新确认
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

                <!-- 明细行：点开看这一次访问的决策链 -->
                <template v-for="row in filteredRows" :key="row.visit.id">
                <tr
                  :data-outcome="row.visit.outcome"
                  :data-open="expandedId === row.visit.id"
                  class="row-openable"
                  tabindex="0"
                  :aria-expanded="expandedId === row.visit.id"
                  @click="toggleRow(row)"
                  @keydown.enter.prevent="toggleRow(row)"
                  @keydown.space.prevent="toggleRow(row)"
                >
                  <!-- 时间:列里只给时分秒,完整年月日悬停看(同一页的记录基本是同一天) -->
                  <td class="shrink">
                    <AppTooltip :title="formatDateTime(row.visit.createdAt)">
                      <span class="mono tiny cursor-help underline decoration-dotted underline-offset-2">
                        {{ formatClock(row.visit.createdAt) }}
                      </span>
                    </AppTooltip>
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

                <!-- ==================== 展开行：真实裁决 + 规则回放 ==================== -->
                <tr v-if="expandedId === row.visit.id" :class="'trace-row'">
                  <td colspan="6">
                    <div class="trace-cell">
                      <!-- 访客画像:表格行里只留摘要(设备徽标 / IP),规则条件能匹配的字段在这里给全 -->
                      <div class="trace-sec">
                        <div class="row flex-wrap" style="gap: 8px; align-items: baseline">
                          <span class="mono micro muted">访客画像 · 本次请求</span>
                          <span class="tiny muted">写「包含 UA」「国家」「IP」类条件时，核对这些字段</span>
                        </div>
                        <dl class="profile">
                          <div class="profile-item">
                            <dt>访问时间</dt>
                            <dd class="mono">{{ formatDateTime(row.visit.createdAt) }}</dd>
                          </div>
                          <div class="profile-item">
                            <dt>来访 IP</dt>
                            <dd class="mono">{{ row.visit.ip || '—' }}</dd>
                          </div>
                          <div class="profile-item">
                            <dt>设备</dt>
                            <dd>
                              <span :class="['badge', getDeviceBadgeClass(row.parsedUa.deviceType)]">
                                {{ row.parsedUa.deviceType }}
                              </span>
                              <span class="tiny muted">{{ row.parsedUa.deviceModel }}</span>
                            </dd>
                          </div>
                          <div class="profile-item">
                            <dt>系统 / 浏览器</dt>
                            <dd>
                              {{ row.parsedUa.os }}
                              <span class="muted">/</span>
                              {{ row.parsedUa.browser }}
                            </dd>
                          </div>
                          <div class="profile-item">
                            <dt>短链</dt>
                            <dd class="mono truncate">
                              {{ row.visit.domain || link?.domains[0] || '未知域名' }}/{{ link?.code }}
                            </dd>
                          </div>
                          <div class="profile-item">
                            <dt>短链类型</dt>
                            <dd>{{ linkTypeLabel }} → {{ row.action.text }}</dd>
                          </div>
                          <div class="profile-item">
                            <dt>来源页</dt>
                            <dd class="truncate">{{ row.visit.referer || '直接访问' }}</dd>
                          </div>
                          <div class="profile-item">
                            <dt>最终抵达</dt>
                            <dd class="mono truncate">{{ row.visit.targetUrl || '—' }}</dd>
                          </div>
                          <div class="profile-item profile-wide">
                            <dt>
                              完整 User-Agent
                              <button
                                type="button"
                                class="mini-btn"
                                aria-label="复制完整 User-Agent"
                                @click.stop="copyUa(row)"
                              >
                                <Copy class="size-3" aria-hidden="true" /> 复制
                              </button>
                            </dt>
                            <dd class="mono ua-box">{{ row.visit.userAgent || '（无 UA 头）' }}</dd>
                          </div>
                        </dl>
                      </div>
                      <!-- ① 真实裁决：后端当时记下的事实 -->
                      <div class="trace-sec">
                        <span class="mono micro muted">真实裁决 · 后端记录</span>
                        <div class="row flex-wrap" style="gap: 6px; align-items: center">
                          <span :class="['badge', realVerdictOf(row).badge]">
                            {{ realVerdictOf(row).text }}
                          </span>
                          <span :class="['badge', row.visit.outcome === 'failed' ? 'badge-danger' : 'badge-ok']">
                            {{ row.visit.outcome === 'failed' ? '✗ 失败' : '✓ 成功' }}
                          </span>
                          <span v-if="row.reasonText" class="tiny" style="color: var(--danger)">
                            {{ row.reasonText }}
                          </span>
                          <span v-if="row.visit.ruleId == null" class="tiny muted">
                            没有规则参与，或命中后规则已被删除
                          </span>
                        </div>
                      </div>

                      <!-- ② 规则回放：按当前规则集重算一遍 -->
                      <div class="trace-sec">
                        <div class="row-between" style="gap: 8px; align-items: baseline">
                          <span class="mono micro muted">规则回放 · 按当前规则集</span>
                          <span class="tiny muted">规则改过之后，回放可能与当时的裁决不一致</span>
                        </div>

                        <div v-if="traceStateOf(row.visit.id)?.loading" class="row" style="gap: 8px">
                          <RefreshCw class="animate-spin" :size="14" />
                          <span class="tiny muted">正在回放规则链…</span>
                        </div>

                        <p v-else-if="traceStateOf(row.visit.id).error" class="tiny" style="color: var(--danger)">
                          回放失败：{{ traceStateOf(row.visit.id).error }}
                        </p>

                        <template v-else-if="traceStateOf(row.visit.id).trace">
                          <div class="row flex-wrap" style="gap: 6px; align-items: center">
                            <span
                              :class="[
                                'badge',
                                traceStateOf(row.visit.id).verdict!.matched
                                  ? traceStateOf(row.visit.id).verdict!.blocking
                                    ? 'badge-danger'
                                    : 'badge-ok'
                                  : 'badge-neutral',
                              ]"
                            >
                              {{ traceStateOf(row.visit.id).verdict!.title }}
                            </span>
                            <span class="tiny muted">{{ traceStateOf(row.visit.id).trace!.scopeNote }}</span>
                          </div>

                          <!-- 回放与历史不一致：多半是规则在这之后被改过 -->
                          <p
                            v-if="replayDiffers(row)"
                            class="tiny"
                            style="color: var(--warn)"
                          >
                            回放与当时的裁决不一致：当时是
                            {{ row.visit.ruleId == null ? '无规则命中' : `命中 #${row.visit.ruleId}` }}，
                            当前规则下会{{ traceStateOf(row.visit.id).verdict!.matched
                              ? `命中 #${traceStateOf(row.visit.id).trace!.matched!.id}`
                              : '无规则命中' }}。
                          </p>

                          <ul class="trace-list">
                            <li
                              v-for="step in traceStateOf(row.visit.id).trace!.steps"
                              :key="step.key"
                              :class="['trace-step', step.status]"
                            >
                              <span class="mono tiny" style="color: var(--ink-faint)">#{{ step.ruleId }}</span>
                              <span class="tiny" style="color: var(--ink); font-weight: 500">{{ step.ruleName }}</span>
                              <span
                                :class="[
                                  'badge',
                                  step.status === 'block'
                                    ? 'badge-danger'
                                    : step.status === 'hit'
                                      ? 'badge-ok'
                                      : 'badge-neutral',
                                ]"
                              >
                                {{ step.statusText }}
                              </span>
                              <span class="tiny muted" style="flex-basis: 100%">{{ step.whyText }}</span>
                              <template v-if="step.facts.length > 0">
                                <span
                                  v-for="(fact, i) in step.facts"
                                  :key="i"
                                  class="mono tiny"
                                  :style="fact.hit ? 'color: var(--ink)' : 'color: var(--ink-faint)'"
                                >{{ fact.hit ? '✓' : '✗' }} {{ fact.text }}</span>
                              </template>
                            </li>
                          </ul>

                          <p
                            v-if="traceStateOf(row.visit.id).trace!.skippedForDetail > 0"
                            class="tiny muted"
                          >
                            {{ traceStateOf(row.visit.id).trace!.skippedForDetail }} 条规则未取到条件，未参与本次回放
                          </p>
                        </template>
                      </div>

                      <!-- ③ 带着这个访客去模拟器改规则 -->
                      <div class="trace-sec row" style="gap: 8px">
                        <button type="button" class="btn btn-sm" @click.stop="openInSimulator(row)">
                          <FlaskConical :size="13" />
                          用此访客在模拟器打开
                        </button>
                        <span class="tiny muted">在模拟器里改条件验一遍，再回到这里看真实流量怎么走</span>
                      </div>
                    </div>
                  </td>
                </tr>
                </template>
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
import { ArrowLeft, Copy, FlaskConical, RefreshCw, Search, X } from '@lucide/vue';

import { getLink } from '@/api/links';
import { listVisits } from '@/api/visits';
import PageHeader from '@/components/PageHeader.vue';
import AppEmpty from '@/components/ui/AppEmpty.vue';
import AppResult from '@/components/ui/AppResult.vue';
import AppSpin from '@/components/ui/AppSpin.vue';
import { ApiError } from '@/types/api';
import type { Link, Visit, VisitAction, VisitReason } from '@/types/api';
import { formatClock, formatDateTime } from '@/utils/format';
import { message } from '@/utils/toast';
import { getDeviceBadgeClass, parseUserAgent } from '@/utils/userAgent';
import type { ParsedUA } from '@/utils/userAgent';
import { actionLabel, isBlockingAction } from '@/views/rules/ruleMeta';
import { buildDecisionTrace, verdictOf } from '@/views/rules/ruleTrace';
import type { DecisionTrace, Verdict } from '@/views/rules/ruleTrace';
import type { SimInput } from '@/views/rules/ruleSim';
import type { RuleAction } from '@/types/api';

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

/**
 * 失败原因 → 中文文案。
 *
 * 分两类：短链自身不可用（排在规则之前，明细里没有规则字段），与规则裁决
 * （一定带 rule_id / rule_action，见 internal/store/visits.go）。规则类的文案在
 * toRow 里拼上规则编号，否则用户只看到「返回 404」却不知道是哪条规则干的。
 */
const REASON_TEXT: Record<VisitReason, string> = {
  link_disabled: '短链已停用',
  link_deleted: '短链已删除',
  no_target: '无可用目标',
  landing_missing: '落地页文件缺失',
  rule_blocked: '规则判定为不存在',
  rule_throttled: '规则判定为限流',
};

/** 规则类失败原因：带规则编号与 HTTP 状态，点开看是哪个条件命中的 */
const RULE_REASON_HTTP: Partial<Record<VisitReason, string>> = {
  rule_blocked: '404',
  rule_throttled: '429',
};

function reasonTextOf(visit: Visit): string {
  if (visit.outcome !== 'failed') return '';
  const reason = visit.reason as VisitReason;
  const http = RULE_REASON_HTTP[reason];
  if (http) {
    return visit.ruleId != null
      ? `命中规则 #${visit.ruleId}：${REASON_TEXT[reason]}（HTTP ${http}）`
      : `${REASON_TEXT[reason]}（HTTP ${http}）`;
  }
  return REASON_TEXT[reason] || visit.reason || '未知原因';
}

/**
 * 设备类型筛选项。
 *
 * 取代原先的「跳转 / 落地页 / 点击」动作 tab——短链类型在进页面时就已经定了，
 * 跳转型只可能有跳转、落地页型只可能有落地页/点击，那三个 tab 里至少有两个永远为空。
 * 设备类型对同一条短链是真的有区分度。
 */
const DEVICE_OPTIONS: { value: 'all' | ParsedUA['deviceType']; label: string }[] = [
  { value: 'all', label: '全部设备' },
  { value: '移动端', label: '移动端' },
  { value: '桌面端', label: '桌面端' },
  { value: '平板', label: '平板' },
  { value: '爬虫机器人', label: '爬虫机器人' },
];

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
const visits = ref<Visit[]>([]);
const total = ref(0);
const page = ref(1);
const pageSize = ref(20);

const deviceFilter = ref<'all' | ParsedUA['deviceType']>('all');
/** 只看失败:后端没有按结果过滤的接口,失败筛选在前端当前页内完成,保持接口契约不变 */
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
}

/** 拉取当前页访问明细(action 过滤交给后端) */
async function loadVisits() {
  if (loadError.value) return;
  try {
    const res = await listVisits(linkId.value, {
      page: page.value,
      pageSize: pageSize.value,
    });
    visits.value = res.items;
    total.value = res.total;
    // 重取后旧回放作废：规则可能已经改过，再展示旧链会误导
    expandedId.value = null;
    traceStates.value = {};
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
    reasonText: reasonTextOf(visit),
  };
}

const rows = computed<VisitRow[]>(() => visits.value.map(toRow));

/** 本地过滤:设备类型 + 只看失败 + IP / UA / 来源 / 目标 关键词(仅限当前页) */
const filteredRows = computed<VisitRow[]>(() => {
  const q = keyword.value.trim().toLowerCase();
  return rows.value.filter((row) => {
    // 无 UA 的行无法判设备，不归入任何设备桶（parseUserAgent 对空 UA 会返回「桌面端」）
    if (deviceFilter.value !== 'all') {
      if (!row.hasUa || row.parsedUa.deviceType !== deviceFilter.value) return false;
    }
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

const hasLocalFilter = computed(
  () => deviceFilter.value !== 'all' || onlyFailed.value || keyword.value.trim() !== '',
);

function resetLocalFilters() {
  keyword.value = '';
  onlyFailed.value = false;
  deviceFilter.value = 'all';
}

// ==================== 分页与筛选交互 ====================
const totalPages = computed(() => Math.max(1, Math.ceil(total.value / pageSize.value)));

/** 切换动作筛选:已移除——短链类型在进页面时就定了,动作 tab 至少有一半永远为空 */

function goToPage(p: number) {
  if (p < 1 || p > totalPages.value || p === page.value) return;
  page.value = p;
  loadVisits();
}

function onPageSizeChange() {
  page.value = 1;
  loadVisits();
}

// ==================== 决策链：真实裁决 + 规则回放 ====================
/** 展开行的回放结果：懒加载一次后缓存，重复展开同一行不再拉规则 */
interface TraceState {
  loading: boolean;
  error: string | null;
  trace: DecisionTrace | null;
  verdict: Verdict | null;
}

const expandedId = ref<number | null>(null);
const traceStates = ref<Record<number, TraceState>>({});

/** 尚未回放时的空状态：让模板可以直接取，不用到处判空 */
const EMPTY_TRACE: TraceState = { loading: false, error: null, trace: null, verdict: null };

function traceStateOf(id: number): TraceState {
  return traceStates.value[id] ?? EMPTY_TRACE;
}

/**
 * 用这条访问明细当时的请求头拼出模拟器输入。
 *
 * 域名优先取明细记录的 domain（当时实际命中的域名），明细没记就退到短链的第一个
 * 域名；两个都没有就只给路径——此时 domain 作用域的规则本就不该命中，不要伪造域名。
 */
function simInputOf(row: VisitRow): SimInput {
  const v = row.visit;
  const code = link.value?.code ?? '';
  const host = v.domain || link.value?.domains?.[0] || '';
  return {
    url: host && code ? `https://${host}/${code}` : `/${code}`,
    ip: v.ip || '',
    ua: v.userAgent || '',
    lang: v.lang || '',
    ref: v.referer || '',
  };
}

async function loadTrace(row: VisitRow) {
  const id = row.visit.id;
  if (traceStates.value[id]) return;
  traceStates.value = { ...traceStates.value, [id]: { loading: true, error: null, trace: null, verdict: null } };
  try {
    const trace = await buildDecisionTrace(simInputOf(row), { link: link.value ?? undefined });
    if (trace.skippedForDetail > 0) {
      message.warning(`${trace.skippedForDetail} 条规则未取到条件，未参与本次回放`);
    }
    traceStates.value = {
      ...traceStates.value,
      [id]: { loading: false, error: null, trace, verdict: verdictOf(trace) },
    };
  } catch (error) {
    traceStates.value = {
      ...traceStates.value,
      [id]: {
        loading: false,
        error: error instanceof Error ? error.message : '回放失败',
        trace: null,
        verdict: null,
      },
    };
  }
}

function toggleRow(row: VisitRow) {
  if (expandedId.value === row.visit.id) {
    expandedId.value = null;
    return;
  }
  expandedId.value = row.visit.id;
  loadTrace(row);
}

/** 后端当时记下的真实裁决：这是事实，回放不是 */
function realVerdictOf(row: VisitRow): { text: string; badge: string } {
  if (row.visit.ruleId == null) return { text: '无规则参与', badge: 'badge-neutral' };
  const action = row.visit.ruleAction as RuleAction;
  const label = actionLabel(action) || action || '未知动作';
  return {
    text: `命中 #${row.visit.ruleId} · ${label}`,
    badge: isBlockingAction(action) ? 'badge-danger' : 'badge-ok',
  };
}

/** 回放结果与历史裁决不一致——规则多半在这次访问之后被改过 */
function replayDiffers(row: VisitRow): boolean {
  const replay = traceStateOf(row.visit.id).trace;
  if (!replay) return false;
  return (replay.matched?.id ?? null) !== row.visit.ruleId;
}

/** 复制完整 UA：写「包含 UA」条件时不用手动选中一长串去粘 */
async function copyUa(row: VisitRow) {
  const ua = row.visit.userAgent;
  if (!ua) {
    message.warning('这条访问没有 User-Agent 头');
    return;
  }
  try {
    await navigator.clipboard.writeText(ua);
    message.success('已复制 User-Agent');
  } catch {
    message.error('复制失败,请手动选中复制');
  }
}

function openInSimulator(row: VisitRow) {
  const input = simInputOf(row);
  router.push({
    path: '/rules/simulator',
    query: { ip: input.ip, ua: input.ua, referrer: input.ref, url: input.url, lang: input.lang },
  });
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

/* 可展开行:整行是展开开关,鼠标/键盘都给出手型 */
.row-openable {
  cursor: pointer;
}

.row-openable:hover {
  background: var(--surface-muted);
}

.row-openable[data-open='true'] {
  background: var(--surface-muted);
  box-shadow: inset 2px 0 0 var(--brand-500);
}

.row-openable:focus-visible {
  outline: 2px solid var(--brand-500);
  outline-offset: -2px;
}

/* 决策链展开区：与表格主体用一条细线区隔 */
.trace-row > td {
  padding: 0;
  background: var(--surface-muted);
  border-top: 1px solid var(--line);
}

.trace-cell {
  display: flex;
  flex-direction: column;
  gap: 12px;
  padding: 14px 16px;
}

.trace-sec {
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.trace-list {
  display: flex;
  flex-direction: column;
  gap: 6px;
  margin: 0;
  padding: 0;
  list-style: none;
}

.trace-step {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 6px;
  padding: 6px 8px;
  border-radius: var(--radius-sm);
  border: 1px solid var(--line);
  background: var(--surface);
}

/* 访客画像：自适应列数的紧凑网格，不按表格列排版，避免长 UA 撑破布局 */
.profile {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(210px, 1fr));
  gap: 8px 16px;
  margin: 0;
}

.profile-item {
  min-width: 0;
}

.profile-item > dt {
  display: flex;
  align-items: center;
  gap: 6px;
  font-size: var(--fs-micro);
  color: var(--muted);
  margin-bottom: 1px;
}

.profile-item > dd {
  margin: 0;
  font-size: 12.5px;
  color: var(--fg);
  display: flex;
  align-items: center;
  gap: 6px;
  min-width: 0;
}

/* UA 独占整行：它是最长的字段，挤在半列里会被截断成看不出规律的碎片 */
.profile-wide {
  grid-column: 1 / -1;
}

.ua-box {
  display: block;
  padding: 6px 8px;
  border: 1px solid var(--line);
  border-radius: var(--radius-sm);
  background: var(--surface);
  font-size: 11.5px;
  line-height: 1.55;
  color: var(--ink-soft);
  word-break: break-all;
}

.mini-btn {
  display: inline-flex;
  align-items: center;
  gap: 3px;
  padding: 1px 6px;
  border: 1px solid var(--line);
  border-radius: var(--radius-sm);
  background: var(--surface);
  color: var(--muted);
  font-size: var(--fs-micro);
  cursor: pointer;
  transition: color 0.14s ease, border-color 0.14s ease;
}

.mini-btn:hover {
  color: var(--brand-600);
  border-color: var(--brand-500);
}

/* 命中/拦截行比跳过的行更显眼 */
.trace-step.hit {
  border-color: color-mix(in srgb, var(--ok) 40%, var(--line));
}

.trace-step.block {
  border-color: color-mix(in srgb, var(--danger) 40%, var(--line));
}
</style>
