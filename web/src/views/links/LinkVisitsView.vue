<template>
  <div class="pb-10" data-od-id="link-visits-view">
    <!-- ==================== 页头:短码 / 承载域名 / 类型 + 返回 ==================== -->
    <PageHeader title="短链访问明细" :description="headerDescription">
      <template #actions>
        <AppTag v-if="link" color="default">{{ linkTypeLabel }}</AppTag>
        <AppButton size="sm" variant="outline" :loading="loading" title="刷新访问明细" @click="loadData">
          刷新
        </AppButton>
        <AppButton size="sm" variant="outline" @click="goBack">
          <template #icon><ArrowLeft :size="13" /></template>
          返回列表
        </AppButton>
      </template>
    </PageHeader>

    <!-- ==================== 短链加载失败(如已被彻底删除) ==================== -->
    <AppCard v-if="loadError" :padding="false" data-od-id="link-visits-error">
      <AppResult status="error" title="无法查看该短链的访问明细" :sub-title="loadError">
        <template #extra>
          <AppButton size="sm" type="primary" @click="goBack">
            <template #icon><ArrowLeft :size="13" /></template>
            返回短链列表
          </AppButton>
        </template>
      </AppResult>
    </AppCard>

    <!-- ==================== 首次加载 ==================== -->
    <AppCard v-else-if="loading && !loaded" :padding="false">
      <div class="flex flex-col items-center justify-center py-16">
        <AppSpin size="large" />
        <p class="mt-3 text-xs text-ink-soft">正在拉取该短链的访问明细…</p>
      </div>
    </AppCard>

    <template v-else>
      <!-- ==================== 左右分栏布局：左表格 + 右画像及规则回放 ==================== -->
      <div class="grid grid-cols-1 items-start gap-5 xl:grid-cols-12" data-od-id="link-visits-split-layout">
        <!-- 左侧：访问明细列表 -->
        <AppCard :padding="false" class="flex min-w-0 flex-col xl:col-span-7" data-od-id="link-visits-list">
          <div class="flex flex-wrap items-center justify-between gap-3.5 border-b border-line px-4 py-3">
            <div class="min-w-0">
              <h2 class="text-base font-semibold tracking-tight text-ink">访问明细</h2>
              <p class="mt-1 text-xs leading-relaxed text-ink-faint">
                逐条记录该短链每次{{ isLanding ? '落地页视图与按钮点击' : '跳转' }}的动作与结果，点击列表行在右侧查看访客画像与规则回放。
              </p>
            </div>
            <span class="mono text-xs text-ink-faint">共 {{ total }} 条</span>
          </div>

          <!-- 工具栏:设备类型筛选 / 只看失败 / 关键词(均仅在当前页数据内过滤) -->
          <div class="flex flex-wrap items-center gap-3 px-4 py-3">
            <div
              role="group"
              aria-label="按设备类型筛选"
              class="inline-flex gap-0.5 rounded-lg border border-line bg-surface-muted p-0.5"
            >
              <AppButton
                v-for="opt in DEVICE_OPTIONS"
                :key="opt.value"
                size="sm"
                :variant="deviceFilter === opt.value ? 'secondary' : 'ghost'"
                :class="deviceFilter === opt.value ? 'font-semibold text-ink' : 'text-ink-soft'"
                :aria-pressed="deviceFilter === opt.value"
                @click="deviceFilter = opt.value"
              >
                {{ opt.label }}
              </AppButton>
            </div>

            <div class="flex items-center gap-1.5">
              <AppSwitch
                v-model="onlyFailed"
                aria-label="只看失败"
                title="仅显示失败的动作(前端过滤,不影响总数)"
              />
              <span class="text-xs text-ink-soft">只看失败</span>
            </div>

            <AppInput
              v-model="keyword"
              allow-clear
              class="min-w-[200px] grow"
              placeholder="搜索 IP、UA、来源或目标…"
              aria-label="搜索访问明细"
            >
              <template #prefix><Search :size="14" /></template>
            </AppInput>

            <AppButton v-if="hasLocalFilter" size="sm" variant="ghost" class="text-ink-soft" @click="resetLocalFilters">
              <template #icon><X :size="13" /></template>
              清空过滤
            </AppButton>

            <span v-if="hasLocalFilter" class="text-xs text-ink-faint" data-od-id="visit-filter-scope-hint">
              设备、搜索与「只看失败」仅筛选当前页
            </span>
          </div>

          <div class="px-4 pb-4">
            <AppSpin :spinning="loading">
              <AppTable
                :columns="columns"
                :data-source="tableData"
                :scroll="{ x: 1040 }"
                row-key="id"
                :row-props="visitRowProps"
                @row-click="onRowClick"
              >
                <template #header="{ column }">
                  <span :title="COLUMN_HINTS[column.key]">{{ column.title }}</span>
                </template>

                <template #empty>
                  <AppEmpty
                    :description="
                      rows.length === 0
                        ? '该短链暂无访问记录,短链被访问后明细将在此处逐条呈现'
                        : '当前页没有符合筛选条件的记录(搜索与「只看失败」仅在当前页数据内过滤)'
                    "
                  />
                  <div v-if="rows.length > 0" class="flex justify-center">
                    <AppButton size="sm" @click="resetLocalFilters">重置本地过滤</AppButton>
                  </div>
                </template>

                <template #cell="{ column, record }">
                  <!-- 时间:列里只给时分秒,完整年月日悬停看 -->
                  <template v-if="column.key === 'time'">
                    <AppTooltip :title="formatDateTime(toRow(record).visit.createdAt)">
                      <span class="mono cursor-help text-xs underline decoration-dotted underline-offset-2">
                        {{ formatClock(toRow(record).visit.createdAt) }}
                      </span>
                    </AppTooltip>
                  </template>

                  <!-- 来访 IP -->
                  <template v-else-if="column.key === 'ip'">
                    <span class="mono text-xs" :title="toRow(record).visit.ip || '未知 IP'">
                      {{ toRow(record).visit.ip || '—' }}
                    </span>
                  </template>

                  <!-- 地理位置:国家 / 数据中心 / 语言 -->
                  <template v-else-if="column.key === 'geo'">
                    <div class="flex min-w-0 flex-col gap-0.5">
                      <span
                        class="text-xs"
                        :title="toRow(record).country === '—' ? '该 IP 查不到国家（私网/回环/库中未收录）' : toRow(record).visit.country"
                      >
                        国家:{{ toRow(record).country }}
                      </span>
                      <AppTag :color="toRow(record).network.color" :title="toRow(record).network.title">
                        {{ toRow(record).network.text }}
                      </AppTag>
                      <span
                        class="text-xs text-ink-soft"
                        :title="toRow(record).lang || '未携带 Accept-Language'"
                      >
                        语言:{{ toRow(record).lang }}
                      </span>
                    </div>
                  </template>

                  <!-- 设备型号 -->
                  <template v-else-if="column.key === 'device'">
                    <div v-if="toRow(record).hasUa" class="flex min-w-0 flex-col gap-0.5">
                      <div class="flex min-w-0 flex-nowrap items-center gap-1.5">
                        <AppTag :color="deviceTagColor(toRow(record).parsedUa.deviceType)">
                          {{ toRow(record).parsedUa.deviceType }}
                        </AppTag>
                        <span class="text-xs" :title="toRow(record).parsedUa.deviceModel">
                          {{ toRow(record).parsedUa.deviceModel }}
                        </span>
                      </div>
                      <span
                        class="text-xs text-ink-soft"
                        :title="toRow(record).parsedUa.os + ' · ' + toRow(record).parsedUa.browser"
                      >
                        {{ toRow(record).parsedUa.os }} · {{ toRow(record).parsedUa.browser }}
                      </span>
                    </div>
                    <span v-else class="text-xs text-ink-soft">未知设备</span>
                  </template>

                  <!-- 动作与结果：清楚显示具体命中的失败规则名称与具体命中的条件 -->
                  <template v-else-if="column.key === 'action'">
                    <div class="flex max-w-[220px] flex-col gap-1">
                      <div class="flex flex-nowrap items-center gap-1">
                        <AppTag :color="toRow(record).action.color">{{ toRow(record).action.text }}</AppTag>
                        <AppTag :color="toRow(record).visit.outcome === 'failed' ? 'error' : 'success'">
                          {{ toRow(record).visit.outcome === 'failed' ? '✗ 失败' : '✓ 成功' }}
                        </AppTag>
                      </div>
                      <span
                        v-if="toRow(record).reasonText"
                        class="text-xs text-err"
                        :title="toRow(record).reasonTooltip"
                      >
                        {{ toRow(record).reasonText }}
                      </span>
                    </div>
                  </template>

                  <!-- 目标 / 来源 -->
                  <template v-else-if="column.key === 'target'">
                    <div class="flex min-w-0 max-w-[320px] flex-col gap-0.5">
                      <span
                        v-if="toRow(record).visit.targetUrl"
                        class="mono text-xs"
                        :title="toRow(record).visit.targetUrl"
                      >
                        {{ toRow(record).visit.targetUrl }}
                      </span>
                      <span
                        v-else-if="toRow(record).visit.referer"
                        class="text-xs"
                        :title="toRow(record).visit.referer"
                      >
                        {{ toRow(record).visit.referer }}
                      </span>
                      <span v-else class="text-xs text-ink-soft">直接访问</span>
                      <span
                        v-if="toRow(record).visit.targetUrl && toRow(record).visit.referer"
                        class="text-xs text-ink-soft"
                        :title="'来源页:' + toRow(record).visit.referer"
                      >
                        来自 {{ toRow(record).visit.referer }}
                      </span>
                    </div>
                  </template>
                </template>
              </AppTable>
            </AppSpin>
          </div>

          <!-- 底栏与真实分页 -->
          <div class="flex flex-wrap items-center justify-between gap-3 border-t border-line px-4 py-3">
            <div class="flex items-center gap-3 text-xs text-ink-soft">
              <span>共 <strong class="font-mono text-ink">{{ total }}</strong> 条访问明细</span>
              <span v-if="hasLocalFilter">
                当前页命中
                <strong class="font-mono text-ink">{{ filteredRows.length }}</strong> / {{ rows.length }} 条
              </span>
            </div>
            <div class="flex items-center gap-2.5">
              <div class="flex items-center gap-1.5 text-xs text-ink-soft">
                <span>每页</span>
                <AppSelect
                  v-model="pageSize"
                  size="sm"
                  class="w-[76px]"
                  :options="PAGE_SIZE_OPTIONS"
                  aria-label="每页条数"
                  @change="onPageSizeChange"
                />
                <span>条</span>
              </div>
              <div class="flex items-center gap-1.5">
                <AppButton size="sm" variant="outline" :disabled="page <= 1 || loading" @click="goToPage(page - 1)">
                  上一页
                </AppButton>
                <span class="mono px-1 text-xs text-ink-soft">{{ page }} / {{ totalPages }}</span>
                <AppButton
                  size="sm"
                  variant="outline"
                  :disabled="page >= totalPages || loading"
                  @click="goToPage(page + 1)"
                >
                  下一页
                </AppButton>
              </div>
            </div>
          </div>
        </AppCard>

        <!-- 右侧：访客画像与规则回放详情卡片 -->
        <AppCard
          :padding="false"
          class="sticky top-20 flex max-h-[calc(100vh-6rem)] min-w-0 flex-col xl:col-span-5"
          data-od-id="link-visit-detail-panel"
        >
          <!-- 未选择行时的引导提示 -->
          <div
            v-if="!selectedRow"
            class="my-auto flex min-h-[380px] flex-col items-center justify-center p-10 text-center"
          >
            <div class="mb-3 flex h-12 w-12 items-center justify-center rounded-full bg-brand-500/10 text-brand-600">
              <MousePointerClick :size="22" />
            </div>
            <h3 class="text-sm font-semibold text-ink">选择访问记录</h3>
            <p class="mt-1.5 max-w-[260px] text-xs leading-relaxed text-ink-soft">
              在左侧列表中点击任意一行，即可在此查看该次请求的完整访客画像、真实裁决以及按当前规则集的决策链路回放。
            </p>
          </div>

          <!-- 选中行时的详情展示 -->
          <template v-else>
            <div class="shrink-0 border-b border-line px-4 py-3">
              <div class="flex flex-wrap items-center justify-between gap-3.5">
                <div class="min-w-0">
                  <div class="flex items-center gap-2">
                    <h2 class="text-base font-semibold tracking-tight text-ink">访客画像与规则回放</h2>
                    <AppTag :color="selectedRow.visit.outcome === 'failed' ? 'error' : 'success'">
                      {{ selectedRow.visit.outcome === 'failed' ? '✗ 访问失败' : '✓ 访问成功' }}
                    </AppTag>
                  </div>
                  <p class="mono mt-1 text-xs text-ink-soft">
                    ID #{{ selectedRow.visit.id }} · {{ formatDateTime(selectedRow.visit.createdAt) }}
                  </p>
                </div>
                <div class="flex items-center gap-1.5">
                  <AppButton
                    size="sm"
                    variant="outline"
                    title="带着这个访客去模拟器改规则"
                    @click="openInSimulator(selectedRow)"
                  >
                    <template #icon><FlaskConical :size="13" /></template>
                    模拟器打开
                  </AppButton>
                  <AppButton
                    size="icon"
                    variant="ghost"
                    class="h-8 w-8 text-ink-soft"
                    title="关闭详情"
                    aria-label="关闭详情"
                    @click="selectedId = null"
                  >
                    <template #icon><X :size="14" /></template>
                  </AppButton>
                </div>
              </div>
            </div>

            <!-- 可滚动的明细主体 -->
            <div class="flex-1 space-y-5 overflow-y-auto p-4">
              <!-- ① 访客画像 -->
              <div class="flex flex-col gap-2">
                <div class="flex flex-wrap items-baseline justify-between gap-2">
                  <span class="mono text-2xs font-semibold text-ink-soft">访客画像 · 本次请求</span>
                  <span class="text-xs text-ink-soft">规则条件匹配的字段明细</span>
                </div>
                <dl class="grid grid-cols-[repeat(auto-fit,minmax(160px,1fr))] gap-x-3.5 gap-y-2.5">
                  <div class="min-w-0">
                    <dt class="mb-0.5 flex items-center gap-1.5 text-2xs text-ink-faint">访问时间</dt>
                    <dd class="mono m-0 flex items-center gap-1.5 text-xs text-ink">
                      {{ formatDateTime(selectedRow.visit.createdAt) }}
                    </dd>
                  </div>
                  <div class="min-w-0">
                    <dt class="mb-0.5 flex items-center gap-1.5 text-2xs text-ink-faint">来访 IP</dt>
                    <dd class="mono m-0 flex items-center gap-1.5 text-xs text-ink">{{ selectedRow.visit.ip || '—' }}</dd>
                  </div>
                  <div class="min-w-0">
                    <dt class="mb-0.5 flex items-center gap-1.5 text-2xs text-ink-faint">国家 / 地区</dt>
                    <dd class="m-0 flex items-center gap-1.5 text-xs text-ink">
                      <AppTag :color="selectedRow.country === '—' ? 'default' : 'success'">
                        {{ selectedRow.country }}
                      </AppTag>
                    </dd>
                  </div>
                  <div class="min-w-0">
                    <dt class="mb-0.5 flex items-center gap-1.5 text-2xs text-ink-faint">网络属性</dt>
                    <dd class="m-0 flex items-center gap-1.5 text-xs text-ink">
                      <AppTag :color="selectedRow.network.color" :title="selectedRow.network.title">
                        {{ selectedRow.network.text }}
                      </AppTag>
                    </dd>
                  </div>
                  <div class="min-w-0">
                    <dt class="mb-0.5 flex items-center gap-1.5 text-2xs text-ink-faint">语言 (Accept-Language)</dt>
                    <dd class="mono m-0 flex min-w-0 items-center gap-1.5 text-xs text-ink" :title="selectedRow.lang">
                      {{ selectedRow.lang }}
                    </dd>
                  </div>
                  <div class="min-w-0">
                    <dt class="mb-0.5 flex items-center gap-1.5 text-2xs text-ink-faint">设备类型与型号</dt>
                    <dd class="m-0 flex min-w-0 items-center gap-1.5 text-xs text-ink">
                      <AppTag :color="deviceTagColor(selectedRow.parsedUa.deviceType)">
                        {{ selectedRow.parsedUa.deviceType }}
                      </AppTag>
                      <span class="text-xs text-ink-soft" :title="selectedRow.parsedUa.deviceModel">
                        {{ selectedRow.parsedUa.deviceModel }}
                      </span>
                    </dd>
                  </div>
                  <div class="min-w-0">
                    <dt class="mb-0.5 flex items-center gap-1.5 text-2xs text-ink-faint">系统 / 浏览器</dt>
                    <dd
                      class="m-0 flex min-w-0 items-center gap-1.5 text-xs text-ink"
                      :title="selectedRow.parsedUa.os + ' / ' + selectedRow.parsedUa.browser"
                    >
                      {{ selectedRow.parsedUa.os }}
                      <span class="text-ink-soft">/</span>
                      {{ selectedRow.parsedUa.browser }}
                    </dd>
                  </div>
                  <div class="min-w-0">
                    <dt class="mb-0.5 flex items-center gap-1.5 text-2xs text-ink-faint">短链</dt>
                    <dd
                      class="mono m-0 flex min-w-0 items-center gap-1.5 text-xs text-ink"
                      :title="(selectedRow.visit.domain || link?.domains?.[0] || '') + '/' + (link?.code || '-')"
                    >
                      {{ selectedRow.visit.domain || link?.domains?.[0] || '未知域名' }}/{{ link?.code || '-' }}
                    </dd>
                  </div>
                  <div class="min-w-0">
                    <dt class="mb-0.5 flex items-center gap-1.5 text-2xs text-ink-faint">短链类型</dt>
                    <dd class="m-0 text-xs text-ink">{{ linkTypeLabel }} → {{ selectedRow.action.text }}</dd>
                  </div>
                  <div class="min-w-0">
                    <dt class="mb-0.5 flex items-center gap-1.5 text-2xs text-ink-faint">来源页 (Referer)</dt>
                    <dd
                      class="m-0 min-w-0 text-xs text-ink"
                      :title="selectedRow.visit.referer || '直接访问'"
                    >
                      {{ selectedRow.visit.referer || '直接访问' }}
                    </dd>
                  </div>
                  <div class="min-w-0">
                    <dt class="mb-0.5 flex items-center gap-1.5 text-2xs text-ink-faint">最终抵达</dt>
                    <dd
                      class="mono m-0 flex min-w-0 items-center gap-1.5 text-xs text-ink"
                      :title="selectedRow.visit.targetUrl || '—'"
                    >
                      {{ selectedRow.visit.targetUrl || '—' }}
                    </dd>
                  </div>
                  <div class="col-span-full min-w-0">
                    <dt class="mb-0.5 flex items-center gap-1.5 text-2xs text-ink-faint">
                      完整 User-Agent
                      <AppButton
                        size="sm"
                        variant="ghost"
                        class="h-5 px-1.5 text-2xs text-ink-soft hover:text-primary"
                        aria-label="复制完整 User-Agent"
                        @click.stop="copyUa(selectedRow)"
                      >
                        <template #icon><Copy :size="11" /></template>
                        复制
                      </AppButton>
                    </dt>
                    <dd class="mono block rounded-sm border border-line bg-surface px-2 py-1.5 text-2xs leading-[1.55] text-ink-soft break-all">
                      {{ selectedRow.visit.userAgent || '（无 UA 头）' }}
                    </dd>
                  </div>
                </dl>
              </div>

              <!-- 分隔线 -->
              <div class="space-y-4 border-t border-line pt-4">
                <!-- ② 真实裁决：后端当时记下的事实 -->
                <div class="flex flex-col gap-2">
                  <div class="flex flex-wrap items-baseline justify-between gap-2">
                    <span class="mono text-2xs font-semibold text-ink-soft">真实裁决 · 后端记录</span>
                    <span class="text-xs text-ink-soft">后端处理该请求时的实际处置结果</span>
                  </div>

                  <!-- 命中的规则卡片 -->
                  <div
                    v-if="selectedRow.matchedRule"
                    class="space-y-1.5 rounded-lg border border-line bg-surface p-3 shadow-xs"
                  >
                    <div class="flex flex-wrap items-center justify-between gap-2 text-xs">
                      <span class="font-medium text-ink">
                        命中规则 #{{ selectedRow.matchedRule.id }} · {{ selectedRow.matchedRule.name }}
                      </span>
                      <AppTag :color="selectedRow.matchedRule.enabled ? 'success' : 'default'">
                        {{ selectedRow.matchedRule.enabled ? '规则当前启用' : '规则当前已停用' }}
                      </AppTag>
                    </div>
                    <!-- 只展示具体命中的条件，不展示全部条件 -->
                    <div v-if="selectedRow.hitConditionText" class="mono text-xs leading-relaxed text-err">
                      <span class="font-medium">命中条件：</span>{{ selectedRow.hitConditionText }}
                    </div>
                    <div
                      v-else-if="selectedRow.matchedRule.conditions?.length"
                      class="mono text-xs leading-relaxed text-ink-soft"
                    >
                      <span class="font-medium">规则条件：</span>{{ conditionSummary(selectedRow.matchedRule) }}
                    </div>
                  </div>

                  <div class="flex flex-wrap items-center gap-1.5">
                    <AppTag :color="realVerdictOf(selectedRow).color">{{ realVerdictOf(selectedRow).text }}</AppTag>
                    <AppTag :color="selectedRow.visit.outcome === 'failed' ? 'error' : 'success'">
                      {{ selectedRow.visit.outcome === 'failed' ? '✗ 失败' : '✓ 成功' }}
                    </AppTag>
                    <span v-if="selectedRow.reasonText" class="text-xs text-err">{{ selectedRow.reasonText }}</span>
                    <span
                      v-if="selectedRow.visit.ruleId == null && selectedRow.visit.outcome !== 'failed'"
                      class="text-xs text-ink-soft"
                    >
                      没有规则参与，按默认短链配置放行
                    </span>
                  </div>
                </div>

                <!-- ③ 规则回放：按当前规则集重算一遍 -->
                <div class="flex flex-col gap-2">
                  <div class="flex flex-wrap items-baseline justify-between gap-2">
                    <span class="mono text-2xs font-semibold text-ink-soft">规则回放 · 按当前规则集</span>
                    <span class="text-xs text-ink-soft">规则若被改动过，回放可能与历史裁决不同</span>
                  </div>

                  <div v-if="traceStateOf(selectedRow.visit.id)?.loading" class="flex items-center gap-2 py-3">
                    <RefreshCw class="animate-spin text-brand-500" :size="14" />
                    <span class="text-xs text-ink-soft">正在回放规则链…</span>
                  </div>

                  <p
                    v-else-if="traceStateOf(selectedRow.visit.id).error"
                    class="text-xs text-err"
                  >
                    回放失败：{{ traceStateOf(selectedRow.visit.id).error }}
                  </p>

                  <template v-else-if="traceStateOf(selectedRow.visit.id).trace">
                    <div class="flex flex-wrap items-center gap-1.5">
                      <AppTag :color="verdictTagColor(traceStateOf(selectedRow.visit.id).verdict)">
                        {{ traceStateOf(selectedRow.visit.id).verdict!.title }}
                      </AppTag>
                      <span class="text-xs text-ink-soft">
                        {{ traceStateOf(selectedRow.visit.id).trace!.scopeNote }}
                      </span>
                    </div>

                    <!-- 回放与历史不一致提示 -->
                    <div
                      v-if="replayDiffers(selectedRow)"
                      class="rounded-lg border border-warn/40 bg-warn/10 p-2.5 text-xs leading-relaxed text-warn"
                    >
                      回放与当时的裁决不一致：当时是
                      {{ selectedRow.visit.ruleId == null ? '无规则命中' : `命中 #${selectedRow.visit.ruleId}` }}，
                      当前规则下会{{ traceStateOf(selectedRow.visit.id).verdict!.matched
                        ? `命中 #${traceStateOf(selectedRow.visit.id).trace!.matched!.id}`
                        : '无规则命中' }}。
                    </div>

                    <ul class="flex flex-col gap-1.5">
                      <li
                        v-for="step in traceStateOf(selectedRow.visit.id).trace!.steps"
                        :key="step.key"
                        class="flex flex-wrap items-center gap-1.5 rounded-sm border border-line bg-surface px-2.5 py-2"
                        :class="step.status === 'hit' ? 'border-ok/40' : step.status === 'block' ? 'border-err/40' : ''"
                      >
                        <span class="mono text-xs text-ink-faint">#{{ step.ruleId }}</span>
                        <span class="text-xs font-medium text-ink">{{ step.ruleName }}</span>
                        <AppTag :color="step.status === 'block' ? 'error' : step.status === 'hit' ? 'success' : 'default'">
                          {{ step.statusText }}
                        </AppTag>
                        <span class="w-full text-xs text-ink-soft">{{ step.whyText }}</span>
                        <!-- 命中的规则只展示具体命中的条件，不展示未命中的其余条件 -->
                        <template v-if="step.facts.length > 0">
                          <template v-if="step.status === 'hit' || step.status === 'block'">
                            <span
                              v-for="(fact, i) in step.facts.filter((f) => f.hit)"
                              :key="i"
                              class="mono text-xs text-ink"
                            >✓ {{ fact.text }}</span>
                          </template>
                          <template v-else>
                            <span
                              v-for="(fact, i) in step.facts"
                              :key="i"
                              class="mono text-xs"
                              :class="fact.hit ? 'text-ink' : 'text-ink-faint'"
                            >{{ fact.hit ? '✓' : '✗' }} {{ fact.text }}</span>
                          </template>
                        </template>
                      </li>
                    </ul>

                    <p v-if="traceStateOf(selectedRow.visit.id).trace!.skippedForDetail > 0" class="text-xs text-ink-soft">
                      {{ traceStateOf(selectedRow.visit.id).trace!.skippedForDetail }} 条规则未取到条件，未参与本次回放
                    </p>
                  </template>
                </div>

                <!-- ④ 带着这个访客去模拟器改规则 -->
                <div class="flex flex-col gap-1.5 pt-2">
                  <AppButton size="sm" type="primary" block @click="openInSimulator(selectedRow)">
                    <template #icon><FlaskConical :size="13" /></template>
                    用此访客在模拟器打开
                  </AppButton>
                  <span class="text-center text-xs text-ink-soft">在模拟器里改条件验一遍，再回到这里看真实流量怎么走</span>
                </div>
              </div>
            </div>
          </template>
        </AppCard>
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
import type { TableColumn } from '@/components/ui/types';
import type { TagColor } from '@/components/ui/AppTag.vue';
import { formatClock, formatCountry, formatDateTime } from '@/utils/format';
import { message } from '@/utils/toast';
import { getDeviceTagColor, parseUserAgent } from '@/utils/userAgent';
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
const ACTION_META: Record<VisitAction, { text: string; color: TagColor }> = {
  redirect: { text: '跳转', color: 'default' },
  landing_view: { text: '落地页', color: 'warning' },
  click: { text: '点击', color: 'success' },
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

/** 表格列定义;key 同时用作 #cell 分支与 #header 提示文案的索引 */
const columns: TableColumn[] = [
  { key: 'time', title: '时间', width: 96 },
  { key: 'ip', title: 'IP', width: 130 },
  { key: 'geo', title: '地理位置' },
  { key: 'device', title: '设备型号' },
  { key: 'action', title: '动作与结果', width: 240 },
  { key: 'target', title: '目标 / 来源' },
];

/** 各列的补充说明,悬停表头可见 */
const COLUMN_HINTS: Record<string, string> = {
  time: '服务端记录的访问时间',
  ip: '来访 IP:X-Forwarded-For 优先,回退 RemoteAddr',
  geo: '国家由后端内嵌离线 GeoIP 库解析',
  device: '由 User-Agent 解析:设备型号 · 操作系统 · 浏览器',
  action: '本次触发的动作与结果;失败的动作不计入访问次数',
  target: '本次动作最终抵达的地址,缺省时回退显示来源页',
};

/** 分页每页条数选项 */
const PAGE_SIZE_OPTIONS = [10, 20, 50, 100].map((size) => ({ value: size, label: String(size) }));

/** 表格行:在访问记录之上补齐解析结果与展示文案 */
interface VisitRow {
  visit: Visit;
  hasUa: boolean;
  parsedUa: ParsedUA;
  country: string;
  lang: string;
  network: { text: string; color: TagColor; title: string };
  action: { text: string; color: TagColor };
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
  const meta = ACTION_META[visit.action] ?? { text: visit.action || '未知动作', color: 'default' as TagColor };
  const rule = visit.ruleId != null ? rulesMap.value.get(visit.ruleId) : undefined;
  const { text: reasonText, tooltip: reasonTooltip, hitConditionText } = formatFailureReason(visit, rule);

  return {
    visit,
    hasUa,
    parsedUa: parseUserAgent(visit.userAgent || ''),
    country: formatCountry(visit.country),
    lang: visit.lang || '—',
    network: visit.isDatacenter
      ? { text: '数据中心', color: 'warning' as TagColor, title: visit.asn ? `ASN ${visit.asn}` : '数据中心出口' }
      : {
          text: '住宅/未知',
          color: 'default' as TagColor,
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

/** #cell 插槽拿到的 record 是 Record<string, unknown>,按仓库既有模式收窄回 VisitRow */
function asRow(record: Record<string, unknown>): VisitRow {
  return record as unknown as VisitRow;
}

/**
 * #cell 插槽拿到的 record 是 Record<string, unknown>,按仓库既有模式收窄回 VisitRow。
 * 另给每行补一个扁平 id 供 AppTable 的 row-key 使用:行数据是包装对象(visit 在里面),
 * 而 AppTable 取 key 的方式是 record[rowKey],撑不起 'visit.id' 这种路径。
 */
const tableData = computed<Record<string, unknown>[]>(() =>
  filteredRows.value.map((row) => ({ ...row, id: row.visit.id })),
);

/** 把行状态透传到真实 <tr>,供 <style scoped> 里的 :deep() 行状态规则消费 */
function visitRowProps(record: Record<string, unknown>): Record<string, unknown> {
  const row = asRow(record);
  return {
    'data-outcome': row.visit.outcome,
    'data-selected': selectedId.value === row.visit.id,
  };
}

function onRowClick(record: Record<string, unknown>): void {
  selectRow(asRow(record));
}

/** 回放裁决对应的徽标色:命中拦截=红,命中非拦截=绿,未命中=中性 */
function verdictTagColor(verdict: Verdict | null): TagColor {
  if (!verdict || !verdict.matched) return 'default';
  return verdict.blocking ? 'error' : 'success';
}

function deviceTagColor(deviceType: ParsedUA['deviceType']): TagColor {
  return getDeviceTagColor(deviceType);
}

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
function realVerdictOf(row: VisitRow): { text: string; color: TagColor } {
  if (row.visit.ruleId == null) {
    if (row.visit.reason === 'rule_blocked') {
      return { text: '规则拦截 · 直接 404', color: 'error' };
    }
    if (row.visit.reason === 'rule_throttled') {
      return { text: '规则限流 · 429', color: 'error' };
    }
    return { text: '无规则参与', color: 'default' };
  }
  const action = row.visit.ruleAction as RuleAction;
  const label = actionLabel(action) || action || '未知动作';
  const rule = row.matchedRule;
  const namePart = rule ? `「${rule.name}」` : '';
  return {
    text: `命中 #${row.visit.ruleId} ${namePart} · ${label}`,
    color: isBlockingAction(action) ? 'error' : 'success',
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
/*
 * 只保留表格「行状态」这四条规则：它们必须打到 AppTable 子组件内部的真实 <tr> 上，
 * 工具类做不到，所以走 :deep()；其余一律用语义令牌 + Tailwind 工具类表达。
 * 底色直接用 color-mix 展开成 --err，不再依赖 Tech-Utility 层的 --danger/--danger-soft。
 */
:deep(.app-table tbody tr[data-outcome='failed']) {
  background: color-mix(in srgb, var(--err) 12%, var(--surface));
}
:deep(.app-table tbody tr[data-outcome='failed']:hover) {
  background: color-mix(in srgb, var(--err) 18%, var(--surface));
}
:deep(.app-table tbody tr[data-selected='true']) {
  background: color-mix(in srgb, var(--color-brand-500) 10%, var(--surface)) !important;
  box-shadow: inset 3px 0 0 var(--color-brand-500);
}
:deep(.app-table tbody tr[data-outcome='failed'][data-selected='true']) {
  background: color-mix(in srgb, var(--err) 22%, var(--surface)) !important;
  box-shadow: inset 3px 0 0 var(--err);
}
:deep(.app-table tbody tr[data-selected='true']:focus-visible) {
  outline: 2px solid var(--color-brand-500);
  outline-offset: -2px;
}
</style>
