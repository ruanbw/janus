<template>
  <div class="space-y-5" data-od-id="overview-view">
    <!--
      时间范围快捷选择。窗口作用于所有访问类指标（总访问数、点击数、CTR、地图、
      排行与各分布图）；短链/域名这类"当前库存"不随时间变化。
      控件放在加载分支之外：切窗口时数据要清空重拉，但控件必须一直在，
      否则用户在等待期间连改回原来的范围都做不到。
    -->
    <div class="flex flex-wrap items-center justify-between gap-x-3 gap-y-2">
      <div
        role="group"
        aria-label="时间范围"
        class="inline-flex gap-0.5 rounded-lg border border-border bg-muted p-0.5"
      >
        <AppButton
          v-for="opt in OVERVIEW_RANGES"
          :key="opt.value"
          size="sm"
          variant="ghost"
          :disabled="loading"
          :aria-pressed="range === opt.value"
          :class="range === opt.value
            ? 'bg-background font-semibold text-foreground shadow-xs hover:bg-background'
            : 'text-muted-foreground'"
          @click="selectRange(opt.value)"
        >
          {{ opt.label }}
        </AppButton>
      </div>
      <!-- 服务端回显的实际窗口边界：展示端不自己推算"今日"，时区不一致时那是第二个窗口 -->
      <p v-if="windowText" class="font-mono text-xs text-ink-faint">{{ windowText }}</p>
    </div>

    <!-- 初次加载骨架屏过渡 (与 kpiList 的 4 项卡片对齐，消除布局跳动 CLS) -->
    <template v-if="loading && !overview">
      <section class="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-3 auto-rows-fr">
        <AppCard v-for="i in 4" :key="'kpi-skel-' + i" class="flex flex-col justify-between min-h-[118px] p-4">
          <div class="space-y-2">
            <AppSkeleton width="60px" height="12px" />
            <AppSkeleton width="110px" height="28px" />
          </div>
          <AppSkeleton width="140px" height="14px" />
        </AppCard>
      </section>
      <AppCard class="p-6">
        <div class="flex items-center justify-between pb-4 border-b border-line">
          <div class="space-y-1.5">
            <AppSkeleton width="140px" height="18px" />
            <AppSkeleton width="220px" height="14px" />
          </div>
          <AppSkeleton width="80px" height="28px" />
        </div>
        <div class="pt-6">
          <AppSkeleton variant="rect" height="260px" />
        </div>
      </AppCard>
    </template>

    <!-- 正常数据内容 -->
    <template v-else>
      <!-- KPI 卡片网格 (真实数据驱动：3列自适应、高度统一) -->
      <section class="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-3 auto-rows-fr" :class="revalidating && 'is-revalidating'">
      <AppCard
        v-for="kpi in kpiList"
        :key="kpi.label"
        :data-od-id="kpi.odId"
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
          <p>{{ kpi.sub }}</p>
          <!-- 环比：只有和上一个等长窗口比，绝对值才说明"在涨还是在跌" -->
          <p v-if="kpi.compare" class="mt-1 font-mono">{{ kpi.compare }}</p>
        </div>

        <!--
          跳转 / 落地页占比是原「短链类型与流量结构」卡唯一独有的信息
          （其余数字与其它卡片完全重复），折进总访问数这张卡里：
          合计恒等于卡片上的总访问数，分母是现成的。
        -->
        <div v-if="kpi.split" class="mt-3">
          <div
            class="flex h-1.5 w-full overflow-hidden rounded-full border border-line"
            role="img"
            :aria-label="kpi.split.ariaLabel"
          >
            <span class="h-full bg-brand" :style="{ width: `${kpi.split.redirectPercent}%` }"></span>
            <span class="h-full bg-info" :style="{ width: `${kpi.split.landingPercent}%` }"></span>
          </div>
          <div class="mt-1.5 font-mono text-2xs text-ink-soft">{{ kpi.split.text }}</div>
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
    <WorldMapPanel
      v-if="totals.visits > 0"
      :countries="facets?.countries ?? []"
      :total-visits="totals.visits"
      :coverage-truncated="userAgentsTruncated"
      :user-agent-limit="USER_AGENT_LIMIT"
    />

    <!--
      分析与分布卡片：宽屏一行三列 / 中屏两列 / 手机一列，用 Tailwind 断点工具类表达。
      卡片内部的条形行则按卡片自身宽度换挡，依赖 @container 容器查询。
    -->
    <section class="grid grid-cols-1 items-stretch gap-4 sm:grid-cols-2 xl:grid-cols-3">
      <!-- 暂无流量数据卡片 -->
      <AppCard v-if="totals.visits === 0" :padding="false" class="sm:col-span-2 xl:col-span-3">
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
              {{ rangeLabel }}内暂无流量访问数据，投放或测试访问后将在此自动汇总。
            </p>
            <p class="mt-1 text-xs text-ink-faint">
              当前已配置 {{ totals.links.toLocaleString() }} 条短链（{{ totals.activeLinks.toLocaleString() }} 条已启用），{{ domains.length }} 个域名（{{ activeDomainsCount }} 个已激活）。
            </p>
            <div class="mt-6 flex flex-wrap items-center justify-center gap-3">
              <AppButton to="/links" size="sm" variant="default">
                {{ totals.links === 0 ? '创建第一条短链' : '前往短链列表' }}
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
      <AppCard v-if="totals.visits > 0" :padding="false" class="@container flex flex-col">
        <div class="flex flex-wrap items-center justify-between gap-3 border-b border-line px-4 py-3">
          <div>
            <h2 class="text-base font-semibold tracking-tight text-ink">热门短链访问排行</h2>
            <p class="mt-0.5 text-xs text-ink-soft">按访问量降序排列，点击单条短链可下钻明细。</p>
          </div>
          <AppButton to="/links" size="sm" variant="outline">
            全部短链 →
          </AppButton>
        </div>
        <div class="flex-1 p-4">
          <div class="flex flex-col gap-2.5" :class="settling && 'is-settling'">
            <div
              v-for="(link, index) in topLinks"
              :key="link.id"
              class="group grid grid-cols-[124px_minmax(0,1fr)_116px] items-center gap-3 rounded-md px-1.5 py-1 -mx-1.5 transition-colors hover:bg-surface-strong/60 @max-[470px]:grid-cols-[minmax(72px,1fr)_minmax(48px,1.3fr)_max-content] @max-[340px]:grid-cols-2 @max-[340px]:gap-y-1.5"
            >
              <div class="flex items-center gap-1.5 min-w-0 @max-[340px]:col-span-1" :title="`/${link.code}`">
                <router-link
                  :to="`/links/${link.id}/visits`"
                  class="truncate font-mono text-xs font-semibold text-ink group-hover:text-brand transition-colors hover:underline"
                >
                  /{{ link.code }}
                </router-link>
                <AppTag :color="link.linkType === 'landing' ? 'info' : 'default'">
                  {{ link.linkType === 'landing' ? '落地页' : '跳转' }}
                </AppTag>
              </div>
              <div class="h-3.5 w-full overflow-hidden rounded bg-surface-strong @max-[340px]:col-span-2 @max-[340px]:row-start-2">
                <div
                  class="app-bar-fill h-full w-full rounded"
                  :class="index === 0 ? 'bg-brand' : 'bg-brand/80'"
                  :style="{ transform: `scaleX(${shareWidth(link.id)})` }"
                />
              </div>
              <div class="flex items-center justify-end gap-1.5 whitespace-nowrap font-mono text-right text-xs text-ink-soft @max-[340px]:col-start-2 @max-[340px]:row-start-1">
                <span>{{ link.visits.toLocaleString() }} · {{ sharePercent(link.visits) }}%</span>
                <router-link
                  :to="`/links/${link.id}/visits`"
                  class="opacity-0 group-hover:opacity-100 transition-opacity text-ink-faint hover:text-brand"
                  title="查看该短链访问明细"
                >
                  <ExternalLink :size="12" />
                </router-link>
              </div>
            </div>
          </div>
        </div>
        <div class="border-t border-line px-4 py-2.5 text-xs text-ink-faint">
          按单条短链累计访问量排序，反映流量在各投放短码上的集中度。
        </div>
      </AppCard>

      <!-- 失败与风险拦截 -->
      <AppCard v-if="totals.visits > 0 || failureTotal > 0" :padding="false" class="@container flex flex-col">
        <div class="flex flex-wrap items-center justify-between gap-3 border-b border-line px-4 py-3">
          <div>
            <h2 class="text-base font-semibold tracking-tight text-ink">失败与风险拦截</h2>
            <p class="mt-0.5 text-xs text-ink-soft">规则拦截（404 / 429）与短链失效的访问。</p>
          </div>
          <span v-if="failureTotal > 0" class="whitespace-nowrap font-mono text-xs font-medium text-warn">失败率 {{ failureRateText }}</span>
        </div>
        <div class="flex-1 p-4">
          <p v-if="failureRows.length === 0" class="py-6 text-center text-xs text-ink-soft">
            窗口内没有失败访问
          </p>
          <div v-else class="flex flex-col gap-2.5" :class="settling && 'is-settling'">
            <div
              v-for="row in failureRows"
              :key="row.reason"
              class="grid grid-cols-[124px_minmax(0,1fr)_116px] items-center gap-3 @max-[470px]:grid-cols-[minmax(72px,1fr)_minmax(48px,1.3fr)_max-content] @max-[340px]:grid-cols-2 @max-[340px]:gap-y-1.5"
            >
              <span class="min-w-0 truncate text-xs text-ink @max-[340px]:col-span-1">{{ row.label }}</span>
              <div class="h-3.5 w-full overflow-hidden rounded bg-surface-strong @max-[340px]:col-span-2 @max-[340px]:row-start-2">
                <div class="app-bar-fill h-full w-full rounded bg-warn" :style="{ transform: `scaleX(${sweepRatioOf('失败', row.reason)})` }" />
              </div>
              <span class="whitespace-nowrap font-mono text-right text-xs text-ink-soft @max-[340px]:col-start-2 @max-[340px]:row-start-1">{{ row.count.toLocaleString() }} 次 · {{ row.percent }}%</span>
            </div>
          </div>
        </div>
        <div class="border-t border-line px-4 py-2.5 text-xs text-ink-faint">
          失败访问不计入总访问数；失败率 = 失败 /（成功访问 + 失败）。短码本身不存在（扫描器乱猜）不落行，故不在此列。
        </div>
      </AppCard>

      <!-- 语言分布 -->
      <AppCard v-if="totals.visits > 0" :padding="false" class="@container flex flex-col">
        <div class="flex flex-wrap items-center justify-between gap-3 border-b border-line px-4 py-3">
          <div>
            <h2 class="text-base font-semibold tracking-tight text-ink">语言分布</h2>
            <p class="mt-0.5 text-xs text-ink-soft">按 Accept-Language 的首个语言标签统计。</p>
          </div>
        </div>
        <div class="flex-1 p-4">
          <p v-if="languageBreakdown.length === 0" class="py-6 text-center text-xs text-ink-soft">
            窗口内没有语言数据
          </p>
          <div v-else class="flex flex-col gap-2.5" :class="settling && 'is-settling'">
            <div
              v-for="(lang, index) in languageBreakdown"
              :key="lang.name"
              class="grid grid-cols-[124px_minmax(0,1fr)_116px] items-center gap-3 @max-[470px]:grid-cols-[minmax(72px,1fr)_minmax(48px,1.3fr)_max-content] @max-[340px]:grid-cols-2 @max-[340px]:gap-y-1.5"
            >
              <span class="min-w-0 truncate text-xs text-ink @max-[340px]:col-span-1">{{ lang.name }}</span>
              <div class="h-3.5 w-full overflow-hidden rounded bg-surface-strong @max-[340px]:col-span-2 @max-[340px]:row-start-2">
                <div
                  class="app-bar-fill h-full w-full rounded"
                  :class="index === 0 && lang.count > 0 ? 'bg-brand' : 'bg-brand/40'"
                  :style="{ transform: `scaleX(${sweepRatioOf('语言', lang.name)})` }"
                />
              </div>
              <span class="whitespace-nowrap font-mono text-right text-xs text-ink-soft @max-[340px]:col-start-2 @max-[340px]:row-start-1">{{ lang.count.toLocaleString() }} 次 · {{ lang.percent }}%</span>
            </div>
          </div>
        </div>
        <div class="border-t border-line px-4 py-2.5 text-xs text-ink-faint">
          未携带该请求头的访问归入「未携带」；规则引擎的「语言」条件用的就是这个标签。
          <template v-if="languageTruncated">标签种类过多，本图按访问量前 {{ USER_AGENT_LIMIT }} 种语言标签统计。</template>
        </div>
      </AppCard>

      <!-- 流量来源分布 -->
      <AppCard v-if="totals.visits > 0" :padding="false" class="@container flex flex-col">
        <div class="flex flex-wrap items-center justify-between gap-3 border-b border-line px-4 py-3">
          <div>
            <h2 class="text-base font-semibold tracking-tight text-ink">流量来源分布</h2>
            <p class="mt-0.5 text-xs text-ink-soft">根据请求 Referrer 与广告点击特征自动归类。</p>
          </div>
        </div>
        <div class="flex-1 p-4">
          <div class="flex flex-col gap-2.5" :class="settling && 'is-settling'">
            <div
              v-for="(src, index) in sourceBreakdown"
              :key="src.name"
              class="grid grid-cols-[124px_minmax(0,1fr)_116px] items-center gap-3 @max-[470px]:grid-cols-[minmax(72px,1fr)_minmax(48px,1.3fr)_max-content] @max-[340px]:grid-cols-2 @max-[340px]:gap-y-1.5"
            >
              <span class="min-w-0 truncate text-xs text-ink @max-[340px]:col-span-1">{{ src.name }}</span>
              <div class="h-3.5 w-full overflow-hidden rounded bg-surface-strong @max-[340px]:col-span-2 @max-[340px]:row-start-2">
                <div
                  class="app-bar-fill h-full w-full rounded"
                  :class="index === 0 && src.count > 0 ? 'bg-brand' : 'bg-brand/40'"
                  :style="{ transform: `scaleX(${sweepRatioOf('来源', src.name)})` }"
                />
              </div>
              <span class="whitespace-nowrap font-mono text-right text-xs text-ink-soft @max-[340px]:col-start-2 @max-[340px]:row-start-1">{{ src.count.toLocaleString() }} 次 · {{ src.percent }}%</span>
            </div>
          </div>
        </div>
        <div class="border-t border-line px-4 py-2.5 text-xs text-ink-faint">
          优先解析主流广告渠道（Meta / TikTok / Google），未携带来源的流量归入直接访问。
        </div>
      </AppCard>

      <!-- 设备类型分布 -->
      <AppCard v-if="totals.visits > 0" :padding="false" class="@container flex flex-col">
        <div class="flex flex-wrap items-center justify-between gap-3 border-b border-line px-4 py-3">
          <div>
            <h2 class="text-base font-semibold tracking-tight text-ink">设备类型分布</h2>
            <p class="mt-0.5 text-xs text-ink-soft">基于 User-Agent 特征与视口画像解析。</p>
          </div>
        </div>
        <div class="flex-1 p-4">
          <div class="flex flex-col gap-2.5" :class="settling && 'is-settling'">
            <div
              v-for="(dev, index) in deviceBreakdown"
              :key="dev.name"
              class="grid grid-cols-[124px_minmax(0,1fr)_116px] items-center gap-3 @max-[470px]:grid-cols-[minmax(72px,1fr)_minmax(48px,1.3fr)_max-content] @max-[340px]:grid-cols-2 @max-[340px]:gap-y-1.5"
            >
              <div class="flex items-center gap-1 min-w-0 @max-[340px]:col-span-1">
                <span class="min-w-0 truncate text-xs text-ink">{{ dev.name }}</span>
                <span v-if="dev.name === '爬虫 / 机器人' && dev.count > 0" class="shrink-0 text-2xs text-warn font-medium px-1 rounded bg-warn/10">风险</span>
              </div>
              <div class="h-3.5 w-full overflow-hidden rounded bg-surface-strong @max-[340px]:col-span-2 @max-[340px]:row-start-2">
                <div
                  class="app-bar-fill h-full w-full rounded"
                  :class="dev.name === '爬虫 / 机器人' && dev.count > 0 ? 'bg-warn' : (index === 0 && dev.count > 0 ? 'bg-brand' : 'bg-brand/40')"
                  :style="{ transform: `scaleX(${sweepRatioOf('设备', dev.name)})` }"
                />
              </div>
              <span class="whitespace-nowrap font-mono text-right text-xs text-ink-soft @max-[340px]:col-start-2 @max-[340px]:row-start-1">{{ dev.count.toLocaleString() }} 次 · {{ dev.percent }}%</span>
            </div>
          </div>
        </div>
        <div class="border-t border-line px-4 py-2.5 text-xs text-ink-faint">
          斗篷准入规则可针对「移动端」进行放行，拦截「桌面端」审查机或爬虫环境。
        </div>
      </AppCard>

      <!-- 操作系统分布 -->
      <AppCard v-if="totals.visits > 0" :padding="false" class="@container flex flex-col">
        <div class="flex flex-wrap items-center justify-between gap-3 border-b border-line px-4 py-3">
          <div>
            <h2 class="text-base font-semibold tracking-tight text-ink">操作系统分布</h2>
            <p class="mt-0.5 text-xs text-ink-soft">终端操作系统内核与主版本统计。</p>
          </div>
        </div>
        <div class="flex-1 p-4">
          <div class="flex flex-col gap-2.5" :class="settling && 'is-settling'">
            <div
              v-for="(os, index) in osBreakdown"
              :key="os.name"
              class="grid grid-cols-[124px_minmax(0,1fr)_116px] items-center gap-3 @max-[470px]:grid-cols-[minmax(72px,1fr)_minmax(48px,1.3fr)_max-content] @max-[340px]:grid-cols-2 @max-[340px]:gap-y-1.5"
            >
              <span class="min-w-0 truncate text-xs text-ink @max-[340px]:col-span-1">{{ os.name }}</span>
              <div class="h-3.5 w-full overflow-hidden rounded bg-surface-strong @max-[340px]:col-span-2 @max-[340px]:row-start-2">
                <div
                  class="app-bar-fill h-full w-full rounded"
                  :class="index === 0 && os.count > 0 ? 'bg-brand' : 'bg-brand/40'"
                  :style="{ transform: `scaleX(${sweepRatioOf('系统', os.name)})` }"
                />
              </div>
              <span class="whitespace-nowrap font-mono text-right text-xs text-ink-soft @max-[340px]:col-start-2 @max-[340px]:row-start-1">{{ os.count.toLocaleString() }} 次 · {{ os.percent }}%</span>
            </div>
          </div>
        </div>
        <div class="border-t border-line px-4 py-2.5 text-xs text-ink-faint">
          支持在规则引擎中配置 OS 白名单（如仅放行 iOS 或 Android）。
        </div>
      </AppCard>

      <!-- 浏览器分布 -->
      <AppCard v-if="totals.visits > 0" :padding="false" class="@container flex flex-col">
        <div class="flex flex-wrap items-center justify-between gap-3 border-b border-line px-4 py-3">
          <div>
            <h2 class="text-base font-semibold tracking-tight text-ink">浏览器分布</h2>
            <p class="mt-0.5 text-xs text-ink-soft">独立浏览器与应用内嵌 WebView 占比。</p>
          </div>
        </div>
        <div class="flex-1 p-4">
          <div class="flex flex-col gap-2.5" :class="settling && 'is-settling'">
            <div
              v-for="(br, index) in browserBreakdown"
              :key="br.name"
              class="grid grid-cols-[124px_minmax(0,1fr)_116px] items-center gap-3 @max-[470px]:grid-cols-[minmax(72px,1fr)_minmax(48px,1.3fr)_max-content] @max-[340px]:grid-cols-2 @max-[340px]:gap-y-1.5"
            >
              <span class="min-w-0 truncate text-xs text-ink @max-[340px]:col-span-1">{{ br.name }}</span>
              <div class="h-3.5 w-full overflow-hidden rounded bg-surface-strong @max-[340px]:col-span-2 @max-[340px]:row-start-2">
                <div
                  class="app-bar-fill h-full w-full rounded"
                  :class="index === 0 && br.count > 0 ? 'bg-brand' : 'bg-brand/40'"
                  :style="{ transform: `scaleX(${sweepRatioOf('浏览器', br.name)})` }"
                />
              </div>
              <span class="whitespace-nowrap font-mono text-right text-xs text-ink-soft @max-[340px]:col-start-2 @max-[340px]:row-start-1">{{ br.count.toLocaleString() }} 次 · {{ br.percent }}%</span>
            </div>
          </div>
        </div>
        <div class="border-t border-line px-4 py-2.5 text-xs text-ink-faint">
          「应用内内置」指微信、抖音等 App 的 WebView，与独立浏览器分列以便看清各投放渠道的真实环境。
        </div>
      </AppCard>
    </section>

    <!-- 口径提示：统计窗口与分布图覆盖到什么范围（被截断时必须如实说明，不能默认当成全量） -->
    <p v-if="totals.visits > 0" class="text-xs text-ink-faint">
      <template v-if="windowText">{{ windowText }}（服务器本地时区的自然日）。</template>
      分布口径：各分布图只统计<strong class="font-medium text-ink-soft">成功</strong>的跳转与落地页视图（不含点击与失败），
      失败访问单列在「失败与风险拦截」，
      <template v-if="retentionExceeded">
        所选范围超过访问明细保留期（{{ retentionDays }} 天），更早的数据已被清理任务删除，
        图表只覆盖保留期内的部分。
      </template>
      地图分母是<strong class="font-medium text-ink-soft">能定位到国家</strong>的
      {{ locatedVisits.toLocaleString() }} 次访问，其余 {{ (totals.visits - locatedVisits).toLocaleString() }} 次
      无法定位（私网 / 回环 / 离线库未收录），不进任何国家。
      <template v-if="userAgentsTruncated">
        User-Agent 种类过多，设备 / 系统 / 浏览器三张图按访问量前 {{ USER_AGENT_LIMIT }} 种 UA 统计。
      </template>
    </p>

    <!-- 合规提示 -->
    <AppAlert type="info" show-icon title="合规提示">
      斗篷的本质是「对不同访问者返回不同内容」。用于绕过平台审核或对审核员定向展示白标内容，可能违反 TikTok / Meta / Google 的广告政策并导致账户封禁。本系统按「流量准入控制 + 品牌合规」的正向用途设计：拦截爬虫与无效流量、地域与语言适配、转化归因。落地生产前请确认业务场景合规性。
    </AppAlert>
    </template>
  </div>
</template>

<script setup lang="ts">
import { computed, inject, onMounted, ref } from 'vue';
import { routeLocationKey, routerKey } from 'vue-router';
import { CircleHelp, ExternalLink, RefreshCw } from '@lucide/vue';

import { listDomains } from '@/api/domains';
import { fetchOverviewStats } from '@/api/visits';
import AppAlert from '@/components/app/AppAlert.vue';
import AppButton from '@/components/app/AppButton.vue';
import AppCard from '@/components/app/AppCard.vue';
import AppSkeleton from '@/components/app/AppSkeleton.vue';
import AppTag from '@/components/app/AppTag.vue';
import AppTooltip from '@/components/app/AppTooltip.vue';
import { useSweep, sweepRatio, type SweepRow } from '@/composables/useSweep';
import { ApiError } from '@/types/api';
import type { Domain, OverviewRange, OverviewStats } from '@/types/api';
import { message } from '@/utils/toast';

import {
  DEFAULT_OVERVIEW_RANGE,
  formatWindowDay,
  isOverviewRange,
  OVERVIEW_RANGES,
  OVERVIEW_RANGE_DAYS,
  overviewRangeLabel,
} from './overviewRange';
import { failureReasonLabel } from './failureReasons';
import { recentCompareText } from './recentCompare';
import {
  countryDistribution,
  facetDistribution,
  sourceDistribution,
  uaDistributions,
} from './trafficBreakdown';
import WorldMapPanel from './WorldMapPanel.vue';

/**
 * UA 分桶上限，与后端 store 的 overviewFacetLimit 同值。
 * 存在的意义是：被截断时界面上要能说出"按前 N 种 UA 统计"，
 * 否则用户会把一个不覆盖全量的占比当成全量结构来读。
 */
const USER_AGENT_LIMIT = 500;

/**
 * 保留期的兜底值（天）。正常路径用响应里的 window.retentionDays ——
 * 它是服务端配置（JANUS_VISIT_RETENTION）的真实值；这里只在老响应/降级
 * 缺 window 时兜底，避免文案出现 undefined。
 */
const FALLBACK_RETENTION_DAYS = 90;

/**
 * 路由用带默认值的 inject 取，而不是 useRoute/useRouter。
 *
 * 总览页必须能在没有路由上下文时渲染（单测、嵌入式挂载都是这种场景），
 * 而那两处 ?range= 的读写是增强而不是渲染前提；inject 的 null 默认值把这个
 * 前提写在了注入点上，也省掉一条每次挂载都会出现的 "injection not found" 警告。
 */
const route = inject(routeLocationKey, null);
const router = inject(routerKey, null);

const loading = ref(false);
const domains = ref<Domain[]>([]);
/** 后端一次性算好的聚合结果；null = 尚未加载或加载失败 */
const overview = ref<OverviewStats | null>(null);
/**
 * 从 URL 读初始范围。
 *
 * 非法值（包括没有路由上下文时的空值）一律回落到默认窗口：页面照常渲染，
 * 地址栏随后由 syncRangeQuery 规范回这个值。
 */
function initialOverviewRange(): OverviewRange {
  const value = route?.query?.range;
  return isOverviewRange(value) ? value : DEFAULT_OVERVIEW_RANGE;
}

/**
 * 当前选中的时间窗口；窗口边界不在这里推算，只用服务端回显的 window.from/to 展示。
 *
 * 初值来自 URL 的 ?range=：范围是"可刷新、可分享"的状态，刷新后还在同一个窗口，
 * 把链接发给同事看到的也是同一个窗口。
 */
const range = ref<OverviewRange>(initialOverviewRange());

/** 尚未加载 / 加载失败时的占位：全 0，让页面显示"暂无数据"而不是 undefined */
const EMPTY_TOTALS: OverviewStats['totals'] = {
  links: 0,
  activeLinks: 0,
  landingLinks: 0,
  visits: 0,
  redirectVisits: 0,
  landingVisits: 0,
  clicks: 0,
};

/** 全部计数都来自这一个对象，不再由前端从明细或短链列表推导 */
const totals = computed(() => overview.value?.totals ?? EMPTY_TOTALS);
const facets = computed(() => overview.value?.facets ?? null);

const userAgents = computed(() => facets.value?.userAgents ?? []);
// 可选链要一路兜到叶子：facets 在但 coverage 字段缺失时，`facets.userAgentCoverage.truncated`
// 会在 computed 求值时直接抛错，整页白屏。两个 coverage 都在这里收窄成完整结构。
const userAgentsTruncated = computed(() => facets.value?.userAgentCoverage?.truncated ?? false);

/** 能定位到国家的访问次数（地图的分母） */
const locatedVisits = computed(() => facets.value?.countryCoverage?.returned ?? 0);

const rangeLabel = computed(() => overviewRangeLabel(range.value));
const windowInfo = computed(() => overview.value?.window ?? null);

/** 服务端回显的实际窗口，如「统计窗口：近 7 天 · 2026-09-30 ~ 2026-10-06」 */
const windowText = computed(() => {
  const w = windowInfo.value;
  if (!w) return '';
  return `统计窗口：${rangeLabel.value} · ${formatWindowDay(w.from)} ~ ${formatWindowDay(w.to)}`;
});

const retentionDays = computed(() => windowInfo.value?.retentionDays ?? FALLBACK_RETENTION_DAYS);

/**
 * 所选范围是否超出访问明细保留期。
 *
 * 「近一年」在默认 90 天保留期下必然超限：更早的明细已被清理任务删除，
 * 图表只能覆盖保留期内的部分，必须如实说明，不能把不足一年的数据当成一年。
 */
const retentionExceeded = computed(
  () => windowInfo.value !== null && OVERVIEW_RANGE_DAYS[range.value] > retentionDays.value,
);

const sourceBreakdown = computed(() => sourceDistribution(facets.value?.sources ?? []));
/** 三张 UA 分布图共用一次解析：见 trafficBreakdown.uaDistributions 的说明 */
const uaBreakdowns = computed(() => uaDistributions(userAgents.value));
const deviceBreakdown = computed(() => uaBreakdowns.value.device);
const osBreakdown = computed(() => uaBreakdowns.value.os);
const browserBreakdown = computed(() => uaBreakdowns.value.browser);

// 承载域名数：真实激活的域名数量（域名不分页，一次拿全）
const activeDomainsCount = computed(() => {
  return domains.value.filter((domain) => domain.status === 'active').length;
});

/**
 * CTR = 成功点击 / 落地页视图。
 *
 * 分母**只能是**落地页访问：点击只可能来自落地页型短链,把跳转型短链的
 * 访问算进分母会系统性压低这个指标(跳转型短链根本不产生点击)。
 *
 * 分子分母都取自 visits 表的同一段 SQL、同一段时间窗口。此前分子取的是
 * links.clicks 这个**永久计数器**,而分母取 visits 表的 count(*) ——
 * 90 天清理会持续削掉分母、分子永不衰减,运行满一个保留期后 CTR 会单调
 * 虚高到超过 100%。
 */
const ctrPercent = computed(() => {
  if (totals.value.landingVisits === 0) return 0;
  return (totals.value.clicks / totals.value.landingVisits) * 100;
});

const ctrText = computed(() => (totals.value.landingVisits === 0 ? '0%' : `${ctrPercent.value.toFixed(1)}%`));

const redirectPercent = computed(() => {
  if (totals.value.visits === 0) return 0;
  return Math.round((totals.value.redirectVisits / totals.value.visits) * 1000) / 10;
});

const landingPercent = computed(() => {
  if (totals.value.visits === 0) return 0;
  return Math.round((totals.value.landingVisits / totals.value.visits) * 1000) / 10;
});

/**
 * 失败与风险拦截：窗口内失败行按原因分桶。
 *
 * 失败行不计入 totals.visits（它们不是访问量），但它们恰恰是斗篷部署里最需要
 * 盯的一类流量：规则拦截 404/429、短链停用/删除、无可用目标、落地页缺失。
 * 一行的占比分母是**失败总数**（不是总访问），否则三行加起来永远不够 100%。
 */
const failureRows = computed(() => {
  const items = facets.value?.failureReasons ?? [];
  const total = items.reduce((sum, item) => sum + item.count, 0);
  return items.map((item) => ({
    reason: item.value,
    label: failureReasonLabel(item.value),
    count: item.count,
    percent: total === 0 ? 0 : Math.round((item.count / total) * 100),
  }));
});

const failureTotal = computed(() => failureRows.value.reduce((sum, row) => sum + row.count, 0));

/**
 * 失败率 = 失败 /（成功访问 + 失败）。
 *
 * 分母不是 totals.visits 本身：那个数只含成功访问，拿它当分母会得到一个
 * "失败占比"而不是"失败率"，失败越多这个比例的语义越离谱（可以超过 100%）。
 */
const failureRateText = computed(() => {
  const denominator = totals.value.visits + failureTotal.value;
  if (denominator === 0) return '0%';
  return `${((failureTotal.value / denominator) * 100).toFixed(1)}%`;
});

/** 访问量环比：缺 previousTotals（老响应/降级）时返回 null，界面整行不显示 */
const visitsCompare = computed(() => {
  const previous = overview.value?.previousTotals;
  if (!previous) return null;
  return recentCompareText(totals.value.visits, previous.visits);
});

/** 语言分布：分母用 coverage.returned（截断时它是"已返回部分"的总数） */
const languageBreakdown = computed(() =>
  facetDistribution(facets.value?.languages ?? [], facets.value?.languageCoverage),
);

const languageTruncated = computed(() => facets.value?.languageCoverage?.truncated ?? false);

/** 热门短链排行：后端已按全租户访问量降序取好，不再在前端对"前 100 条"重排 */
const topLinks = computed(() => overview.value?.topLinks ?? []);

/** 单条短链占总访问的比例（百分比，一位小数） */
function sharePercent(visits: number): string {
  if (totals.value.visits === 0) return '0.0';
  return ((visits / totals.value.visits) * 100).toFixed(1);
}

/**
 * 各处条形共用一套扫出状态，但键必须互不重名：维度用 `来源:`/`设备:`… 前缀，
 * 排行用 `link:` + 短链 id。共用一套只是省一个 watcher，不是为了共享"进度"——
 * 每张图都是独立的画面，被同一个进度条连起来反而会互相拖慢。
 */
const sweepRows = computed<SweepRow[]>(() => {
  const visitTotal = totals.value.visits || 1;
  return [
    ...topLinks.value.map((link) => ({
      key: `link:${link.id}`,
      ratio: sweepRatio(link.visits / visitTotal),
    })),
    ...sourceBreakdown.value.map((item) => ({
      key: `来源:${item.name}`,
      ratio: sweepRatio(item.percent / 100),
    })),
    ...deviceBreakdown.value.map((item) => ({
      key: `设备:${item.name}`,
      ratio: sweepRatio(item.percent / 100),
    })),
    ...osBreakdown.value.map((item) => ({
      key: `系统:${item.name}`,
      ratio: sweepRatio(item.percent / 100),
    })),
    ...browserBreakdown.value.map((item) => ({
      key: `浏览器:${item.name}`,
      ratio: sweepRatio(item.percent / 100),
    })),
    ...languageBreakdown.value.map((item) => ({
      key: `语言:${item.name}`,
      ratio: sweepRatio(item.percent / 100),
    })),
    ...failureRows.value.map((row) => ({
      key: `失败:${row.reason}`,
      ratio: sweepRatio(row.percent / 100),
    })),
  ];
});

const { value: sweepValue, settling } = useSweep(() => sweepRows.value);

/**
 * 排行条形当前应当显示的比例（0–1，直接喂给 scaleX）。
 *
 * 分母是总访问，与 sharePercent 同一分母，所以图形和旁边的百分比永远一致；
 * 比例下限由 sweepRatio 统一兜（不足 2% 仍留一条发丝），文字不受影响。
 * 取的是「应当显示」的值而不是目标值：入场时它从 0 走到这里，之后每次刷新
 * 都是旧值滑到新值。
 */
function shareWidth(linkId: number): number {
  return sweepValue(`link:${linkId}`);
}

/** 分布图通用：按维度键取当前应当显示的比例 */
function sweepRatioOf(dimension: '来源' | '设备' | '系统' | '浏览器' | '语言' | '失败', name: string): number {
  return sweepValue(`${dimension}:${name}`);
}

interface KPIItem {
  /** 卡片根节点的 data-od-id（自动化测试锚点，与其它页面同一约定） */
  odId: string;
  label: string;
  value: string;
  unit?: string;
  sub: string;
  tip: string;
  /** 环比文案；缺 previousTotals 时为 null，整行不渲染 */
  compare?: string | null;
  /** 结构分段条：跳转 vs 落地页占比（目前只有总访问数卡片用） */
  split?: {
    ariaLabel: string;
    text: string;
    redirectPercent: number;
    landingPercent: number;
  };
}

const kpiList = computed<KPIItem[]>(() => [
  {
    odId: 'kpi-links',
    label: '活跃短链',
    value: totals.value.activeLinks.toLocaleString(),
    unit: '条',
    sub: `已启用 · 共 ${totals.value.links.toLocaleString()} 条短链`,
    tip: '状态为「启用」的短链数量。已停用的短链不计入，不参与重定向与流量承接。',
  },
  {
    odId: 'kpi-domains',
    label: '承载域名',
    value: activeDomainsCount.value.toLocaleString(),
    unit: '个',
    sub: `已激活 · 共 ${domains.value.length.toLocaleString()} 个域名`,
    tip: 'DNS 解析已指向本服务器且状态为「已激活」的域名数量，可正常签发证书并承载短链跳转。',
  },
  {
    odId: 'kpi-visits',
    label: '总访问数',
    value: totals.value.visits.toLocaleString(),
    unit: '次',
    sub: totals.value.visits > 0 ? `${rangeLabel.value} · 所有在册短链` : `${rangeLabel.value}暂无访问记录`,
    tip: `${rangeLabel.value}内所有在册短链收到的成功跳转与落地页视图。跳转型短链重定向即记录一次，落地页型短链落地页展现即记录一次；点击与失败都不计入。统计窗口由上方时间范围决定，最长只覆盖访问明细保留期（${retentionDays.value} 天）。进度条是跳转型与落地页型的构成占比，两段合计等于这个总数。`,
    compare: visitsCompare.value,
    split: {
      ariaLabel: `跳转型 ${totals.value.redirectVisits.toLocaleString()} 次占 ${redirectPercent.value}%，落地页型 ${totals.value.landingVisits.toLocaleString()} 次占 ${landingPercent.value}%`,
      text: `跳转 ${totals.value.redirectVisits.toLocaleString()} / 落地页 ${totals.value.landingVisits.toLocaleString()}`,
      redirectPercent: redirectPercent.value,
      landingPercent: landingPercent.value,
    },
  },
  {
    odId: 'kpi-ctr',
    label: '整体转化率 (CTR)',
    value: ctrText.value,
    sub: totals.value.visits > 0
      ? `点击 ${totals.value.clicks.toLocaleString()} / 落地页访问 ${totals.value.landingVisits.toLocaleString()}`
      : '访问量为 0 时显示 0%',
    tip: `转化率（CTR）= 落地页点击数 / 落地页访问数。分母只取落地页访问 —— 跳转型短链的访问不可能产生点击，算进去会系统性压低这个指标。分子与分母同源同窗口（${rangeLabel.value}），不会跨窗口拼数。`,
  },
]);

/**
 * 二次拉取时已有数据在场：把内容压暗而不是清空。
 *
 * 清空会把「正在核对」说成「没有数据」，而空态还要走另一条分支（骨架屏/空卡），
 * 一次刷新就闪一次骨架屏是自找的抖动。压暗一档 + 按钮转圈就够了：
 * 用户知道请求在飞，同时旧数字还留着可读。
 */
const revalidating = computed(() => loading.value && overview.value !== null);

/**
 * 切换时间窗口。
 *
 * 旧窗口的数字必须立即离场：把 A 窗口的数字留在 B 窗口的标签下，比让用户
 * 看到加载态更糟 —— 数字和它所属的窗口对不上，是口径事故而不是体验问题。
 * 同窗口刷新（空态里的"刷新数据"）不走这里，仍走 revalidating 的压暗保留旧值路径。
 */
function selectRange(next: OverviewRange) {
  if (next === range.value) return;
  range.value = next;
  void syncRangeQuery(next);
  overview.value = null;
  void loadData();
}

/**
 * 把选中的范围写回地址栏（?range=）。
 *
 * 范围是页面状态而不只是组件状态：刷新后要还在同一个窗口，把链接发给同事
 * 看到的也必须是同一个窗口。用 replace 而不是 push —— 连点几个范围不该在
 * 浏览器历史里堆出一串中间态，"返回"要能直接回到上一页。
 *
 * 没有路由上下文时（单测/嵌入式挂载）静默跳过。
 */
async function syncRangeQuery(value: OverviewRange) {
  if (!router || !route) return;
  if (route.query.range === value) return;
  await router.replace({ query: { ...route.query, range: value } });
}

/**
 * 把一次失败拆成「给用户看」与「给工程师看」两路。
 *
 * 只有 ApiError 是预期内的失败（后端明确回了状态码与文案），直接弹提示。
 * 其余异常 —— 程序错误、被 catch 住的动态 import 失败、toast 本身抛错 ——
 * 一律 DEV 下记一条 console.error：这类故障在生产环境没有任何其它出口，
 * 页面只会安静地停在旧数据上，不留痕就等于不存在。
 */
function reportLoadError(scope: string, error: unknown) {
  if (error instanceof ApiError) {
    // 401 由全局拦截器统一跳登录，重复弹一次只会多一条无意义提示
    if (error.status !== 401) message.error(error.message);
    return;
  }
  if (import.meta.env.DEV) {
    console.error(`[overview] ${scope}加载失败`, error);
  }
}

async function loadData() {
  loading.value = true;
  try {
    // 两个请求互不依赖，并发拿；域名列表本身不分页。
    // 用 allSettled 而不是 all：all 下一个失败会把另一个**已经拿到的**结果一起丢掉，
    // /api/domains 挂掉时页面谎报「暂无流量访问数据」，而统计其实已经取到了。
    const [statsResult, domainsResult] = await Promise.allSettled([
      fetchOverviewStats(range.value),
      listDomains(),
    ]);

    if (statsResult.status === 'fulfilled') {
      overview.value = statsResult.value;
    } else {
      // 失败时**不清空** overview：旧数据比空态诚实
      reportLoadError('总览统计', statsResult.reason);
    }

    if (domainsResult.status === 'fulfilled') {
      domains.value = Array.isArray(domainsResult.value) ? domainsResult.value : [];
    } else {
      reportLoadError('域名列表', domainsResult.reason);
    }
  } finally {
    loading.value = false;
  }
}

onMounted(() => {
  // 地址栏与页面不允许各说各话：URL 里是非法范围时，按默认窗口加载后
  // 顺手把地址栏规范回合法值，否则用户复制出去的链接会把别人也带进降级态。
  void syncRangeQuery(range.value);
  loadData();
});
</script>
