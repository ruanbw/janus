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
      <!-- ==================== 左右分栏布局：左表格 + 右画像及规则回放 ==================== -->
      <div class="grid grid-cols-1 gap-5 xl:grid-cols-12 items-start" data-od-id="link-visits-split-layout">
        <!-- 左侧：访问明细列表 -->
        <section class="panel xl:col-span-7 flex flex-col min-w-0" data-od-id="link-visits-list">
          <div class="panel-hd">
            <div>
              <h2>访问明细</h2>
              <p>逐条记录该短链每次{{ isLanding ? '落地页视图与按钮点击' : '跳转' }}的动作与结果，点击列表行在右侧查看访客画像与规则回放。</p>
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

              <div class="relative grow min-w-[200px]">
                <input
                  v-model="keyword"
                  class="input input-icon"
                  id="visitSearch"
                  placeholder="搜索 IP、UA、来源或目标…"
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

              <span v-if="hasLocalFilter" class="tiny muted" data-od-id="visit-filter-scope-hint">
                设备、搜索与「只看失败」仅筛选当前页
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
                    <th title="国家由后端内嵌离线 GeoIP 库解析">地理位置</th>
                    <th title="由 User-Agent 解析:设备型号 · 操作系统 · 浏览器">设备型号</th>
                    <th class="shrink" title="本次触发的动作与结果;失败的动作不计入访问次数">动作与结果</th>
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

                  <!-- 明细行：点击选中，右侧查看访客画像与规则决策链 -->
                  <tr
                    v-for="row in filteredRows"
                    :key="row.visit.id"
                    :data-outcome="row.visit.outcome"
                    :data-selected="selectedId === row.visit.id"
                    class="row-selectable"
                    tabindex="0"
                    :aria-selected="selectedId === row.visit.id"
                    @click="selectRow(row)"
                    @keydown.enter.prevent="selectRow(row)"
                    @keydown.space.prevent="selectRow(row)"
                  >
                    <!-- 时间:列里只给时分秒,完整年月日悬停看 -->
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

                    <!-- 地理位置:国家 / 数据中心 / 语言 -->
                    <td>
                      <div class="stack" style="gap: 2px; min-width: 0">
                        <span
                          class="tiny truncate"
                          :title="row.country === '—' ? '该 IP 查不到国家（私网/回环/库中未收录）' : row.visit.country"
                        >
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

                    <!-- 动作与结果：清楚显示具体命中的失败规则名称与具体命中的条件 -->
                    <td class="shrink">
                      <div class="stack" style="gap: 3px; max-width: 220px">
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
                        <span
                          v-if="row.reasonText"
                          class="tiny truncate"
                          style="color: var(--danger)"
                          :title="row.reasonTooltip"
                        >
                          {{ row.reasonText }}
                        </span>
                      </div>
                    </td>

                    <!-- 目标 / 来源 -->
                    <td>
                      <div class="stack" style="gap: 2px; min-width: 0; max-width: 320px">
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

        <!-- 右侧：访客画像与规则回放详情卡片 -->
        <section
          class="panel xl:col-span-5 sticky top-20 flex flex-col min-w-0 max-h-[calc(100vh-6rem)]"
          data-od-id="link-visit-detail-panel"
        >
          <!-- 未选择行时的引导提示 -->
          <div
            v-if="!selectedRow"
            class="flex flex-col items-center justify-center p-10 text-center my-auto min-h-[380px]"
          >
            <div class="flex h-12 w-12 items-center justify-center rounded-full bg-brand-500/10 text-brand-600 mb-3">
              <MousePointerClick :size="22" />
            </div>
            <h3 class="text-sm font-semibold text-ink">选择访问记录</h3>
            <p class="text-xs text-muted mt-1.5 max-w-[260px] leading-relaxed">
              在左侧列表中点击任意一行，即可在此查看该次请求的完整访客画像、真实裁决以及按当前规则集的决策链路回放。
            </p>
          </div>

          <!-- 选中行时的详情展示 -->
          <template v-else>
            <div class="panel-hd shrink-0 border-b border-line">
              <div class="min-w-0">
                <div class="row" style="gap: 8px; align-items: center">
                  <h2>访客画像与规则回放</h2>
                  <span
                    :class="[
                      'badge',
                      selectedRow.visit.outcome === 'failed' ? 'badge-danger' : 'badge-ok',
                    ]"
                  >
                    {{ selectedRow.visit.outcome === 'failed' ? '✗ 访问失败' : '✓ 访问成功' }}
                  </span>
                </div>
                <p class="truncate mono tiny muted mt-0.5">
                  ID #{{ selectedRow.visit.id }} · {{ formatDateTime(selectedRow.visit.createdAt) }}
                </p>
              </div>
              <div class="btn-row">
                <button
                  type="button"
                  class="btn btn-sm"
                  title="带着这个访客去模拟器改规则"
                  @click="openInSimulator(selectedRow)"
                >
                  <FlaskConical :size="13" />
                  模拟器打开
                </button>
                <button
                  type="button"
                  class="btn btn-sm btn-ghost text-muted hover:text-fg"
                  title="关闭详情"
                  aria-label="关闭详情"
                  @click="selectedId = null"
                >
                  <X :size="14" />
                </button>
              </div>
            </div>

            <!-- 可滚动的明细主体 -->
            <div class="panel-bd flex-1 overflow-y-auto space-y-5 p-4">
              <!-- ① 访客画像 -->
              <div class="trace-sec">
                <div class="row-between" style="gap: 8px; align-items: baseline">
                  <span class="mono micro font-semibold text-ink-soft">访客画像 · 本次请求</span>
                  <span class="tiny muted">规则条件匹配的字段明细</span>
                </div>
                <dl class="profile">
                  <div class="profile-item">
                    <dt>访问时间</dt>
                    <dd class="mono">{{ formatDateTime(selectedRow.visit.createdAt) }}</dd>
                  </div>
                  <div class="profile-item">
                    <dt>来访 IP</dt>
                    <dd class="mono">{{ selectedRow.visit.ip || '—' }}</dd>
                  </div>
                  <div class="profile-item">
                    <dt>国家 / 地区</dt>
                    <dd>
                      <span :class="['badge', selectedRow.country === '—' ? 'badge-neutral' : 'badge-ok']">
                        {{ selectedRow.country }}
                      </span>
                    </dd>
                  </div>
                  <div class="profile-item">
                    <dt>网络属性</dt>
                    <dd>
                      <span :class="['badge', selectedRow.network.badge]" :title="selectedRow.network.title">
                        {{ selectedRow.network.text }}
                      </span>
                    </dd>
                  </div>
                  <div class="profile-item">
                    <dt>语言 (Accept-Language)</dt>
                    <dd class="mono truncate" :title="selectedRow.lang">{{ selectedRow.lang }}</dd>
                  </div>
                  <div class="profile-item">
                    <dt>设备类型与型号</dt>
                    <dd>
                      <span :class="['badge', getDeviceBadgeClass(selectedRow.parsedUa.deviceType)]">
                        {{ selectedRow.parsedUa.deviceType }}
                      </span>
                      <span class="tiny muted truncate" :title="selectedRow.parsedUa.deviceModel">
                        {{ selectedRow.parsedUa.deviceModel }}
                      </span>
                    </dd>
                  </div>
                  <div class="profile-item">
                    <dt>系统 / 浏览器</dt>
                    <dd class="truncate" :title="selectedRow.parsedUa.os + ' / ' + selectedRow.parsedUa.browser">
                      {{ selectedRow.parsedUa.os }}
                      <span class="muted">/</span>
                      {{ selectedRow.parsedUa.browser }}
                    </dd>
                  </div>
                  <div class="profile-item">
                    <dt>短链</dt>
                    <dd class="mono truncate" :title="(selectedRow.visit.domain || link?.domains?.[0] || '') + '/' + (link?.code || '-')">
                      {{ selectedRow.visit.domain || link?.domains?.[0] || '未知域名' }}/{{ link?.code || '-' }}
                    </dd>
                  </div>
                  <div class="profile-item">
                    <dt>短链类型</dt>
                    <dd>{{ linkTypeLabel }} → {{ selectedRow.action.text }}</dd>
                  </div>
                  <div class="profile-item">
                    <dt>来源页 (Referer)</dt>
                    <dd class="truncate" :title="selectedRow.visit.referer || '直接访问'">
                      {{ selectedRow.visit.referer || '直接访问' }}
                    </dd>
                  </div>
                  <div class="profile-item">
                    <dt>最终抵达</dt>
                    <dd class="mono truncate" :title="selectedRow.visit.targetUrl || '—'">
                      {{ selectedRow.visit.targetUrl || '—' }}
                    </dd>
                  </div>
                  <div class="profile-item profile-wide">
                    <dt>
                      完整 User-Agent
                      <button
                        type="button"
                        class="mini-btn"
                        aria-label="复制完整 User-Agent"
                        @click.stop="copyUa(selectedRow)"
                      >
                        <Copy class="size-3" aria-hidden="true" /> 复制
                      </button>
                    </dt>
                    <dd class="mono ua-box">{{ selectedRow.visit.userAgent || '（无 UA 头）' }}</dd>
                  </div>
                </dl>
              </div>

              <!-- 分隔线 -->
              <div class="border-t border-line pt-4 space-y-4">
                <!-- ② 真实裁决：后端当时记下的事实 -->
                <div class="trace-sec">
                  <div class="row-between" style="gap: 8px; align-items: baseline">
                    <span class="mono micro font-semibold text-ink-soft">真实裁决 · 后端记录</span>
                    <span class="tiny muted">后端处理该请求时的实际处置结果</span>
                  </div>

                  <!-- 命中的规则卡片 -->
                  <div
                    v-if="selectedRow.matchedRule"
                    class="rounded-lg border border-line bg-surface p-3 space-y-1.5 shadow-2xs"
                  >
                    <div class="row-between text-xs">
                      <span class="font-medium text-ink">
                        命中规则 #{{ selectedRow.matchedRule.id }} · {{ selectedRow.matchedRule.name }}
                      </span>
                      <span class="badge" :class="selectedRow.matchedRule.enabled ? 'badge-ok' : 'badge-neutral'">
                        {{ selectedRow.matchedRule.enabled ? '规则当前启用' : '规则当前已停用' }}
                      </span>
                    </div>
                    <!-- 只展示具体命中的条件，不展示全部条件 -->
                    <div v-if="selectedRow.hitConditionText" class="mono tiny leading-relaxed" style="color: var(--danger)">
                      <span class="font-medium">命中条件：</span>{{ selectedRow.hitConditionText }}
                    </div>
                    <div v-else-if="selectedRow.matchedRule.conditions?.length" class="mono tiny text-ink-soft leading-relaxed">
                      <span class="font-medium">规则条件：</span>{{ conditionSummary(selectedRow.matchedRule) }}
                    </div>
                  </div>

                  <div class="row flex-wrap" style="gap: 6px; align-items: center">
                    <span :class="['badge', realVerdictOf(selectedRow).badge]">
                      {{ realVerdictOf(selectedRow).text }}
                    </span>
                    <span :class="['badge', selectedRow.visit.outcome === 'failed' ? 'badge-danger' : 'badge-ok']">
                      {{ selectedRow.visit.outcome === 'failed' ? '✗ 失败' : '✓ 成功' }}
                    </span>
                    <span v-if="selectedRow.reasonText" class="tiny" style="color: var(--danger)">
                      {{ selectedRow.reasonText }}
                    </span>
                    <span v-if="selectedRow.visit.ruleId == null && selectedRow.visit.outcome !== 'failed'" class="tiny muted">
                      没有规则参与，按默认短链配置放行
                    </span>
                  </div>
                </div>

                <!-- ③ 规则回放：按当前规则集重算一遍 -->
                <div class="trace-sec">
                  <div class="row-between" style="gap: 8px; align-items: baseline">
                    <span class="mono micro font-semibold text-ink-soft">规则回放 · 按当前规则集</span>
                    <span class="tiny muted">规则若被改动过，回放可能与历史裁决不同</span>
                  </div>

                  <div v-if="traceStateOf(selectedRow.visit.id)?.loading" class="row py-3" style="gap: 8px">
                    <RefreshCw class="animate-spin text-brand-500" :size="14" />
                    <span class="tiny muted">正在回放规则链…</span>
                  </div>

                  <p v-else-if="traceStateOf(selectedRow.visit.id).error" class="tiny" style="color: var(--danger)">
                    回放失败：{{ traceStateOf(selectedRow.visit.id).error }}
                  </p>

                  <template v-else-if="traceStateOf(selectedRow.visit.id).trace">
                    <div class="row flex-wrap" style="gap: 6px; align-items: center">
                      <span
                        :class="[
                          'badge',
                          traceStateOf(selectedRow.visit.id).verdict!.matched
                            ? traceStateOf(selectedRow.visit.id).verdict!.blocking
                              ? 'badge-danger'
                              : 'badge-ok'
                            : 'badge-neutral',
                        ]"
                      >
                        {{ traceStateOf(selectedRow.visit.id).verdict!.title }}
                      </span>
                      <span class="tiny muted">{{ traceStateOf(selectedRow.visit.id).trace!.scopeNote }}</span>
                    </div>

                    <!-- 回放与历史不一致提示 -->
                    <div
                      v-if="replayDiffers(selectedRow)"
                      class="rounded-lg border border-amber-300/40 bg-amber-50/50 p-2.5 text-xs text-amber-800 dark:border-amber-700/40 dark:bg-amber-950/30 dark:text-amber-300 leading-relaxed"
                    >
                      回放与当时的裁决不一致：当时是
                      {{ selectedRow.visit.ruleId == null ? '无规则命中' : `命中 #${selectedRow.visit.ruleId}` }}，
                      当前规则下会{{ traceStateOf(selectedRow.visit.id).verdict!.matched
                        ? `命中 #${traceStateOf(selectedRow.visit.id).trace!.matched!.id}`
                        : '无规则命中' }}。
                    </div>

                    <ul class="trace-list">
                      <li
                        v-for="step in traceStateOf(selectedRow.visit.id).trace!.steps"
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
                        <!-- 命中的规则只展示具体命中的条件，不展示未命中的其余条件 -->
                        <template v-if="step.facts.length > 0">
                          <template v-if="step.status === 'hit' || step.status === 'block'">
                            <span
                              v-for="(fact, i) in step.facts.filter((f) => f.hit)"
                              :key="i"
                              class="mono tiny"
                              style="color: var(--ink)"
                            >✓ {{ fact.text }}</span>
                          </template>
                          <template v-else>
                            <span
                              v-for="(fact, i) in step.facts"
                              :key="i"
                              class="mono tiny"
                              :style="fact.hit ? 'color: var(--ink)' : 'color: var(--ink-faint)'"
                            >{{ fact.hit ? '✓' : '✗' }} {{ fact.text }}</span>
                          </template>
                        </template>
                      </li>
                    </ul>

                    <p
                      v-if="traceStateOf(selectedRow.visit.id).trace!.skippedForDetail > 0"
                      class="tiny muted"
                    >
                      {{ traceStateOf(selectedRow.visit.id).trace!.skippedForDetail }} 条规则未取到条件，未参与本次回放
                    </p>
                  </template>
                </div>

                <!-- ④ 带着这个访客去模拟器改规则 -->
                <div class="trace-sec pt-2">
                  <button
                    type="button"
                    class="btn btn-sm btn-primary w-full justify-center"
                    @click="openInSimulator(selectedRow)"
                  >
                    <FlaskConical :size="13" />
                    用此访客在模拟器打开
                  </button>
                  <span class="tiny muted text-center">在模拟器里改条件验一遍，再回到这里看真实流量怎么走</span>
                </div>
              </div>
            </div>
          </template>
        </section>
      </div>
    </template>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue';
import { useRoute, useRouter } from 'vue-router';
import { ArrowLeft, Copy, FlaskConical, MousePointerClick, RefreshCw, Search, X } from '@lucide/vue';

import { getLink } from '@/api/links';
import { listVisits } from '@/api/visits';
import PageHeader from '@/components/PageHeader.vue';
import AppEmpty from '@/components/ui/AppEmpty.vue';
import AppResult from '@/components/ui/AppResult.vue';
import AppSpin from '@/components/ui/AppSpin.vue';
import AppTooltip from '@/components/ui/AppTooltip.vue';
import { ApiError } from '@/types/api';
import type { Link, Rule, RuleAction, RuleCondition, Visit, VisitAction, VisitReason } from '@/types/api';
import { formatClock, formatCountry, formatDateTime } from '@/utils/format';
import { message } from '@/utils/toast';
import { getDeviceBadgeClass, parseUserAgent } from '@/utils/userAgent';
import type { ParsedUA } from '@/utils/userAgent';
import { actionLabel, conditionSummary, describeCondition, isBlockingAction } from '@/views/rules/ruleMeta';
import { buildDecisionTrace, loadAllRules, verdictOf } from '@/views/rules/ruleTrace';
import type { DecisionTrace, Verdict } from '@/views/rules/ruleTrace';
import { buildVisitorFacts, evalCondition } from '@/views/rules/ruleSim';
import type { SimInput } from '@/views/rules/ruleSim';

const route = useRoute();
const router = useRouter();

// ==================== 展示文案映射 ====================
/** 动作 → 徽标文案/配色 */
const ACTION_META: Record<VisitAction, { text: string; badge: string }> = {
  redirect: { text: '跳转', badge: 'badge-neutral' },
  landing_view: { text: '落地页', badge: 'badge-warn' },
  click: { text: '点击', badge: 'badge-ok' },
};

/**
 * 非规则类的固定失败原因说明。
 * 规则类的失败拦截由 formatFailureReason 按命中的具体条件动态生成，不再展示模糊的「规则判定为不存在」。
 */
const REASON_TEXT: Record<VisitReason, string> = {
  link_disabled: '短链已停用',
  link_deleted: '短链已删除',
  no_target: '无可用目标',
  landing_missing: '落地页文件缺失',
  rule_blocked: '规则拦截（HTTP 404）',
  rule_throttled: '规则限流（HTTP 429）',
};

const RULE_REASON_HTTP: Partial<Record<VisitReason, string>> = {
  rule_blocked: '404',
  rule_throttled: '429',
};

/** 设备类型筛选项 */
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
  matchedRule?: Rule;
  reasonText: string;
  reasonTooltip: string;
  hitConditionText: string;
}

// ==================== 响应式状态 ====================
const link = ref<Link | null>(null);
const visits = ref<Visit[]>([]);
const total = ref(0);
const page = ref(1);
const pageSize = ref(20);

/** 租户规则列表缓存，用于将明细中的 ruleId 快速映射到规则名与具体条件 */
const rulesList = ref<Rule[]>([]);
const rulesMap = computed(() => new Map(rulesList.value.map((r) => [r.id, r])));

const deviceFilter = ref<'all' | ParsedUA['deviceType']>('all');
const onlyFailed = ref(false);
const keyword = ref('');

const loading = ref(false);
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

function simInputOf(rowOrVisit: Visit | VisitRow): SimInput {
  const v = 'visit' in rowOrVisit ? rowOrVisit.visit : rowOrVisit;
  const code = link.value?.code ?? '';
  const host = v.domain || link.value?.domains?.[0] || '';
  return {
    url: host && code ? `https://${host}/${code}` : `/${code}`,
    ip: v.ip || '',
    ua: v.userAgent || '',
    lang: v.lang || '',
    ref: v.referer || '',
    country: v.country || '',
  };
}

/**
 * 提取某个访问真实命中的那条（或那些）条件。
 * 用户要求：“一个规则中会有多个条件 只展示命中的那个条件就可以了 不用展示全部条件”。
 */
function getHitConditions(rule: Rule, visit: Visit): RuleCondition[] {
  if (!rule.conditions || rule.conditions.length === 0) return [];
  const facts = buildVisitorFacts(simInputOf(visit));
  return rule.conditions.filter((cond) => {
    return evalCondition(cond, facts).hit;
  });
}

function getHitConditionsSummary(rule: Rule, visit: Visit): { hitCondsText: string; isHit: boolean } {
  const hitConds = getHitConditions(rule, visit);
  if (hitConds.length > 0) {
    return {
      hitCondsText: hitConds.map(describeCondition).join(' · '),
      isHit: true,
    };
  }
  // 兜底：如果规则被修改过导致当前条件未命中，退化展示原规则条件
  return {
    hitCondsText: conditionSummary(rule),
    isHit: false,
  };
}

/**
 * 格式化失败原因：清楚说明命中的是哪个规则及具体命中的那个条件，杜绝直接展示生硬的「规则判定为不存在」。
 */
function formatFailureReason(
  visit: Visit,
  rule?: Rule,
): { text: string; tooltip: string; hitConditionText: string } {
  if (visit.outcome !== 'failed') return { text: '', tooltip: '', hitConditionText: '' };
  const reason = visit.reason as VisitReason;
  const http = RULE_REASON_HTTP[reason];

  if (visit.ruleId != null) {
    if (rule) {
      const { hitCondsText, isHit } = getHitConditionsSummary(rule, visit);
      const hasConds = hitCondsText && hitCondsText !== '—';
      const httpText = http ? `（HTTP ${http}）` : '';

      // 提取核心描述：只展示命中的那个条件，避免把未命中的多条规则全部摆出来
      let detail = rule.name;
      if (hasConds) {
        const nameClean = rule.name.toLowerCase().replace(/[\s\-_]+/g, '');
        const condClean = hitCondsText.toLowerCase().replace(/[\s\-_]+/g, '');
        if (nameClean.includes(condClean) || condClean.includes(nameClean)) {
          detail = rule.name;
        } else {
          detail = `${rule.name}：${hitCondsText}`;
        }
      }
      return {
        text: `命中 #${rule.id} ${detail}`,
        tooltip: `命中规则 #${rule.id}「${rule.name}」${hasConds ? (isHit ? ' · 命中条件：' : ' · 条件：') + hitCondsText : ''}${httpText}`,
        hitConditionText: isHit && hasConds ? hitCondsText : '',
      };
    }
    const httpText = http ? `（HTTP ${http}）` : '';
    return {
      text: `命中规则 #${visit.ruleId}（规则已删除）`,
      tooltip: `命中规则 #${visit.ruleId}${httpText}，该规则后续已被删除`,
      hitConditionText: '',
    };
  }

  if (reason === 'rule_blocked') {
    return {
      text: '规则拦截（HTTP 404）',
      tooltip: '触发规则拦截返回 404，无具体规则记录或规则已被删除',
      hitConditionText: '',
    };
  }
  if (reason === 'rule_throttled') {
    return {
      text: '规则限流（HTTP 429）',
      tooltip: '触发限流返回 429，无具体规则记录或规则已被删除',
      hitConditionText: '',
    };
  }

  const base = REASON_TEXT[reason] || visit.reason || '未知原因';
  return { text: base, tooltip: base, hitConditionText: '' };
}

// ==================== 数据加载 ====================
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
  }
}

async function loadRules() {
  try {
    rulesList.value = await loadAllRules();
  } catch (err) {
    console.error('Failed to load rules for visit failure mapping', err);
  }
}

async function loadVisits() {
  if (loadError.value) return;
  try {
    const res = await listVisits(linkId.value, {
      page: page.value,
      pageSize: pageSize.value,
    });
    visits.value = res.items;
    total.value = res.total;
    selectedId.value = null;
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
    await Promise.all([loadLink(), loadVisits(), loadRules()]);
  } finally {
    loading.value = false;
    loaded.value = true;
  }
}

// ==================== 行数据组装 ====================
function toRow(visit: Visit): VisitRow {
  const hasUa = Boolean(visit.userAgent && visit.userAgent.trim());
  const meta = ACTION_META[visit.action] ?? { text: visit.action || '未知动作', badge: 'badge-neutral' };
  const rule = visit.ruleId != null ? rulesMap.value.get(visit.ruleId) : undefined;
  const { text: reasonText, tooltip: reasonTooltip, hitConditionText } = formatFailureReason(visit, rule);

  return {
    visit,
    hasUa,
    parsedUa: parseUserAgent(visit.userAgent || ''),
    country: formatCountry(visit.country),
    lang: visit.lang || '—',
    network: visit.isDatacenter
      ? { text: '数据中心', badge: 'badge-warn', title: visit.asn ? `ASN ${visit.asn}` : '数据中心出口' }
      : {
          text: '住宅/未知',
          badge: 'badge-neutral',
          title: visit.asn ? `ASN ${visit.asn}` : '未标记为数据中心出口(该字段暂无数据源)',
        },
    action: meta,
    matchedRule: rule,
    reasonText,
    reasonTooltip,
    hitConditionText,
  };
}

const rows = computed<VisitRow[]>(() => visits.value.map(toRow));

/** 本地过滤:设备类型 + 只看失败 + 搜索(仅限当前页) */
const filteredRows = computed<VisitRow[]>(() => {
  const q = keyword.value.trim().toLowerCase();
  return rows.value.filter((row) => {
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

// ==================== 分页交互 ====================
const totalPages = computed(() => Math.max(1, Math.ceil(total.value / pageSize.value)));

function goToPage(p: number) {
  if (p < 1 || p > totalPages.value || p === page.value) return;
  page.value = p;
  selectedId.value = null;
  loadVisits();
}

function onPageSizeChange() {
  page.value = 1;
  selectedId.value = null;
  loadVisits();
}

// ==================== 决策链：真实裁决 + 规则回放 ====================
interface TraceState {
  loading: boolean;
  error: string | null;
  trace: DecisionTrace | null;
  verdict: Verdict | null;
}

const selectedId = ref<number | null>(null);
const selectedRow = computed<VisitRow | null>(() => {
  if (selectedId.value == null) return null;
  return filteredRows.value.find((r) => r.visit.id === selectedId.value) ?? null;
});

const traceStates = ref<Record<number, TraceState>>({});

const EMPTY_TRACE: TraceState = { loading: false, error: null, trace: null, verdict: null };

function traceStateOf(id: number): TraceState {
  return traceStates.value[id] ?? EMPTY_TRACE;
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

function selectRow(row: VisitRow) {
  selectedId.value = row.visit.id;
  loadTrace(row);
}

/** 后端当时记下的真实裁决 */
function realVerdictOf(row: VisitRow): { text: string; badge: string } {
  if (row.visit.ruleId == null) {
    if (row.visit.reason === 'rule_blocked') {
      return { text: '规则拦截 · 直接 404', badge: 'badge-danger' };
    }
    if (row.visit.reason === 'rule_throttled') {
      return { text: '规则限流 · 429', badge: 'badge-danger' };
    }
    return { text: '无规则参与', badge: 'badge-neutral' };
  }
  const action = row.visit.ruleAction as RuleAction;
  const label = actionLabel(action) || action || '未知动作';
  const rule = row.matchedRule;
  const namePart = rule ? `「${rule.name}」` : '';
  return {
    text: `命中 #${row.visit.ruleId} ${namePart} · ${label}`,
    badge: isBlockingAction(action) ? 'badge-danger' : 'badge-ok',
  };
}

function replayDiffers(row: VisitRow): boolean {
  const replay = traceStateOf(row.visit.id).trace;
  if (!replay) return false;
  return (replay.matched?.id ?? null) !== row.visit.ruleId;
}

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
    query: {
      ip: input.ip,
      ua: input.ua,
      referrer: input.ref,
      url: input.url,
      lang: input.lang,
      country: input.country,
    },
  });
}

function goBack() {
  router.push({ name: 'links' });
}

onMounted(() => {
  loadData();
});

watch(linkId, () => {
  link.value = null;
  visits.value = [];
  total.value = 0;
  page.value = 1;
  selectedId.value = null;
  loadError.value = null;
  loaded.value = false;
  loadData();
});
</script>

<style scoped>
/* 失败动作整行淡红底 */
.tbl tbody tr[data-outcome='failed'] {
  background: var(--danger-soft);
}

.tbl tbody tr[data-outcome='failed']:hover {
  background: color-mix(in srgb, var(--danger) 18%, var(--surface));
}

/* 选中行样式：左侧指示条与主题高亮底色 */
.row-selectable {
  cursor: pointer;
  transition: background-color 0.12s ease;
}

.row-selectable:hover {
  background: var(--surface-muted);
}

.row-selectable[data-selected='true'] {
  background: color-mix(in srgb, var(--brand-500) 10%, var(--surface)) !important;
  box-shadow: inset 3px 0 0 var(--brand-500);
}

.tbl tbody tr[data-outcome='failed'][data-selected='true'] {
  background: color-mix(in srgb, var(--danger) 22%, var(--surface)) !important;
  box-shadow: inset 3px 0 0 var(--danger);
}

.row-selectable:focus-visible {
  outline: 2px solid var(--brand-500);
  outline-offset: -2px;
}

.trace-sec {
  display: flex;
  flex-direction: column;
  gap: 8px;
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
  padding: 8px 10px;
  border-radius: var(--radius-sm);
  border: 1px solid var(--line);
  background: var(--surface);
}

/* 访客画像：自适应网格，适应右侧卡片宽度 */
.profile {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(160px, 1fr));
  gap: 10px 14px;
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
  margin-bottom: 2px;
}

.profile-item > dd {
  margin: 0;
  font-size: 12px;
  color: var(--fg);
  display: flex;
  align-items: center;
  gap: 6px;
  min-width: 0;
}

/* UA 独占整行 */
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
