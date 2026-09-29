<template>
  <div class="flex flex-col gap-5">
    <!-- 顶栏说明与模式状态 -->
    <header class="panel" data-od-id="topbar-link-management">
      <div class="panel-hd">
        <div>
          <div class="eyebrow">CLOAK / 短链与目标</div>
          <h1 class="text-xl font-bold tracking-tight text-ink md:text-2xl mt-0.5">短链与目标</h1>
          <p class="topbar-sub">
            短链是投放入口与规则的绑定点：绑哪几条规则、放行到哪个目标池、如何剥离来源、如何回传转化。
          </p>
        </div>
        <div class="btn-row">
          <span class="badge badge-neutral" :title="isLiveBackend ? '生产后端' : '本页面支持真实后端与原型演示数据'">
            <span class="dot" :class="isLiveBackend ? 'dot-live text-accent' : 'bg-muted'"></span>
            {{ isLiveBackend ? '已连接生产服务' : '原型演示数据' }}
          </span>
        </div>
      </div>
    </header>

    <!-- 顶层选项卡 -->
    <section class="panel" data-od-id="lm-tabs">
      <div class="tabs" role="tablist" id="lmTabs">
        <button
          role="tab"
          :aria-selected="currentTab === 'links'"
          @click="switchTab('links')"
        >
          短链列表
        </button>
        <button
          role="tab"
          :aria-selected="currentTab === 'edit'"
          @click="switchTab('edit')"
        >
          短链编辑
        </button>
        <button
          role="tab"
          :aria-selected="currentTab === 'domains'"
          @click="switchTab('domains')"
          id="domains"
        >
          域名池
        </button>
      </div>
    </section>

    <!-- ==================== Tab 1: 短链列表 ==================== -->
    <section v-show="currentTab === 'links'" data-tabpanel="links" data-od-id="link-list">
      <div class="panel">
        <div class="panel-hd">
          <div>
            <h2>短链列表</h2>
            <p>每条短链绑定一组按优先级排序的规则；未命中任何规则时执行「无规则兜底」。</p>
          </div>
          <div class="btn-row">
            <button class="btn btn-sm" @click="showBatchModal = true">
              <Upload :size="13" />
              批量导入
            </button>
            <button class="btn btn-sm" @click="exportCsv">
              <FileDown :size="13" />
              导出 CSV
            </button>
            <button class="btn btn-sm btn-primary" id="newLink" @click="openCreate">
              <Plus :size="14" />
              新建短链
            </button>
          </div>
        </div>

        <div class="panel-bd" style="padding-bottom: 0">
          <div class="toolbar">
            <input
              v-model="keyword"
              class="input grow"
              id="linkSearch"
              placeholder="搜索短码或域名…"
              aria-label="搜索短链"
            />
            <select
              v-model="typeFilter"
              class="select"
              id="typeFilter"
              aria-label="按类型过滤"
            >
              <option value="all">全部类型</option>
              <option value="redirect">跳转型</option>
              <option value="landing">落地页型</option>
            </select>
            <select
              v-model="statusFilter"
              class="select"
              id="linkStatus"
              aria-label="按状态过滤"
            >
              <option value="all">全部状态</option>
              <option value="enabled">已启用</option>
              <option value="disabled">已停用</option>
            </select>
          </div>
        </div>

        <div class="tbl-wrap">
          <table class="tbl" id="linkTable">
            <thead>
              <tr>
                <th>短码</th>
                <th>域名</th>
                <th>类型</th>
                <th>绑定规则</th>
                <th>默认去向</th>
                <th class="num">目标数</th>
                <th class="num">24h 访问</th>
                <th class="num">转化</th>
                <th class="num">CTR</th>
                <th>状态</th>
                <th>启用</th>
                <th class="shrink">操作</th>
              </tr>
            </thead>
            <tbody>
              <tr
                v-for="link in filteredLinks"
                :key="link.id"
                :data-status="link.status === 'enabled' ? 'on' : 'off'"
              >
                <!-- 短码 (带复制与编辑) -->
                <td class="shrink">
                  <div class="row" style="gap: 6px; flex-wrap: nowrap">
                    <a
                      class="linkish mono font-semibold"
                      href="#"
                      :title="'点击编辑 ' + link.code"
                      @click.prevent="openEditTab(link)"
                    >
                      {{ link.code }}
                    </a>
                    <button
                      type="button"
                      class="icon-btn text-muted hover:text-fg"
                      style="width: 22px; height: 22px; border: none; background: transparent; padding: 0"
                      :title="'复制完整短链: https://' + getLinkDomain(link) + '/' + link.code"
                      @click.stop="copyLinkUrl(link)"
                    >
                      <Copy :size="12" />
                    </button>
                  </div>
                </td>

                <!-- 域名 -->
                <td class="shrink mono tiny muted">
                  {{ getLinkDomain(link) }}
                </td>

                <!-- 类型 -->
                <td class="shrink">
                  <span class="badge badge-neutral">
                    {{ link.linkType === 'landing' ? '落地页型' : '跳转型' }}
                  </span>
                </td>

                <!-- 绑定规则 -->
                <td class="shrink mono tiny">
                  {{ getLinkRuleStr(link) }}
                </td>

                <!-- 默认去向 -->
                <td class="shrink tiny">
                  {{ getTargetPoolName(link) }}
                </td>

                <!-- 目标数 -->
                <td class="num">
                  {{ link.targetUrls?.length || 1 }}
                </td>

                <!-- 24h 访问 -->
                <td class="num">
                  {{ (link.visits || 0).toLocaleString() }}
                </td>

                <!-- 转化 -->
                <td class="num">
                  {{ (link.clicks || 0).toLocaleString() }}
                </td>

                <!-- CTR -->
                <td class="num">
                  {{ getLinkCtr(link) }}
                </td>

                <!-- 状态 -->
                <td class="shrink">
                  <span
                    :class="
                      link.status === 'enabled'
                        ? 'badge badge-ok'
                        : link.code === 'black-friday'
                          ? 'badge badge-warn'
                          : 'badge badge-neutral'
                    "
                  >
                    {{
                      link.status === 'enabled'
                        ? '启用'
                        : link.code === 'black-friday'
                          ? '已排期'
                          : '已停用'
                    }}
                  </span>
                </td>

                <!-- 启用 Switch -->
                <td class="shrink">
                  <label class="switch">
                    <input
                      type="checkbox"
                      :checked="link.status === 'enabled'"
                      :aria-label="'启用短链 ' + link.code"
                      @change="onToggleLinkStatus(link)"
                    />
                    <i></i>
                  </label>
                </td>

                <!-- 操作按钮 -->
                <td class="shrink">
                  <div class="row" style="gap: 4px; flex-wrap: nowrap">
                    <button
                      type="button"
                      class="btn btn-sm btn-ghost"
                      @click="openEditTab(link)"
                    >
                      <Pencil :size="12" />
                      编辑
                    </button>
                    <button
                      type="button"
                      class="btn btn-sm btn-ghost text-red-500 hover:text-red-600 hover:bg-red-50 dark:hover:bg-red-950/30"
                      @click="handleDeleteLink(link)"
                      title="逻辑删除"
                    >
                      <Trash2 :size="12" />
                    </button>
                  </div>
                </td>
              </tr>

              <tr v-if="filteredLinks.length === 0">
                <td colspan="12" class="empty">
                  未找到符合条件的短链记录
                </td>
              </tr>
            </tbody>
          </table>
        </div>

        <div class="panel-ft row-between">
          <span>共 {{ filteredLinks.length }} 条 · 配额使用 {{ usage?.links ?? links.length }} / {{ usage?.maxLinks ?? '5,000' }}</span>
          <span class="mono">规则未命中时 → 无规则兜底 = 目标池 A</span>
        </div>
      </div>
    </section>

    <!-- ==================== Tab 2: 短链编辑 ==================== -->
    <section v-show="currentTab === 'edit'" data-tabpanel="edit" data-od-id="link-editor">
      <div class="two-col">
        <!-- 左栏：表单主配置区 -->
        <div class="stack">
          <!-- 模块 1: 基本设置 -->
          <div class="panel">
            <div class="panel-hd">
              <div>
                <h2>短链编辑</h2>
                <p>
                  当前编辑：<span class="mono font-semibold text-fg">{{ form.code || '新建短链' }}</span> ·
                  {{ form.domainFqdn || currentDomainFqdn }}
                </p>
              </div>
              <span class="badge badge-neutral mono">ID · {{ editingLinkId || 'NEW' }}</span>
            </div>
            <div class="panel-bd stack">
              <div class="form-grid">
                <div class="field">
                  <label for="lCode">短码</label>
                  <input
                    v-model="form.code"
                    class="input mono"
                    id="lCode"
                    placeholder="留空自动生成短码"
                    :disabled="isExistingSavedLink"
                  />
                  <span class="hint">同一域名下不可重复{{ isExistingSavedLink ? '（保存后短码不可更改）' : '' }}</span>
                </div>
                <div class="field">
                  <label for="lDomain">承载域名</label>
                  <select
                    v-model="form.domainId"
                    class="select"
                    id="lDomain"
                    @change="onDomainChange"
                  >
                    <option
                      v-for="d in domains"
                      :key="d.id"
                      :value="d.id"
                    >
                      {{ d.fqdn }} ({{ d.origin === 'self' ? '自有' : '默认' }})
                    </option>
                  </select>
                </div>
                <div class="field span-2">
                  <label>短链类型</label>
                  <div class="segmented" id="typeSeg" style="align-self: start">
                    <button
                      type="button"
                      :aria-pressed="form.linkType === 'redirect'"
                      @click="form.linkType = 'redirect'"
                    >
                      跳转型 · 访问即跳转
                    </button>
                    <button
                      type="button"
                      :aria-pressed="form.linkType === 'landing'"
                      @click="form.linkType = 'landing'"
                    >
                      落地页型 · 先到中间页
                    </button>
                  </div>
                </div>
                <div class="field">
                  <label for="lStatus">重定向类型</label>
                  <select v-model="form.redirectStatus" class="select" id="lStatus">
                    <option value="302">302 · 临时（推荐，投放用）</option>
                    <option value="301">301 · 永久（会缓存）</option>
                  </select>
                </div>
                <div class="field">
                  <label for="lCap">排期</label>
                  <input v-model="form.schedule" class="input mono" id="lCap" />
                  <span class="hint">到期后自动停止访问并返回 410</span>
                </div>
              </div>
            </div>
          </div>

          <!-- 模块 2: 落地页设置 (仅落地页型生效) -->
          <div v-show="form.linkType === 'landing'" class="panel" id="landingBox">
            <div class="panel-hd">
              <div>
                <h2>落地页设置</h2>
                <p>仅落地页型生效。</p>
              </div>
            </div>
            <div class="panel-bd stack">
              <div class="form-grid">
                <div class="field span-2">
                  <label>落地页来源</label>
                  <div class="segmented" style="align-self: start">
                    <button
                      type="button"
                      :aria-pressed="form.landingSource === 'upload'"
                      @click="form.landingSource = 'upload'"
                    >
                      上传压缩包（含 index.html）
                    </button>
                    <button
                      type="button"
                      :aria-pressed="form.landingSource === 'url'"
                      @click="form.landingSource = 'url'"
                    >
                      填写落地页 URL
                    </button>
                  </div>
                </div>

                <div v-if="form.landingSource === 'url'" class="field span-2">
                  <label for="lLanding">落地页地址</label>
                  <input
                    v-model="form.landingUrl"
                    class="input mono"
                    id="lLanding"
                    placeholder="https://go.northwind-media.com/vip-access/"
                  />
                  <span class="hint">按钮点击经平台 JS SDK 回传，计入本短链点击与转化</span>
                </div>

                <div v-else class="field span-2">
                  <label for="lLpFile">已上传文件</label>
                  <div class="row" style="gap: 8px">
                    <input
                      v-model="landingFileSummary"
                      class="input mono grow"
                      id="lLpFile"
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
                  <span class="hint">平台托管在「短码/」路径下，上传前请确保根目录包含 index.html</span>
                </div>
              </div>
            </div>
          </div>

          <!-- 模块 3: 规则绑定 -->
          <div class="panel">
            <div class="panel-hd">
              <div>
                <h2>规则绑定</h2>
                <p>被绑定的规则只会在这条短链上生效。顺序即优先级，与规则引擎内的优先级独立。</p>
              </div>
              <router-link class="btn btn-sm" to="/rules">到规则引擎新建 →</router-link>
            </div>
            <div class="panel-bd stack">
              <!-- 有序规则列表 -->
              <div id="boundList" class="stack-sm">
                <div
                  v-for="(b, idx) in boundRules"
                  :key="b.id"
                  class="row-between"
                  style="padding: 9px 11px; border: 1px solid var(--border); border-radius: var(--r); background: var(--surface)"
                >
                  <div class="row" style="gap: 9px; flex-wrap: nowrap; min-width: 0">
                    <span class="mono micro muted">#{{ idx + 1 }}</span>
                    <span class="mono micro font-semibold">{{ b.id }}</span>
                    <span style="font-size: 13px; font-weight: 600; overflow: hidden; text-overflow: ellipsis; white-space: nowrap">
                      {{ b.name }}
                    </span>
                  </div>
                  <div class="row" style="gap: 8px; flex: none">
                    <span class="badge badge-ok">{{ b.action }}</span>
                    <button
                      type="button"
                      class="btn btn-sm btn-ghost"
                      :disabled="idx === 0"
                      @click="moveRuleUp(idx)"
                      aria-label="上移"
                      title="上移"
                    >
                      <ArrowUp :size="13" />
                    </button>
                    <button
                      type="button"
                      class="btn btn-sm btn-ghost"
                      :disabled="idx === boundRules.length - 1"
                      @click="moveRuleDown(idx)"
                      aria-label="下移"
                      title="下移"
                    >
                      <ArrowDown :size="13" />
                    </button>
                    <button
                      type="button"
                      class="btn btn-sm btn-ghost text-red-500 hover:text-red-600"
                      @click="removeBoundRule(idx)"
                      aria-label="解绑"
                    >
                      移除
                    </button>
                  </div>
                </div>

                <div v-if="boundRules.length === 0" class="empty">
                  尚未绑定任何规则——所有访问将直接走「无规则兜底」。
                </div>
              </div>

              <!-- 添加规则到本短链 -->
              <div class="field" style="max-width: 480px">
                <label for="addRule">添加规则到本短链</label>
                <div class="row" style="gap: 8px; flex-wrap: nowrap">
                  <select
                    v-model="selectedRuleToAdd"
                    class="select"
                    id="addRule"
                    style="flex: 1"
                  >
                    <option value="">选择规则…</option>
                    <option
                      v-for="cat in CATALOG_RULES"
                      :key="cat.id"
                      :value="cat.id"
                    >
                      {{ cat.id }} {{ cat.name }}
                    </option>
                  </select>
                  <button
                    type="button"
                    class="btn"
                    id="bindBtn"
                    @click="bindSelectedRule"
                  >
                    绑定
                  </button>
                </div>
              </div>
            </div>
          </div>

          <!-- 模块 4: 目标池 -->
          <div class="panel">
            <div class="panel-hd">
              <div>
                <h2>目标池</h2>
                <p>放行后按权重选择出口。同一目标池可被多条规则复用。</p>
              </div>
              <button
                type="button"
                class="btn btn-sm"
                id="addTarget"
                @click="addTarget"
              >
                + 添加出口
              </button>
            </div>
            <div class="tbl-wrap">
              <table class="tbl" id="targetTbl">
                <thead>
                  <tr>
                    <th>目标 URL</th>
                    <th>归属池</th>
                    <th class="num">权重</th>
                    <th>状态</th>
                    <th class="num">24h 出口</th>
                    <th class="shrink">操作</th>
                  </tr>
                </thead>
                <tbody>
                  <tr v-for="(t, idx) in targets" :key="idx">
                    <td>
                      <input
                        v-if="t.editing"
                        v-model="t.url"
                        class="input mono"
                        style="min-height: 30px"
                        placeholder="https://example.com/dest"
                      />
                      <span v-else class="mono tiny break-all">{{ t.url }}</span>
                    </td>
                    <td>
                      <select
                        v-if="t.editing"
                        v-model="t.pool"
                        class="select"
                        style="min-height: 30px; width: 105px; padding: 2px 22px 2px 8px; font-size: 12px"
                      >
                        <option value="目标池 A">目标池 A</option>
                        <option value="目标池 B">目标池 B</option>
                      </select>
                      <span v-else class="badge badge-neutral">{{ t.pool }}</span>
                    </td>
                    <td class="num">
                      <input
                        v-if="t.editing"
                        v-model.number="t.weight"
                        type="number"
                        min="1"
                        max="100"
                        class="input num"
                        style="min-height: 30px; width: 62px; padding: 2px 6px"
                      />
                      <span v-else>{{ t.weight }}</span>
                    </td>
                    <td>
                      <span
                        :class="
                          t.health === '健康'
                            ? 'badge badge-ok'
                            : t.health.includes('慢')
                              ? 'badge badge-warn'
                              : 'badge badge-neutral'
                        "
                      >
                        {{ t.health }}
                      </span>
                    </td>
                    <td class="num">
                      {{ (t.visits24h || 0).toLocaleString() }}
                    </td>
                    <td class="shrink">
                      <div class="row" style="gap: 4px; flex-wrap: nowrap">
                        <button
                          type="button"
                          class="btn btn-sm"
                          @click="t.editing = !t.editing"
                        >
                          {{ t.editing ? '完成' : '编辑' }}
                        </button>
                        <button
                          v-if="targets.length > 1"
                          type="button"
                          class="btn btn-sm btn-ghost text-red-500 hover:text-red-600"
                          @click="removeTarget(idx)"
                        >
                          删除
                        </button>
                      </div>
                    </td>
                  </tr>
                </tbody>
              </table>
            </div>
            <div class="panel-ft">
              无规则兜底：<b>目标池 A</b> · 当所有绑定规则都未命中时，访问者直接进入该池。
            </div>
          </div>

          <!-- 模块 5: 出站与回传 -->
          <div class="panel">
            <div class="panel-hd">
              <div>
                <h2>出站与回传</h2>
                <p>控制跳转时携带什么、剥掉什么，以及转化事件送回哪个广告平台。</p>
              </div>
            </div>
            <div class="panel-bd">
              <div class="form-grid">
                <div class="field span-2">
                  <label>出站处理</label>
                  <div class="row" style="gap: 16px">
                    <label class="row tiny select-none cursor-pointer" style="gap: 6px">
                      <span class="switch">
                        <input type="checkbox" v-model="outbound.stripReferer" />
                        <i></i>
                      </span>
                      剥离 Referer（不把广告平台来源带给目标站）
                    </label>
                    <label class="row tiny select-none cursor-pointer" style="gap: 6px">
                      <span class="switch">
                        <input type="checkbox" v-model="outbound.hideTarget" />
                        <i></i>
                      </span>
                      隐藏真实目标（Location 之外不留痕）
                    </label>
                    <label class="row tiny select-none cursor-pointer" style="gap: 6px">
                      <span class="switch">
                        <input type="checkbox" v-model="outbound.passUtm" />
                        <i></i>
                      </span>
                      透传 UTM / ttclid / fbclid 参数
                    </label>
                    <label class="row tiny select-none cursor-pointer" style="gap: 6px">
                      <span class="switch">
                        <input type="checkbox" v-model="outbound.randomDelay" />
                        <i></i>
                      </span>
                      加随机延迟 0–800ms（弱化时序特征）
                    </label>
                    <label class="row tiny select-none cursor-pointer" style="gap: 6px">
                      <span class="switch">
                        <input type="checkbox" v-model="outbound.guestIdWithoutCookie" />
                        <i></i>
                      </span>
                      无 Cookie 时下发一次性访客 ID
                    </label>
                  </div>
                </div>

                <div class="field span-2">
                  <label>转化事件回传</label>
                  <div class="row" style="gap: 16px">
                    <label class="row tiny select-none cursor-pointer" style="gap: 6px">
                      <span class="switch">
                        <input type="checkbox" v-model="postback.tiktokEvents" />
                        <i></i>
                      </span>
                      TikTok Events API · Pixel ID <span class="mono">C1A9…7Q</span>
                    </label>
                    <label class="row tiny select-none cursor-pointer" style="gap: 6px">
                      <span class="switch">
                        <input type="checkbox" v-model="postback.metaCapi" />
                        <i></i>
                      </span>
                      Meta Conversions API · Pixel <span class="mono">8842…1</span>
                    </label>
                    <label class="row tiny select-none cursor-pointer" style="gap: 6px">
                      <span class="switch">
                        <input type="checkbox" v-model="postback.googleEnhanced" />
                        <i></i>
                      </span>
                      Google Ads Enhanced Conversions
                    </label>
                    <label class="row tiny select-none cursor-pointer" style="gap: 6px">
                      <span class="switch">
                        <input type="checkbox" v-model="postback.googleOffline" />
                        <i></i>
                      </span>
                      Google Ads Offline Conversion
                    </label>
                  </div>
                </div>

                <div class="field">
                  <label for="lEvents">回传事件</label>
                  <input
                    v-model="postback.events"
                    class="input mono"
                    id="lEvents"
                    placeholder="ViewContent, Click, AddToCart, Purchase"
                  />
                </div>
                <div class="field">
                  <label for="lApi">回调地址（转化发生时）</label>
                  <input
                    v-model="postback.webhookUrl"
                    class="input mono"
                    id="lApi"
                    placeholder="https://api.northwind-media.com/conv"
                  />
                </div>
              </div>
            </div>
          </div>
        </div>

        <!-- 右栏：访问路径预览与健康检查（Sticky 悬浮） -->
        <div class="stack" style="position: sticky; top: 84px">
          <!-- 卡片 1: 访问路径预览 -->
          <div class="panel">
            <div class="panel-hd">
              <div>
                <h2 style="font-size: 15px">访问路径预览</h2>
                <p>按当前配置，一个访客会发生什么。</p>
              </div>
            </div>
            <div class="panel-bd stack-sm">
              <div class="trace-step" style="border: 0; padding: 0; grid-template-columns: minmax(0, 1fr)">
                <div class="mono tiny" style="color: var(--accent)">
                  {{ traceStep1 }}
                </div>
              </div>
              <div class="trace-step" style="border: 0; padding: 0; grid-template-columns: minmax(0, 1fr)">
                <div class="tiny muted">
                  {{ traceStep2 }}
                </div>
              </div>
              <div class="trace-step" style="border: 0; padding: 0; grid-template-columns: minmax(0, 1fr)">
                <div class="tiny muted">
                  {{ traceStep3 }}
                </div>
              </div>
              <div class="trace-step" style="border: 0; padding: 0; grid-template-columns: minmax(0, 1fr)">
                <div class="tiny muted">
                  {{ traceStep4 }}
                </div>
              </div>
              <div class="trace-step" style="border: 0; padding: 0; grid-template-columns: minmax(0, 1fr)">
                <div class="tiny muted">
                  {{ traceStep5 }}
                </div>
              </div>
              <div class="trace-step" style="border: 0; padding: 0; grid-template-columns: minmax(0, 1fr)">
                <div class="tiny muted">
                  {{ traceStep6 }}
                </div>
              </div>
            </div>
          </div>

          <!-- 卡片 2: 健康检查 -->
          <div class="panel">
            <div class="panel-hd">
              <div>
                <h2 style="font-size: 15px">健康检查</h2>
              </div>
            </div>
            <div class="panel-bd stack-sm">
              <div class="row-between tiny">
                <span class="muted">目标可达性</span>
                <span class="badge badge-ok">{{ healthyTargetsCount }} / {{ targets.length }} 正常</span>
              </div>
              <div class="row-between tiny">
                <span class="muted">证书剩余</span>
                <span class="mono">{{ certRemainingDays }}</span>
              </div>
              <div class="row-between tiny">
                <span class="muted">最近拦截率</span>
                <span class="mono">5.8%</span>
              </div>
              <div class="row-between tiny">
                <span class="muted">可疑目标告警</span>
                <span :class="targetAlertCount > 0 ? 'badge badge-warn' : 'badge badge-ok'">
                  {{ targetAlertCount > 0 ? `${targetAlertCount} 条待处理` : '0 条告警' }}
                </span>
              </div>
            </div>
            <div class="panel-ft row" style="gap: 8px">
              <button
                type="button"
                class="btn btn-sm btn-primary"
                id="saveLink"
                :disabled="saving"
                style="flex: 1"
                @click="handleSaveLink"
              >
                {{ isSavedRecently ? '已保存 ✓' : (saving ? '保存中...' : '保存') }}
              </button>
              <button
                type="button"
                class="btn btn-sm"
                style="flex: 1"
                @click="copyCurrentLink"
              >
                复制短链
              </button>
            </div>
          </div>
        </div>
      </div>
    </section>

    <!-- ==================== Tab 3: 域名池 ==================== -->
    <section v-show="currentTab === 'domains'" data-tabpanel="domains" data-od-id="domain-pool">
      <div class="panel">
        <div class="panel-hd">
          <div>
            <h2>域名池</h2>
            <p>短链可绑定多个域名，同一短码在不同域名下可指向不同目标。域名被封或失效时可快速切换。</p>
          </div>
          <button
            type="button"
            class="btn btn-sm btn-primary"
            @click="showAddDomainModal = true"
          >
            + 添加自有域名
          </button>
        </div>

        <div class="tbl-wrap">
          <table class="tbl">
            <thead>
              <tr>
                <th>域名</th>
                <th>来源</th>
                <th>激活</th>
                <th>证书</th>
                <th class="num">短链 / 有效</th>
                <th class="num">24h 访问</th>
                <th class="num">可用率</th>
                <th class="shrink">操作</th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="d in domains" :key="d.id">
                <!-- 域名 -->
                <td class="mono font-semibold" style="font-size: 12.5px">
                  {{ d.fqdn }}
                </td>

                <!-- 来源 -->
                <td>
                  <span class="badge badge-neutral">
                    {{ d.origin === 'self' ? '自有域名' : '平台默认域名' }}
                  </span>
                </td>

                <!-- 激活 -->
                <td>
                  <span
                    :class="
                      d.status === 'active'
                        ? 'badge badge-ok'
                        : d.status === 'stopped'
                          ? 'badge badge-neutral'
                          : 'badge badge-warn'
                    "
                  >
                    {{
                      d.status === 'active'
                        ? '已激活'
                        : d.status === 'stopped'
                          ? '已停用'
                          : '待验证'
                    }}
                  </span>
                </td>

                <!-- 证书 -->
                <td class="shrink">
                  <span
                    :class="
                      d.certStatus === 'issued'
                        ? 'badge badge-ok'
                        : d.certStatus === 'pending'
                          ? 'badge badge-warn'
                          : 'badge badge-neutral'
                    "
                  >
                    {{
                      d.certStatus === 'issued'
                        ? '已签发'
                        : d.certStatus === 'pending'
                          ? '签发中'
                          : d.certStatus === 'failed'
                            ? '签发异常'
                            : '—'
                    }}
                  </span>
                </td>

                <!-- 短链 / 有效 -->
                <td class="num">
                  {{ getDomainLinkStats(d) }}
                </td>

                <!-- 24h 访问 -->
                <td class="num">
                  {{ getDomainVisitsFormatted(d) }}
                </td>

                <!-- 可用率 -->
                <td class="num">
                  {{ d.status === 'active' ? '99.98%' : '—' }}
                </td>

                <!-- 操作 -->
                <td class="shrink">
                  <div class="row" style="gap: 4px; flex-wrap: nowrap">
                    <button
                      type="button"
                      class="btn btn-sm"
                      @click="handleRecheckDomain(d)"
                      title="手动重新校验 DNS 与证书"
                    >
                      重检
                    </button>
                    <button
                      type="button"
                      class="btn btn-sm"
                      v-if="d.status !== 'stopped'"
                      @click="handleToggleDomainStatus(d, 'stopped')"
                    >
                      停用
                    </button>
                    <button
                      type="button"
                      class="btn btn-sm"
                      v-else
                      @click="handleToggleDomainStatus(d, 'active')"
                    >
                      恢复
                    </button>
                    <button
                      v-if="d.origin === 'self'"
                      type="button"
                      class="btn btn-sm btn-ghost text-red-500 hover:text-red-600"
                      @click="handleDeleteDomain(d)"
                    >
                      删除
                    </button>
                  </div>
                </td>
              </tr>
            </tbody>
          </table>
        </div>

        <div class="panel-ft row-between">
          <span>域名配额 {{ usage?.domains ?? domains.length }} / {{ usage?.maxDomains ?? 10 }} · 平台默认域名不计入配额</span>
          <span class="mono">失效切换：健康检查每 60s，失败域名自动降权 10 分钟</span>
        </div>
      </div>
    </section>

    <!-- ==================== 模态框: 批量导入 ==================== -->
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
              >
                {{ d.fqdn }}
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
              placeholder="vip-deal https://example.com/vip&#10;blackfriday https://example.com/bf"
            ></textarea>
            <span class="hint">每行一条，短码与 URL 用空格分隔</span>
          </div>
        </div>
        <div class="panel-ft row-between">
          <span class="tiny text-muted">有效行数：{{ parsedBatchLinesCount }}</span>
          <div class="row" style="gap: 8px">
            <button
              type="button"
              class="btn btn-sm"
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

    <!-- ==================== 模态框: 添加自有域名 ==================== -->
    <div
      v-if="showAddDomainModal"
      class="fixed inset-0 z-50 flex items-center justify-center bg-black/45 backdrop-blur-xs p-4"
    >
      <div class="panel w-full max-w-md shadow-2xl">
        <div class="panel-hd">
          <div>
            <h2 class="text-base font-bold">添加自有域名</h2>
            <p>需先在 DNS 服务商将该域名的 A/CNAME 解析指向本服务器。</p>
          </div>
          <button
            type="button"
            class="icon-btn text-muted hover:text-fg"
            @click="showAddDomainModal = false"
          >
            <X :size="15" />
          </button>
        </div>
        <div class="panel-bd stack">
          <div class="field">
            <label for="newFqdn">域名 (FQDN)</label>
            <input
              v-model="newDomainFqdn"
              class="input mono"
              id="newFqdn"
              placeholder="e.g. go.yourbrand.com"
            />
            <span class="hint">全限定域名，不带 http:// 与尾部斜杠</span>
          </div>
          <div class="field">
            <label for="newDesc">备注描述 (可选)</label>
            <input
              v-model="newDomainDesc"
              class="input"
              id="newDesc"
              placeholder="如：北美投放主域名"
            />
          </div>
        </div>
        <div class="panel-ft row-between">
          <router-link
            to="/domains/new"
            class="linkish tiny text-muted"
            @click="showAddDomainModal = false"
          >
            前往完整向导 →
          </router-link>
          <div class="row" style="gap: 8px">
            <button
              type="button"
              class="btn btn-sm"
              @click="showAddDomainModal = false"
            >
              取消
            </button>
            <button
              type="button"
              class="btn btn-sm btn-primary"
              :disabled="addingDomain || !newDomainFqdn.trim()"
              @click="handleCreateDomain"
            >
              {{ addingDomain ? '添加中...' : '确认添加' }}
            </button>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref, watch } from 'vue';
import { useRoute, useRouter } from 'vue-router';
import dayjs from 'dayjs';
import {
  ArrowDown,
  ArrowUp,
  Copy,
  FileDown,
  Pencil,
  Plus,
  Trash2,
  Upload,
  X,
} from '@lucide/vue';

import {
  createDomain,
  deleteDomain,
  listDomains,
  recheckDomain,
  updateDomainStatus,
} from '@/api/domains';
import {
  createLink,
  deleteLink,
  listLinks,
  updateLink,
  uploadLanding,
} from '@/api/links';
import { confirm } from '@/components/ui/confirm';
import { useAuthStore } from '@/stores/auth';
import { ApiError } from '@/types/api';
import type {
  Domain,
  DomainStatus,
  LandingSource,
  Link,
  LinkStatus,
  LinkType,
  RedirectStatus,
} from '@/types/api';
import { formatDateTime } from '@/utils/format';
import { message } from '@/utils/toast';

const route = useRoute();
const router = useRouter();
const auth = useAuthStore();

// ==================== 原型静态/演示数据兜底 ====================
const DEMO_DOMAINS: Domain[] = [
  {
    id: 1,
    fqdn: 'go.northwind-media.com',
    description: '主投放域名',
    origin: 'self',
    status: 'active',
    certStatus: 'issued',
    activatedAt: '2025-10-01T00:00:00Z',
    createdAt: '2025-10-01T00:00:00Z',
  },
  {
    id: 2,
    fqdn: 'eu.northwind-media.com',
    description: '欧洲区投放域名',
    origin: 'self',
    status: 'active',
    certStatus: 'issued',
    activatedAt: '2025-10-05T00:00:00Z',
    createdAt: '2025-10-05T00:00:00Z',
  },
  {
    id: 3,
    fqdn: 'bl.northwind-media.com',
    description: '黑五专属域名',
    origin: 'self',
    status: 'active',
    certStatus: 'issued',
    activatedAt: '2025-10-15T00:00:00Z',
    createdAt: '2025-10-15T00:00:00Z',
  },
  {
    id: 4,
    fqdn: 'nwmedia.cloak.link',
    description: '平台默认域名',
    origin: 'platform',
    status: 'active',
    certStatus: 'issued',
    activatedAt: '2025-09-20T00:00:00Z',
    createdAt: '2025-09-20T00:00:00Z',
  },
  {
    id: 5,
    fqdn: 'old.northwind-media.com',
    description: '已停用旧域名',
    origin: 'self',
    status: 'stopped',
    certStatus: 'issued',
    activatedAt: '2025-08-01T00:00:00Z',
    createdAt: '2025-08-01T00:00:00Z',
  },
  {
    id: 6,
    fqdn: 'shop.northwind-media.com',
    description: '待验证电商域名',
    origin: 'self',
    status: 'pending',
    certStatus: 'pending',
    activatedAt: '',
    createdAt: '2025-11-27T00:00:00Z',
  },
];

const DEMO_LINKS: Link[] = [
  {
    id: 1042,
    code: 'vip-access',
    targetUrls: [
      'https://northwind-media.com/landing/vip-access',
      'https://secure-checkout.net/offer/9f2a',
      'https://northwind-media.com/pt/acesso',
    ],
    redirectStatus: '302',
    linkType: 'redirect',
    landingSource: 'url',
    landingUrl: '',
    status: 'enabled',
    domains: ['go.northwind-media.com'],
    visits: 48206,
    clicks: 3842,
    landingUploaded: false,
    createdAt: '2025-11-20T10:00:00Z',
  },
  {
    id: 1043,
    code: 'tiktok-9d1',
    targetUrls: ['https://shop.northwind-media.com/prod/9d1'],
    redirectStatus: '302',
    linkType: 'landing',
    landingSource: 'upload',
    landingUrl: 'https://go.northwind-media.com/tiktok-9d1/',
    status: 'enabled',
    domains: ['go.northwind-media.com'],
    visits: 31884,
    clicks: 2106,
    landingUploaded: true,
    createdAt: '2025-11-22T14:30:00Z',
  },
  {
    id: 1044,
    code: 'fb-ck7',
    targetUrls: [
      'https://northwind-media.com/pt/acesso',
      'https://acesso-vip.com.br/entrada',
    ],
    redirectStatus: '302',
    linkType: 'landing',
    landingSource: 'url',
    landingUrl: 'https://go.northwind-media.com/fb-ck7/',
    status: 'enabled',
    domains: ['go.northwind-media.com'],
    visits: 19442,
    clicks: 1530,
    landingUploaded: false,
    createdAt: '2025-11-23T09:15:00Z',
  },
  {
    id: 1045,
    code: 'promo-eu',
    targetUrls: ['https://eu.northwind-media.com/special'],
    redirectStatus: '302',
    linkType: 'redirect',
    landingSource: 'url',
    landingUrl: '',
    status: 'enabled',
    domains: ['eu.northwind-media.com'],
    visits: 8127,
    clicks: 311,
    landingUploaded: false,
    createdAt: '2025-11-24T16:00:00Z',
  },
  {
    id: 1046,
    code: 'black-friday',
    targetUrls: [
      'https://bl.northwind-media.com/sale',
      'https://backup.northwind-media.com/sale',
    ],
    redirectStatus: '302',
    linkType: 'redirect',
    landingSource: 'url',
    landingUrl: '',
    status: 'enabled',
    domains: ['bl.northwind-media.com'],
    visits: 0,
    clicks: 0,
    landingUploaded: false,
    createdAt: '2025-11-25T11:00:00Z',
  },
  {
    id: 1047,
    code: 'legacy-cp',
    targetUrls: ['https://old.northwind-media.com/landing'],
    redirectStatus: '301',
    linkType: 'redirect',
    landingSource: 'url',
    landingUrl: '',
    status: 'disabled',
    domains: ['old.northwind-media.com'],
    visits: 640,
    clicks: 12,
    landingUploaded: false,
    createdAt: '2025-10-10T08:00:00Z',
  },
  {
    id: 1048,
    code: 'whitelist-test',
    targetUrls: ['https://qa.northwind-media.com/test-pass'],
    redirectStatus: '302',
    linkType: 'redirect',
    landingSource: 'url',
    landingUrl: '',
    status: 'enabled',
    domains: ['qa.northwind-media.com'],
    visits: 412,
    clicks: 36,
    landingUploaded: false,
    createdAt: '2025-11-26T18:20:00Z',
  },
];

interface BoundRule {
  id: string;
  name: string;
  action: string;
}

const CATALOG_RULES: BoundRule[] = [
  { id: 'R-001', name: '目标市场 · 移动端放行', action: '放行 → 目标池 A' },
  { id: 'R-002', name: '拦截 · 平台审查爬虫', action: '白标页' },
  { id: 'R-003', name: '拦截 · 代理与机房出口', action: '404 兜底' },
  { id: 'R-004', name: '语言分流 · 葡语市场', action: '放行 → 目标池 B' },
  { id: 'R-005', name: '设备型号白名单（已停用）', action: '放行 → 目标池 A' },
  { id: 'R-006', name: '内部测试强制放行', action: '放行 → 目标池 A' },
  { id: 'R-007', name: '频次风控 · 单 IP 限流', action: '限流拦截' },
  { id: 'R-008', name: '兜底 · 品牌白标页', action: '白标页' },
];

const linkRulesMap: Record<string, string> = {
  'vip-access': 'R-001 / R-004 / R-008',
  'tiktok-9d1': 'R-001',
  'fb-ck7': 'R-004',
  'promo-eu': '（未绑定）',
  'black-friday': 'R-001 / R-007',
  'legacy-cp': 'R-002 / R-003',
  'whitelist-test': 'R-006',
};

interface TargetEntry {
  url: string;
  pool: string;
  weight: number;
  health: string;
  visits24h: number;
  editing?: boolean;
}

// ==================== 响应式状态 ====================
const currentTab = ref<'links' | 'edit' | 'domains'>('links');
const isLiveBackend = ref(false);

const links = ref<Link[]>([]);
const domains = ref<Domain[]>([]);
const loading = ref(false);
const total = ref(0);

const keyword = ref('');
const typeFilter = ref<string>('all');
const statusFilter = ref<string>('all');

const usage = computed(() => auth.config?.usage);

// 编辑器状态
const editingLinkId = ref<number | null>(1042);
const isExistingSavedLink = computed(() => editingLinkId.value !== null && editingLinkId.value > 0);
const saving = ref(false);
const isSavedRecently = ref(false);
const uploadingZip = ref(false);
const landingFileInput = ref<HTMLInputElement | null>(null);
const landingFileSummary = ref('vip-access-v7.zip · 412 KB · 上传于 2025-11-28');

const form = reactive({
  code: 'vip-access',
  domainId: 1,
  domainFqdn: 'go.northwind-media.com',
  linkType: 'redirect' as LinkType,
  redirectStatus: '302' as RedirectStatus,
  schedule: '2026-01-01 → 长期',
  landingSource: 'url' as LandingSource,
  landingUrl: 'https://go.northwind-media.com/vip-access/',
  status: 'enabled' as LinkStatus,
});

const boundRules = ref<BoundRule[]>([
  { id: 'R-001', name: '目标市场 · 移动端放行', action: '放行 → 目标池 A' },
  { id: 'R-004', name: '语言分流 · 葡语市场', action: '放行 → 目标池 B' },
  { id: 'R-008', name: '兜底 · 品牌白标页', action: '白标页' },
]);
const selectedRuleToAdd = ref('');

const targets = ref<TargetEntry[]>([
  { url: 'https://northwind-media.com/landing/vip-access', pool: '目标池 A', weight: 5, health: '健康', visits24h: 28410 },
  { url: 'https://secure-checkout.net/offer/9f2a', pool: '目标池 A', weight: 3, health: '健康', visits24h: 14602 },
  { url: 'https://northwind-media.com/pt/acesso', pool: '目标池 B', weight: 1, health: '响应慢 · 412ms', visits24h: 12880 },
  { url: 'https://acesso-vip.com.br/entrada', pool: '目标池 B', weight: 1, health: '健康', visits24h: 5723 },
]);

const outbound = reactive({
  stripReferer: true,
  hideTarget: true,
  passUtm: false,
  randomDelay: true,
  guestIdWithoutCookie: true,
});

const postback = reactive({
  tiktokEvents: true,
  metaCapi: true,
  googleEnhanced: true,
  googleOffline: false,
  events: 'ViewContent, Click, AddToCart, Purchase',
  webhookUrl: 'https://api.northwind-media.com/conv',
});

// 模态框状态
const showBatchModal = ref(false);
const batchText = ref('');
const batchDomainId = ref<number | undefined>(undefined);
const batchImporting = ref(false);

const showAddDomainModal = ref(false);
const newDomainFqdn = ref('');
const newDomainDesc = ref('');
const addingDomain = ref(false);

// ==================== 计算属性 ====================
const currentDomainFqdn = computed(() => {
  const d = domains.value.find((item) => item.id === form.domainId);
  return d?.fqdn || form.domainFqdn || domains.value[0]?.fqdn || 'go.northwind-media.com';
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

const parsedBatchLinesCount = computed(() => {
  return batchText.value
    .split('\n')
    .map((s) => s.trim())
    .filter(Boolean).length;
});

// 6 步访问路径裁决推演 (实时联动)
const traceStep1 = computed(() => {
  if (boundRules.value.length > 0) {
    const r = boundRules.value[0];
    return `① 命中 ${r.id}（${r.name.split('·')[0].trim()}）`;
  }
  return '① 未命中绑定规则 → 执行兜底：无规则兜底';
});

const traceStep2 = computed(() => {
  if (boundRules.value.length > 0) {
    return `② 动作：${boundRules.value[0].action}`;
  }
  return '② 动作：放行 → 目标池 A（默认兜底池）';
});

const traceStep3 = computed(() => {
  const t = targets.value[0];
  if (!t || !t.url) return '③ 权重选择：尚未配置出口';
  try {
    const host = new URL(t.url).hostname;
    return `③ 权重选择：${host}（权重 ${t.weight}，目标池调度）`;
  } catch {
    return `③ 权重选择：${t.url}（权重 ${t.weight}）`;
  }
});

const traceStep4 = computed(() => {
  const parts: string[] = [];
  parts.push(outbound.stripReferer ? '剥离 Referer' : '保留 Referer');
  if (outbound.randomDelay) parts.push('随机延迟 0–800ms');
  if (outbound.passUtm) parts.push('透传 UTM 参数');
  if (outbound.hideTarget) parts.push('隐藏真实目标');
  return `④ ${parts.join(' + ')}`;
});

const traceStep5 = computed(() => {
  const t = targets.value[0];
  const urlDisplay = t?.url ? truncateUrl(t.url) : 'northwind-media.com/landing';
  return `⑤ ${form.redirectStatus || '302'} → ${urlDisplay}`;
});

const traceStep6 = computed(() => {
  const platforms: string[] = [];
  if (postback.tiktokEvents) platforms.push('TikTok');
  if (postback.metaCapi) platforms.push('Meta');
  if (postback.googleEnhanced || postback.googleOffline) platforms.push('Google');
  const firstEvt = postback.events.split(',')[0]?.trim() || 'ViewContent';
  if (platforms.length === 0) return `⑥ 回传转化：未开启广告回传`;
  return `⑥ 回传 ${firstEvt} 至 ${platforms.join(' / ')}`;
});

const healthyTargetsCount = computed(() => {
  return targets.value.filter((t) => t.health === '健康').length;
});

const certRemainingDays = computed(() => {
  const cur = domains.value.find((d) => d.id === form.domainId || d.fqdn === form.domainFqdn);
  if (cur?.certStatus === 'issued') return '38 天';
  if (cur?.certStatus === 'pending') return '签发中';
  return '—';
});

const targetAlertCount = computed(() => {
  return targets.value.filter((t) => t.health.includes('慢') || t.health.includes('未')).length;
});

// ==================== 工具辅助方法 ====================
function truncateUrl(url: string): string {
  try {
    const u = new URL(url);
    return `${u.hostname}${u.pathname}`;
  } catch {
    return url;
  }
}

function getLinkDomain(link: Link): string {
  return link.domains?.[0] || domains.value[0]?.fqdn || 'go.northwind-media.com';
}

function getLinkRuleStr(link: Link): string {
  if (linkRulesMap[link.code]) return linkRulesMap[link.code];
  if (editingLinkId.value === link.id && boundRules.value.length > 0) {
    return boundRules.value.map((r) => r.id).join(' / ');
  }
  return '（未绑定）';
}

function getTargetPoolName(link: Link): string {
  if (link.code === 'legacy-cp') return '白标页';
  if (link.code === 'fb-ck7') return '目标池 B';
  return '目标池 A';
}

function getLinkCtr(link: Link): string {
  if (!link.visits || link.visits === 0) return '—';
  if (!link.clicks || link.clicks === 0) return '0.00%';
  return `${((link.clicks / link.visits) * 100).toFixed(2)}%`;
}

function getDomainLinkStats(d: Domain): string {
  const matched = links.value.filter((l) => l.domains?.includes(d.fqdn));
  const activeCount = matched.filter((l) => l.status === 'enabled').length;
  if (matched.length > 0) return `${matched.length} / ${activeCount}`;
  if (d.fqdn.includes('go.')) return '6 / 4';
  if (d.fqdn.includes('eu.')) return '3 / 3';
  if (d.fqdn.includes('bl.')) return '2 / 2';
  if (d.fqdn.includes('nwmedia')) return '5 / 5';
  if (d.fqdn.includes('old.')) return '1 / 1';
  return '0 / 0';
}

function getDomainVisitsFormatted(d: Domain): string {
  const matched = links.value.filter((l) => l.domains?.includes(d.fqdn));
  const sum = matched.reduce((acc, l) => acc + (l.visits || 0), 0);
  if (sum > 0) return sum.toLocaleString();
  if (d.fqdn.includes('go.')) return '1,042,338';
  if (d.fqdn.includes('eu.')) return '188,204';
  if (d.fqdn.includes('nwmedia')) return '72,406';
  if (d.fqdn.includes('old.')) return '640';
  return '0';
}

// ==================== 选项卡切换与 URL 同步 ====================
function switchTab(tab: 'links' | 'edit' | 'domains') {
  currentTab.value = tab;
  try {
    localStorage.setItem('cloak.lm.tab', tab);
  } catch {}
  router.replace({
    query: { ...route.query, tab },
    hash: tab === 'domains' ? '#domains' : tab === 'edit' ? '#edit' : '#links',
  });
}

function syncTabFromRoute() {
  const qTab = route.query.tab as string | undefined;
  const hash = route.hash;
  const qEdit = route.query.edit as string | undefined;

  if (qTab === 'links' || qTab === 'edit' || qTab === 'domains') {
    currentTab.value = qTab;
  } else if (hash === '#domains') {
    currentTab.value = 'domains';
  } else if (hash === '#edit') {
    currentTab.value = 'edit';
  } else if (hash === '#links') {
    currentTab.value = 'links';
  } else if (qEdit) {
    currentTab.value = 'edit';
  } else {
    try {
      const saved = localStorage.getItem('cloak.lm.tab') as 'links' | 'edit' | 'domains' | null;
      if (saved && ['links', 'edit', 'domains'].includes(saved)) {
        currentTab.value = saved;
      }
    } catch {}
  }

  if (qEdit) {
    const found = links.value.find((l) => l.code === qEdit || String(l.id) === qEdit);
    if (found) {
      openEditTab(found);
    }
  }
}

watch(
  () => [route.query.tab, route.hash, route.query.edit],
  () => {
    syncTabFromRoute();
  },
);

// ==================== 数据加载 ====================
async function loadData() {
  loading.value = true;
  try {
    const [linksRes, domainsRes] = await Promise.allSettled([
      listLinks({ page: 1, pageSize: 100 }),
      listDomains(),
    ]);

    if (linksRes.status === 'fulfilled' && linksRes.value.items.length > 0) {
      links.value = linksRes.value.items;
      total.value = linksRes.value.total;
      isLiveBackend.value = true;
    } else {
      links.value = [...DEMO_LINKS];
      total.value = DEMO_LINKS.length;
    }

    if (domainsRes.status === 'fulfilled' && domainsRes.value.length > 0) {
      domains.value = domainsRes.value;
      isLiveBackend.value = true;
    } else {
      domains.value = [...DEMO_DOMAINS];
    }

    if (domains.value.length > 0 && !batchDomainId.value) {
      batchDomainId.value = domains.value[0].id;
    }
  } catch {
    links.value = [...DEMO_LINKS];
    total.value = DEMO_LINKS.length;
    domains.value = [...DEMO_DOMAINS];
  } finally {
    loading.value = false;
    syncTabFromRoute();
  }
}

onMounted(() => {
  loadData();
});

// ==================== 短链列表操作 ====================
async function onToggleLinkStatus(link: Link) {
  const nextStatus: LinkStatus = link.status === 'enabled' ? 'disabled' : 'enabled';
  const label = nextStatus === 'disabled' ? '停用' : '启用';
  try {
    if (link.id > 0) {
      await updateLink(link.id, { status: nextStatus });
    }
    link.status = nextStatus;
    message.success(`短链「${link.code}」已${label}`);
  } catch (error) {
    if (error instanceof ApiError) message.error(error.message);
    else {
      link.status = nextStatus;
      message.success(`短链「${link.code}」已${label} (演示)`);
    }
  }
}

async function copyLinkUrl(link: Link) {
  const fqdn = getLinkDomain(link);
  const url = `https://${fqdn}/${link.code}`;
  try {
    await navigator.clipboard.writeText(url);
    message.success('已复制: ' + url);
  } catch {
    message.error('复制失败，请手动复制');
  }
}

function handleDeleteLink(link: Link) {
  confirm({
    title: `删除短链「${link.code}」?`,
    content: '删除为逻辑删除：记录、关联与访问信息保留，但「域名/短码」将不再命中。',
    okText: '删除',
    cancelText: '取消',
    danger: true,
    onOk: async () => {
      try {
        if (link.id > 0) await deleteLink(link.id);
        links.value = links.value.filter((l) => l.id !== link.id);
        message.success('短链已逻辑删除');
      } catch (err) {
        if (err instanceof ApiError) message.error(err.message);
        else {
          links.value = links.value.filter((l) => l.id !== link.id);
          message.success('短链已删除 (演示)');
        }
      }
    },
  });
}

function exportCsv() {
  const headers = ['短码', '承载域名', '类型', '绑定规则', '默认去向', '目标数', '24h访问', '转化', 'CTR', '状态', '创建时间'];
  const rows = filteredLinks.value.map((l) => [
    l.code,
    getLinkDomain(l),
    l.linkType === 'landing' ? '落地页型' : '跳转型',
    getLinkRuleStr(l),
    getTargetPoolName(l),
    l.targetUrls?.length || 1,
    l.visits || 0,
    l.clicks || 0,
    getLinkCtr(l),
    l.status === 'enabled' ? '启用' : '停用',
    formatDateTime(l.createdAt),
  ]);

  const csvContent = [headers.join(','), ...rows.map((r) => r.map((c) => `"${String(c).replace(/"/g, '""')}"`).join(','))].join('\n');
  const blob = new Blob(['\uFEFF' + csvContent], { type: 'text/csv;charset=utf-8;' });
  const url = URL.createObjectURL(blob);
  const a = document.createElement('a');
  a.href = url;
  a.download = `links-export-${dayjs().format('YYYYMMDD-HHmmss')}.csv`;
  a.click();
  URL.revokeObjectURL(a.href);
  message.success(`已导出 ${rows.length} 条短链记录`);
}

// ==================== 编辑器操作 ====================
function openCreate() {
  editingLinkId.value = null;
  form.code = '';
  form.linkType = 'redirect';
  form.redirectStatus = '302';
  form.schedule = '2026-01-01 → 长期';
  form.landingSource = 'url';
  form.landingUrl = '';
  form.status = 'enabled';
  landingFileSummary.value = '未上传压缩包';

  if (domains.value.length > 0) {
    form.domainId = domains.value[0].id;
    form.domainFqdn = domains.value[0].fqdn;
  }

  targets.value = [
    {
      url: 'https://',
      pool: '目标池 A',
      weight: 1,
      health: '健康',
      visits24h: 0,
      editing: true,
    },
  ];

  boundRules.value = [
    { id: 'R-001', name: '目标市场 · 移动端放行', action: '放行 → 目标池 A' },
  ];

  switchTab('edit');
}

function openEditTab(link: Link) {
  editingLinkId.value = link.id;
  form.code = link.code;
  form.linkType = link.linkType || 'redirect';
  form.redirectStatus = link.redirectStatus || '302';
  form.schedule = '2026-01-01 → 长期';
  form.landingSource = link.landingSource || 'url';
  form.landingUrl = link.landingUrl || '';
  form.status = link.status || 'enabled';

  if (link.landingUploaded) {
    landingFileSummary.value = `${link.code}-landing.zip · 已托管 · 状态正常`;
  } else {
    landingFileSummary.value = '未上传压缩包';
  }

  const fqdn = link.domains?.[0];
  if (fqdn) {
    const d = domains.value.find((item) => item.fqdn === fqdn);
    if (d) {
      form.domainId = d.id;
      form.domainFqdn = d.fqdn;
    } else {
      form.domainFqdn = fqdn;
      if (domains.value.length > 0) form.domainId = domains.value[0].id;
    }
  } else if (domains.value.length > 0) {
    form.domainId = domains.value[0].id;
    form.domainFqdn = domains.value[0].fqdn;
  }

  if (link.targetUrls && link.targetUrls.length > 0) {
    targets.value = link.targetUrls.map((url, i) => ({
      url,
      pool: i % 2 === 0 ? '目标池 A' : '目标池 B',
      weight: i === 0 ? 5 : i === 1 ? 3 : 1,
      health: '健康',
      visits24h: Math.round((link.visits || 0) / link.targetUrls.length),
      editing: false,
    }));
  } else {
    targets.value = [
      {
        url: 'https://northwind-media.com/landing/' + link.code,
        pool: '目标池 A',
        weight: 1,
        health: '健康',
        visits24h: link.visits || 0,
        editing: false,
      },
    ];
  }

  const ruleIds = (linkRulesMap[link.code] || '').split(' / ');
  const rulesFound = CATALOG_RULES.filter((r) => ruleIds.includes(r.id));
  boundRules.value = rulesFound.length > 0 ? [...rulesFound] : [
    { id: 'R-001', name: '目标市场 · 移动端放行', action: '放行 → 目标池 A' },
  ];

  switchTab('edit');
}

function onDomainChange() {
  const d = domains.value.find((item) => item.id === form.domainId);
  if (d) form.domainFqdn = d.fqdn;
}

function moveRuleUp(idx: number) {
  if (idx <= 0) return;
  const temp = boundRules.value[idx - 1];
  boundRules.value[idx - 1] = boundRules.value[idx];
  boundRules.value[idx] = temp;
}

function moveRuleDown(idx: number) {
  if (idx >= boundRules.value.length - 1) return;
  const temp = boundRules.value[idx + 1];
  boundRules.value[idx + 1] = boundRules.value[idx];
  boundRules.value[idx] = temp;
}

function removeBoundRule(idx: number) {
  boundRules.value.splice(idx, 1);
}

function bindSelectedRule() {
  if (!selectedRuleToAdd.value) return;
  const rid = selectedRuleToAdd.value;
  if (boundRules.value.some((r) => r.id === rid)) {
    message.warning('该规则已在此短链绑定列表中');
    return;
  }
  const found = CATALOG_RULES.find((r) => r.id === rid);
  if (found) {
    boundRules.value.push({ ...found });
    selectedRuleToAdd.value = '';
    message.success(`已绑定规则 ${found.id}`);
  }
}

function addTarget() {
  targets.value.push({
    url: 'https://',
    pool: '目标池 A',
    weight: 1,
    health: '健康',
    visits24h: 0,
    editing: true,
  });
}

function removeTarget(idx: number) {
  if (targets.value.length <= 1) {
    message.warning('请至少保留一个出口目标');
    return;
  }
  targets.value.splice(idx, 1);
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
    return;
  }

  if (!editingLinkId.value || editingLinkId.value <= 0) {
    landingFileSummary.value = `${file.name} · ${(file.size / 1024).toFixed(0)} KB · 待保存后上传`;
    message.info('新建短链请先保存基本设置，随后自动上传落地页包');
    return;
  }

  uploadingZip.value = true;
  try {
    const updated = await uploadLanding(editingLinkId.value, file);
    landingFileSummary.value = `${file.name} · ${(file.size / 1024).toFixed(0)} KB · 刚刚上传`;
    message.success('落地页压缩包托管成功');
    const idx = links.value.findIndex((l) => l.id === updated.id);
    if (idx >= 0) links.value[idx] = updated;
  } catch (err) {
    if (err instanceof ApiError) message.error(err.message);
    else {
      landingFileSummary.value = `${file.name} · ${(file.size / 1024).toFixed(0)} KB · 已上传 (演示)`;
      message.success('压缩包上传成功 (演示)');
    }
  } finally {
    uploadingZip.value = false;
    input.value = '';
  }
}

async function handleSaveLink() {
  const validTargetUrls = targets.value
    .map((t) => t.url.trim())
    .filter((u) => u && u !== 'https://');

  if (validTargetUrls.length === 0) {
    message.error('请至少配置一个有效的目标 URL');
    return;
  }

  saving.value = true;
  try {
    const domainIds = form.domainId ? [form.domainId] : [];
    if (editingLinkId.value && editingLinkId.value > 0) {
      const updated = await updateLink(editingLinkId.value, {
        targetUrls: validTargetUrls,
        domainIds,
        redirectStatus: form.redirectStatus,
        status: form.status,
        linkType: form.linkType,
        landingSource: form.landingSource,
        landingUrl: form.landingUrl,
      });
      const idx = links.value.findIndex((l) => l.id === editingLinkId.value);
      if (idx >= 0) links.value[idx] = updated;
    } else {
      const created = await createLink({
        code: form.code.trim() || undefined,
        targetUrls: validTargetUrls,
        domainIds,
        redirectStatus: form.redirectStatus,
        linkType: form.linkType,
        landingSource: form.landingSource,
        landingUrl: form.landingUrl,
      });
      editingLinkId.value = created.id;
      form.code = created.code;
      links.value.unshift(created);
      total.value += 1;
    }

    if (form.code) {
      linkRulesMap[form.code] = boundRules.value.map((r) => r.id).join(' / ') || '（未绑定）';
    }

    isSavedRecently.value = true;
    setTimeout(() => {
      isSavedRecently.value = false;
    }, 1600);
    message.success('短链已成功保存');
  } catch (error) {
    if (error instanceof ApiError) {
      message.error(error.message);
    } else {
      if (form.code) {
        linkRulesMap[form.code] = boundRules.value.map((r) => r.id).join(' / ') || '（未绑定）';
      }
      isSavedRecently.value = true;
      setTimeout(() => {
        isSavedRecently.value = false;
      }, 1600);
      message.success('配置已保存 (演示模式)');
    }
  } finally {
    saving.value = false;
  }
}

async function copyCurrentLink() {
  const fqdn = form.domainFqdn || currentDomainFqdn.value;
  const code = form.code || 'vip-access';
  const url = `https://${fqdn}/${code}`;
  try {
    await navigator.clipboard.writeText(url);
    message.success('已复制短链: ' + url);
  } catch {
    message.error('复制失败，请手动选择复制');
  }
}

// ==================== 模态框操作 ====================
async function executeBatchImport() {
  const lines = batchText.value
    .split('\n')
    .map((l) => l.trim())
    .filter(Boolean);

  if (lines.length === 0) return;

  batchImporting.value = true;
  let successCount = 0;

  for (const line of lines) {
    const parts = line.split(/[\s,]+/).filter(Boolean);
    let code = '';
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
      const created = await createLink({
        code: code || undefined,
        targetUrls: [url],
        domainIds: batchDomainId.value ? [batchDomainId.value] : [],
        redirectStatus: '302',
        linkType: 'redirect',
      });
      links.value.unshift(created);
      successCount++;
    } catch {
      const mockId = Date.now() + Math.floor(Math.random() * 1000);
      links.value.unshift({
        id: mockId,
        code: code || 'b-' + Math.random().toString(36).slice(2, 7),
        targetUrls: [url],
        redirectStatus: '302',
        linkType: 'redirect',
        landingSource: 'url',
        landingUrl: '',
        status: 'enabled',
        domains: [currentDomainFqdn.value],
        visits: 0,
        clicks: 0,
        landingUploaded: false,
        createdAt: new Date().toISOString(),
      });
      successCount++;
    }
  }

  total.value = links.value.length;
  batchImporting.value = false;
  showBatchModal.value = false;
  batchText.value = '';
  message.success(`成功导入 ${successCount} 条短链`);
}

async function handleCreateDomain() {
  const fqdn = newDomainFqdn.value.trim().toLowerCase();
  if (!fqdn) return;

  addingDomain.value = true;
  try {
    const created = await createDomain({
      fqdn,
      description: newDomainDesc.value.trim() || undefined,
    });
    domains.value.push(created);
    message.success(`自有域名「${created.fqdn}」已提交`);
    showAddDomainModal.value = false;
    newDomainFqdn.value = '';
    newDomainDesc.value = '';
  } catch (error) {
    if (error instanceof ApiError) {
      message.error(error.message);
    } else {
      const mockDomain: Domain = {
        id: Date.now(),
        fqdn,
        description: newDomainDesc.value.trim() || '自定义自有域名',
        origin: 'self',
        status: 'pending',
        certStatus: 'pending',
        activatedAt: '',
        createdAt: new Date().toISOString(),
      };
      domains.value.push(mockDomain);
      message.success(`自有域名「${fqdn}」已添加 (演示)`);
      showAddDomainModal.value = false;
      newDomainFqdn.value = '';
      newDomainDesc.value = '';
    }
  } finally {
    addingDomain.value = false;
  }
}

// ==================== 域名池操作 ====================
async function handleRecheckDomain(d: Domain) {
  try {
    await recheckDomain(d.id);
    message.success(`已提交 ${d.fqdn} 的重新校验，请稍后刷新状态`);
  } catch (err) {
    if (err instanceof ApiError) message.error(err.message);
    else message.success(`已重新触发 ${d.fqdn} 的 DNS 校验`);
  }
}

function handleToggleDomainStatus(d: Domain, next: DomainStatus) {
  const label = next === 'stopped' ? '停用' : '恢复';
  confirm({
    title: `${label}域名「${d.fqdn}」?`,
    content:
      next === 'stopped'
        ? '停用后，该域名下的所有短链将立即返回 404 未命中。'
        : '恢复后，该域名下的短链将恢复解析服务。',
    okText: label,
    danger: next === 'stopped',
    cancelText: '取消',
    onOk: async () => {
      try {
        await updateDomainStatus(d.id, next);
        d.status = next;
        message.success(`域名已${label}`);
      } catch (err) {
        if (err instanceof ApiError) message.error(err.message);
        else {
          d.status = next;
          message.success(`域名已${label} (演示)`);
        }
      }
    },
  });
}

function handleDeleteDomain(d: Domain) {
  confirm({
    title: `删除域名「${d.fqdn}」?`,
    content: '删除为物理删除。如果仍有关联的未删除短链将被拒绝，请确认已清空关联。',
    okText: '删除',
    cancelText: '取消',
    danger: true,
    onOk: async () => {
      try {
        await deleteDomain(d.id);
        domains.value = domains.value.filter((item) => item.id !== d.id);
        message.success('域名已删除');
      } catch (err) {
        if (err instanceof ApiError) message.error(err.message);
        else {
          domains.value = domains.value.filter((item) => item.id !== d.id);
          message.success('域名已删除 (演示)');
        }
      }
    },
  });
}
</script>
