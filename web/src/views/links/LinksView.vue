<template>
  <div class="flex flex-col gap-5 pb-10" data-od-id="links-view">
    <!-- ==================== 顶栏说明与状态 ==================== -->
    <header class="panel" data-od-id="topbar-link-management">
      <div class="panel-hd">
        <div>
          <div class="eyebrow">CLOAK / 短链管理</div>
          <h1 class="text-xl font-bold tracking-tight text-ink md:text-2xl mt-0.5">短链与目标</h1>
          <p class="topbar-sub">
            短链是访问入口与路由分发的绑定点：承载域名、短码识别、跳转分流与落地页托管。
          </p>
        </div>
        <div class="btn-row">
          <span class="badge badge-neutral" :title="`当前短链用量：${usage?.links ?? total} / ${usage?.maxLinks ?? '不限'}`">
            <span class="dot dot-live" style="color: var(--accent)"></span>
            已连接服务
          </span>
        </div>
      </div>
    </header>

    <!-- ==================== 配额条与 4 个 KPI 指标 ==================== -->
    <QuotaBar :links-used="usage?.links" :links-max="usage?.maxLinks" />

    <section class="kpi-grid" data-od-id="links-kpi">
      <!-- KPI 1: 短链配额 -->
      <div class="kpi">
        <div class="kpi-k">短链配额使用</div>
        <div class="kpi-v">
          {{ usage?.links ?? total }}
          <span class="text-[14px] text-muted font-normal">/ {{ usage?.maxLinks ?? '不限' }}</span>
        </div>
        <div class="kpi-sub">
          已用 {{ quotaPercent }}% · 剩余 {{ remainingQuota }}
        </div>
      </div>

      <!-- KPI 2: 活跃短链 -->
      <div class="kpi">
        <div class="kpi-k">活跃服务中</div>
        <div class="kpi-v">
          {{ activeLinksCount }}<span class="text-[14px] text-muted font-normal"> 条</span>
        </div>
        <div class="kpi-sub">
          <span class="dot dot-live" style="display: inline-block; color: var(--accent)"></span>
          已启用短链正常对外重定向
        </div>
      </div>

      <!-- KPI 3: 累计访问 -->
      <div class="kpi">
        <div class="kpi-k">本页累计访问量</div>
        <div class="kpi-v">
          {{ totalVisits.toLocaleString() }}<span class="text-[14px] text-muted font-normal"> 次</span>
        </div>
        <div class="kpi-sub">包含跳转型与落地页访问统计</div>
      </div>

      <!-- KPI 4: 落地页转化与 CTR -->
      <div class="kpi">
        <div class="kpi-k">落地页点击转化</div>
        <div class="kpi-v">
          {{ totalClicks.toLocaleString() }}<span class="text-[14px] text-muted font-normal"> 次</span>
        </div>
        <div class="kpi-sub">落地页综合转化率 {{ overallCtr }}</div>
      </div>
    </section>

    <!-- ==================== 短链列表主面板 ==================== -->
    <section class="panel" data-od-id="link-list">
      <div class="panel-hd">
        <div>
          <h2>短链列表</h2>
          <p>管理租户名下的短链，支持类型过滤、状态切换与出口多目标轮询配置。</p>
        </div>
        <div class="btn-row">
          <button
            type="button"
            class="btn btn-sm"
            :disabled="loading"
            title="刷新短链列表"
            @click="loadData"
          >
            <RefreshCw :size="13" :class="loading ? 'animate-spin' : ''" />
            刷新
          </button>
          <button
            type="button"
            class="btn btn-sm"
            @click="openBatchModal"
          >
            <Upload :size="13" />
            批量导入
          </button>
          <button
            type="button"
            class="btn btn-sm"
            :disabled="links.length === 0"
            @click="exportCsv"
          >
            <FileDown :size="13" />
            导出 CSV
          </button>
          <button
            type="button"
            class="btn btn-sm btn-primary"
            id="newLink"
            @click="openCreateDrawer"
          >
            <Plus :size="14" />
            新建短链
          </button>
        </div>
      </div>

      <!-- 搜索与筛选工具栏 -->
      <div class="panel-bd" style="padding-bottom: 0">
        <div class="toolbar">
          <div class="relative grow min-w-[200px]">
            <input
              v-model="keyword"
              class="input w-full pl-8"
              id="linkSearch"
              placeholder="搜索短码、域名或目标 URL…"
              aria-label="搜索短链"
            />
            <Search
              :size="14"
              class="absolute left-2.5 top-1/2 -translate-y-1/2 text-muted pointer-events-none"
            />
          </div>
          <select
            v-model="typeFilter"
            class="select"
            id="typeFilter"
            aria-label="按类型过滤"
          >
            <option value="all">全部类型</option>
            <option value="redirect">跳转型 (redirect)</option>
            <option value="landing">落地页型 (landing)</option>
          </select>
          <select
            v-model="statusFilter"
            class="select"
            id="linkStatus"
            aria-label="按状态过滤"
          >
            <option value="all">全部状态</option>
            <option value="enabled">已启用 (enabled)</option>
            <option value="disabled">已停用 (disabled)</option>
          </select>
          <button
            v-if="hasActiveFilter"
            type="button"
            class="btn btn-sm btn-ghost text-muted hover:text-fg"
            @click="resetFilters"
          >
            <X :size="13" />
            清空过滤
          </button>
        </div>
      </div>

      <!-- 表格数据区 -->
      <div class="tbl-wrap">
        <table class="tbl" id="linkTable">
          <thead>
            <tr>
              <th class="shrink">短码</th>
              <th class="shrink">承载域名</th>
              <th class="shrink">类型</th>
              <th>出口目标 URL</th>
              <th class="num">24h 访问</th>
              <th class="num">转化点击</th>
              <th class="num">CTR</th>
              <th class="shrink">状态</th>
              <th class="shrink">启用</th>
              <th class="shrink">操作</th>
            </tr>
          </thead>
          <tbody>
            <!-- 加载态 -->
            <tr v-if="loading && links.length === 0">
              <td colspan="10" class="empty">
                <div class="flex items-center justify-center gap-2 text-muted py-6">
                  <RefreshCw class="animate-spin" :size="16" />
                  正在加载短链数据...
                </div>
              </td>
            </tr>

            <!-- 空状态 -->
            <tr v-else-if="filteredLinks.length === 0">
              <td colspan="10" class="py-8">
                <AppEmpty
                  :description="links.length === 0 ? '暂无短链记录，请点击下方按钮创建第一条短链' : '未找到符合当前筛选条件的短链记录'"
                />
                <div class="flex justify-center mt-3">
                  <button
                    v-if="links.length === 0"
                    type="button"
                    class="btn btn-sm btn-primary"
                    @click="openCreateDrawer"
                  >
                    <Plus :size="14" />
                    新建短链
                  </button>
                  <button
                    v-else
                    type="button"
                    class="btn btn-sm"
                    @click="resetFilters"
                  >
                    重置过滤条件
                  </button>
                </div>
              </td>
            </tr>

            <!-- 列表行 -->
            <tr
              v-for="link in filteredLinks"
              :key="link.id"
              :data-status="link.status === 'enabled' ? 'on' : 'off'"
            >
              <!-- 短码 -->
              <td class="shrink">
                <div class="row items-center" style="gap: 6px; flex-wrap: nowrap">
                  <button
                    type="button"
                    class="linkish mono font-semibold text-left"
                    :title="'点击编辑 ' + link.code"
                    @click="openEditDrawer(link)"
                  >
                    {{ link.code }}
                  </button>
                  <button
                    type="button"
                    class="icon-btn text-muted hover:text-fg"
                    style="width: 22px; height: 22px; border: none; background: transparent; padding: 0"
                    title="复制完整短链 URL"
                    @click.stop="copyLinkUrl(link)"
                  >
                    <Copy :size="12" />
                  </button>
                  <a
                    :href="'https://' + getPrimaryDomain(link) + '/' + link.code"
                    target="_blank"
                    rel="noopener noreferrer"
                    class="icon-btn text-muted hover:text-fg inline-flex items-center justify-center"
                    style="width: 22px; height: 22px; border: none; background: transparent; padding: 0"
                    title="在新标签页测试访问短链"
                    @click.stop
                  >
                    <ExternalLink :size="12" />
                  </a>
                </div>
              </td>

              <!-- 承载域名 -->
              <td class="shrink mono tiny muted">
                <div class="flex items-center gap-1.5 flex-wrap">
                  <span>{{ getPrimaryDomain(link) }}</span>
                  <span
                    v-if="link.domains && link.domains.length > 1"
                    class="badge badge-neutral micro"
                    :title="link.domains.join(', ')"
                  >
                    +{{ link.domains.length - 1 }}
                  </span>
                </div>
              </td>

              <!-- 类型 -->
              <td class="shrink">
                <span v-if="link.linkType === 'landing'" class="badge badge-neutral">
                  落地页型
                </span>
                <span v-else class="badge badge-neutral">
                  跳转型 · {{ link.redirectStatus || '302' }}
                </span>
              </td>

              <!-- 目标 URL -->
              <td>
                <div class="flex items-center gap-1.5 min-w-0 max-w-md">
                  <span class="mono tiny truncate" :title="link.targetUrls?.[0] || '—'">
                    {{ formatTargetDisplay(link.targetUrls) }}
                  </span>
                  <span
                    v-if="link.targetUrls && link.targetUrls.length > 1"
                    class="badge badge-neutral micro shrink-0"
                    :title="`多目标轮询 (${link.targetUrls.length} 个出口):\n` + link.targetUrls.join('\n')"
                  >
                    +{{ link.targetUrls.length - 1 }} 轮询
                  </span>
                </div>
              </td>

              <!-- 24h 访问 -->
              <td class="num">
                {{ (link.visits || 0).toLocaleString() }}
              </td>

              <!-- 转化点击 -->
              <td class="num">
                {{ link.linkType === 'landing' ? (link.clicks || 0).toLocaleString() : '—' }}
              </td>

              <!-- CTR -->
              <td class="num">
                {{ getLinkCtr(link) }}
              </td>

              <!-- 状态 -->
              <td class="shrink">
                <span :class="link.status === 'enabled' ? 'badge badge-ok' : 'badge badge-neutral'">
                  {{ link.status === 'enabled' ? '已启用' : '已停用' }}
                </span>
              </td>

              <!-- 启用 Switch -->
              <td class="shrink">
                <label class="switch">
                  <input
                    type="checkbox"
                    :checked="link.status === 'enabled'"
                    :disabled="statusUpdatingId === link.id"
                    :aria-label="'启用短链 ' + link.code"
                    @change="onToggleLinkStatus(link)"
                  />
                  <i></i>
                </label>
              </td>

              <!-- 操作 -->
              <td class="shrink">
                <div class="row" style="gap: 4px; flex-wrap: nowrap">
                  <button
                    type="button"
                    class="btn btn-sm btn-ghost"
                    title="编辑短链"
                    @click="openEditDrawer(link)"
                  >
                    <Pencil :size="12" />
                    编辑
                  </button>
                  <button
                    type="button"
                    class="btn btn-sm btn-ghost text-red-500 hover:text-red-600 hover:bg-red-50 dark:hover:bg-red-950/30"
                    title="逻辑删除（保留记录与历史数据）"
                    @click="handleDeleteLink(link)"
                  >
                    <Trash2 :size="12" />
                    删除
                  </button>
                  <button
                    type="button"
                    class="btn btn-sm btn-ghost text-xs text-red-600 hover:text-red-700 opacity-60 hover:opacity-100"
                    title="彻底清除（物理删除全部数据）"
                    @click="handlePurgeLink(link)"
                  >
                    彻底删除
                  </button>
                </div>
              </td>
            </tr>
          </tbody>
        </table>
      </div>

      <!-- 底栏与真实分页 -->
      <div class="panel-ft row-between flex-wrap gap-3">
        <div class="row tiny muted" style="gap: 12px">
          <span>共 <strong class="text-ink font-mono">{{ total }}</strong> 条短链</span>
          <span>配额使用 <strong class="text-ink font-mono">{{ usage?.links ?? total }}</strong> / {{ usage?.maxLinks ?? '不限' }}</span>
        </div>
        <div class="row" style="gap: 10px">
          <div class="row tiny muted" style="gap: 6px">
            <span>每页</span>
            <select
              v-model.number="pageSize"
              class="select"
              style="min-height: 28px; padding: 2px 20px 2px 8px; font-size: 12px"
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

    <!-- ==================== 侧边抽屉: 新建 / 编辑短链 ==================== -->
    <Transition name="drawer-backdrop">
      <div
        v-if="drawerVisible"
        class="fixed inset-0 z-40 bg-black/45 backdrop-blur-xs"
        @click="closeDrawer"
      />
    </Transition>

    <Transition name="drawer-slide">
      <div
        v-if="drawerVisible"
        class="fixed inset-y-0 right-0 z-50 flex w-full max-w-2xl flex-col border-l border-line bg-surface shadow-2xl"
        role="dialog"
        aria-modal="true"
      >
        <!-- 抽屉头部 -->
        <div class="panel-hd shrink-0">
          <div>
            <div class="eyebrow">{{ isEdit ? '短链配置 / 编辑' : '短链配置 / 新建' }}</div>
            <h2 class="text-base font-bold tracking-tight text-ink mt-0.5">
              {{ isEdit ? `编辑短链 · ${form.code}` : '新建短链' }}
            </h2>
            <p class="text-xs text-muted">
              {{ isEdit ? '更新出口目标、落地页设置及出站处理参数' : '配置短码、承载域名、分发目标与行为参数' }}
            </p>
          </div>
          <div class="row" style="gap: 8px">
            <span class="badge badge-neutral mono">
              {{ isEdit ? `ID · ${editingLinkId}` : 'NEW' }}
            </span>
            <button
              type="button"
              class="icon-btn text-muted hover:text-fg"
              title="关闭抽屉"
              @click="closeDrawer"
            >
              <X :size="16" />
            </button>
          </div>
        </div>

        <!-- 抽屉主体表单 (滚动区) -->
        <div class="flex-1 overflow-y-auto p-5 space-y-5">
          <!-- 模块 1: 基本设置 -->
          <div class="panel">
            <div class="panel-hd">
              <div>
                <h3 class="text-sm font-semibold">基本信息</h3>
                <p>短链的公开标识短码与行为类型</p>
              </div>
            </div>
            <div class="panel-bd stack">
              <div class="form-grid">
                <div class="field">
                  <label for="drawerCode">短码</label>
                  <input
                    v-model="form.code"
                    class="input mono"
                    id="drawerCode"
                    placeholder="留空自动生成短码"
                    :disabled="isEdit"
                  />
                  <span class="hint">
                    同一域名下短码不可重复{{ isEdit ? '（创建后短码不可更改）' : '' }}
                  </span>
                </div>

                <div class="field">
                  <label for="drawerDomain">承载域名</label>
                  <select
                    v-model="primaryDomainId"
                    class="select"
                    id="drawerDomain"
                    @change="onPrimaryDomainSelectChange"
                  >
                    <option v-if="domains.length === 0" :value="0" disabled>
                      正在加载域名...
                    </option>
                    <option
                      v-for="d in domains"
                      :key="d.id"
                      :value="d.id"
                      :disabled="d.status !== 'active'"
                    >
                      {{ d.fqdn }} ({{ d.origin === 'self' ? '自有' : '默认' }}){{ d.status !== 'active' ? ' - ' + (DOMAIN_STATUS_NOTE[d.status] || '未激活') : '' }}
                    </option>
                  </select>
                  <span class="hint">仅已激活 (active) 域名可承载短链访问</span>
                </div>

                <!-- 多域名复选绑定 -->
                <div v-if="domains.length > 1" class="field span-2">
                  <label>关联承载域名（可多选）</label>
                  <div class="grid grid-cols-1 sm:grid-cols-2 gap-2 p-2.5 rounded border border-line bg-surface-muted/50 max-h-32 overflow-y-auto">
                    <label
                      v-for="d in domains"
                      :key="d.id"
                      class="flex items-center gap-2 text-xs cursor-pointer select-none"
                      :class="d.status !== 'active' ? 'opacity-50' : ''"
                    >
                      <input
                        type="checkbox"
                        :value="d.id"
                        v-model="form.domainIds"
                        :disabled="d.status !== 'active'"
                      />
                      <span class="mono truncate">{{ d.fqdn }}</span>
                      <span class="badge badge-neutral micro shrink-0">
                        {{ d.origin === 'self' ? '自有' : '默认' }}
                      </span>
                    </label>
                  </div>
                </div>

                <div class="field span-2">
                  <label>短链类型</label>
                  <div class="segmented" style="align-self: start">
                    <button
                      type="button"
                      :aria-pressed="form.linkType === 'redirect'"
                      @click="form.linkType = 'redirect'"
                    >
                      跳转型 · 访问即重定向
                    </button>
                    <button
                      type="button"
                      :aria-pressed="form.linkType === 'landing'"
                      @click="form.linkType = 'landing'"
                    >
                      落地页型 · 先到落地页
                    </button>
                  </div>
                </div>

                <div v-if="form.linkType === 'redirect'" class="field span-2">
                  <label for="drawerRedirectStatus">重定向状态码</label>
                  <select v-model="form.redirectStatus" class="select" id="drawerRedirectStatus">
                    <option value="302">302 · 临时重定向（推荐，不缓存目标地址）</option>
                    <option value="301">301 · 永久重定向（浏览器与搜索引擎长期缓存）</option>
                  </select>
                </div>

                <div v-if="isEdit" class="field span-2">
                  <label>短链状态</label>
                  <div class="segmented" style="align-self: start">
                    <button
                      type="button"
                      :aria-pressed="form.status === 'enabled'"
                      @click="form.status = 'enabled'"
                    >
                      正常启用
                    </button>
                    <button
                      type="button"
                      :aria-pressed="form.status === 'disabled'"
                      @click="form.status = 'disabled'"
                    >
                      暂停停用 (返回 404)
                    </button>
                  </div>
                </div>
              </div>
            </div>
          </div>

          <!-- 模块 2: 目标 URL 列表 (多目标动态管理) -->
          <div class="panel">
            <div class="panel-hd">
              <div>
                <h3 class="text-sm font-semibold">目标 URL (出口)</h3>
                <p>短链的最终目的地。配置多个目标时，系统将按顺序轮询（Round-Robin）选择出口。</p>
              </div>
              <button type="button" class="btn btn-sm" @click="addTargetUrl">
                <Plus :size="13" /> 添加目标
              </button>
            </div>
            <div class="panel-bd stack-sm">
              <div
                v-for="(target, idx) in form.targetUrls"
                :key="idx"
                class="flex items-center gap-2"
              >
                <span class="mono micro muted w-6 text-right shrink-0">#{{ idx + 1 }}</span>
                <input
                  v-model="form.targetUrls[idx]"
                  class="input mono grow"
                  placeholder="https://example.com/dest"
                />
                <button
                  type="button"
                  class="icon-btn text-muted hover:text-fg"
                  :disabled="idx === 0"
                  title="上移（提高轮询次序）"
                  @click="moveTargetUp(idx)"
                >
                  <ArrowUp :size="14" />
                </button>
                <button
                  type="button"
                  class="icon-btn text-muted hover:text-fg"
                  :disabled="idx === form.targetUrls.length - 1"
                  title="下移（延后轮询次序）"
                  @click="moveTargetDown(idx)"
                >
                  <ArrowDown :size="14" />
                </button>
                <button
                  type="button"
                  class="icon-btn text-red-500 hover:text-red-600"
                  :disabled="form.targetUrls.length <= 1"
                  title="移除该目标"
                  @click="removeTargetUrl(idx)"
                >
                  <Trash2 :size="14" />
                </button>
              </div>
              <p class="hint">开放重定向支持任意合法协议 URL（如 https://、http:// 或自定义 scheme）</p>
            </div>
          </div>

          <!-- 模块 3: 落地页设置 (仅落地页型显示) -->
          <div v-show="form.linkType === 'landing'" class="panel">
            <div class="panel-hd">
              <div>
                <h3 class="text-sm font-semibold">落地页设置</h3>
                <p>访问者访问短链时先到达落地页，落地页内按钮经平台 JS SDK 回传点击并跳转至目标 URL。</p>
              </div>
            </div>
            <div class="panel-bd stack">
              <div class="form-grid">
                <div class="field span-2">
                  <label>落地页来源</label>
                  <div class="segmented" style="align-self: start">
                    <button
                      type="button"
                      :aria-pressed="form.landingSource === 'url'"
                      @click="form.landingSource = 'url'"
                    >
                      填写落地页 URL
                    </button>
                    <button
                      type="button"
                      :aria-pressed="form.landingSource === 'upload'"
                      @click="form.landingSource = 'upload'"
                    >
                      上传静态 ZIP 包
                    </button>
                  </div>
                </div>

                <!-- 来源: URL -->
                <div v-if="form.landingSource === 'url'" class="field span-2">
                  <label for="drawerLandingUrl">落地页地址 (URL)</label>
                  <input
                    v-model="form.landingUrl"
                    class="input mono"
                    id="drawerLandingUrl"
                    placeholder="https://yourbrand.com/landing-page"
                  />
                  <span class="hint">访问短链时将先重定向至此地址；落地页按钮经 JS SDK 触发转化回传</span>
                </div>

                <!-- 来源: Upload -->
                <div v-else class="field span-2">
                  <label>静态落地页压缩包</label>
                  <div class="row" style="gap: 8px">
                    <input
                      v-model="landingFileSummary"
                      class="input mono grow text-xs"
                      readonly
                    />
                    <input
                      ref="landingFileInput"
                      type="file"
                      accept=".zip"
                      class="hidden"
                      @change="handleLandingFileUpload"
                    />
                    <button
                      type="button"
                      class="btn btn-sm"
                      :disabled="uploadingZip"
                      @click="triggerLandingFileUpload"
                    >
                      <Upload :size="13" />
                      {{ uploadingZip ? '上传中...' : '选择 ZIP' }}
                    </button>
                  </div>
                  <span class="hint">平台直接托管在「短码/」路径下，压缩包根目录下必须包含 index.html</span>
                </div>
              </div>
            </div>
          </div>

          <!-- 模块 4: 出站参数处理 -->
          <div class="panel">
            <div class="panel-hd">
              <div>
                <h3 class="text-sm font-semibold">出站处理配置</h3>
                <p>控制短链重定向时携带与剥离的参数特征</p>
              </div>
            </div>
            <div class="panel-bd stack-sm">
              <div class="grid grid-cols-1 sm:grid-cols-2 gap-3">
                <label class="flex items-center gap-2.5 text-xs cursor-pointer select-none">
                  <span class="switch">
                    <input type="checkbox" v-model="outbound.stripReferer" />
                    <i></i>
                  </span>
                  <span>剥离 Referer（不向目标站泄露来源）</span>
                </label>
                <label class="flex items-center gap-2.5 text-xs cursor-pointer select-none">
                  <span class="switch">
                    <input type="checkbox" v-model="outbound.passParams" />
                    <i></i>
                  </span>
                  <span>透传查询参数（继承 UTM 与广告 ID）</span>
                </label>
                <label class="flex items-center gap-2.5 text-xs cursor-pointer select-none">
                  <span class="switch">
                    <input type="checkbox" v-model="outbound.hideTarget" />
                    <i></i>
                  </span>
                  <span>隐藏真实目标（Location 外不留痕迹）</span>
                </label>
                <label class="flex items-center gap-2.5 text-xs cursor-pointer select-none">
                  <span class="switch">
                    <input type="checkbox" v-model="outbound.randomDelay" />
                    <i></i>
                  </span>
                  <span>加随机微延迟 0–300ms（弱化时序指纹）</span>
                </label>
              </div>
            </div>
          </div>

          <!-- 模块 5: 短链地址预览 -->
          <div class="panel">
            <div class="panel-hd">
              <div>
                <h3 class="text-sm font-semibold">短链地址预览</h3>
                <p>生成并核对对外分发的短链 URL</p>
              </div>
              <button
                type="button"
                class="btn btn-sm"
                @click="copyPreviewUrl"
              >
                <Copy :size="13" /> 复制短链
              </button>
            </div>
            <div class="panel-bd">
              <div class="p-3 bg-surface-muted rounded border border-line mono text-xs break-all text-ink font-semibold flex items-center justify-between gap-2">
                <span>{{ previewUrl }}</span>
                <span class="badge badge-neutral micro shrink-0">
                  {{ form.linkType === 'landing' ? '落地页型' : '跳转型' }}
                </span>
              </div>
            </div>
          </div>
        </div>

        <!-- 抽屉底栏 -->
        <div class="panel-ft row-between shrink-0 bg-surface">
          <span class="tiny text-muted">
            目标出口数：{{ validTargetsCount }} 个 · 绑定域名：{{ form.domainIds.length }} 个
          </span>
          <div class="row" style="gap: 8px">
            <button
              type="button"
              class="btn btn-sm"
              :disabled="saving"
              @click="closeDrawer"
            >
              取消
            </button>
            <button
              type="button"
              class="btn btn-sm btn-primary"
              :disabled="saving"
              @click="handleSaveLink"
            >
              {{ saving ? '保存中...' : (isEdit ? '保存修改' : '确认创建') }}
            </button>
          </div>
        </div>
      </div>
    </Transition>

    <!-- ==================== 模态框: 批量导入短链 ==================== -->
    <div
      v-if="showBatchModal"
      class="fixed inset-0 z-50 flex items-center justify-center bg-black/45 backdrop-blur-xs p-4"
    >
      <div class="panel w-full max-w-lg shadow-2xl">
        <div class="panel-hd">
          <div>
            <h2 class="text-base font-bold">批量导入短链</h2>
            <p>每行一条短链，支持「短码 目标URL」或仅「目标URL」自动生成短码。</p>
          </div>
          <button
            type="button"
            class="icon-btn text-muted hover:text-fg"
            @click="showBatchModal = false"
          >
            <X :size="15" />
          </button>
        </div>
        <div class="panel-bd stack">
          <div class="field">
            <label for="batchDomain">指定承载域名</label>
            <select v-model="batchDomainId" class="select" id="batchDomain">
              <option
                v-for="d in domains"
                :key="d.id"
                :value="d.id"
                :disabled="d.status !== 'active'"
              >
                {{ d.fqdn }} ({{ d.origin === 'self' ? '自有' : '默认' }}){{ d.status !== 'active' ? ' - ' + (DOMAIN_STATUS_NOTE[d.status] || '未激活') : '' }}
              </option>
            </select>
          </div>
          <div class="field">
            <label for="batchInput">短链行列表</label>
            <textarea
              v-model="batchText"
              class="textarea mono"
              id="batchInput"
              rows="6"
              placeholder="deal-a https://example.com/target-a&#10;deal-b https://example.com/target-b&#10;https://example.com/target-c"
            ></textarea>
            <span class="hint">每行一条，短码与 URL 用空格分隔；若只有 URL 则由后端自动生成短码</span>
          </div>
        </div>
        <div class="panel-ft row-between">
          <span class="tiny text-muted">有效行数：{{ parsedBatchLinesCount }}</span>
          <div class="row" style="gap: 8px">
            <button
              type="button"
              class="btn btn-sm"
              :disabled="batchImporting"
              @click="showBatchModal = false"
            >
              取消
            </button>
            <button
              type="button"
              class="btn btn-sm btn-primary"
              :disabled="batchImporting || parsedBatchLinesCount === 0"
              @click="executeBatchImport"
            >
              {{ batchImporting ? '导入中...' : '开始导入' }}
            </button>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, onUnmounted, reactive, ref } from 'vue';
import { useRoute } from 'vue-router';
import dayjs from 'dayjs';
import {
  ArrowDown,
  ArrowUp,
  Copy,
  ExternalLink,
  FileDown,
  Pencil,
  Plus,
  RefreshCw,
  Search,
  Trash2,
  Upload,
  X,
} from '@lucide/vue';

import { listDomains } from '@/api/domains';
import {
  createLink,
  deleteLink,
  getLink,
  listLinks,
  purgeLink,
  updateLink,
  uploadLanding,
} from '@/api/links';
import QuotaBar from '@/components/QuotaBar.vue';
import AppEmpty from '@/components/ui/AppEmpty.vue';
import { confirm } from '@/components/ui/confirm';
import { useAuthStore } from '@/stores/auth';
import { ApiError } from '@/types/api';
import type {
  Domain,
  LandingSource,
  Link,
  LinkStatus,
  LinkType,
  RedirectStatus,
} from '@/types/api';
import { formatDateTime } from '@/utils/format';
import { message } from '@/utils/toast';

const route = useRoute();
const auth = useAuthStore();

const DOMAIN_STATUS_NOTE: Record<string, string> = {
  pending: 'DNS 待验证',
  active: '已激活',
  failed: '校验失败',
  stopped: '已停用',
};

// ==================== 响应式基础数据 ====================
const links = ref<Link[]>([]);
const domains = ref<Domain[]>([]);
const loading = ref(false);
const total = ref(0);
const page = ref(1);
const pageSize = ref(20);

const keyword = ref('');
const typeFilter = ref<string>('all');
const statusFilter = ref<string>('all');
const statusUpdatingId = ref<number | null>(null);

// ==================== 计算用量与 KPI ====================
const usage = computed(() => auth.config?.usage);

const quotaPercent = computed(() => {
  const max = usage.value?.maxLinks;
  if (!max || max <= 0) return 0;
  const used = usage.value?.links ?? total.value;
  return Math.min(100, Math.round((used / max) * 100));
});

const remainingQuota = computed(() => {
  const max = usage.value?.maxLinks;
  if (!max) return '不限';
  const rem = max - (usage.value?.links ?? total.value);
  return rem > 0 ? `${rem} 条` : '已耗尽';
});

const activeLinksCount = computed(() => {
  return links.value.filter((l) => l.status === 'enabled').length;
});

const totalVisits = computed(() => {
  return links.value.reduce((acc, l) => acc + (l.visits || 0), 0);
});

const totalClicks = computed(() => {
  return links.value.reduce((acc, l) => acc + (l.linkType === 'landing' ? l.clicks || 0 : 0), 0);
});

const overallCtr = computed(() => {
  const landingVisits = links.value
    .filter((l) => l.linkType === 'landing')
    .reduce((acc, l) => acc + (l.visits || 0), 0);
  if (landingVisits === 0 || totalClicks.value === 0) return '—';
  return `${((totalClicks.value / landingVisits) * 100).toFixed(2)}%`;
});

const totalPages = computed(() => Math.max(1, Math.ceil(total.value / pageSize.value)));

const activeDomains = computed(() => {
  return domains.value.filter((d) => d.status === 'active');
});

const hasActiveFilter = computed(() => {
  return keyword.value.trim() !== '' || typeFilter.value !== 'all' || statusFilter.value !== 'all';
});

const filteredLinks = computed(() => {
  const q = keyword.value.trim().toLowerCase();
  const tf = typeFilter.value;
  const sf = statusFilter.value;

  return links.value.filter((l) => {
    const codeMatch = l.code.toLowerCase().includes(q);
    const domainMatch = l.domains?.some((d) => d.toLowerCase().includes(q)) ?? false;
    const targetMatch = l.targetUrls?.some((u) => u.toLowerCase().includes(q)) ?? false;
    const okKeyword = !q || codeMatch || domainMatch || targetMatch;

    let okType = true;
    if (tf === 'redirect') okType = l.linkType === 'redirect';
    else if (tf === 'landing') okType = l.linkType === 'landing';

    let okStatus = true;
    if (sf === 'enabled') okStatus = l.status === 'enabled';
    else if (sf === 'disabled') okStatus = l.status === 'disabled';

    return okKeyword && okType && okStatus;
  });
});

// ==================== 抽屉表单状态 ====================
const drawerVisible = ref(false);
const editingLinkId = ref<number | null>(null);
const isEdit = computed(() => editingLinkId.value !== null && editingLinkId.value > 0);
const saving = ref(false);
const uploadingZip = ref(false);
const landingFileInput = ref<HTMLInputElement | null>(null);
const pendingLandingFile = ref<File | null>(null);
const landingFileSummary = ref('未上传压缩包');

const form = reactive({
  code: '',
  domainIds: [] as number[],
  linkType: 'redirect' as LinkType,
  redirectStatus: '302' as RedirectStatus,
  status: 'enabled' as LinkStatus,
  targetUrls: ['https://'],
  landingSource: 'url' as LandingSource,
  landingUrl: '',
});

const outbound = reactive({
  stripReferer: true,
  passParams: false,
  hideTarget: true,
  randomDelay: false,
});

const primaryDomainId = computed({
  get: () => {
    if (form.domainIds.length > 0) return form.domainIds[0];
    return 0;
  },
  set: (val: number) => {
    if (!val) return;
    if (!form.domainIds.includes(val)) {
      form.domainIds = [val, ...form.domainIds.filter((id) => id !== val)];
    }
  },
});

function onPrimaryDomainSelectChange(e: Event) {
  const target = e.target as HTMLSelectElement;
  const id = Number(target.value);
  if (id && !form.domainIds.includes(id)) {
    form.domainIds.unshift(id);
  }
}

const previewDomain = computed(() => {
  const chosen = domains.value.find((d) => form.domainIds.includes(d.id));
  if (chosen) return chosen.fqdn;
  if (domains.value.length > 0) return domains.value[0].fqdn;
  return 'your-domain.com';
});

const previewUrl = computed(() => {
  const code = form.code.trim() || '自动生成';
  return `https://${previewDomain.value}/${code}`;
});

const validTargetsCount = computed(() => {
  return form.targetUrls
    .map((u) => u.trim())
    .filter((u) => u && u !== 'https://' && u !== 'http://').length;
});

// ==================== 模态框: 批量导入 ====================
const showBatchModal = ref(false);
const batchText = ref('');
const batchDomainId = ref<number | undefined>(undefined);
const batchImporting = ref(false);

const parsedBatchLinesCount = computed(() => {
  return batchText.value
    .split('\n')
    .map((s) => s.trim())
    .filter(Boolean).length;
});

// ==================== 数据加载 ====================
async function loadData() {
  loading.value = true;
  try {
    const [linksRes, domainsRes] = await Promise.all([
      listLinks({ page: page.value, pageSize: pageSize.value }),
      listDomains(),
    ]);

    links.value = linksRes.items;
    total.value = linksRes.total;
    domains.value = domainsRes;

    if (domainsRes.length > 0 && !batchDomainId.value) {
      const active = domainsRes.find((d) => d.status === 'active');
      batchDomainId.value = active ? active.id : domainsRes[0].id;
    }
  } catch (error) {
    if (error instanceof ApiError) {
      message.error(error.message);
    } else {
      message.error('加载短链数据失败');
    }
    links.value = [];
    total.value = 0;
  } finally {
    loading.value = false;
  }
}

// ==================== 列表与表格操作 ====================
function getPrimaryDomain(link: Link): string {
  if (link.domains && link.domains.length > 0) {
    return link.domains[0];
  }
  return domains.value[0]?.fqdn || '—';
}

function formatTargetDisplay(urls: string[] | undefined): string {
  if (!urls || urls.length === 0) return '—';
  try {
    const u = new URL(urls[0]);
    return `${u.hostname}${u.pathname !== '/' ? u.pathname : ''}`;
  } catch {
    return urls[0];
  }
}

function getLinkCtr(link: Link): string {
  if (link.linkType !== 'landing') return '—';
  if (!link.visits || link.visits === 0) return '—';
  if (!link.clicks || link.clicks === 0) return '0.00%';
  return `${((link.clicks / link.visits) * 100).toFixed(2)}%`;
}

async function copyLinkUrl(link: Link) {
  const fqdn = getPrimaryDomain(link);
  const url = `https://${fqdn}/${link.code}`;
  try {
    await navigator.clipboard.writeText(url);
    message.success('已复制短链: ' + url);
  } catch {
    message.error('复制失败，请手动复制');
  }
}

async function copyPreviewUrl() {
  try {
    await navigator.clipboard.writeText(previewUrl.value);
    message.success('已复制预览地址: ' + previewUrl.value);
  } catch {
    message.error('复制失败，请手动复制');
  }
}

async function onToggleLinkStatus(link: Link) {
  if (statusUpdatingId.value === link.id) return;
  const nextStatus: LinkStatus = link.status === 'enabled' ? 'disabled' : 'enabled';
  const label = nextStatus === 'disabled' ? '停用' : '启用';

  statusUpdatingId.value = link.id;
  try {
    const updated = await updateLink(link.id, { status: nextStatus });
    link.status = updated.status;
    message.success(`短链「${link.code}」已${label}`);
  } catch (error) {
    if (error instanceof ApiError) {
      message.error(error.message);
    } else {
      message.error(`${label}失败，请稍后重试`);
    }
  } finally {
    statusUpdatingId.value = null;
  }
}

function handleDeleteLink(link: Link) {
  confirm({
    title: `逻辑删除短链「${link.code}」?`,
    content: '删除为逻辑删除：短链记录与历史访问明细保留，但「域名/短码」将不再对外重定向。',
    okText: '确认逻辑删除',
    cancelText: '取消',
    danger: true,
    onOk: async () => {
      try {
        await deleteLink(link.id);
        message.success(`短链「${link.code}」已逻辑删除`);
        await loadData();
        await auth.fetchMe();
      } catch (err) {
        if (err instanceof ApiError) message.error(err.message);
        else message.error('删除短链失败');
      }
    },
  });
}

function handlePurgeLink(link: Link) {
  confirm({
    title: `彻底清除短链「${link.code}」?`,
    content: '警告：彻底清除为物理删除！将永久移除该短链及全部历史访问明细，此操作不可撤销！',
    okText: '彻底物理删除',
    cancelText: '取消',
    danger: true,
    onOk: async () => {
      try {
        await purgeLink(link.id);
        message.success(`短链「${link.code}」已彻底清除`);
        await loadData();
        await auth.fetchMe();
      } catch (err) {
        if (err instanceof ApiError) message.error(err.message);
        else message.error('彻底删除短链失败');
      }
    },
  });
}

function exportCsv() {
  const headers = [
    '短码',
    '承载域名',
    '类型',
    '重定向状态码',
    '目标URL',
    '24h访问',
    '转化点击',
    'CTR',
    '状态',
    '创建时间',
  ];
  const rows = filteredLinks.value.map((l) => [
    l.code,
    (l.domains || []).join('; '),
    l.linkType === 'landing' ? '落地页型' : '跳转型',
    l.redirectStatus || '302',
    (l.targetUrls || []).join('; '),
    l.visits || 0,
    l.linkType === 'landing' ? l.clicks || 0 : 0,
    getLinkCtr(l),
    l.status === 'enabled' ? '启用' : '停用',
    formatDateTime(l.createdAt),
  ]);

  const csvContent = [
    headers.join(','),
    ...rows.map((r) => r.map((c) => `"${String(c).replace(/"/g, '""')}"`).join(',')),
  ].join('\n');

  const blob = new Blob(['\uFEFF' + csvContent], { type: 'text/csv;charset=utf-8;' });
  const url = URL.createObjectURL(blob);
  const a = document.createElement('a');
  a.href = url;
  a.download = `links-export-${dayjs().format('YYYYMMDD-HHmmss')}.csv`;
  a.click();
  URL.revokeObjectURL(a.href);
  message.success(`已导出 ${rows.length} 条短链记录`);
}

function resetFilters() {
  keyword.value = '';
  typeFilter.value = 'all';
  statusFilter.value = 'all';
}

function goToPage(p: number) {
  if (p < 1 || p > totalPages.value || p === page.value) return;
  page.value = p;
  loadData();
}

function onPageSizeChange() {
  page.value = 1;
  loadData();
}

// ==================== 抽屉操作 ====================
function openCreateDrawer() {
  editingLinkId.value = null;
  form.code = '';
  form.linkType = 'redirect';
  form.redirectStatus = '302';
  form.status = 'enabled';
  form.targetUrls = ['https://'];
  form.landingSource = 'url';
  form.landingUrl = '';
  pendingLandingFile.value = null;
  landingFileSummary.value = '未上传压缩包';

  const defaultDomain = activeDomains.value[0] || domains.value[0];
  if (defaultDomain) {
    form.domainIds = [defaultDomain.id];
  } else {
    form.domainIds = [];
  }

  drawerVisible.value = true;
}

function openEditDrawer(link: Link) {
  editingLinkId.value = link.id;
  form.code = link.code;
  form.linkType = link.linkType || 'redirect';
  form.redirectStatus = link.redirectStatus || '302';
  form.status = link.status || 'enabled';
  form.landingSource = link.landingSource || 'url';
  form.landingUrl = link.landingUrl || '';
  pendingLandingFile.value = null;

  if (link.landingUploaded) {
    landingFileSummary.value = `${link.code}-landing.zip · 已托管 · 状态正常`;
  } else {
    landingFileSummary.value = '未上传压缩包';
  }

  const matched = domains.value.filter((d) => link.domains?.includes(d.fqdn)).map((d) => d.id);
  if (matched.length > 0) {
    form.domainIds = matched;
  } else if (domains.value.length > 0) {
    form.domainIds = [domains.value[0].id];
  } else {
    form.domainIds = [];
  }

  if (link.targetUrls && link.targetUrls.length > 0) {
    form.targetUrls = [...link.targetUrls];
  } else {
    form.targetUrls = ['https://'];
  }

  drawerVisible.value = true;
}

function closeDrawer() {
  drawerVisible.value = false;
}

function addTargetUrl() {
  form.targetUrls.push('https://');
}

function removeTargetUrl(idx: number) {
  if (form.targetUrls.length <= 1) {
    message.warning('请至少保留一个出口目标');
    return;
  }
  form.targetUrls.splice(idx, 1);
}

function moveTargetUp(idx: number) {
  if (idx <= 0) return;
  const temp = form.targetUrls[idx - 1];
  form.targetUrls[idx - 1] = form.targetUrls[idx];
  form.targetUrls[idx] = temp;
}

function moveTargetDown(idx: number) {
  if (idx >= form.targetUrls.length - 1) return;
  const temp = form.targetUrls[idx + 1];
  form.targetUrls[idx + 1] = form.targetUrls[idx];
  form.targetUrls[idx] = temp;
}

function triggerLandingFileUpload() {
  landingFileInput.value?.click();
}

async function handleLandingFileUpload(e: Event) {
  const input = e.target as HTMLInputElement;
  const file = input.files?.[0];
  if (!file) return;

  if (!file.name.endsWith('.zip')) {
    message.error('落地页文件必须为 .zip 格式压缩包');
    input.value = '';
    return;
  }

  if (editingLinkId.value) {
    uploadingZip.value = true;
    try {
      await uploadLanding(editingLinkId.value, file);
      landingFileSummary.value = `${file.name} · ${(file.size / 1024).toFixed(0)} KB · 刚刚上传`;
      message.success('落地页压缩包已上传并托管');
      await loadData();
    } catch (err) {
      if (err instanceof ApiError) message.error(err.message);
      else message.error('上传压缩包失败');
    } finally {
      uploadingZip.value = false;
      input.value = '';
    }
  } else {
    pendingLandingFile.value = file;
    landingFileSummary.value = `${file.name} · ${(file.size / 1024).toFixed(0)} KB · 待创建后自动上传`;
    message.info('压缩包已选定，将在保存短链后自动上传托管');
    input.value = '';
  }
}

async function handleSaveLink() {
  const cleanedTargets = form.targetUrls
    .map((u) => u.trim())
    .filter((u) => u && u !== 'https://' && u !== 'http://')
    .map((u) => {
      if (!u.startsWith('http://') && !u.startsWith('https://')) {
        return 'https://' + u;
      }
      return u;
    });

  if (cleanedTargets.length === 0) {
    message.error('请至少配置一个有效的目标 URL');
    return;
  }

  for (const url of cleanedTargets) {
    try {
      new URL(url);
    } catch {
      message.error(`目标 URL 格式不合法：${url}`);
      return;
    }
  }

  if (form.domainIds.length === 0) {
    message.error('请至少选择一个承载域名');
    return;
  }

  if (form.linkType === 'landing' && form.landingSource === 'url' && !form.landingUrl.trim()) {
    message.error('落地页型（URL 来源）必须填写落地页地址');
    return;
  }

  saving.value = true;
  try {
    if (editingLinkId.value) {
      await updateLink(editingLinkId.value, {
        targetUrls: cleanedTargets,
        domainIds: form.domainIds,
        redirectStatus: form.redirectStatus,
        status: form.status,
        linkType: form.linkType,
        landingSource: form.landingSource,
        landingUrl: form.landingUrl.trim() || undefined,
      });
      message.success(`短链「${form.code}」修改成功`);
    } else {
      const created = await createLink({
        code: form.code.trim() || undefined,
        targetUrls: cleanedTargets,
        domainIds: form.domainIds,
        redirectStatus: form.redirectStatus,
        linkType: form.linkType,
        landingSource: form.landingSource,
        landingUrl: form.landingUrl.trim() || undefined,
      });

      if (form.linkType === 'landing' && form.landingSource === 'upload' && pendingLandingFile.value) {
        try {
          await uploadLanding(created.id, pendingLandingFile.value);
        } catch {
          message.warning('短链已创建，但落地页压缩包上传失败，可稍后在编辑中重新上传');
        }
      }

      message.success(`短链「${created.code}」创建成功`);
    }

    closeDrawer();
    await loadData();
    await auth.fetchMe();
  } catch (error) {
    if (error instanceof ApiError) {
      message.error(error.message);
    } else {
      message.error('保存短链失败，请检查输入或稍后重试');
    }
  } finally {
    saving.value = false;
  }
}

// ==================== 模态框操作: 批量导入 ====================
function openBatchModal() {
  batchText.value = '';
  const firstActive = activeDomains.value[0] || domains.value[0];
  if (firstActive) {
    batchDomainId.value = firstActive.id;
  }
  showBatchModal.value = true;
}

async function executeBatchImport() {
  const lines = batchText.value
    .split('\n')
    .map((l) => l.trim())
    .filter(Boolean);

  if (lines.length === 0) return;
  if (!batchDomainId.value) {
    message.error('请选择承载域名');
    return;
  }

  batchImporting.value = true;
  let successCount = 0;
  let failCount = 0;
  const errors: string[] = [];

  for (const line of lines) {
    const parts = line.split(/[\s,]+/).filter(Boolean);
    let code: string | undefined = undefined;
    let url = '';
    if (parts.length >= 2) {
      code = parts[0];
      url = parts[1];
    } else if (parts.length === 1) {
      url = parts[0];
    }

    if (!url.startsWith('http://') && !url.startsWith('https://')) {
      url = 'https://' + url;
    }

    try {
      new URL(url);
    } catch {
      failCount++;
      errors.push(`URL 非法: ${url}`);
      continue;
    }

    try {
      await createLink({
        code: code || undefined,
        targetUrls: [url],
        domainIds: [batchDomainId.value],
        redirectStatus: '302',
        linkType: 'redirect',
      });
      successCount++;
    } catch (err) {
      failCount++;
      if (err instanceof ApiError) {
        errors.push(`${code || url}: ${err.message}`);
      }
    }
  }

  batchImporting.value = false;
  showBatchModal.value = false;
  batchText.value = '';

  if (successCount > 0) {
    message.success(`成功导入 ${successCount} 条短链${failCount > 0 ? `，失败 ${failCount} 条` : ''}`);
    await loadData();
    await auth.fetchMe();
  } else {
    message.error(`导入失败：全部 ${failCount} 条均未成功 (${errors[0] || '未知错误'})`);
  }
}

// ==================== 键盘事件与路由监听 ====================
function onKeydown(e: KeyboardEvent) {
  if (e.key === 'Escape') {
    if (drawerVisible.value) {
      closeDrawer();
    } else if (showBatchModal.value) {
      showBatchModal.value = false;
    }
  }
}

onMounted(async () => {
  window.addEventListener('keydown', onKeydown);
  await loadData();

  if (route.query.new === '1') {
    openCreateDrawer();
  } else if (route.query.edit) {
    const editIdOrCode = String(route.query.edit);
    const found = links.value.find(
      (l) => String(l.id) === editIdOrCode || l.code === editIdOrCode,
    );
    if (found) {
      openEditDrawer(found);
    } else {
      const numId = Number(editIdOrCode);
      if (!isNaN(numId) && numId > 0) {
        try {
          const detail = await getLink(numId);
          openEditDrawer(detail);
        } catch {}
      }
    }
  }
});

onUnmounted(() => {
  window.removeEventListener('keydown', onKeydown);
});
</script>

<style scoped>
/* 侧边抽屉遮罩过渡 */
.drawer-backdrop-enter-active,
.drawer-backdrop-leave-active {
  transition: opacity 0.22s ease;
}
.drawer-backdrop-enter-from,
.drawer-backdrop-leave-to {
  opacity: 0;
}

/* 侧边抽屉滑入过渡 */
.drawer-slide-enter-active {
  transition: transform 0.26s cubic-bezier(0.16, 1, 0.3, 1), opacity 0.2s ease;
}
.drawer-slide-leave-active {
  transition: transform 0.2s cubic-bezier(0.4, 0, 1, 1), opacity 0.15s ease;
}
.drawer-slide-enter-from,
.drawer-slide-leave-to {
  transform: translateX(100%);
  opacity: 0.8;
}
</style>
