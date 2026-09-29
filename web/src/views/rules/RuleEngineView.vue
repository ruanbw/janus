<template>
  <div class="flex flex-col gap-5">
    <!-- ==================== 顶部选项卡 ==================== -->
    <section class="panel" data-od-id="rule-engine-tabs">
      <div class="tabs" role="tablist" id="reTabs">
        <button
          role="tab"
          type="button"
          :aria-selected="currentTab === 'rules'"
          data-tab="rules"
          @click="switchTab('rules')"
        >
          规则列表
        </button>
        <button
          role="tab"
          type="button"
          :aria-selected="currentTab === 'editor'"
          data-tab="editor"
          @click="switchTab('editor')"
        >
          规则编辑器
        </button>
        <button
          role="tab"
          type="button"
          :aria-selected="currentTab === 'simulator'"
          data-tab="simulator"
          data-pending="1"
          title="前端等价实现的求值预览,最终以服务端执行结果为准"
          @click="switchTab('simulator')"
        >
          规则模拟器
          <span class="badge badge-neutral" style="margin-left: 6px">预览</span>
        </button>
        <button
          role="tab"
          type="button"
          :aria-selected="currentTab === 'lists'"
          data-tab="lists"
          id="lists"
          @click="switchTab('lists')"
        >
          名单库
        </button>
      </div>
    </section>

    <!-- ==================== Tab 1: 规则列表 ==================== -->
    <section v-show="currentTab === 'rules'" class="panel" data-tabpanel="rules" data-od-id="rule-list">
      <div class="panel-hd">
        <div>
          <h2>规则列表</h2>
          <p>按优先级从低到高串行求值，首条命中即裁决。优先级数值越小越先求值。</p>
        </div>
        <div class="btn-row">
          <button
            type="button"
            class="btn btn-sm"
            id="refreshBtn"
            :disabled="loading"
            @click="loadRules"
          >
            <RefreshCw :size="13" :class="loading ? 'animate-spin' : ''" />
            刷新
          </button>
          <button type="button" class="btn btn-sm btn-primary" id="newRuleBtn" @click="openCreateRule">
            <Plus :size="14" />
            新建规则
          </button>
        </div>
      </div>

      <div class="panel-bd">
        <div class="toolbar">
          <!-- 动作过滤分段器 -->
          <div class="seg-filter" id="actFilter" role="group" aria-label="按动作过滤">
            <button
              v-for="act in actionFilters"
              :key="act.key"
              type="button"
              :aria-pressed="selectedActFilter === act.key"
              :data-f="act.key"
              @click="selectedActFilter = act.key"
            >
              {{ act.label }}
            </button>
          </div>

          <!-- 搜索框 -->
          <div class="relative grow min-w-[200px]">
            <input
              v-model="searchKeyword"
              class="input input-affix"
              id="ruleSearch"
              placeholder="搜索规则名、编号、条件内容或关联短码…"
              aria-label="搜索规则"
            />
            <button
              v-if="searchKeyword"
              type="button"
              class="absolute right-2 top-1/2 -translate-y-1/2 text-muted hover:text-fg text-xs"
              @click="searchKeyword = ''"
            >
              ✕
            </button>
          </div>

          <!-- 状态过滤 -->
          <label class="row tiny muted" style="gap: 6px">
            <span>状态</span>
            <select v-model="statusFilter" class="select" id="statusFilter" aria-label="按状态过滤">
              <option value="all">全部</option>
              <option value="on">已启用</option>
              <option value="off">已停用</option>
            </select>
          </label>
        </div>
      </div>

      <!-- 高密表格 -->
      <div class="tbl-wrap">
        <table class="tbl" id="ruleTable">
          <thead>
            <tr>
              <th class="shrink">编号</th>
              <th style="width: 22%">规则</th>
              <th class="shrink">作用域</th>
              <th>条件摘要</th>
              <th class="shrink">动作</th>
              <th>去向</th>
              <th class="num">24h 命中</th>
              <th class="shrink">启用</th>
              <th class="shrink col-actions">操作</th>
            </tr>
          </thead>
          <tbody>
            <!-- 加载态 -->
            <tr v-if="loading && rules.length === 0">
              <td colspan="9" class="empty">
                <div class="flex items-center justify-center gap-2 py-6 text-muted">
                  <RefreshCw class="animate-spin" :size="16" />
                  正在加载规则数据...
                </div>
              </td>
            </tr>

            <!-- 空状态 -->
            <tr v-else-if="filteredRules.length === 0">
              <td colspan="9" class="py-8">
                <AppEmpty
                  :description="rules.length === 0 ? '暂无规则，点击「新建规则」创建第一条规则' : '未找到符合当前筛选条件的规则'"
                />
                <div class="flex justify-center">
                  <button
                    v-if="rules.length === 0"
                    type="button"
                    class="btn btn-sm btn-primary"
                    @click="openCreateRule"
                  >
                    <Plus :size="14" />
                    新建规则
                  </button>
                  <button v-else type="button" class="btn btn-sm" @click="resetFilters">
                    重置筛选条件
                  </button>
                </div>
              </td>
            </tr>

            <template v-else>
              <tr
                v-for="rule in filteredRules"
                :key="rule.id"
                :data-row="rule.id"
                :data-act="rule.action"
                :data-status="rule.enabled ? 'on' : 'off'"
                :data-name="rule.name"
              >
                <!-- 规则编号 -->
                <td class="shrink">
                  <span class="mono muted">{{ rule.id }}</span>
                </td>

                <!-- 名称与说明 -->
                <td>
                  <div class="truncate font-semibold text-[13px] text-ink" :title="rule.name">
                    {{ rule.name }}
                  </div>
                  <div class="micro muted mono mt-0.5">
                    优先级 {{ rule.priority }} · {{ logicLabel(rule.logic) }}
                  </div>
                </td>

                <!-- 作用域:全局 / N 条短链 / 未关联告警(spec D1、D2) -->
                <td class="shrink">
                  <div class="stack" style="gap: 3px">
                    <span v-if="rule.scope === 'global'" class="badge badge-ok">全局</span>
                    <template v-else>
                      <span
                        class="badge badge-neutral"
                        :title="
                          (rule.linkNames || []).length > 0
                            ? '已关联：' + rule.linkNames.join('、') + (rule.linkCount > rule.linkNames.length ? ' 等 ' + rule.linkCount + ' 条' : '')
                            : '未关联任何短链'
                        "
                      >
                        {{ rule.linkCount }} 条短链
                      </span>
                      <!-- 零关联的 scoped 规则永远不命中,必须显式暴露(spec D2) -->
                      <span
                        v-if="rule.linkCount === 0"
                        class="badge badge-warn"
                        title="spec D2:作用域为「指定短链」但未关联任何短链的规则不会退化成全局规则,它永远不命中"
                      >
                        未关联短链 · 不会命中
                      </span>
                    </template>
                  </div>
                </td>

                <!-- 条件摘要:列表接口不回传 conditions 时留空,不编造内容 -->
                <td>
                  <div class="truncate muted text-[12.5px]" :title="conditionSummary(rule)">
                    {{ conditionSummary(rule) }}
                  </div>
                </td>

                <!-- 动作 badge -->
                <td class="shrink">
                  <span :class="['badge', getActionBadgeClass(rule.action)]">
                    {{ actionLabel(rule.action) }}
                  </span>
                </td>

                <!-- 去向目标(仅 redirect 动作有意义) -->
                <td class="shrink mono tiny">
                  <span v-if="rule.action === 'redirect'" :title="rule.destination">
                    {{ rule.destination || '未填写' }}
                  </span>
                  <span v-else class="muted">—</span>
                </td>

                <!-- 24h 命中 -->
                <td class="num">
                  {{ rule.hits24h.toLocaleString() }}
                </td>

                <!-- Switch 启用/停用 -->
                <td class="shrink">
                  <label class="switch" :title="rule.enabled ? '点击停用' : '点击启用'">
                    <input
                      type="checkbox"
                      :checked="rule.enabled"
                      :disabled="togglingId === rule.id"
                      :aria-label="`启用规则 ${rule.name}`"
                      @change="onToggleRule(rule)"
                    />
                    <i></i>
                  </label>
                </td>

                <!-- 编辑 / 删除 -->
                <td class="shrink col-actions">
                  <div class="row" style="gap: 4px; flex-wrap: nowrap">
                    <button
                      type="button"
                      class="btn btn-sm btn-ghost"
                      :data-edit="rule.id"
                      @click="openEditRule(rule.id)"
                    >
                      编辑
                    </button>
                    <button
                      type="button"
                      class="btn btn-sm btn-ghost btn-ghost-danger"
                      :title="'删除规则「' + rule.name + '」'"
                      @click="handleDeleteRule(rule)"
                    >
                      删除
                    </button>
                  </div>
                </td>
              </tr>
            </template>
          </tbody>
        </table>
      </div>

      <!-- 表格底栏 -->
      <div class="panel-ft row-between flex-wrap gap-3">
        <div class="row tiny muted" style="gap: 12px">
          <span>共 <strong class="mono text-ink">{{ total }}</strong> 条规则</span>
          <span>已启用 <strong class="mono text-ink">{{ enabledCount }}</strong> 条</span>
          <span>24h 命中合计 <strong class="mono text-ink">{{ totalHits.toLocaleString() }}</strong></span>
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
            <span class="mono tiny muted px-1">{{ page }} / {{ totalPages }}</span>
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

    <!-- ==================== Tab 2: 规则编辑器 ==================== -->
    <section v-show="currentTab === 'editor'" data-tabpanel="editor" data-od-id="rule-editor">
      <div class="two-col">
        <!-- 左侧编辑区 -->
        <div class="stack">
          <!-- 面板 1：基本信息 -->
          <div class="panel">
            <div class="panel-hd">
              <div>
                <h2>{{ editor.id === null ? '新建规则' : '编辑规则' }}</h2>
                <p>左边定义条件，右边定义命中后做什么。</p>
              </div>
              <span class="badge badge-neutral mono">{{ editor.id === null ? '未保存' : editor.id }}</span>
            </div>
            <div class="panel-bd stack">
              <div class="form-grid">
                <div class="field span-2">
                  <label for="rName">规则名称</label>
                  <input
                    v-model="editor.name"
                    class="input"
                    id="rName"
                    maxlength="120"
                    placeholder="输入规则名称（同一租户内唯一）"
                  />
                </div>
                <div class="field span-2">
                  <label for="rDesc">说明（仅内部可见）</label>
                  <input
                    v-model="editor.description"
                    class="input"
                    id="rDesc"
                    maxlength="500"
                    placeholder="简述规则用途及适用受众"
                  />
                </div>
                <div class="field">
                  <label for="rPrio">优先级</label>
                  <input
                    v-model.number="editor.priority"
                    class="input mono"
                    id="rPrio"
                    type="number"
                    min="1"
                    max="9999"
                  />
                  <span class="hint">数值越小越先求值</span>
                </div>
                <div class="field">
                  <label>多条件关系</label>
                  <div class="segmented" id="modeSeg" style="align-self: start">
                    <button
                      type="button"
                      :aria-pressed="editor.logic === 'all'"
                      data-m="all"
                      @click="editor.logic = 'all'"
                    >
                      全部满足 (AND)
                    </button>
                    <button
                      type="button"
                      :aria-pressed="editor.logic === 'any'"
                      data-m="any"
                      @click="editor.logic = 'any'"
                    >
                      任一满足 (OR)
                    </button>
                  </div>
                  <span class="hint">决定多条条件之间是「与」还是「或」</span>
                </div>
                <div class="field span-2">
                  <label>规则状态</label>
                  <label class="row tiny" style="gap: 8px; cursor: pointer">
                    <span class="switch">
                      <input v-model="editor.enabled" type="checkbox" />
                      <i></i>
                    </span>
                    <span>{{ editor.enabled ? '已启用（会参与线上求值）' : '已停用（不参与求值）' }}</span>
                  </label>
                </div>
              </div>
            </div>
          </div>

          <!-- 面板 2：作用域(spec D1 / D2) -->
          <div class="panel">
            <div class="panel-hd">
              <div>
                <h2>作用域</h2>
                <p>决定这条规则作用在哪些短链上。规则侧是关联的唯一写入口。</p>
              </div>
            </div>
            <div class="panel-bd stack">
              <div class="segmented" id="scopeSeg" style="align-self: start">
                <button
                  type="button"
                  :aria-pressed="editor.scope === 'global'"
                  data-scope="global"
                  @click="setScope('global')"
                >
                  全局（对该租户所有短链生效）
                </button>
                <button
                  type="button"
                  :aria-pressed="editor.scope === 'links'"
                  data-scope="links"
                  @click="setScope('links')"
                >
                  指定短链
                </button>
              </div>

              <template v-if="editor.scope === 'links'">
                <div class="field">
                  <label for="scopeLinks">关联短链</label>
                  <AppSelect
                    v-model="editor.linkIds"
                    multiple
                    show-search
                    :options="linkOptions"
                    :max-tag-count="3"
                    :loading="linkCatalogLoading"
                    placeholder="选择本租户的短链（可多选）"
                  />
                  <span class="hint">
                    选项格式为「短码@域名」，共 {{ linkCatalogTotal }} 条短链，已加载
                    {{ linkCatalog.length }} 条
                    <button
                      v-if="linkCatalogHasMore"
                      type="button"
                      class="btn btn-sm btn-ghost"
                      style="margin-left: 6px"
                      :disabled="linkCatalogLoading"
                      @click="loadLinkCatalog(true)"
                    >
                      加载更多
                    </button>
                  </span>
                </div>

                <!-- spec D2:零关联的 scoped 规则不兜底、不静默,必须在界面上显式暴露 -->
                <AppAlert
                  v-if="editor.linkIds.length === 0"
                  type="warning"
                  title="未关联短链 · 不会命中"
                >
                  作用域为「指定短链」但未关联任何短链的规则<strong>永远不命中</strong>，也不会退化成全局规则。
                  请至少关联一条短链后再保存。
                </AppAlert>
                <AppAlert v-else type="info" :title="`已关联 ${editor.linkIds.length} 条短链`">
                  规则的关联写在 <span class="mono">rule_links</span> 上；短链表单的「适用规则」区改的是同一份数据。
                </AppAlert>
              </template>
              <AppAlert v-else type="info" title="全局规则">
                作用范围为该租户下的<strong>全部短链</strong>。全局规则无法在单条短链上关闭，需要收窄作用域请改为「指定短链」。
              </AppAlert>
            </div>
          </div>

          <!-- 面板 3：条件构造器 -->
          <div class="panel">
            <div class="panel-hd">
              <div>
                <h2>条件</h2>
                <p>
                  条件之间用
                  <span class="mono font-semibold">{{ editor.logic === 'all' ? 'AND' : 'OR' }}</span>
                  连接；取值支持逗号或换行分隔，一次可配多个。
                </p>
              </div>
              <button type="button" class="btn btn-sm" id="addCond" @click="addCondition">
                <Plus :size="13" />
                添加条件
              </button>
            </div>
            <div class="panel-bd stack">
              <div id="condHost" class="stack-sm" style="gap: 8px">
                <div v-for="(cond, cIndex) in editor.conditions" :key="cond.key" class="cond-grid" data-cond>
                  <!-- 13 个判定字段(spec D5) -->
                  <select
                    v-model="cond.field"
                    class="select"
                    data-f
                    aria-label="判定字段"
                    @change="onFieldChange(cond)"
                  >
                    <option
                      v-for="f in FIELD_OPTIONS"
                      :key="f.value"
                      :value="f.value"
                      :disabled="f.pending"
                    >
                      {{ f.label }}{{ f.pending ? '（数据源待接入 · 置灰）' : '' }}
                    </option>
                  </select>

                  <!-- 运算符(后端白名单) -->
                  <select v-model="cond.operator" class="select" aria-label="运算符">
                    <option v-for="op in OPERATOR_OPTIONS" :key="op.value" :value="op.value">
                      {{ op.label }}
                    </option>
                  </select>

                  <!-- 条件值 -->
                  <div class="relative flex items-center">
                    <input
                      v-model="cond.raw"
                      class="input mono"
                      :placeholder="fieldOption(cond.field)?.placeholder"
                      :title="fieldOption(cond.field)?.hint"
                      :aria-label="'条件值：' + (fieldOption(cond.field)?.label ?? cond.field)"
                    />
                  </div>

                  <!-- 删除条件 -->
                  <button
                    type="button"
                    class="icon-btn"
                    data-del
                    aria-label="删除条件"
                    :disabled="editor.conditions.length <= 1"
                    @click="removeCondition(cIndex)"
                  >
                    <X :size="14" />
                  </button>
                </div>
              </div>

              <!-- 字段取值提示 -->
              <p class="tiny muted">
                取值用逗号或换行分隔；鼠标悬停输入框可看该字段的取值示例。
                <span class="mono">ip</span> 支持 CIDR 网段，<span class="mono">in / not_in</span>
                支持多个取值。
              </p>

              <!-- 依赖未接入数据源的条件必须显式提示,否则租户会疑惑"为什么从不生效" -->
              <AppAlert
                v-if="pendingConditions.length > 0"
                type="warning"
                title="存在依赖未接入数据源的条件"
              >
                条件 {{ pendingConditions.map((c) => c.label).join('、') }} 依赖 GeoIP / ASN 数据源，
                该数据源接入前这些条件<strong>恒不命中</strong>。
              </AppAlert>

              <div class="note">
                <AlertTriangle :size="16" class="shrink-0 text-warn" />
                <div>
                  <b>取值只接受字面量。</b>
                  名单库（CIDR / CSV 集中引用）本次不接入，条件值请直接填 IP 段、枚举值或正则；
                  字段仅限下方 13 项，其余字段后端不识别，提交会被拒绝。
                </div>
              </div>
            </div>
          </div>

          <!-- 面板 4：命中动作 -->
          <div class="panel">
            <div class="panel-hd">
              <div>
                <h2>命中动作</h2>
                <p>动作决定这个访问者最终看到什么。首条命中即裁决，不做多规则叠加。</p>
              </div>
            </div>
            <div class="panel-bd stack">
              <div class="segmented" id="rAct" style="align-self: start; flex-wrap: wrap">
                <button
                  v-for="opt in ACTION_OPTIONS"
                  :key="opt.value"
                  type="button"
                  :data-a="opt.value"
                  :aria-pressed="editor.action === opt.value"
                  @click="editor.action = opt.value"
                >
                  {{ opt.label }}
                </button>
              </div>
              <p class="tiny muted">{{ currentActionDesc }}</p>

              <div v-if="editor.action === 'redirect'" class="field">
                <label for="rDest">改写目标 URL</label>
                <input
                  v-model="editor.destination"
                  class="input mono"
                  id="rDest"
                  maxlength="2048"
                  placeholder="https://example.com/safe-landing"
                />
                <span class="hint">
                  命中 redirect 时直接改写到该地址，<strong>不参与</strong>短链目标池的轮询。
                </span>
              </div>
            </div>
          </div>
        </div>

        <!-- 右侧：实时规则摘要 -->
        <div class="stack" style="position: sticky; top: 72px">
          <div class="panel">
            <div class="panel-hd">
              <div>
                <h2 style="font-size: 15px">规则摘要</h2>
                <p>人话版本，用于团队对齐。</p>
              </div>
            </div>
            <div class="panel-bd stack-sm">
              <div class="tiny muted">
                当访问者{{ editor.logic === 'all' ? '同时满足' : '任一满足' }}下列条件、且访问的是
                <b>{{ scopeSummaryText }}</b> 短链时：
              </div>

              <!-- 实时条件摘要 Chips -->
              <div class="chipwrap" id="sumChips">
                <span v-for="chip in summaryChips" :key="chip.key" class="chip">
                  {{ chip.text }}
                </span>
                <span v-if="summaryChips.length === 0" class="tiny muted">尚未添加有效条件</span>
              </div>

              <div class="tiny muted" style="margin-top: 8px">则：</div>
              <div class="row" style="gap: 6px; flex-wrap: wrap">
                <span :class="['badge', getActionBadgeClass(editor.action)]">
                  {{ actionLabel(editor.action) }}
                </span>
                <span v-if="editor.action === 'redirect'" class="mono tiny break-all">
                  → {{ editor.destination || '未填写目标 URL' }}
                </span>
                <span v-else class="mono tiny muted">→ 短链自身流程</span>
              </div>

              <hr style="margin: 8px 0; border: 0; border-top: 1px solid var(--border)" />

              <dl class="kv">
                <dt>作用域</dt>
                <dd>{{ editor.scope === 'global' ? '全局' : editor.linkIds.length + ' 条短链' }}</dd>
                <dt>条件条数</dt>
                <dd>{{ editor.conditions.length }} 条</dd>
                <dt>状态</dt>
                <dd>{{ editor.enabled ? '已启用' : '已停用' }}</dd>
                <dt v-if="editor.id !== null">24h 命中</dt>
                <dd v-if="editor.id !== null">{{ (currentRule?.hits24h ?? 0).toLocaleString() }}</dd>
                <dt>更新时间</dt>
                <dd>{{ currentRule ? formatDateTime(currentRule.updatedAt) : '—' }}</dd>
              </dl>
            </div>
            <div class="panel-ft stack-sm">
              <button
                type="button"
                class="btn btn-sm w-full font-medium"
                :class="isSavedRecently ? 'btn' : 'btn-primary'"
                id="saveRuleTop"
                :disabled="saving"
                @click="saveCurrentRule"
              >
                {{ isSavedRecently ? '已保存 · 生效中' : saving ? '保存中…' : '保存并生效' }}
              </button>
              <div class="row" style="gap: 8px">
                <button
                  type="button"
                  class="btn btn-sm grow"
                  data-goto="simulator"
                  @click="previewInSimulator"
                >
                  在模拟器中预览
                </button>
                <button
                  v-if="editor.id !== null"
                  type="button"
                  class="btn btn-sm btn-ghost btn-ghost-danger"
                  @click="handleDeleteEditorRule"
                >
                  删除规则
                </button>
              </div>
            </div>
          </div>
        </div>
      </div>
    </section>

    <!-- ==================== Tab 3: 规则模拟器(前端预览) ==================== -->
    <section v-show="currentTab === 'simulator'" data-tabpanel="simulator" data-od-id="rule-simulator">
      <div class="panel" style="margin-bottom: 14px">
        <div class="panel-bd">
          <AppAlert type="info" title="这是前端求值预览，不是服务端执行结果">
            模拟器在本页用与后端相同的字段、运算符白名单与「首命中即裁决」口径做等价求值，
            用于保存前自查；线上真实结果以服务端执行日志为准。评估耗时与命中计数不会写回。
            <br />
            预览范围为规则列表<strong>当前页</strong>（第 {{ page }} 页 · 每页 {{ pageSize }} 条）的已启用规则；
            列表接口不回传条件，运行时会按需拉取这些规则的详情。
          </AppAlert>
        </div>
      </div>

      <div class="two-col-rev">
        <!-- 左侧：输入访客 -->
        <div class="panel">
          <div class="panel-hd">
            <div>
              <h2>输入访客</h2>
              <p>粘贴一条真实请求，或手填字段。不会产生任何真实访问。</p>
            </div>
          </div>
          <div class="panel-bd stack">
            <div class="field">
              <label for="simUrl">完整请求 URL</label>
              <input
                v-model="simInput.url"
                class="input mono"
                id="simUrl"
                placeholder="https://example.com/promo"
              />
              <span class="hint">URL 用于解析请求路径、Host 与 utm_source；短码能命中本租户短链时按该短链的实际适用范围求值。</span>
            </div>
            <div class="field">
              <label for="simIp">来访 IP</label>
              <input v-model="simInput.ip" class="input mono" id="simIp" placeholder="例如 1.2.3.4" />
            </div>
            <div class="field">
              <label for="simUa">User-Agent</label>
              <textarea
                v-model="simInput.ua"
                class="textarea"
                id="simUa"
                rows="3"
                placeholder="Mozilla/5.0..."
              ></textarea>
            </div>
            <div class="field">
              <label for="simLang">Accept-Language</label>
              <input
                v-model="simInput.lang"
                class="input mono"
                id="simLang"
                placeholder="pt-BR,pt;q=0.9,en-US;q=0.8"
              />
            </div>
            <div class="field">
              <label for="simRef">Referer</label>
              <input
                v-model="simInput.ref"
                class="input mono"
                id="simRef"
                placeholder="https://www.facebook.com/..."
              />
            </div>
            <div class="row flex-wrap" style="gap: 8px; margin-top: 6px">
              <button type="button" class="btn btn-primary" id="runSim" :disabled="isSimulating" @click="runSimulation">
                <Play :size="14" class="fill-current" />
                {{ isSimulating ? '求值中…' : '运行模拟' }}
              </button>
              <button type="button" class="btn btn-sm" id="loadSample" @click="loadSampleBot">
                载入示例：爬虫
              </button>
              <button type="button" class="btn btn-sm" id="loadSample2" @click="loadSampleDatacenter">
                载入示例：机房 IP
              </button>
              <button type="button" class="btn btn-sm" id="loadSample3" @click="loadSampleMobile">
                载入示例：正常海外移动
              </button>
              <button
                v-if="previewRuleId !== null"
                type="button"
                class="btn btn-sm btn-ghost"
                @click="previewRuleId = null"
              >
                退出单规则预览
              </button>
            </div>
          </div>
        </div>

        <!-- 右侧：访客画像与决策链 -->
        <div class="panel">
          <div class="panel-hd">
            <div>
              <h2>解析出的访客画像</h2>
              <p>系统从请求中解析出的可判定字段，规则即基于这些字段求值。</p>
            </div>
            <span class="badge badge-neutral mono">13 字段</span>
          </div>
          <div class="panel-bd" style="padding-bottom: 8px">
            <div class="cols-3" style="gap: 12px">
              <div v-for="f in visitorFieldViews" :key="f.field">
                <div class="mono micro muted">{{ f.label }}</div>
                <div class="mono tiny font-semibold text-ink truncate" :title="f.value">
                  {{ f.value }}
                </div>
                <div v-if="f.note" class="tiny" :class="f.pending ? 'text-warn' : 'muted'">
                  {{ f.note }}
                </div>
              </div>
            </div>
          </div>

          <div class="panel-hd" style="border-top: 1px solid var(--border)">
            <div>
              <h2>决策链</h2>
              <p>{{ simScopeNote || '逐条规则求值过程，可直接用于排查「为什么这个 IP 被拦」。' }}</p>
            </div>
            <span :class="['badge mono font-semibold', simVerdict.badgeClass]" id="simVerdict">
              {{ simVerdict.title }}
            </span>
          </div>

          <div class="panel-bd">
            <div
              v-if="simTraceSteps.length === 0"
              class="empty py-8 text-center text-xs text-muted"
            >
              请在左侧输入访客参数并点击「运行模拟」，系统将按当前已启用的规则链路生成求值推演。
            </div>
            <div v-else class="trace" id="simTrace">
              <div v-for="step in simTraceSteps" :key="step.key" :class="['trace-step', step.status]">
                <div class="trace-rail">
                  <span class="trace-node"></span>
                </div>
                <div>
                  <div class="trace-name flex items-center justify-between">
                    <span>
                      <span class="mono">{{ step.ruleId }}</span> {{ step.ruleName }}
                      <span :class="['badge mono ml-1.5', step.badgeClass]">{{ step.statusText }}</span>
                    </span>
                  </div>

                  <div class="trace-why">
                    <template v-if="step.facts && step.facts.length > 0">
                      <span
                        v-for="(f, fIdx) in step.facts"
                        :key="fIdx"
                        :class="['fact', f.hit ? 'hit' : 'miss']"
                      >
                        {{ f.text }}
                      </span>
                    </template>
                    <span v-if="step.whyText" class="ml-1 text-[12.5px] text-muted">{{ step.whyText }}</span>
                  </div>
                </div>
              </div>
            </div>

            <!-- 裁决结果卡片 -->
            <div class="mt-4 rounded-[var(--r)] border p-3.5" :style="simVerdict.cardStyle">
              <div class="mono micro font-semibold" :style="{ color: simVerdict.accentColor }">裁决结果</div>
              <div class="mt-1 text-[15px] font-[640] text-ink">{{ simVerdict.actionText }}</div>
              <p class="tiny muted mt-1 break-all font-mono">{{ simVerdict.detailText }}</p>
            </div>
          </div>
        </div>
      </div>
    </section>

    <!-- ==================== Tab 4: 名单库(未接入) ==================== -->
    <section v-show="currentTab === 'lists'" data-tabpanel="lists" id="lists" data-od-id="ip-lists">
      <div class="panel">
        <div class="panel-bd">
          <AppEmpty description="名单库未接入，本次不提供任何名单的创建、导入与编辑能力" />
          <div class="stack" style="gap: 10px">
            <div class="note">
              <AlertTriangle :size="16" class="shrink-0 text-warn" />
              <div>
                <b>为什么置灰。</b>
                名单库（CIDR / CSV 集中管理、规则中按编号引用）需要独立的表、导入解析与条目 CRUD，
                不在「规则与短链关联」本次范围内。选项卡保留仅用于占位说明，<strong>不做半成品交互</strong>，
                以免出现「能点但存不住」的假能力。
              </div>
            </div>
            <div class="tiny muted">
              当前可行的替代做法：把 CIDR 列表、逗号分隔枚举直接写进条件的取值里
              （<span class="mono">ip</span> 字段支持 CIDR，
              <span class="mono">devtype</span> / <span class="mono">os</span> / <span class="mono">browser</span>
              等枚举字段支持多个取值）。名单库接入后，这些取值可平滑改为引用名单编号。
            </div>
          </div>
        </div>
      </div>
    </section>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue';
import { useRoute, useRouter } from 'vue-router';
import { AlertTriangle, Play, Plus, RefreshCw, X } from '@lucide/vue';
import { UAParser } from 'ua-parser-js';

import { listLinks } from '@/api/links';
import {
  createRule,
  deleteRule,
  getRule,
  listLinkRules,
  listRules,
  updateRule,
} from '@/api/rules';
import type { RuleCreatePayload } from '@/api/rules';
import AppAlert from '@/components/ui/AppAlert.vue';
import AppEmpty from '@/components/ui/AppEmpty.vue';
import AppSelect from '@/components/ui/AppSelect.vue';
import { confirm } from '@/components/ui/confirm';
import { ApiError } from '@/types/api';
import type {
  Link,
  LinkRule,
  Rule,
  RuleAction,
  RuleCondition,
  RuleField,
  RuleLogic,
  RuleOperator,
  RuleScope,
} from '@/types/api';
import { formatDateTime } from '@/utils/format';
import { message } from '@/utils/toast';

// ==================== 枚举与字段定义 ====================
type TabType = 'rules' | 'editor' | 'simulator' | 'lists';

const TAB_STORAGE_KEY = 'cloak.re.tab';

interface FieldOption {
  value: RuleField;
  label: string;
  defaultOp: RuleOperator;
  placeholder: string;
  /** 取值示例,展示在条件区提示里 */
  hint?: string;
  /** 数据源待接入:后端恒取不到值,置灰不可选(spec D5) */
  pending?: boolean;
  /** 接入后的数据来源,写进代码注释便于将来接回 */
  source: string;
}

/**
 * 条件判定字段(spec D5):收敛到后端从请求即可真实求值的 13 个。
 * 原型里的 18 个字段里,region / city / tz / screen / tls(JA3) / canvas / cookie
 * 后端当前没有任何数据源,已移出 v1 —— 不把不可求值的字段落库,否则就是「能配不能跑」的假能力。
 * country / asn 保留占位:依赖 GeoIP / ASN mmdb,接入前恒空、恒不命中,UI 上置灰并显式说明。
 */
const FIELD_OPTIONS: FieldOption[] = [
  { value: 'ip', label: 'IP 地址 / CIDR', defaultOp: 'in', placeholder: '198.51.100.0/24, 203.0.113.7', hint: '支持 CIDR 网段与单个 IP', source: 'X-Forwarded-For / RemoteAddr' },
  { value: 'ipattr', label: 'IP 属性', defaultOp: 'in', placeholder: 'private, loopback, linklocal', hint: 'private / loopback / linklocal', source: 'net.IP 判定' },
  { value: 'country', label: '国家 / 地区', defaultOp: 'in', placeholder: 'US, CN', hint: 'ISO 国家码', pending: true, source: 'visits.country（GeoIP 接入前恒空）' },
  { value: 'asn', label: 'ASN / 运营商', defaultOp: 'in', placeholder: 'AS15169, AS16509', hint: 'AS 号', pending: true, source: 'visits.asn（ASN mmdb 接入前恒空）' },
  { value: 'lang', label: '语言 (Accept-Language)', defaultOp: 'in', placeholder: 'zh-CN, pt-BR', hint: '取首个语言标签', source: 'Accept-Language 首标签' },
  { value: 'ref', label: 'Referrer 主机名', defaultOp: 'in', placeholder: 'facebook.com, google.com', hint: '取主机名，不含协议与路径', source: 'Referer 主机名' },
  { value: 'utm', label: 'UTM 来源', defaultOp: 'eq', placeholder: 'wechat, google', hint: '取 utm_source 查询参数', source: 'utm_source 查询参数' },
  { value: 'ua', label: 'User-Agent', defaultOp: 'contains', placeholder: 'bot, spider, crawler', hint: 'User-Agent 原串', source: 'User-Agent 原串' },
  { value: 'devtype', label: '设备类型', defaultOp: 'in', placeholder: 'bot, mobile, tablet, desktop', hint: 'bot / mobile / tablet / desktop', source: 'UA 轻量判定' },
  { value: 'os', label: '操作系统', defaultOp: 'in', placeholder: 'iOS, Android, Windows, macOS, Linux', hint: 'iOS / Android / Windows / macOS / Linux / 其他', source: 'UA 轻量判定' },
  { value: 'browser', label: '浏览器', defaultOp: 'in', placeholder: 'Chrome, Safari, Firefox, Edge', hint: 'Chrome / Safari / Firefox / Edge / 其他', source: 'UA 轻量判定' },
  { value: 'path', label: '请求路径', defaultOp: 'in', placeholder: '/promo, /black-friday', hint: '形如 /abc 的短码路径', source: '请求路径' },
  { value: 'domain', label: '请求域名 (Host)', defaultOp: 'in', placeholder: 'go.example.com', hint: '访问所用的域名', source: '请求 Host' },
];

/**
 * 运算符选项：后端 Evaluate 白名单的**可放出子集**。
 *
 * 刻意不含 `duplicated`：后端白名单接受它、求值也已接线到 Fact.Seen 通道，
 * 但平台没有指纹/计数器数据源，Fact.Seen 恒为 nil → 该运算符恒不命中。
 * 留着就是「能配不能跑」的假能力，而它在每条条件的下拉里都出现，
 * 一个永远点不亮的死选项比没有更糟：后端接线已完成，
 * **待接入访问计数数据源（滑动窗口）后再放出**。
 * 类型 RuleOperator 仍保留该值，用于读回经 API 写入的历史数据。
 */
const OPERATOR_OPTIONS: { value: RuleOperator; label: string; hint: string }[] = [
  { value: 'in', label: '属于', hint: '访客值落在任一取值内（ip 字段按 CIDR / 字面量匹配）' },
  { value: 'not_in', label: '不属于', hint: 'in 的反面：任一取值命中即不成立' },
  { value: 'eq', label: '等于', hint: '与取值完全相等（忽略大小写）' },
  { value: 'neq', label: '不等于', hint: '与取值均不相等' },
  { value: 'contains', label: '包含', hint: '访客值包含任一取值' },
  { value: 'not_contains', label: '不包含', hint: '不包含任一取值' },
  { value: 'gt', label: '大于', hint: '按数值比较，非数值恒不命中' },
  { value: 'lt', label: '小于', hint: '按数值比较，非数值恒不命中' },
  { value: 'regex', label: '正则匹配', hint: '后端为 RE2 语法且大小写敏感（需忽略大小写请在表达式里写 (?i)）；复杂表达式的前端预览结果可能与线上略有差异' },
];

/** 命中动作:收敛为四项(spec D4 裁决顺序) */
const ACTION_OPTIONS: { value: RuleAction; label: string; desc: string }[] = [
  { value: 'pass', label: '放行', desc: '记录命中后继续走短链自身的跳转 / 落地页流程（不改写目标）。' },
  { value: 'redirect', label: '重定向到指定 URL', desc: '把访问改写到下方填写的目标 URL；该地址不参与短链目标池轮询。' },
  { value: 'notfound', label: '直接 404', desc: '视为未命中短链，记 outcome=failed、reason=rule_blocked。' },
  { value: 'throttle', label: '限流 429', desc: '记 outcome=failed、reason=rule_throttled，不计入访问量。' },
];

const LOGIC_OPTIONS: { value: RuleLogic; label: string }[] = [
  { value: 'all', label: '全部满足' },
  { value: 'any', label: '任一满足' },
];

function actionLabel(action: RuleAction): string {
  return ACTION_OPTIONS.find((o) => o.value === action)?.label ?? action;
}

function logicLabel(logic: RuleLogic): string {
  return LOGIC_OPTIONS.find((o) => o.value === logic)?.label ?? logic;
}

function getActionBadgeClass(action: RuleAction): string {
  switch (action) {
    case 'pass':
      return 'badge-ok';
    case 'redirect':
      return 'badge-neutral';
    case 'notfound':
      return 'badge-danger';
    case 'throttle':
      return 'badge-warn';
    default:
      return 'badge-neutral';
  }
}

function fieldOption(field: string): FieldOption | undefined {
  return FIELD_OPTIONS.find((f) => f.value === field);
}

function operatorLabel(operator: string): string {
  return OPERATOR_OPTIONS.find((o) => o.value === operator)?.label ?? operator;
}

// ==================== 本地数据类型 ====================
/** 编辑器中的单条条件:value 以原始串录入,保存时拆成 values 数组 */
interface EditableCondition {
  key: string;
  field: string;
  operator: string;
  raw: string;
}

interface EditorState {
  /** null = 新建 */
  id: number | null;
  name: string;
  description: string;
  priority: number;
  scope: RuleScope;
  enabled: boolean;
  logic: RuleLogic;
  action: RuleAction;
  destination: string;
  linkIds: number[];
  conditions: EditableCondition[];
}

interface TraceFact {
  text: string;
  hit: boolean;
}

interface TraceStep {
  key: string;
  ruleId: number;
  ruleName: string;
  status: 'hit' | 'block' | 'skip';
  statusText: string;
  badgeClass: string;
  facts: TraceFact[];
  whyText: string;
}

let keySeq = 0;
function nextKey(prefix: string): string {
  keySeq += 1;
  return `${prefix}-${keySeq}`;
}

// ==================== 路由与选项卡状态 ====================
const route = useRoute();
const router = useRouter();

const currentTab = ref<TabType>('rules');

function switchTab(tab: TabType) {
  currentTab.value = tab;
  try {
    localStorage.setItem(TAB_STORAGE_KEY, tab);
  } catch {
    // localStorage 不可用时忽略:仅影响选项卡记忆
  }
  router.replace({
    query: { ...route.query, tab },
    hash: `#${tab}`,
  });
}

function syncTabFromRoute() {
  const qTab = route.query.tab as TabType | undefined;
  const hash = route.hash;
  const valid: TabType[] = ['rules', 'editor', 'simulator', 'lists'];

  if (qTab && valid.includes(qTab)) {
    currentTab.value = qTab;
    return;
  }
  if (hash === '#lists' || hash === '#editor' || hash === '#simulator' || hash === '#rules') {
    currentTab.value = hash.slice(1) as TabType;
    return;
  }
  try {
    const saved = localStorage.getItem(TAB_STORAGE_KEY) as TabType | null;
    if (saved && valid.includes(saved)) {
      currentTab.value = saved;
      return;
    }
  } catch {
    // 忽略存储异常
  }
  currentTab.value = 'rules';
}

watch(
  () => [route.query.tab, route.hash],
  () => {
    syncTabFromRoute();
  },
);

// ==================== 规则列表 ====================
const rules = ref<Rule[]>([]);
const total = ref(0);
const page = ref(1);
const pageSize = ref(20);
const loading = ref(false);
const togglingId = ref<number | null>(null);

const actionFilters: { key: string; label: string }[] = [
  { key: 'all', label: '全部' },
  ...ACTION_OPTIONS.map((o) => ({ key: o.value, label: o.label })),
];

const selectedActFilter = ref('all');
const searchKeyword = ref('');
const statusFilter = ref<'all' | 'on' | 'off'>('all');

const totalPages = computed(() => Math.max(1, Math.ceil(total.value / pageSize.value)));
const enabledCount = computed(() => rules.value.filter((r) => r.enabled).length);
const totalHits = computed(() => rules.value.reduce((acc, r) => acc + (r.hits24h || 0), 0));

const filteredRules = computed(() =>
  rules.value.filter((r) => {
    if (selectedActFilter.value !== 'all' && r.action !== selectedActFilter.value) return false;
    if (statusFilter.value === 'on' && !r.enabled) return false;
    if (statusFilter.value === 'off' && r.enabled) return false;

    const q = searchKeyword.value.trim().toLowerCase();
    if (!q) return true;
    const haystack = [
      String(r.id),
      r.name,
      r.description,
      actionLabel(r.action),
      r.destination,
      (r.linkNames || []).join(' '),
      conditionSummary(r),
    ]
      .join(' ')
      .toLowerCase();
    return haystack.includes(q);
  }),
);

function resetFilters() {
  selectedActFilter.value = 'all';
  searchKeyword.value = '';
  statusFilter.value = 'all';
}

async function loadRules() {
  loading.value = true;
  try {
    const res = await listRules({ page: page.value, pageSize: pageSize.value });
    rules.value = res.items;
    total.value = res.total;
    if (rules.value.length === 0 && page.value > 1) {
      page.value -= 1;
      return loadRules();
    }
  } catch (error) {
    if (error instanceof ApiError) message.error(error.message);
    else message.error('加载规则失败,请稍后重试');
    rules.value = [];
    total.value = 0;
  } finally {
    loading.value = false;
  }
}

function goToPage(p: number) {
  if (p < 1 || p > totalPages.value || p === page.value) return;
  page.value = p;
  loadRules();
}

function onPageSizeChange() {
  page.value = 1;
  loadRules();
}

/** 列表接口不回传 conditions,没有条件就不编造摘要 */
function conditionSummary(rule: Rule): string {
  if (!rule.conditions || rule.conditions.length === 0) return '—';
  return rule.conditions.map(describeCondition).join(' · ');
}

function describeCondition(cond: RuleCondition): string {
  const label = fieldOption(cond.field)?.label ?? cond.field;
  const op = operatorLabel(cond.operator);
  const values = (cond.values || []).join(' / ') || '（空）';
  return `${label} ${op} ${values}`;
}

async function onToggleRule(rule: Rule) {
  if (togglingId.value === rule.id) return;
  const next = !rule.enabled;
  togglingId.value = rule.id;
  try {
    const updated = await updateRule(rule.id, { enabled: next });
    rule.enabled = updated.enabled;
    message.success(`规则「${rule.name}」已${next ? '启用' : '停用'}`);
  } catch (error) {
    if (error instanceof ApiError) message.error(error.message);
    else message.error('更新规则状态失败,请稍后重试');
  } finally {
    togglingId.value = null;
  }
}

function handleDeleteRule(rule: Rule) {
  confirm({
    title: `删除规则「${rule.name}」?`,
    content:
      '删除后该规则不再参与求值，其与短链的关联一并移除；历史访问明细保留（规则标识置空）。此操作不可撤销。',
    okText: '确认删除',
    cancelText: '取消',
    danger: true,
    onOk: async () => {
      try {
        await deleteRule(rule.id);
        message.success(`规则「${rule.name}」已删除`);
        if (editor.value.id === rule.id) resetEditor();
        if (previewRuleId.value === rule.id) previewRuleId.value = null;
        await loadRules();
      } catch (error) {
        if (error instanceof ApiError) message.error(error.message);
        else message.error('删除规则失败,请稍后重试');
      }
    },
  });
}

// ==================== 编辑器状态 ====================
function createEditorState(): EditorState {
  return {
    id: null,
    name: '',
    description: '',
    priority: 100,
    scope: 'global',
    enabled: true,
    logic: 'all',
    action: 'pass',
    destination: '',
    linkIds: [],
    conditions: [
      { key: nextKey('cond'), field: 'ip', operator: 'in', raw: '' },
    ],
  };
}

const editor = ref<EditorState>(createEditorState());
const saving = ref(false);
const isSavedRecently = ref(false);

const currentRule = computed(() => rules.value.find((r) => r.id === editor.value.id) ?? null);

const currentActionDesc = computed(
  () => ACTION_OPTIONS.find((o) => o.value === editor.value.action)?.desc ?? '',
);

/** 依赖未接入数据源的条件(UI 必须显式提示,否则租户会疑惑"为什么从不生效") */
const pendingConditions = computed(() =>
  editor.value.conditions
    .filter((c) => fieldOption(c.field)?.pending)
    .map((c) => ({ label: fieldOption(c.field)?.label ?? c.field })),
);

const scopeSummaryText = computed(() =>
  editor.value.scope === 'global'
    ? '本租户的全部'
    : `所关联的 ${editor.value.linkIds.length} 条`,
);

const summaryChips = computed(() =>
  editor.value.conditions.map((cond) => {
    const opt = fieldOption(cond.field);
    const raw = cond.raw.trim();
    return {
      key: cond.key,
      text: `${opt?.label ?? cond.field} ${operatorLabel(cond.operator)} ${raw || '（空）'}`,
    };
  }),
);

function setScope(scope: RuleScope) {
  // 保留已选的 linkIds：切回「指定短链」时不用重选；真正保存为 global 时会提交空数组清理关联
  editor.value.scope = scope;
}

function onFieldChange(cond: EditableCondition) {
  const opt = fieldOption(cond.field);
  if (opt) cond.operator = opt.defaultOp;
}

function addCondition() {
  editor.value.conditions.push({
    key: nextKey('cond'),
    field: 'ip',
    operator: 'in',
    raw: '',
  });
}

function removeCondition(index: number) {
  if (editor.value.conditions.length <= 1) return;
  editor.value.conditions.splice(index, 1);
}

/** 把录入串拆成后端的 values 数组;同时做字段 / 运算符白名单校验 */
function buildConditions(): RuleCondition[] {
  return editor.value.conditions.map((cond, idx) => {
    const opt = fieldOption(cond.field);
    if (!opt) {
      throw new Error(`第 ${idx + 1} 条条件的字段「${cond.field}」不在后端支持的 13 个字段内`);
    }
    if (opt.pending) {
      throw new Error(`第 ${idx + 1} 条条件使用了「${opt.label}」,该字段数据源尚未接入,无法保存`);
    }
    const op = OPERATOR_OPTIONS.find((o) => o.value === cond.operator);
    if (!op) {
      // 多为经 API 直写入的 duplicated：后端恒不命中，不在当前版本的可选范围里
      throw new Error(
        `第 ${idx + 1} 条条件的运算符「${cond.operator}」当前版本不可选（恒不命中，未接入数据源），请改为其他运算符`,
      );
    }
    const values = cond.raw
      .split(/[\n,;]+/)
      .map((s) => s.trim())
      .filter(Boolean);
    if (values.length === 0) {
      throw new Error(`第 ${idx + 1} 条条件（「${opt.label}」）的取值不能为空`);
    }
    return { field: opt.value, operator: op.value, values };
  });
}

/**
 * 回填已关联短链：直接用后端返回的完整 linkIds。
 * 不用短码反推——短码在租户内不唯一(唯一性是「同一域名下短码不重复」)，
 * 同一短码可以属于不同短链，按短码猜出来的关联天然歧义。
 */
function preselectLinkIds(rule: Rule): number[] {
  return (rule.linkIds || []).slice();
}

function applyRuleToEditor(rule: Rule): void {
  editor.value = {
    id: rule.id,
    name: rule.name,
    description: rule.description,
    priority: rule.priority,
    scope: rule.scope,
    enabled: rule.enabled,
    logic: rule.logic,
    action: rule.action,
    destination: rule.destination,
    linkIds: preselectLinkIds(rule),
    conditions: (rule.conditions && rule.conditions.length > 0
      ? rule.conditions
      : [{ field: 'ip', operator: 'in', values: [] }]
    ).map((c) => ({
      key: nextKey('cond'),
      field: c.field,
      operator: c.operator,
      raw: (c.values || []).join(', '),
    })),
  };
}

function resetEditor() {
  editor.value = createEditorState();
}

function openCreateRule() {
  resetEditor();
  switchTab('editor');
}

async function openEditRule(id: number) {
  loading.value = true;
  try {
    // 列表接口不含 conditions 与完整 linkIds,必须取详情;
    // 短链目录用于关联多选的选项,后台加载即可(不必阻塞进编辑器)
    if (linkCatalog.value.length === 0) {
      void loadLinkCatalog();
    }
    const detail = await getRule(id);
    applyRuleToEditor(detail);
    switchTab('editor');
  } catch (error) {
    if (error instanceof ApiError) message.error(error.message);
    else message.error('加载规则详情失败,请稍后重试');
  } finally {
    loading.value = false;
  }
}

function handleDeleteEditorRule() {
  const id = editor.value.id;
  if (id === null) return;
  const rule = rules.value.find((r) => r.id === id);
  if (rule) {
    handleDeleteRule(rule);
    return;
  }
  // 规则不在当前页（列表分页）：仍可按 id 删除，但拿不到行数据
  confirm({
    title: `删除规则 #${id}?`,
    content: '该规则不在当前页，无法展示名称。删除后不再参与求值，其与短链的关联一并移除。',
    okText: '确认删除',
    cancelText: '取消',
    danger: true,
    onOk: async () => {
      try {
        await deleteRule(id);
        message.success(`规则 #${id} 已删除`);
        resetEditor();
        await loadRules();
      } catch (error) {
        if (error instanceof ApiError) message.error(error.message);
        else message.error('删除规则失败,请稍后重试');
      }
    },
  });
}

function saveCurrentRule() {
  const name = editor.value.name.trim();
  if (!name) {
    message.error('请填写规则名称');
    return;
  }
  if (editor.value.action === 'redirect' && !editor.value.destination.trim()) {
    message.error('动作为「重定向到指定 URL」时必须填写改写目标');
    return;
  }

  let conditions: RuleCondition[];
  try {
    conditions = buildConditions();
  } catch (error) {
    message.error(error instanceof Error ? error.message : '条件配置有误,请检查');
    return;
  }

  // 编辑器与后端状态精确一致(关联直接回填自 rule.linkIds)，因此始终提交完整的 linkIds：
  // 传即整体替换，不传则不动关联；scope=global 传空数组，与后端“切回全局即清空关联”的语义一致。
  const linkIds = editor.value.scope === 'links' ? [...editor.value.linkIds] : [];
  const payload: RuleCreatePayload = {
    name,
    description: editor.value.description.trim(),
    priority: Math.trunc(editor.value.priority) || 100,
    scope: editor.value.scope,
    enabled: editor.value.enabled,
    logic: editor.value.logic,
    action: editor.value.action,
    destination: editor.value.action === 'redirect' ? editor.value.destination.trim() : '',
    conditions,
    linkIds,
  };

  // spec D2:零关联的 scoped 规则永远不命中,保存前必须让租户明确知道自己在做什么
  if (payload.scope === 'links' && linkIds.length === 0) {
    confirm({
      title: '该规则未关联任何短链,保存后不会命中',
      content: '作用域为「指定短链」但关联数为 0 的规则永远不会命中,也不会退化成全局规则。确定要保存吗?',
      okText: '仍然保存',
      cancelText: '去关联短链',
      onOk: () => doSave(payload, linkIds),
    });
    return;
  }

  void doSave(payload, linkIds);
}

async function doSave(payload: RuleCreatePayload, linkIds: number[]) {
  saving.value = true;
  try {
    if (editor.value.id === null) {
      const created = await createRule(payload);
      applyRuleToEditor(created);
      message.success(`规则「${created.name}」已创建并生效`);
    } else {
      const updated = await updateRule(editor.value.id, payload);
      // 详情里的 conditions 已归一化,避免下次进编辑器读到被规整过的结构
      const merged: Rule = { ...updated, conditions: payload.conditions };
      applyRuleToEditor(merged);
      message.success(`规则「${updated.name}」已保存并生效`);
    }
    // 关联以本次提交被接受的内容为准，避免写入响应未回传 linkIds 时编辑器瞬间变回“未关联”
    editor.value.linkIds = [...linkIds];
    isSavedRecently.value = true;
    setTimeout(() => {
      isSavedRecently.value = false;
    }, 2200);
    await loadRules();
  } catch (error) {
    if (error instanceof ApiError) message.error(error.message);
    else message.error('保存规则失败,请稍后重试');
  } finally {
    saving.value = false;
  }
}

function previewInSimulator() {
  previewRuleId.value = editor.value.id;
  switchTab('simulator');
}

// ==================== 短链目录(作用域选择器) ====================
const linkCatalog = ref<Link[]>([]);
const linkCatalogPage = ref(0);
const linkCatalogTotal = ref(0);
const linkCatalogLoading = ref(false);
const LINK_CATALOG_PAGE_SIZE = 50;

const linkCatalogHasMore = computed(() => linkCatalog.value.length < linkCatalogTotal.value);

const linkOptions = computed(() =>
  linkCatalog.value.map((l) => ({
    value: l.id,
    // 与后端 linkNames 的 `短码@域名` 保持同一形态(短码在租户内不唯一,必须带域名)
    label: `${l.code}@${l.domains[0] ?? '未关联域名'}`,
  })),
);

async function loadLinkCatalog(append = false) {
  if (linkCatalogLoading.value) return;
  linkCatalogLoading.value = true;
  try {
    const nextPage = append ? linkCatalogPage.value + 1 : 1;
    const res = await listLinks({ page: nextPage, pageSize: LINK_CATALOG_PAGE_SIZE });
    linkCatalogTotal.value = res.total;
    linkCatalogPage.value = nextPage;
    linkCatalog.value = append ? [...linkCatalog.value, ...res.items] : res.items;
  } catch (error) {
    if (error instanceof ApiError) message.error(error.message);
    else message.error('加载短链列表失败,请稍后重试');
  } finally {
    linkCatalogLoading.value = false;
  }
}

// 编辑器首次打开时拉短链目录
watch(currentTab, (tab) => {
  if (tab === 'editor' && linkCatalog.value.length === 0) {
    void loadLinkCatalog();
  }
});

// ==================== 规则模拟器(前端等价求值,仅预览) ====================
const isSimulating = ref(false);
const previewRuleId = ref<number | null>(null);

const simInput = ref({
  url: typeof window !== 'undefined' ? `${window.location.origin}/promo` : 'https://example.com/promo',
  ip: '',
  ua: typeof navigator !== 'undefined' ? navigator.userAgent : '',
  lang: typeof navigator !== 'undefined' ? navigator.language : '',
  ref: '',
});

/** 从请求中解析出的 13 个可判定字段 */
interface VisitorFacts {
  ip: string;
  ipattr: string;
  country: string;
  asn: string;
  lang: string;
  ref: string;
  utm: string;
  ua: string;
  devtype: string;
  os: string;
  browser: string;
  path: string;
  domain: string;
}

function classifyIpAttr(ip: string): string {
  const parts = ip.trim().split('.');
  if (parts.length !== 4) return '';
  const nums = parts.map((p) => Number(p));
  if (nums.some((n) => !Number.isInteger(n) || n < 0 || n > 255)) return '';
  const [a, b] = nums;
  if (a === 127) return 'loopback';
  if (a === 10) return 'private';
  if (a === 192 && b === 168) return 'private';
  if (a === 172 && b >= 16 && b <= 31) return 'private';
  if (a === 169 && b === 254) return 'linklocal';
  return '';
}

/** UA 轻量判定,输出与后端一致的取值词表 */
function detectDevice(ua: string): { devtype: string; os: string; browser: string } {
  const lower = ua.toLowerCase();
  const parsed = new UAParser(ua);
  const osRaw = (parsed.getOS().name || '').toLowerCase();
  const browserRaw = (parsed.getBrowser().name || '').toLowerCase();
  const type = parsed.getDevice().type;

  const isBot = /bot|crawl|spider|slurp|facebookexternalhit|curl|wget|python|headless|scrapy|monitor|probe|scan/.test(lower);
  let devtype = 'desktop';
  if (isBot) devtype = 'bot';
  else if (type === 'tablet' || /ipad|tablet/.test(lower)) devtype = 'tablet';
  else if (type === 'mobile' || /iphone|android|mobile/.test(lower)) devtype = 'mobile';

  let os = '其他';
  if (/iphone|ipad|ios/.test(osRaw) || /iphone|ipad/.test(lower)) os = 'iOS';
  else if (osRaw.includes('android')) os = 'Android';
  else if (osRaw.includes('windows')) os = 'Windows';
  else if (osRaw.includes('mac')) os = 'macOS';
  else if (osRaw.includes('linux')) os = 'Linux';

  let browser = '其他';
  if (browserRaw.includes('edge') || /\bedg\//.test(lower)) browser = 'Edge';
  else if (browserRaw.includes('firefox') || /firefox\//.test(lower)) browser = 'Firefox';
  else if (browserRaw.includes('chrome') || /chrome\//.test(lower)) browser = 'Chrome';
  else if (browserRaw.includes('safari') || /safari\//.test(lower)) browser = 'Safari';

  return { devtype, os, browser };
}

function buildVisitorFacts(): VisitorFacts {
  const url = simInput.value.url.trim();
  let domain = '';
  let path = '';
  let utm = '';
  try {
    const u = new URL(url);
    domain = u.hostname;
    path = u.pathname || '/';
    utm = u.searchParams.get('utm_source') ?? '';
  } catch {
    // URL 不合法时按裸路径处理
    path = url.startsWith('/') ? url : '';
  }

  let ref = '';
  try {
    if (simInput.value.ref.trim()) ref = new URL(simInput.value.ref.trim()).hostname;
  } catch {
    ref = '';
  }

  const ua = simInput.value.ua;
  const { devtype, os, browser } = detectDevice(ua);

  return {
    ip: simInput.value.ip.trim(),
    ipattr: classifyIpAttr(simInput.value.ip),
    // GeoIP / ASN 数据源未接入,恒空 → 依赖这两个字段的条件恒不命中(spec D5)
    country: '',
    asn: '',
    lang: (simInput.value.lang.split(',')[0] ?? '').trim(),
    ref,
    utm,
    ua,
    devtype,
    os,
    browser,
    path,
    domain,
  };
}

const simProfile = ref<VisitorFacts | null>(null);

const visitorFieldViews = computed(() => {
  const f = simProfile.value;
  const views: { field: RuleField; label: string; value: string; note: string; pending: boolean }[] = [
    { field: 'ip', label: 'IP 地址', value: f?.ip || '—', note: '', pending: false },
    { field: 'ipattr', label: 'IP 属性', value: f?.ipattr || '公网 IP', note: f?.ipattr ? '' : '非 private / loopback / linklocal', pending: false },
    { field: 'country', label: '国家 / 地区', value: '—', note: '数据源待接入 · 恒不命中', pending: true },
    { field: 'asn', label: 'ASN / 运营商', value: '—', note: '数据源待接入 · 恒不命中', pending: true },
    { field: 'lang', label: '语言', value: f?.lang || '—', note: '', pending: false },
    { field: 'ref', label: 'Referrer 主机', value: f?.ref || '—', note: '', pending: false },
    { field: 'utm', label: 'UTM 来源', value: f?.utm || '—', note: '', pending: false },
    { field: 'ua', label: 'User-Agent', value: f?.ua || '—', note: '', pending: false },
    { field: 'devtype', label: '设备类型', value: f?.devtype || '—', note: '', pending: false },
    { field: 'os', label: '操作系统', value: f?.os || '—', note: '', pending: false },
    { field: 'browser', label: '浏览器', value: f?.browser || '—', note: '', pending: false },
    { field: 'path', label: '请求路径', value: f?.path || '—', note: '', pending: false },
    { field: 'domain', label: '请求 Host', value: f?.domain || '—', note: '', pending: false },
  ];
  return views;
});

const simVerdict = ref({
  title: '待运行求值',
  badgeClass: 'badge-neutral',
  accentColor: 'var(--muted)',
  cardStyle: '',
  actionText: '输入访客参数后点击「运行模拟」',
  detailText: '系统将解析当前输入的访客画像，并按优先级依次匹配适用且已启用的规则',
});

const simTraceSteps = ref<TraceStep[]>([]);
const simScopeNote = ref('');

/** 简单的并发限制映射（模拟器要取详情，避免一次性打满浏览器连接） */
async function mapWithConcurrency<T, R>(
  items: T[],
  limit: number,
  fn: (item: T) => Promise<R>,
): Promise<R[]> {
  const results: R[] = [];
  let cursor = 0;
  const workers = Array.from({ length: Math.max(1, Math.min(limit, items.length)) }, async () => {
    while (cursor < items.length) {
      const idx = cursor;
      cursor += 1;
      results[idx] = await fn(items[idx]);
    }
  });
  await Promise.all(workers);
  return results;
}

// —— 条件求值(与后端白名单一致的前端等价实现) ——
function ipToInt(ip: string): number | null {
  const parts = ip.trim().split('.');
  if (parts.length !== 4) return null;
  let out = 0;
  for (const p of parts) {
    if (!/^\d{1,3}$/.test(p)) return null;
    const n = Number(p);
    if (n > 255) return null;
    out = out * 256 + n;
  }
  return out;
}

/** CIDR 匹配(仅 IPv4,与后端 net.IP.ParseCIDR + Contains 一致) */
function ipInCidr(ip: string, cidr: string): boolean {
  const [net, bitsRaw] = cidr.split('/');
  const netInt = ipToInt(net ?? '');
  const ipInt = ipToInt(ip);
  if (netInt === null || ipInt === null) return false;
  const bits = Number(bitsRaw);
  if (!Number.isInteger(bits) || bits < 0 || bits > 32) return false;
  if (bits === 0) return true;
  const mask = (0xffffffff << (32 - bits)) >>> 0;
  return (netInt & mask) === (ipInt & mask);
}

function matchValue(value: string, pattern: string): boolean {
  if (pattern.includes('/') && /^[0-9./]+$/.test(pattern)) {
    return ipInCidr(value, pattern);
  }
  return value.toLowerCase() === pattern.toLowerCase();
}

function containsValue(value: string, pattern: string): boolean {
  return value.toLowerCase().includes(pattern.toLowerCase());
}

function evalCondition(cond: RuleCondition, facts: VisitorFacts): TraceFact {
  const actual = facts[cond.field] ?? '';
  const values = (cond.values || []).filter((v) => v.trim() !== '');
  const valuesText = values.join(' / ') || '（空）';
  const label = fieldOption(cond.field)?.label ?? cond.field;
  const prefix = `${label} = ${actual || '（空）'}`;
  let note = '';

  let hit = false;
  switch (cond.operator) {
    case 'in':
    case 'eq':
    case 'neq':
    case 'not_in': {
      // ip 字段的集合比较走后端的 IP/CIDR 口径：实际值不是合法 IP 时恒不命中
      if (cond.field === 'ip' && ipToInt(actual) === null && !actual.includes(':')) {
        note = '（IP 不可解析）';
        break;
      }
      const anyHit = values.some((v) => matchValue(actual, v));
      if (cond.operator === 'in' || cond.operator === 'eq') {
        hit = anyHit;
      } else {
        // 实际值为空(数据源缺失)时 not_in / neq 也判不成立,避免“恒命中”的假拦截
        hit = actual !== '' && !anyHit;
      }
      break;
    }
    case 'contains':
      hit = values.some((v) => containsValue(actual, v));
      break;
    case 'not_contains':
      hit = actual !== '' && values.every((v) => !containsValue(actual, v));
      break;
    case 'gt':
    case 'lt': {
      const left = Number(actual);
      const right = Number(values[0]);
      if (actual === '' || values.length === 0 || Number.isNaN(left) || Number.isNaN(right)) {
        hit = false;
      } else {
        hit = cond.operator === 'gt' ? left > right : left < right;
      }
      break;
    }
    case 'regex': {
      // 后端是 RE2 且大小写敏感(要忽略大小写得自己写 (?i))；JS 不认 (?i)，
      // 因此先按原样试，编译不过再退化为“剥掉 (?i) + 不区分大小写”的近似匹配。
      let patternBroken = false;
      hit = values.some((v) => {
        try {
          return new RegExp(v).test(actual);
        } catch {
          // 继续尝试近似
        }
        try {
          return new RegExp(v.replace(/^\(\?i\)/, ''), 'i').test(actual);
        } catch {
          patternBroken = true;
          return false;
        }
      });
      if (patternBroken) note = '（正则无法在前端编译，已按不命中处理）';
      break;
    }
    case 'duplicated':
      // 不在 OPERATOR_OPTIONS 里放出（恒不命中的假能力，见该常量注释）；
      // 仅为读回经 API 写入的历史条件而保留：没有计数数据源时恒判不成立
      hit = false;
      break;
    default:
      hit = false;
  }

  return {
    text: `${prefix} ${operatorLabel(cond.operator)} ${valuesText} → ${hit ? '成立' : '不成立'}${note}`,
    hit,
  };
}

/** 按 URL 里的短码定位本租户短链,用于确定规则的适用范围 */
function findSimLink(): Link | undefined {
  let hostname = '';
  let pathname = '';
  try {
    const u = new URL(simInput.value.url.trim());
    hostname = u.hostname;
    pathname = u.pathname;
  } catch {
    return undefined;
  }
  const code = pathname.replace(/^\/+/, '').split('/')[0];
  if (!code) return undefined;
  return linkCatalog.value.find(
    (l) => l.code === code && (l.domains.length === 0 || l.domains.includes(hostname)),
  );
}

async function runSimulation() {
  isSimulating.value = true;
  try {
    if (linkCatalog.value.length === 0) {
      await loadLinkCatalog();
    }

    const facts = buildVisitorFacts();
    simProfile.value = facts;

    // 适用范围:能定位到短链时,以该短链实际的适用规则为准(含全局继承项)
    const link = findSimLink();
    let applicable: Set<number> | null = null;
    if (link) {
      const items: LinkRule[] = await listLinkRules(link.id);
      applicable = new Set(items.map((i) => i.id));
      simScopeNote.value = `URL 命中短链 /${link.code}，已按该短链实际的适用规则（含全局继承）求值。`;
    } else {
      simScopeNote.value =
        'URL 未能解析出本租户短链，本次仅按 scope=global 的全局规则预览；「指定短链」的规则不在预览范围内。';
    }

    const candidates = rules.value
      .filter((r) => r.enabled)
      .filter((r) => previewRuleId.value === null || r.id === previewRuleId.value)
      .sort((a, b) => a.priority - b.priority);

    // 列表接口不带 conditions:按需取详情(仅当前页已启用规则,失败则退化为不参与求值)
    const detailed = await mapWithConcurrency(candidates, 8, async (r) => {
      if (r.conditions && r.conditions.length > 0) return r;
      try {
        return await getRule(r.id);
      } catch {
        return r;
      }
    });
    const conditionless = detailed.filter((r) => (r.conditions?.length ?? 0) === 0);
    if (conditionless.length > 0) {
      message.warning(
        `${conditionless.length} 条规则未取到条件（未配置条件或详情接口失败），未参与本次求值`,
      );
    }

    const steps: TraceStep[] = [];
    let matched: Rule | null = null;

    for (const rule of detailed) {
      const key = `step-${rule.id}`;
      const inScope = rule.scope === 'global' || (applicable ? applicable.has(rule.id) : false);
      if (!inScope) {
        steps.push({
          key,
          ruleId: rule.id,
          ruleName: rule.name,
          status: 'skip',
          statusText: '不适用',
          badgeClass: 'badge-neutral',
          facts: [],
          whyText:
            rule.scope === 'links' && rule.linkCount === 0
              ? '未关联短链 · 不会命中（不会退化为全局规则）'
              : '作用域为「指定短链」，本次请求的短链不在其关联列表中',
        });
        continue;
      }
      if (matched) {
        steps.push({
          key,
          ruleId: rule.id,
          ruleName: rule.name,
          status: 'skip',
          statusText: '已跳过',
          badgeClass: 'badge-neutral',
          facts: [],
          whyText: '首条命中即裁决（First-Match-Wins），后续规则不再求值',
        });
        continue;
      }

      const factsOfConds = (rule.conditions ?? []).map((c) => evalCondition(c, facts));
      if (factsOfConds.length === 0) {
        steps.push({
          key,
          ruleId: rule.id,
          ruleName: rule.name,
          status: 'skip',
          statusText: '无条件',
          badgeClass: 'badge-neutral',
          facts: [],
          whyText: '该规则没有配置条件，不参与求值',
        });
        continue;
      }
      const ruleMatched =
        rule.logic === 'all' ? factsOfConds.every((f) => f.hit) : factsOfConds.some((f) => f.hit);

      if (ruleMatched) {
        matched = rule;
        const isBlock = rule.action === 'notfound' || rule.action === 'throttle';
        steps.push({
          key,
          ruleId: rule.id,
          ruleName: rule.name,
          status: isBlock ? 'block' : 'hit',
          statusText: '命中 · 裁决',
          badgeClass: isBlock ? 'badge-danger' : 'badge-ok',
          facts: factsOfConds,
          whyText: `条件${rule.logic === 'all' ? '全部' : '任一'}满足，执行动作：${actionLabel(rule.action)}${
            rule.action === 'redirect' ? ` → ${rule.destination || '（未填写目标）'}` : ''
          }`,
        });
      } else {
        steps.push({
          key,
          ruleId: rule.id,
          ruleName: rule.name,
          status: 'skip',
          statusText: '未命中',
          badgeClass: 'badge-neutral',
          facts: factsOfConds,
          whyText: '条件不满足，继续求值下一条规则',
        });
      }
    }

    simTraceSteps.value = steps;

    if (matched) {
      const isBlock = matched.action === 'notfound' || matched.action === 'throttle';
      simVerdict.value = {
        title: `命中 #${matched.id} · ${actionLabel(matched.action)}`,
        badgeClass: isBlock ? 'badge-danger' : 'badge-ok',
        accentColor: isBlock ? 'var(--danger)' : 'var(--accent)',
        cardStyle: isBlock
          ? 'background: var(--danger-soft); border-color: color-mix(in srgb, var(--danger) 30%, transparent);'
          : 'background: var(--accent-soft); border-color: color-mix(in srgb, var(--accent) 30%, transparent);',
        actionText:
          matched.action === 'redirect'
            ? `${actionLabel(matched.action)} → ${matched.destination || '（未填写目标）'}`
            : actionLabel(matched.action),
        detailText: `依据规则「${matched.name}」（优先级 ${matched.priority}）判定；命中只计 visits 的动作不灌水访问量。`,
      };
    } else {
      const evaluableCount = detailed.filter((r) => (r.conditions?.length ?? 0) > 0).length;
      simVerdict.value = {
        title: '无规则命中',
        badgeClass: 'badge-neutral',
        accentColor: 'var(--muted)',
        cardStyle: '',
        actionText: evaluableCount === 0 ? '无可用规则（没有已启用且带条件的规则）' : '未命中任何规则',
        detailText:
          evaluableCount === 0
            ? '当前页没有已启用且带条件的规则，请先在规则列表中创建并启用。'
            : '全部适用规则均未命中，按 spec D4 继续走短链自身的目标选择流程。',
      };
    }

    message.success('规则链模拟求值完成');
  } catch (error) {
    if (error instanceof ApiError) message.error(error.message);
    else message.error('模拟求值失败,请稍后重试');
  } finally {
    isSimulating.value = false;
  }
}

function loadSampleBot() {
  simInput.value = {
    url: `${window.location.origin}/promo?fbclid=IwAR27abc`,
    ip: '157.240.1.35',
    ua: 'facebookexternalhit/1.1 (+http://www.facebook.com/externalhit_uatext.php)',
    lang: 'en-US,en;q=0.9',
    ref: 'https://www.facebook.com/',
  };
  void runSimulation();
}

function loadSampleDatacenter() {
  simInput.value = {
    url: `${window.location.origin}/promo`,
    ip: '52.95.245.14',
    ua: 'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36',
    lang: 'de-DE,de;q=0.9,en;q=0.8',
    ref: 'https://news.ycombinator.com/',
  };
  void runSimulation();
}

function loadSampleMobile() {
  simInput.value = {
    url: `${window.location.origin}/promo?utm_source=wechat`,
    ip: '189.45.71.13',
    ua: 'Mozilla/5.0 (iPhone; CPU iPhone OS 18_1 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/18.1 Mobile/15E148 Safari/604.1',
    lang: 'pt-BR,pt;q=0.9,en-US;q=0.8',
    ref: 'https://www.google.com/',
  };
  void runSimulation();
}

// ==================== 初始化 ====================
onMounted(async () => {
  syncTabFromRoute();
  await loadRules();
});
</script>
