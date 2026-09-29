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
          @click="switchTab('simulator')"
        >
          规则模拟器
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
          <button type="button" class="btn btn-sm" id="reorderBtn" @click="handleToggleReorder">
            <ArrowUpDown :size="13" />
            {{ sortBtnLabel }}
          </button>
          <button type="button" class="btn btn-sm btn-primary" id="newRuleBtn" @click="openCreateRule">
            <Plus :size="14" />
            新建规则
          </button>
        </div>
      </div>

      <div class="panel-bd" style="padding-bottom: 0">
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
              class="input grow pr-8"
              id="ruleSearch"
              placeholder="搜索规则名、编号或条件内容…"
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
            <select
              v-model="statusFilter"
              class="select"
              id="statusFilter"
              aria-label="按状态过滤"
            >
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
              <th>编号</th>
              <th style="width: 22%">规则</th>
              <th>条件摘要</th>
              <th>动作</th>
              <th>去向</th>
              <th class="num">24h 命中</th>
              <th class="num">占比</th>
              <th>启用</th>
              <th></th>
            </tr>
          </thead>
          <tbody>
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
                <div class="font-semibold text-[13px] text-ink">{{ rule.name }}</div>
                <div class="micro muted mono mt-0.5">
                  优先级 {{ rule.priority }} · {{ rule.logic }}
                </div>
              </td>

              <!-- 条件摘要 chips -->
              <td>
                <div class="truncate muted text-[12.5px]" :title="rule.conditionSummary">
                  {{ rule.conditionSummary }}
                </div>
              </td>

              <!-- 动作 badge -->
              <td>
                <span :class="['badge', getActionBadgeClass(rule.action)]">
                  {{ rule.action }}
                </span>
              </td>

              <!-- 去向目标 -->
              <td class="shrink mono tiny">
                {{ rule.destination }}
              </td>

              <!-- 24h 命中 -->
              <td class="num">
                {{ rule.hits24h.toLocaleString() }}
              </td>

              <!-- 命中占比 -->
              <td class="num">
                {{ rule.hitRate.toFixed(1) }}%
              </td>

              <!-- Switch 启用/停用 -->
              <td class="shrink">
                <label class="switch" :title="rule.enabled ? '点击停用' : '点击启用'">
                  <input
                    type="checkbox"
                    v-model="rule.enabled"
                    :aria-label="`启用规则 ${rule.name}`"
                    @change="onToggleRule(rule)"
                  />
                  <i></i>
                </label>
              </td>

              <!-- 编辑按钮 -->
              <td class="shrink">
                <button
                  type="button"
                  class="btn btn-sm"
                  :data-edit="rule.id"
                  @click="openEditRule(rule)"
                >
                  编辑
                </button>
              </td>
            </tr>

            <!-- 空状态 -->
            <tr v-if="filteredRules.length === 0">
              <td colspan="9" class="empty text-center py-10 text-muted text-sm">
                无匹配规则，请尝试调整筛选条件
              </td>
            </tr>
          </tbody>
        </table>
      </div>

      <!-- 表格底栏 -->
      <div class="panel-ft row-between text-[12.5px] text-muted">
        <span>
          共 {{ rules.length }} 条规则 · {{ enabledCount }} 条已启用 · 24h 命中合计 {{ totalHits.toLocaleString() }}（占今日访问 {{ totalHitRate }}%）；未命中任何绑定规则的 {{ fallbackMissHits.toLocaleString() }} 次走短链自身的无规则兜底
        </span>
        <span class="mono">评估策略：first-match-wins</span>
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
                <h2>规则编辑器</h2>
                <p>左边定义条件，右边定义命中后做什么。</p>
              </div>
              <span class="badge badge-neutral mono" id="edId">{{ editingRule.id }}</span>
            </div>
            <div class="panel-bd stack">
              <div class="form-grid">
                <div class="field span-2">
                  <label for="rName">规则名称</label>
                  <input
                    v-model="editingRule.name"
                    class="input"
                    id="rName"
                    placeholder="输入规则名称"
                  />
                </div>
                <div class="field span-2">
                  <label for="rDesc">说明（仅内部可见）</label>
                  <input
                    v-model="editingRule.description"
                    class="input"
                    id="rDesc"
                    placeholder="简述规则用途及适用受众"
                  />
                </div>
                <div class="field">
                  <label for="rPrio">优先级</label>
                  <input
                    v-model.number="editingRule.priority"
                    class="input mono"
                    id="rPrio"
                    type="number"
                    min="1"
                    max="999"
                  />
                  <span class="hint">数值越小越先求值</span>
                </div>
                <div class="field">
                  <label>多条件关系</label>
                  <div class="segmented" id="modeSeg" style="align-self: start">
                    <button
                      type="button"
                      :aria-pressed="editingRule.logic === '全部满足'"
                      data-m="全部满足"
                      @click="editingRule.logic = '全部满足'"
                    >
                      全部满足 (AND)
                    </button>
                    <button
                      type="button"
                      :aria-pressed="editingRule.logic === '任一满足'"
                      data-m="任一满足"
                      @click="editingRule.logic = '任一满足'"
                    >
                      任一满足 (OR)
                    </button>
                  </div>
                </div>
              </div>
            </div>
          </div>

          <!-- 面板 2：条件组构造器 -->
          <div class="panel">
            <div class="panel-hd">
              <div>
                <h2>条件组</h2>
                <p>
                  可添加多个条件组，组间用
                  <span class="mono font-semibold" id="joinWord">
                    {{ editingRule.logic === '全部满足' ? 'AND' : 'OR' }}
                  </span>
                  连接。引用名单可复用，不必重复录入 IP 段。
                </p>
              </div>
              <button
                type="button"
                class="btn btn-sm"
                id="addCond"
                @click="addConditionGroup"
              >
                + 添加条件组
              </button>
            </div>
            <div class="panel-bd stack">
              <!-- 条件组列表 -->
              <div id="condHost" class="stack-sm" style="gap: 8px">
                <div
                  v-for="(group, gIndex) in editingRule.conditionGroups"
                  :key="group.id"
                  class="cond-group"
                >
                  <div class="cond-group-hd">
                    <span class="cond-join">
                      条件组 <span>{{ gIndex + 1 }}</span> · {{ editingRule.logic }}
                    </span>
                    <div class="row" style="gap: 6px">
                      <button
                        type="button"
                        class="btn btn-sm btn-ghost"
                        @click="addConditionToGroup(group)"
                      >
                        + 加条件
                      </button>
                      <button
                        v-if="editingRule.conditionGroups.length > 1"
                        type="button"
                        class="btn btn-sm btn-ghost text-err hover:bg-err/10"
                        @click="removeConditionGroup(gIndex)"
                      >
                        删除组
                      </button>
                    </div>
                  </div>

                  <!-- 组内单条条件 -->
                  <div
                    v-for="(cond, cIndex) in group.conditions"
                    :key="cond.id"
                    class="cond-grid"
                    data-cond
                  >
                    <!-- 18 个判定字段 -->
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
                      >
                        {{ f.label }}
                      </option>
                    </select>

                    <!-- 运算符 -->
                    <select
                      v-model="cond.operator"
                      class="select"
                      aria-label="运算符"
                    >
                      <option
                        v-for="op in OPERATOR_OPTIONS"
                        :key="op"
                        :value="op"
                      >
                        {{ op }}
                      </option>
                    </select>

                    <!-- 条件值 / 名单引用 -->
                    <div class="relative flex items-center">
                      <input
                        v-model="cond.value"
                        class="input mono"
                        placeholder="值，逗号分隔；或输入 L-01 引用名单"
                        aria-label="条件值"
                      />
                      <span
                        v-if="isListRef(cond.value)"
                        class="badge badge-ok absolute right-2 pointer-events-none text-[10px]"
                      >
                        引用名单
                      </span>
                    </div>

                    <!-- 删除条件 -->
                    <button
                      type="button"
                      class="icon-btn"
                      data-del
                      aria-label="删除条件"
                      :disabled="group.conditions.length <= 1 && editingRule.conditionGroups.length <= 1"
                      @click="removeCondition(group, cIndex)"
                    >
                      <X :size="14" />
                    </button>
                  </div>
                </div>
              </div>

              <!-- 冲突提示 -->
              <div class="note">
                <AlertTriangle :size="16" class="text-warn shrink-0 mt-0.5" />
                <div>
                  <b>冲突提示。</b>本规则与 <span class="mono font-semibold">R-005 设备型号白名单</span> 在「移动端 + 目标国家」区间重叠 78%。由于本规则优先级更高，R-005 实际上只对平板与桌面生效。建议停用 R-005 或调整其优先级。
                </div>
              </div>
            </div>
          </div>

          <!-- 面板 3：命中动作配置 -->
          <div class="panel">
            <div class="panel-hd">
              <div>
                <h2>命中动作</h2>
                <p>动作决定这个访问者最终看到什么。</p>
              </div>
            </div>
            <div class="panel-bd stack">
              <div class="form-grid">
                <div class="field">
                  <label for="rAct">动作</label>
                  <select
                    v-model="editingRule.actionConfig.action"
                    class="select"
                    id="rAct"
                    @change="onActionChange"
                  >
                    <option value="放行 → 目标池">放行 → 目标池</option>
                    <option value="拦截 → 白标页">拦截 → 白标页</option>
                    <option value="直接 404">直接 404</option>
                    <option value="限流 429">限流 429</option>
                    <option value="重定向到指定 URL">重定向到指定 URL</option>
                    <option value="要求二次验证后放行">要求二次验证后放行</option>
                  </select>
                </div>
                <div class="field">
                  <label for="rDest">去向</label>
                  <select
                    v-model="editingRule.actionConfig.destination"
                    class="select"
                    id="rDest"
                    @change="editingRule.destination = editingRule.actionConfig.destination"
                  >
                    <option value="目标池 A（3 个出口 · 权重 5:3:2）">目标池 A（3 个出口 · 权重 5:3:2）</option>
                    <option value="目标池 B（2 个出口 · 权重 1:1）">目标池 B（2 个出口 · 权重 1:1）</option>
                    <option value="白标 · 品牌页">白标 · 品牌页</option>
                    <option value="直接丢弃">直接丢弃</option>
                    <option value="自定义 URL">自定义重定向 URL</option>
                  </select>
                </div>

                <!-- 自定义重定向 URL 输入框 -->
                <div
                  v-if="editingRule.actionConfig.destination === '自定义 URL' || editingRule.actionConfig.action === '重定向到指定 URL'"
                  class="field span-2"
                >
                  <label for="customRedirect">自定义跳转目的地 (Target URL)</label>
                  <input
                    v-model="editingRule.actionConfig.customUrl"
                    class="input mono"
                    id="customRedirect"
                    placeholder="https://example.com/safe-landing"
                  />
                </div>

                <div class="field span-2">
                  <label for="rTag">命中后打标签（用于统计与人群包）</label>
                  <input
                    v-model="editingRule.actionConfig.tags"
                    class="input mono"
                    id="rTag"
                    placeholder="tier=核心市场, device=移动, cohort=高价值"
                  />
                  <span class="hint">逗号分隔，可回传到广告平台的 Custom Audiences</span>
                </div>
                <div class="field span-2">
                  <label>命中后触发</label>
                  <div class="row flex-wrap" style="gap: 16px">
                    <label class="row tiny cursor-pointer" style="gap: 6px">
                      <span class="switch">
                        <input
                          type="checkbox"
                          v-model="editingRule.actionConfig.triggers.count"
                        />
                        <i></i>
                      </span>
                      <span>计数 +1</span>
                    </label>
                    <label class="row tiny cursor-pointer" style="gap: 6px">
                      <span class="switch">
                        <input
                          type="checkbox"
                          v-model="editingRule.actionConfig.triggers.log"
                        />
                        <i></i>
                      </span>
                      <span>写入决策链日志</span>
                    </label>
                    <label class="row tiny cursor-pointer" style="gap: 6px">
                      <span class="switch">
                        <input
                          type="checkbox"
                          v-model="editingRule.actionConfig.triggers.webhook"
                        />
                        <i></i>
                      </span>
                      <span>Webhook 通知</span>
                    </label>
                    <label class="row tiny cursor-pointer" style="gap: 6px">
                      <span class="switch">
                        <input
                          type="checkbox"
                          v-model="editingRule.actionConfig.triggers.riskList"
                        />
                        <i></i>
                      </span>
                      <span>自动加入风控名单</span>
                    </label>
                  </div>
                </div>
              </div>
            </div>
          </div>
        </div>

        <!-- 右侧：实时规则摘要与预估 -->
        <div class="stack" style="position: sticky; top: 72px">
          <!-- 实时摘要卡片 -->
          <div class="panel">
            <div class="panel-hd">
              <div>
                <h2 style="font-size: 15px">规则摘要</h2>
                <p>人话版本，用于团队对齐。</p>
              </div>
            </div>
            <div class="panel-bd stack-sm">
              <div class="tiny muted">
                当访问者{{ editingRule.logic === '全部满足' ? '同时满足' : '任一满足' }}：
              </div>

              <!-- 实时条件摘要 Chips -->
              <div class="row flex-wrap" style="gap: 6px" id="sumChips">
                <span
                  v-for="(chip, idx) in summaryChips"
                  :key="idx"
                  class="chip"
                >
                  {{ chip.fieldLabel }} <b>{{ chip.value || '（空）' }}</b>
                </span>
                <span v-if="summaryChips.length === 0" class="tiny muted">
                  尚未添加有效条件
                </span>
              </div>

              <div class="tiny muted" style="margin-top: 8px">则：</div>
              <div class="row" style="gap: 6px">
                <span :class="['badge', getActionBadgeClass(editingRule.action)]">
                  {{ editingRule.action }}
                </span>
                <span class="mono tiny">→ {{ displayDestination }}</span>
              </div>

              <hr class="rule" style="border: 0; border-top: 1px solid var(--border); margin: 8px 0" />

              <!-- 命中率预估 -->
              <dl class="kv">
                <dt>预计覆盖</dt>
                <dd>{{ estimateCoverage }}</dd>
                <dt>近 7 日趋势</dt>
                <dd class="text-ok font-semibold">↑ 3.2%</dd>
                <dt>预计月增量</dt>
                <dd>+12,400 访问 / 月</dd>
                <dt>与兜底重叠</dt>
                <dd>无</dd>
                <dt>生效域名</dt>
                <dd>全部 46 个自有及默认域名</dd>
              </dl>
            </div>
            <div class="panel-ft">
              <button
                type="button"
                class="btn btn-sm w-full font-medium"
                :class="isSavedRecently ? 'btn' : 'btn-primary'"
                id="saveRuleTop"
                @click="saveCurrentRule"
              >
                {{ isSavedRecently ? '已保存 · 生效中' : '保存并生效' }}
              </button>
            </div>
          </div>

          <!-- 用真实访客验证卡片 -->
          <div class="panel">
            <div class="panel-hd">
              <div>
                <h2 style="font-size: 15px">用真实访客验证</h2>
                <p>改完先在这里跑一次，不满意再上线。</p>
              </div>
            </div>
            <div class="panel-bd">
              <button
                type="button"
                class="btn btn-sm w-full"
                data-goto="simulator"
                @click="switchTab('simulator')"
              >
                打开模拟器并载入当前规则 →
              </button>
            </div>
          </div>
        </div>
      </div>
    </section>

    <!-- ==================== Tab 3: 规则模拟器 ==================== -->
    <section v-show="currentTab === 'simulator'" data-tabpanel="simulator" data-od-id="rule-simulator">
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
                placeholder="https://example.com/path"
              />
            </div>
            <div class="field">
              <label for="simIp">来访 IP</label>
              <input
                v-model="simInput.ip"
                class="input mono"
                id="simIp"
                placeholder="例如 1.2.3.4"
              />
            </div>
            <div class="field">
              <label for="simUa">User-Agent</label>
              <textarea
                v-model="simInput.ua"
                class="textarea"
                id="simUa"
                rows="4"
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
            <div class="row flex-wrap" style="gap: 8px; margin-top: 6px">
              <button
                type="button"
                class="btn btn-primary"
                id="runSim"
                :disabled="isSimulating"
                @click="runSimulation"
              >
                <Play :size="14" class="fill-current" />
                {{ isSimulating ? '求值中…' : '运行模拟' }}
              </button>
              <button
                type="button"
                class="btn btn-sm"
                id="loadSample"
                @click="loadSampleBot"
              >
                载入示例：爬虫
              </button>
              <button
                type="button"
                class="btn btn-sm"
                id="loadSample2"
                @click="loadSampleDatacenter"
              >
                载入示例：机房 IP
              </button>
              <button
                type="button"
                class="btn btn-sm"
                id="loadSample3"
                @click="loadSampleMobile"
              >
                载入示例：正常海外移动
              </button>
            </div>
          </div>
        </div>

        <!-- 右侧：解析与决策链 -->
        <div class="panel">
          <!-- 访客画像卡片 -->
          <div class="panel-hd">
            <div>
              <h2>解析出的访客画像</h2>
              <p>系统从请求中解析出的可判定字段，规则即基于这些字段求值。</p>
            </div>
            <span class="badge badge-neutral mono" id="simFps">24 字段</span>
          </div>
          <div class="panel-bd" style="padding-bottom: 8px">
            <div class="cols-3" style="gap: 12px">
              <div>
                <div class="mono micro muted">IP 段</div>
                <div class="mono tiny font-semibold text-ink">{{ simProfile.ipRange }}</div>
                <div class="tiny muted">{{ simProfile.ipAttr }}</div>
              </div>
              <div>
                <div class="mono micro muted">ASN / 运营商</div>
                <div class="mono tiny font-semibold text-ink">{{ simProfile.asn }}</div>
                <div class="tiny muted">{{ simProfile.isp }}</div>
              </div>
              <div>
                <div class="mono micro muted">国家 / 州</div>
                <div class="mono tiny font-semibold text-ink">{{ simProfile.country }}</div>
                <div class="tiny muted">{{ simProfile.region }}</div>
              </div>
              <div>
                <div class="mono micro muted">设备类型</div>
                <div class="mono tiny font-semibold text-ink">{{ simProfile.devType }}</div>
                <div class="tiny muted">{{ simProfile.devAttr }}</div>
              </div>
              <div>
                <div class="mono micro muted">系统</div>
                <div class="mono tiny font-semibold text-ink">{{ simProfile.os }}</div>
                <div class="tiny muted">{{ simProfile.osAttr }}</div>
              </div>
              <div>
                <div class="mono micro muted">设备型号</div>
                <div class="mono tiny font-semibold text-ink">{{ simProfile.devModel }}</div>
                <div class="tiny muted">{{ simProfile.modelAttr }}</div>
              </div>
              <div>
                <div class="mono micro muted">浏览器</div>
                <div class="mono tiny font-semibold text-ink">{{ simProfile.browser }}</div>
                <div class="tiny muted">{{ simProfile.browserAttr }}</div>
              </div>
              <div>
                <div class="mono micro muted">语言</div>
                <div class="mono tiny font-semibold text-ink">{{ simProfile.lang }}</div>
                <div class="tiny muted">{{ simProfile.langAttr }}</div>
              </div>
              <div>
                <div class="mono micro muted">时区</div>
                <div class="mono tiny font-semibold text-ink">{{ simProfile.tz }}</div>
                <div class="tiny muted">{{ simProfile.tzOffset }}</div>
              </div>
              <div>
                <div class="mono micro muted">分辨率</div>
                <div class="mono tiny font-semibold text-ink">{{ simProfile.screen }}</div>
                <div class="tiny muted">{{ simProfile.screenAttr }}</div>
              </div>
              <div>
                <div class="mono micro muted">Referrer</div>
                <div class="mono tiny font-semibold text-ink">{{ simProfile.ref }}</div>
                <div class="tiny muted">{{ simProfile.refAttr }}</div>
              </div>
              <div>
                <div class="mono micro muted">Canvas 指纹 / 风控</div>
                <div class="mono tiny font-semibold text-ink">{{ simProfile.canvas }}</div>
                <div class="tiny muted">
                  风控评分：<span :class="simProfile.riskScore > 50 ? 'text-err font-bold' : 'text-ok font-semibold'">{{ simProfile.riskScore }}分</span>
                </div>
              </div>
            </div>
          </div>

          <!-- 决策链回放 -->
          <div class="panel-hd" style="border-top: 1px solid var(--border)">
            <div>
              <h2>决策链</h2>
              <p>逐条规则求值过程，可直接用于排查「为什么这个 IP 被拦」。</p>
            </div>
            <span
              :class="['badge mono font-semibold', simVerdict.badgeClass]"
              id="simVerdict"
            >
              {{ simVerdict.title }}
            </span>
          </div>

          <div class="panel-bd">
            <div v-if="simTraceSteps.length === 0" class="empty py-8 text-center text-xs text-muted">
              请在左侧输入访客参数并点击「运行模拟」，系统将根据当前启用的规则链路生成决策推演。
            </div>
            <!-- 链式回放步骤 -->
            <div v-else class="trace" id="simTrace">
              <div
                v-for="step in simTraceSteps"
                :key="step.ruleId"
                :class="['trace-step', step.status]"
              >
                <div class="trace-rail">
                  <span class="trace-node"></span>
                </div>
                <div>
                  <div class="trace-name flex items-center justify-between">
                    <span>
                      {{ step.ruleId }} {{ step.ruleName }}
                      <span :class="['badge mono ml-1.5', step.badgeClass]">
                        {{ step.statusText }}
                      </span>
                    </span>
                    <span v-if="step.latency" class="tiny text-muted font-mono">
                      {{ step.latency }}
                    </span>
                  </div>

                  <div class="trace-why">
                    <!-- 事实匹配标签 -->
                    <template v-if="step.facts && step.facts.length > 0">
                      <span
                        v-for="(f, fIdx) in step.facts"
                        :key="fIdx"
                        :class="['fact', f.hit ? 'hit' : 'miss']"
                      >
                        {{ f.text }}
                      </span>
                    </template>
                    <span v-if="step.whyText" class="text-muted text-[12.5px] ml-1">
                      {{ step.whyText }}
                    </span>
                  </div>
                </div>
              </div>
            </div>

            <!-- 裁决结果卡片 -->
            <div
              class="mt-4 p-3.5 rounded-[var(--r)] border"
              :style="simVerdict.cardStyle"
            >
              <div class="mono micro font-semibold" :style="{ color: simVerdict.accentColor }">
                裁决结果
              </div>
              <div class="font-[640] text-[15px] mt-1 text-ink">
                {{ simVerdict.actionText }}
              </div>
              <p class="tiny muted mt-1 font-mono break-all">
                {{ simVerdict.detailText }}
              </p>
            </div>
          </div>
        </div>
      </div>
    </section>

    <!-- ==================== Tab 4: 名单库 ==================== -->
    <section v-show="currentTab === 'lists'" data-tabpanel="lists" id="lists" data-od-id="ip-lists">
      <div class="panel">
        <div class="panel-hd">
          <div>
            <h2>名单库</h2>
            <p>IP 段、代理出口、国家、设备型号的集中管理。规则中通过名单编号引用，改名单不用改规则。</p>
          </div>
          <div class="btn-row">
            <button
              type="button"
              class="btn btn-sm"
              @click="showImportModal = true"
            >
              <UploadCloud :size="13" />
              导入 CIDR / CSV
            </button>
            <button
              type="button"
              class="btn btn-sm btn-primary"
              @click="showNewListModal = true"
            >
              <Plus :size="14" />
              新建名单
            </button>
          </div>
        </div>

        <div class="tbl-wrap">
          <table class="tbl">
            <thead>
              <tr>
                <th>编号</th>
                <th>名单</th>
                <th>类型</th>
                <th class="num">条目</th>
                <th>来源 / 备注</th>
                <th>被引用</th>
                <th></th>
              </tr>
            </thead>
            <tbody>
              <template v-for="list in lists" :key="list.id">
                <tr>
                  <td class="shrink mono muted">{{ list.id }}</td>
                  <td class="font-semibold text-[13px] text-ink">{{ list.name }}</td>
                  <td>
                    <span class="badge badge-neutral">{{ list.type }}</span>
                  </td>
                  <td class="num font-semibold">{{ list.items.length.toLocaleString() }}</td>
                  <td class="tiny muted truncate max-w-[200px]" :title="list.source">
                    {{ list.source }}
                  </td>
                  <td class="shrink mono tiny">{{ list.refRules }}</td>
                  <td class="shrink">
                    <button
                      type="button"
                      class="btn btn-sm"
                      @click="toggleListExpand(list.id)"
                    >
                      {{ expandedListId === list.id ? '收起' : '展开' }}
                    </button>
                  </td>
                </tr>

                <!-- 展开条目详情 -->
                <tr v-if="expandedListId === list.id" class="bg-surface-muted/50">
                  <td colspan="7" class="p-4 border-b border-line">
                    <div class="panel p-4 bg-surface shadow-xs">
                      <div class="row-between mb-3">
                        <span class="text-xs font-semibold text-ink">
                          名单「{{ list.name }}」条目清单（共 {{ list.items.length }} 条）
                        </span>
                        <span class="text-xs text-muted">支持单条删除与追加条目</span>
                      </div>

                      <!-- 条目 Chips 容器 -->
                      <div class="flex flex-wrap gap-1.5 max-h-48 overflow-y-auto p-2 bg-surface-muted rounded-[var(--r)] border border-line">
                        <span
                          v-for="(item, iIdx) in list.items"
                          :key="iIdx"
                          class="chip"
                        >
                          <span class="font-mono text-xs">{{ item }}</span>
                          <button
                            type="button"
                            class="hover:text-err text-muted"
                            title="从名单中移除"
                            @click="removeListItem(list, iIdx)"
                          >
                            ×
                          </button>
                        </span>
                        <span v-if="list.items.length === 0" class="tiny muted p-1">
                          名单暂无条目，请在下方添加
                        </span>
                      </div>

                      <!-- 快速添加条目栏 -->
                      <div class="mt-3 flex items-center gap-2">
                        <input
                          v-model="quickItemInputs[list.id]"
                          class="input mono max-w-sm"
                          placeholder="输入单条或逗号分隔的内容并回车..."
                          @keydown.enter="addQuickItem(list)"
                        />
                        <button
                          type="button"
                          class="btn btn-sm btn-primary"
                          @click="addQuickItem(list)"
                        >
                          添加条目
                        </button>
                      </div>
                    </div>
                  </td>
                </tr>
              </template>
            </tbody>
          </table>
        </div>

        <div class="panel-ft text-[12.5px] text-muted">
          名单条目支持三种来源：手工录入、文件上传、以及访问日志里的自动累积（高频风控名单 L-04 即由访问流自动生成）。
        </div>
      </div>
    </section>

    <!-- ==================== 弹窗 1: 导入 CIDR / CSV ==================== -->
    <div
      v-if="showImportModal"
      class="fixed inset-0 z-50 flex items-center justify-center bg-black/45 backdrop-blur-xs p-4"
    >
      <div class="panel w-full max-w-lg shadow-2xl">
        <div class="panel-hd">
          <div>
            <h2 class="text-base font-bold">导入 CIDR / CSV</h2>
            <p>选择目标名单并将 IP、CIDR 网段或条目列表粘贴或上传至库中。</p>
          </div>
          <button
            type="button"
            class="icon-btn text-muted hover:text-fg"
            @click="showImportModal = false"
          >
            <X :size="15" />
          </button>
        </div>

        <div class="panel-bd stack">
          <div class="field">
            <label for="targetList">目标名单</label>
            <select
              v-model="importForm.targetListId"
              class="select"
              id="targetList"
            >
              <option
                v-for="l in lists"
                :key="l.id"
                :value="l.id"
              >
                {{ l.id }} · {{ l.name }} ({{ l.type }})
              </option>
            </select>
          </div>

          <div class="field">
            <label>导入模式</label>
            <div class="segmented" style="align-self: start">
              <button
                type="button"
                :aria-pressed="importForm.mode === 'append'"
                @click="importForm.mode = 'append'"
              >
                追加到现有条目
              </button>
              <button
                type="button"
                :aria-pressed="importForm.mode === 'overwrite'"
                @click="importForm.mode = 'overwrite'"
              >
                覆盖已有条目
              </button>
            </div>
          </div>

          <div class="field">
            <label for="importText">条目内容 (CIDR、IP 或文本)</label>
            <textarea
              v-model="importForm.text"
              class="textarea mono"
              id="importText"
              rows="6"
              placeholder="支持换行或逗号分隔&#10;198.51.100.0/24&#10;203.0.113.0/24&#10;192.168.1.1"
            ></textarea>
            <span class="hint">每行一个条目，或用逗号隔开</span>
          </div>
        </div>

        <div class="panel-ft row-between">
          <span class="tiny text-muted">预计导入：{{ parsedImportLines.length }} 条</span>
          <div class="row" style="gap: 8px">
            <button
              type="button"
              class="btn btn-sm"
              @click="showImportModal = false"
            >
              取消
            </button>
            <button
              type="button"
              class="btn btn-sm btn-primary"
              :disabled="parsedImportLines.length === 0"
              @click="handleConfirmImport"
            >
              确认导入
            </button>
          </div>
        </div>
      </div>
    </div>

    <!-- ==================== 弹窗 2: 新建名单 ==================== -->
    <div
      v-if="showNewListModal"
      class="fixed inset-0 z-50 flex items-center justify-center bg-black/45 backdrop-blur-xs p-4"
    >
      <div class="panel w-full max-w-lg shadow-2xl">
        <div class="panel-hd">
          <div>
            <h2 class="text-base font-bold">新建名单</h2>
            <p>为规则引擎创建专属的数据名单库，便于跨规则统一复用。</p>
          </div>
          <button
            type="button"
            class="icon-btn text-muted hover:text-fg"
            @click="showNewListModal = false"
          >
            <X :size="15" />
          </button>
        </div>

        <div class="panel-bd stack">
          <div class="form-grid">
            <div class="field">
              <label for="nlId">名单编号</label>
              <input
                v-model="newListForm.id"
                class="input mono"
                id="nlId"
                disabled
              />
            </div>
            <div class="field">
              <label for="nlType">类型</label>
              <select
                v-model="newListForm.type"
                class="select"
                id="nlType"
              >
                <option value="IP 段">IP 段 (CIDR)</option>
                <option value="IP">单 IP</option>
                <option value="国家代码">国家 / 地区代码</option>
                <option value="设备型号">设备型号</option>
                <option value="UA 特征">UA 正则 / 特征</option>
              </select>
            </div>
            <div class="field span-2">
              <label for="nlName">名单名称</label>
              <input
                v-model="newListForm.name"
                class="input"
                id="nlName"
                placeholder="例如：亚太高危机房网段"
              />
            </div>
            <div class="field span-2">
              <label for="nlSource">来源 / 备注</label>
              <input
                v-model="newListForm.source"
                class="input"
                id="nlSource"
                placeholder="例如：自建 · 运营人工维护"
              />
            </div>
            <div class="field span-2">
              <label for="nlItems">初始条目（可选）</label>
              <textarea
                v-model="newListForm.initialItems"
                class="textarea mono"
                id="nlItems"
                rows="4"
                placeholder="每行一个或逗号分隔..."
              ></textarea>
            </div>
          </div>
        </div>

        <div class="panel-ft row-between">
          <span class="tiny text-muted"></span>
          <div class="row" style="gap: 8px">
            <button
              type="button"
              class="btn btn-sm"
              @click="showNewListModal = false"
            >
              取消
            </button>
            <button
              type="button"
              class="btn btn-sm btn-primary"
              :disabled="!newListForm.name.trim()"
              @click="handleCreateList"
            >
              创建名单
            </button>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, nextTick, onMounted, reactive, ref, watch } from 'vue';
import { useRoute, useRouter } from 'vue-router';
import {
  AlertTriangle,
  ArrowUpDown,
  Play,
  Plus,
  UploadCloud,
  X,
} from '@lucide/vue';
import { message } from '@/utils/toast';
import { UAParser } from 'ua-parser-js';

// ==================== 常量与 18 个判定字段 ====================
export type TabType = 'rules' | 'editor' | 'simulator' | 'lists';

const TAB_STORAGE_KEY = 'cloak.re.tab';

interface FieldOption {
  value: string;
  label: string;
  defaultOp: string;
}

const FIELD_OPTIONS: FieldOption[] = [
  { value: 'ip', label: 'IP 地址 / CIDR', defaultOp: '属于' },
  { value: 'ipattr', label: 'IP 属性', defaultOp: '属于' },
  { value: 'asn', label: 'ASN / 运营商', defaultOp: '属于' },
  { value: 'country', label: '国家 / 地区', defaultOp: '属于' },
  { value: 'region', label: '省 / 州', defaultOp: '属于' },
  { value: 'city', label: '城市', defaultOp: '属于' },
  { value: 'devtype', label: '设备类型', defaultOp: '等于' },
  { value: 'os', label: '操作系统', defaultOp: '属于' },
  { value: 'devmodel', label: '设备型号', defaultOp: '属于' },
  { value: 'browser', label: '浏览器', defaultOp: '属于' },
  { value: 'lang', label: '语言 (Accept-Language)', defaultOp: '等于' },
  { value: 'tz', label: '时区', defaultOp: '属于' },
  { value: 'screen', label: '屏幕分辨率', defaultOp: '属于' },
  { value: 'ref', label: 'Referrer 域名', defaultOp: '包含' },
  { value: 'utm', label: 'UTM 来源', defaultOp: '等于' },
  { value: 'tls', label: 'TLS / JA3 指纹', defaultOp: '属于' },
  { value: 'canvas', label: 'Canvas 指纹', defaultOp: '重复出现' },
  { value: 'cookie', label: 'Cookie 状态', defaultOp: '等于' },
];

const OPERATOR_OPTIONS = [
  '属于',
  '不属于',
  '等于',
  '不等于',
  '包含',
  '不包含',
  '大于',
  '小于',
  '正则匹配',
  '重复出现',
];

// ==================== 数据类型定义 ====================
interface ConditionItem {
  id: string;
  field: string;
  operator: string;
  value: string;
}

interface ConditionGroup {
  id: string;
  conditions: ConditionItem[];
}

interface RuleItem {
  id: string;
  name: string;
  description: string;
  priority: number;
  logic: '全部满足' | '任一满足';
  conditionSummary: string;
  action: string;
  destination: string;
  hits24h: number;
  hitRate: number;
  enabled: boolean;
  conditionGroups: ConditionGroup[];
  actionConfig: {
    action: string;
    destination: string;
    customUrl?: string;
    tags: string;
    triggers: {
      count: boolean;
      log: boolean;
      webhook: boolean;
      riskList: boolean;
    };
  };
}

interface ListItem {
  id: string;
  name: string;
  type: string;
  source: string;
  refRules: string;
  items: string[];
}

interface TraceFact {
  text: string;
  hit: boolean;
}

interface TraceStep {
  ruleId: string;
  ruleName: string;
  status: 'hit' | 'block' | 'skip';
  statusText: string;
  badgeClass: string;
  facts?: TraceFact[];
  whyText?: string;
  latency?: string;
}

// ==================== 路由与选项卡状态 ====================
const route = useRoute();
const router = useRouter();

const currentTab = ref<TabType>('rules');

function switchTab(tab: TabType) {
  currentTab.value = tab;
  try {
    localStorage.setItem(TAB_STORAGE_KEY, tab);
  } catch {}
  router.replace({
    query: { ...route.query, tab },
    hash: `#${tab}`,
  });
}

function syncTabFromRoute() {
  const qTab = route.query.tab as TabType | undefined;
  const hash = route.hash;

  if (qTab && ['rules', 'editor', 'simulator', 'lists'].includes(qTab)) {
    currentTab.value = qTab;
    return;
  }

  if (hash === '#lists') {
    currentTab.value = 'lists';
    return;
  }
  if (hash === '#editor') {
    currentTab.value = 'editor';
    return;
  }
  if (hash === '#simulator') {
    currentTab.value = 'simulator';
    return;
  }
  if (hash === '#rules') {
    currentTab.value = 'rules';
    return;
  }

  try {
    const saved = localStorage.getItem(TAB_STORAGE_KEY) as TabType | null;
    if (saved && ['rules', 'editor', 'simulator', 'lists'].includes(saved)) {
      currentTab.value = saved;
      return;
    }
  } catch {}

  currentTab.value = 'rules';
}

watch(
  () => [route.query.tab, route.hash],
  () => {
    syncTabFromRoute();
  },
);

onMounted(() => {
  syncTabFromRoute();
});

// ==================== 初始名单库 (L-01 至 L-07) ====================
const lists = ref<ListItem[]>([
  {
    id: 'L-01',
    name: '内部测试网段',
    type: 'IP 段',
    source: '自建 · 已启用',
    refRules: 'R-006',
    items: [
      '198.51.100.0/24',
      '203.0.113.0/24',
      '10.8.0.0/16',
      '192.168.10.0/24',
      '100.64.0.0/10',
      '172.16.50.0/24',
    ],
  },
  {
    id: 'L-02',
    name: '平台审查爬虫段',
    type: 'IP 段',
    source: '官方公布段 · 已启用',
    refRules: 'R-002',
    items: [
      '157.240.0.0/16',
      '31.13.64.0/18',
      '66.220.144.0/20',
      '69.63.176.0/20',
      '69.171.224.0/19',
      '74.119.76.0/22',
      '173.252.64.0/18',
      '204.15.20.0/22',
    ],
  },
  {
    id: 'L-03',
    name: '代理 / VPN 出口',
    type: 'IP 段',
    source: '威胁情报源 · 已启用',
    refRules: 'R-003',
    items: [
      '52.95.240.0/20',
      '3.120.0.0/14',
      '185.220.101.0/24',
      '194.26.29.0/24',
    ],
  },
  {
    id: 'L-04',
    name: '高频可疑 IP (自定义风控黑名单)',
    type: 'IP',
    source: '自动累积 + 人工',
    refRules: 'R-007',
    items: [
      '45.145.185.10',
      '193.106.191.22',
      '89.248.163.54',
      '185.191.171.12',
      '194.26.29.117',
      '185.220.101.5',
      '23.106.56.88',
      '195.123.210.4',
    ],
  },
  {
    id: 'L-05',
    name: '目标放行国家 (VIP 白名单)',
    type: '国家代码',
    source: 'US CA GB AU DE',
    refRules: 'R-001 / R-003',
    items: ['US', 'CA', 'GB', 'AU', 'DE'],
  },
  {
    id: 'L-06',
    name: '白名单设备型号',
    type: '设备型号',
    source: '预设库 · 已启用',
    refRules: 'R-005',
    items: ['iPhone 15', 'iPhone 14', 'Galaxy S23', 'Pixel 8', 'iPad Air'],
  },
  {
    id: 'L-07',
    name: '优质访客网段',
    type: 'IP 段',
    source: '住宅 · 已启用',
    refRules: '—',
    items: ['202.159.44.0/24', '172.56.0.0/16', '98.142.128.0/17'],
  },
]);

// ==================== 初始规则列表 (R-001 至 R-008) ====================
const rules = ref<RuleItem[]>([
  {
    id: 'R-006',
    name: '内部测试强制放行',
    description: '内部测试人员 IP 段白名单，无视任何审查直接放行直达',
    priority: 5,
    logic: '全部满足',
    conditionSummary: 'IP 段 ∈ 名单 内部测试网段',
    action: '放行',
    destination: '目标池 · 直达',
    hits24h: 0,
    hitRate: 0,
    enabled: true,
    conditionGroups: [
      {
        id: 'cg-6-1',
        conditions: [
          { id: 'c-6-1', field: 'ip', operator: '属于', value: 'L-01' },
        ],
      },
    ],
    actionConfig: {
      action: '放行 → 目标池',
      destination: '目标池 · 直达',
      tags: 'tier=internal_test, cohort=dev',
      triggers: { count: true, log: true, webhook: false, riskList: false },
    },
  },
  {
    id: 'R-001',
    name: '目标市场 · 移动端放行',
    description: '五大目标市场 + 移动端，命中后进入主目标池 A',
    priority: 10,
    logic: '全部满足',
    conditionSummary: '国家 ∈ [US, CA, GB, AU, DE] · 设备类型 = 移动 · 系统 = iOS / Android',
    action: '放行',
    destination: '目标池 A',
    hits24h: 0,
    hitRate: 0,
    enabled: true,
    conditionGroups: [
      {
        id: 'cg-1-1',
        conditions: [
          { id: 'c-1-1', field: 'country', operator: '属于', value: 'US, CA, GB, AU, DE' },
          { id: 'c-1-2', field: 'devtype', operator: '等于', value: '移动' },
          { id: 'c-1-3', field: 'os', operator: '属于', value: 'iOS, Android' },
        ],
      },
    ],
    actionConfig: {
      action: '放行 → 目标池',
      destination: '目标池 A（3 个出口 · 权重 5:3:2）',
      tags: 'tier=核心市场, device=移动, cohort=高价值',
      triggers: { count: true, log: true, webhook: false, riskList: true },
    },
  },
  {
    id: 'R-002',
    name: '拦截 · 平台审查爬虫',
    description: '主流广告平台审核机器人与抓取爬虫特征拦截',
    priority: 20,
    logic: '任一满足',
    conditionSummary: 'UA 匹配 bot|spider|crawler|facebookexternalhit · IP 段 ∈ 名单 审查爬虫段',
    action: '直接 404',
    destination: '—',
    hits24h: 0,
    hitRate: 0,
    enabled: true,
    conditionGroups: [
      {
        id: 'cg-2-1',
        conditions: [
          { id: 'c-2-1', field: 'browser', operator: '包含', value: 'bot, spider, crawler, facebookexternalhit' },
          { id: 'c-2-2', field: 'ip', operator: '属于', value: 'L-02' },
        ],
      },
    ],
    actionConfig: {
      action: '直接 404',
      destination: '直接丢弃',
      tags: 'risk=平台爬虫, action=drop',
      triggers: { count: true, log: true, webhook: false, riskList: true },
    },
  },
  {
    id: 'R-003',
    name: '拦截 · 代理与机房出口',
    description: '非目标国家的数据中心服务器、公共代理出口降级限流',
    priority: 30,
    logic: '任一满足',
    conditionSummary: 'IP 属性 ∈ 代理 / VPN / 机房 · 国家 ∉ 名单 目标放行国家',
    action: '限流 + 计数',
    destination: '风控观察',
    hits24h: 0,
    hitRate: 0,
    enabled: true,
    conditionGroups: [
      {
        id: 'cg-3-1',
        conditions: [
          { id: 'c-3-1', field: 'ipattr', operator: '属于', value: '代理, VPN, 机房, 数据中心' },
          { id: 'c-3-2', field: 'country', operator: '不属于', value: 'L-05' },
        ],
      },
    ],
    actionConfig: {
      action: '限流 429',
      destination: '白标 · 品牌页',
      tags: 'risk=代理机房, action=throttle',
      triggers: { count: true, log: true, webhook: true, riskList: true },
    },
  },
  {
    id: 'R-004',
    name: '语言分流 · 葡语市场',
    description: '巴西和葡萄牙受众定向分流到专属目标池 B',
    priority: 40,
    logic: '全部满足',
    conditionSummary: '语言 = pt-BR / pt-PT · 设备类型 ≠ 爬虫',
    action: '放行',
    destination: '目标池 B',
    hits24h: 0,
    hitRate: 0,
    enabled: true,
    conditionGroups: [
      {
        id: 'cg-4-1',
        conditions: [
          { id: 'c-4-1', field: 'lang', operator: '属于', value: 'pt-BR, pt-PT' },
          { id: 'c-4-2', field: 'devtype', operator: '不等于', value: '爬虫' },
        ],
      },
    ],
    actionConfig: {
      action: '放行 → 目标池',
      destination: '目标池 B（2 个出口 · 权重 1:1）',
      tags: 'market=LATAM_PT, pool=B',
      triggers: { count: true, log: true, webhook: false, riskList: false },
    },
  },
  {
    id: 'R-005',
    name: '设备型号白名单',
    description: '指定最新旗舰移动设备白名单进行定向放行',
    priority: 50,
    logic: '全部满足',
    conditionSummary: '设备型号 ∈ [iPhone 15, iPhone 14, Galaxy S23, Pixel 8, iPad Air]',
    action: '放行',
    destination: '目标池 A',
    hits24h: 0,
    hitRate: 0,
    enabled: false,
    conditionGroups: [
      {
        id: 'cg-5-1',
        conditions: [
          { id: 'c-5-1', field: 'devmodel', operator: '属于', value: 'iPhone 15, iPhone 14, Galaxy S23, Pixel 8, iPad Air' },
        ],
      },
    ],
    actionConfig: {
      action: '放行 → 目标池',
      destination: '目标池 A（3 个出口 · 权重 5:3:2）',
      tags: 'cohort=旗舰设备',
      triggers: { count: true, log: true, webhook: false, riskList: false },
    },
  },
  {
    id: 'R-007',
    name: '频次风控 · 单 IP 限流',
    description: '短时高频可疑访问强制返回 429 请求过多',
    priority: 60,
    logic: '全部满足',
    conditionSummary: '单 IP 10 分钟内访问 > 20 次 · 命中名单 高频可疑',
    action: '限流',
    destination: '429 · 计数',
    hits24h: 0,
    hitRate: 0,
    enabled: true,
    conditionGroups: [
      {
        id: 'cg-7-1',
        conditions: [
          { id: 'c-7-1', field: 'ip', operator: '属于', value: 'L-04' },
        ],
      },
    ],
    actionConfig: {
      action: '限流 429',
      destination: '直接丢弃',
      tags: 'rate_limit=10m20req, penalty=429',
      triggers: { count: true, log: true, webhook: false, riskList: true },
    },
  },
  {
    id: 'R-008',
    name: '兜底 · 品牌白标页',
    description: '所有未命中前面过滤规则的流量，展示品牌白标安全落地页',
    priority: 999,
    logic: '全部满足',
    conditionSummary: '所有未命中前面规则的访问',
    action: '白标页',
    destination: '白标 · 品牌页',
    hits24h: 0,
    hitRate: 0,
    enabled: true,
    conditionGroups: [
      {
        id: 'cg-8-1',
        conditions: [
          { id: 'c-8-1', field: 'country', operator: '包含', value: '*' },
        ],
      },
    ],
    actionConfig: {
      action: '拦截 → 白标页',
      destination: '白标 · 品牌页',
      tags: 'fallback=true, brand_safe=true',
      triggers: { count: true, log: true, webhook: false, riskList: false },
    },
  },
]);

// ==================== 规则列表：过滤与重排 ====================
const actionFilters = [
  { key: 'all', label: '全部' },
  { key: '放行', label: '放行' },
  { key: '拦截', label: '拦截' },
  { key: '限流', label: '限流' },
  { key: '白标页', label: '白标' },
];

const selectedActFilter = ref('all');
const searchKeyword = ref('');
const statusFilter = ref<'all' | 'on' | 'off'>('all');
const isSortedByHitRate = ref(false);
const sortBtnLabel = ref('按命中率重排');

function handleToggleReorder() {
  if (!isSortedByHitRate.value) {
    rules.value.sort((a, b) => b.hitRate - a.hitRate);
    isSortedByHitRate.value = true;
    sortBtnLabel.value = '已按命中率降序';
    message.success('规则已按 24h 命中率降序重排');
    setTimeout(() => {
      sortBtnLabel.value = '按优先级升序';
    }, 2000);
  } else {
    rules.value.sort((a, b) => a.priority - b.priority);
    isSortedByHitRate.value = false;
    sortBtnLabel.value = '已按优先级升序';
    message.success('规则已恢复按求值优先级升序排序');
    setTimeout(() => {
      sortBtnLabel.value = '按命中率重排';
    }, 2000);
  }
}

const filteredRules = computed(() => {
  return rules.value.filter((r) => {
    // 动作过滤
    let actMatch = true;
    if (selectedActFilter.value !== 'all') {
      if (selectedActFilter.value === '拦截') {
        actMatch = r.action.startsWith('拦截') || r.action.includes('404');
      } else if (selectedActFilter.value === '限流') {
        actMatch = r.action.includes('限流');
      } else {
        actMatch = r.action.includes(selectedActFilter.value);
      }
    }

    // 状态过滤
    let statusMatch = true;
    if (statusFilter.value === 'on') statusMatch = r.enabled;
    if (statusFilter.value === 'off') statusMatch = !r.enabled;

    // 搜索关键字
    let qMatch = true;
    const q = searchKeyword.value.trim().toLowerCase();
    if (q) {
      const combined = `${r.id} ${r.name} ${r.description} ${r.conditionSummary} ${r.destination} ${r.action}`.toLowerCase();
      qMatch = combined.includes(q);
    }

    return actMatch && statusMatch && qMatch;
  });
});

const enabledCount = computed(() => rules.value.filter((r) => r.enabled).length);
const totalHits = computed(() => rules.value.reduce((acc, r) => acc + r.hits24h, 0));
const totalHitRate = computed(() => {
  const sum = rules.value.reduce((acc, r) => acc + r.hitRate, 0);
  return sum.toFixed(1);
});
const fallbackMissHits = ref(32816);

function getActionBadgeClass(act: string) {
  if (act.includes('放行')) return 'badge-ok';
  if (act.includes('404') || act.includes('拦截')) return 'badge-danger';
  if (act.includes('限流')) return 'badge-warn';
  return 'badge-neutral';
}

function onToggleRule(rule: RuleItem) {
  message.info(`规则「${rule.name}」已${rule.enabled ? '启用' : '停用'}`);
}

// ==================== 规则编辑器状态 ====================
const editingRule = reactive<RuleItem>({
  id: 'R-001',
  name: '目标市场 · 移动端放行',
  description: '五大目标市场 + 移动端，命中后进入主目标池',
  priority: 10,
  logic: '全部满足',
  conditionSummary: '国家 ∈ [US, CA, GB, AU, DE] · 设备类型 = 移动 · 系统 = iOS / Android',
  action: '放行',
  destination: '目标池 A',
  hits24h: 0,
  hitRate: 0,
  enabled: true,
  conditionGroups: [
    {
      id: 'cg-1',
      conditions: [
        { id: 'c-1', field: 'country', operator: '属于', value: 'US, CA, GB, AU, DE' },
        { id: 'c-2', field: 'devtype', operator: '等于', value: '移动' },
        { id: 'c-3', field: 'os', operator: '属于', value: 'iOS, Android' },
      ],
    },
  ],
  actionConfig: {
    action: '放行 → 目标池',
    destination: '目标池 A（3 个出口 · 权重 5:3:2）',
    tags: 'tier=核心市场, device=移动, cohort=高价值',
    triggers: {
      count: true,
      log: true,
      webhook: false,
      riskList: true,
    },
  },
});

const isSavedRecently = ref(false);

function openCreateRule() {
  const nextNum = rules.value.length + 1;
  const newId = `R-${String(nextNum).padStart(3, '0')}`;
  editingRule.id = newId;
  editingRule.name = '未命名规则';
  editingRule.description = '新建准入控制规则，请配置判定条件与动作';
  editingRule.priority = 70;
  editingRule.logic = '全部满足';
  editingRule.action = '放行';
  editingRule.destination = '目标池 A（3 个出口 · 权重 5:3:2）';
  editingRule.hits24h = 0;
  editingRule.hitRate = 0.0;
  editingRule.enabled = true;
  editingRule.conditionGroups = [
    {
      id: 'cg-new-1',
      conditions: [
        { id: 'c-new-1', field: 'country', operator: '属于', value: 'US, CA, GB' },
        { id: 'c-new-2', field: 'devtype', operator: '等于', value: '移动' },
      ],
    },
  ];
  editingRule.actionConfig = {
    action: '放行 → 目标池',
    destination: '目标池 A（3 个出口 · 权重 5:3:2）',
    tags: 'cohort=新建规则',
    triggers: {
      count: true,
      log: true,
      webhook: false,
      riskList: false,
    },
  };
  switchTab('editor');
}

function openEditRule(rule: RuleItem) {
  editingRule.id = rule.id;
  editingRule.name = rule.name;
  editingRule.description = rule.description;
  editingRule.priority = rule.priority;
  editingRule.logic = rule.logic;
  editingRule.conditionSummary = rule.conditionSummary;
  editingRule.action = rule.action;
  editingRule.destination = rule.destination;
  editingRule.hits24h = rule.hits24h;
  editingRule.hitRate = rule.hitRate;
  editingRule.enabled = rule.enabled;
  editingRule.conditionGroups = JSON.parse(JSON.stringify(rule.conditionGroups));
  editingRule.actionConfig = JSON.parse(JSON.stringify(rule.actionConfig));

  switchTab('editor');
}

function onFieldChange(cond: ConditionItem) {
  const f = FIELD_OPTIONS.find((opt) => opt.value === cond.field);
  if (f) {
    cond.operator = f.defaultOp;
  }
}

function addConditionGroup() {
  const gId = `cg-${Date.now()}`;
  editingRule.conditionGroups.push({
    id: gId,
    conditions: [
      { id: `c-${Date.now()}`, field: 'country', operator: '属于', value: '' },
    ],
  });
}

function removeConditionGroup(index: number) {
  if (editingRule.conditionGroups.length > 1) {
    editingRule.conditionGroups.splice(index, 1);
  }
}

function addConditionToGroup(group: ConditionGroup) {
  group.conditions.push({
    id: `c-${Date.now()}-${Math.random().toString(36).slice(2, 6)}`,
    field: 'devtype',
    operator: '等于',
    value: '',
  });
}

function removeCondition(group: ConditionGroup, index: number) {
  if (group.conditions.length <= 1 && editingRule.conditionGroups.length <= 1) {
    group.conditions[0].value = '';
    return;
  }
  group.conditions.splice(index, 1);
  if (group.conditions.length === 0 && editingRule.conditionGroups.length > 1) {
    const gIdx = editingRule.conditionGroups.indexOf(group);
    if (gIdx >= 0) editingRule.conditionGroups.splice(gIdx, 1);
  }
}

function isListRef(val: string) {
  return /^L-0[1-9]/i.test(val.trim());
}

function onActionChange() {
  const act = editingRule.actionConfig.action;
  if (act.startsWith('放行')) {
    editingRule.action = '放行';
  } else if (act.includes('404')) {
    editingRule.action = '直接 404';
  } else if (act.includes('限流')) {
    editingRule.action = '限流';
  } else if (act.includes('白标')) {
    editingRule.action = '白标页';
  } else {
    editingRule.action = act;
  }
}

// 实时条件摘要计算
const summaryChips = computed(() => {
  const chips: { fieldLabel: string; value: string }[] = [];
  editingRule.conditionGroups.forEach((g) => {
    g.conditions.forEach((c) => {
      const f = FIELD_OPTIONS.find((item) => item.value === c.field);
      const label = f ? f.label : c.field;
      chips.push({
        fieldLabel: label,
        value: c.value,
      });
    });
  });
  return chips;
});

const displayDestination = computed(() => {
  const d = editingRule.actionConfig.destination;
  if (d === '目标池 A（3 个出口 · 权重 5:3:2）') return '目标池 A';
  if (d === '目标池 B（2 个出口 · 权重 1:1）') return '目标池 B';
  if (d === '白标 · 品牌页') return '白标 · 品牌页';
  if (d === '直接丢弃') return '直接 404 丢弃';
  if (d === '自定义 URL') return editingRule.actionConfig.customUrl || '自定义 URL';
  return d;
});

const estimateCoverage = computed(() => {
  if (editingRule.id === 'R-001') return '40.8% 访问';
  if (editingRule.id === 'R-004') return '22.3% 访问';
  if (editingRule.id === 'R-008') return '6.6% 访问';
  return '约 15.4% 访问';
});

function saveCurrentRule() {
  // 查找或更新
  const existingIdx = rules.value.findIndex((r) => r.id === editingRule.id);
  const summaryStr = summaryChips.value
    .map((c) => `${c.fieldLabel}: ${c.value || '全部'}`)
    .join(' · ');

  editingRule.conditionSummary = summaryStr || '自定义多条件';
  editingRule.destination = displayDestination.value;

  const ruleCopy: RuleItem = JSON.parse(JSON.stringify(editingRule));

  if (existingIdx >= 0) {
    rules.value[existingIdx] = ruleCopy;
  } else {
    rules.value.unshift(ruleCopy);
  }

  isSavedRecently.value = true;
  message.success(`规则「${editingRule.name}」已保存并下发至 8/8 边缘节点生效`);
  setTimeout(() => {
    isSavedRecently.value = false;
  }, 2200);
}

// ==================== 规则模拟器状态与逻辑 ====================
const isSimulating = ref(false);

const simInput = reactive({
  url: typeof window !== 'undefined' ? `${window.location.origin}/promo` : 'https://example.com/promo',
  ip: '',
  ua: typeof navigator !== 'undefined' ? navigator.userAgent : 'Mozilla/5.0 (iPhone; CPU iPhone OS 18_1 like Mac OS X) AppleWebKit/605.1.15',
  lang: typeof navigator !== 'undefined' ? navigator.language : 'zh-CN,zh;q=0.9,en;q=0.8',
});

function syncSimInputFromQuery() {
  if (route.query.ip) simInput.ip = String(route.query.ip);
  if (route.query.ua) simInput.ua = String(route.query.ua);
  if (route.query.lang) simInput.lang = String(route.query.lang);
}

watch(
  () => [route.query.ip, route.query.ua, route.query.lang],
  () => {
    syncSimInputFromQuery();
  },
  { immediate: true },
);

const simProfile = reactive({
  ipRange: '202.159.44.0/24',
  ipAttr: '住宅 · 非代理',
  asn: 'AS28573',
  isp: 'Claro NXT Telecom',
  country: 'BR / SP',
  region: 'São Paulo',
  devType: '移动',
  devAttr: '触屏 · 高 DPR',
  os: 'iOS 18.1',
  osAttr: '非越狱系统',
  devModel: 'iPhone 15',
  modelAttr: '在白名单内',
  browser: 'Mobile Safari 18.1',
  browserAttr: '非 headless 正常浏览器',
  lang: 'pt-BR',
  langAttr: '与 IP 地区一致',
  tz: 'America/Sao_Paulo',
  tzOffset: '-03:00 GMT',
  screen: '393 × 852',
  screenAttr: '物理视口规格正常',
  ref: 'facebook.com',
  refAttr: 'ttclid 广告点击参数有效',
  canvas: 'a3f1…9c22',
  canvasAttr: '24h 内首次出现',
  riskScore: 12,
});

const simVerdict = reactive({
  title: '待运行求值',
  badgeClass: 'badge-neutral',
  accentColor: 'var(--muted)',
  cardStyle: '',
  actionText: '输入访客参数后点击「运行模拟」',
  detailText: '系统将解析当前输入的访客画像，并按优先级依次匹配启用的规则集',
});

const simTraceSteps = ref<TraceStep[]>([]);

function runSimulation() {
  isSimulating.value = true;
  setTimeout(() => {
    isSimulating.value = false;

    // 1. 使用 UAParser 解析真实访客环境
    const parser = new UAParser(simInput.ua);
    const os = parser.getOS();
    const browser = parser.getBrowser();
    const device = parser.getDevice();

    const uaLower = simInput.ua.toLowerCase();
    const isBot = /bot|crawl|spider|facebookexternalhit|curl|wget|python|bytespider|googlebot/i.test(uaLower) || (device.type as string | undefined) === 'bot';
    const isMobile = device.type === 'mobile' || /iphone|android|mobile/i.test(uaLower);
    const isTablet = device.type === 'tablet' || /ipad/i.test(uaLower);

    const devTypeLabel = isBot ? '爬虫 / 机器人' : (isMobile ? '移动' : (isTablet ? '平板' : '桌面'));
    const osName = os.name || (isMobile ? (/iphone|ipad/i.test(uaLower) ? 'iOS' : 'Android') : 'Windows');
    const browserName = browser.name || (isBot ? 'Robot/Crawler' : 'Browser');

    // 解析国家
    let countryCode = 'US';
    let countryName = 'US / 美国';
    if (/pt/i.test(simInput.lang)) { countryCode = 'BR'; countryName = 'BR / 巴西'; }
    else if (/de/i.test(simInput.lang)) { countryCode = 'DE'; countryName = 'DE / 德国'; }
    else if (/zh/i.test(simInput.lang)) { countryCode = 'CN'; countryName = 'CN / 中国'; }
    else if (/gb/i.test(simInput.lang) || /uk/i.test(simInput.lang)) { countryCode = 'GB'; countryName = 'GB / 英国'; }

    const isDatacenter = simInput.ip.startsWith('52.') || simInput.ip.startsWith('54.') || simInput.ip.startsWith('3.') || simInput.ip.startsWith('34.') || simInput.ip.startsWith('157.240.');

    simProfile.devType = devTypeLabel;
    simProfile.os = osName + (os.version ? ' ' + os.version : '');
    simProfile.browser = browserName + (browser.version ? ' ' + browser.version : '');
    simProfile.devModel = device.model || (isMobile ? '移动手机' : 'PC 桌面');
    simProfile.country = countryName;
    simProfile.lang = simInput.lang.split(',')[0] || 'en-US';
    simProfile.ipAttr = isBot ? '审查抓取节点' : (isDatacenter ? '机房 / 数据中心' : '住宅 / 移动宽带');
    simProfile.asn = isDatacenter ? 'AS16509 (Cloud/DC)' : (isBot ? 'AS32934 (Crawler)' : 'AS28573 (Telecom)');
    simProfile.riskScore = isBot ? 98 : (isDatacenter ? 78 : 12);

    // 2. 按优先级升序求值启用的规则
    const activeRules = rules.value.filter((r) => r.enabled).sort((a, b) => a.priority - b.priority);
    const traceSteps: TraceStep[] = [];
    let matchedRule: RuleItem | null = null;

    for (const rule of activeRules) {
      if (matchedRule) {
        traceSteps.push({
          ruleId: rule.id,
          ruleName: rule.name,
          status: 'skip',
          statusText: '已跳过',
          badgeClass: 'badge-neutral',
          whyText: '首条命中即裁决 (First-Match-Wins)，后续规则不再求值',
        });
        continue;
      }

      // 求值该规则的条件组
      const groupResults = rule.conditionGroups.map((cg) => {
        return cg.conditions.map((c) => {
          let hit = false;
          let factText = '';
          const val = (c.value || '').toLowerCase();
          if (c.field === 'ip') {
            hit = val.includes('l-01')
              ? simInput.ip.startsWith('10.') || simInput.ip.startsWith('192.') || simInput.ip === '127.0.0.1'
              : simInput.ip.includes(val);
            factText = `IP ${simInput.ip} ${hit ? '符合' : '不符合'} ${c.value}`;
          } else if (c.field === 'devtype') {
            hit = (val.includes('移动') && isMobile) || (val.includes('桌面') && !isMobile && !isBot) || (val.includes('爬虫') && isBot);
            factText = `设备类型 = ${devTypeLabel} (${hit ? '匹配' : '不匹配'})`;
          } else if (c.field === 'os') {
            hit = val.split(/[,|\s]+/).some((x) => x && osName.toLowerCase().includes(x));
            factText = `系统 = ${osName} (${hit ? '属于' : '不属于'} ${c.value})`;
          } else if (c.field === 'browser') {
            hit = browserName.toLowerCase().includes(val);
            factText = `浏览器 = ${browserName} (${hit ? '匹配' : '不匹配'})`;
          } else if (c.field === 'country') {
            hit = val.toUpperCase().includes(countryCode);
            factText = `国家 = ${countryCode} (${hit ? '属于' : '不属于'} ${c.value})`;
          } else if (c.field === 'lang') {
            hit = simInput.lang.toLowerCase().includes(val);
            factText = `语言 = ${simProfile.lang} (${hit ? '匹配' : '不匹配'})`;
          } else {
            hit = !isBot;
            factText = `${c.field} 判定: ${hit ? '满足' : '未满足'}`;
          }
          return { hit, text: factText };
        });
      });

      const isAnd = rule.logic !== '任一满足';
      const ruleMatched = isAnd
        ? groupResults.every((g) => g.every((c) => c.hit))
        : groupResults.some((g) => g.some((c) => c.hit));

      const facts: TraceFact[] = groupResults.flat();

      if (ruleMatched) {
        matchedRule = rule;
        rule.hits24h = (rule.hits24h || 0) + 1;
        const isBlock = rule.action.includes('拦截') || rule.action.includes('404') || rule.action.includes('限流');
        traceSteps.push({
          ruleId: rule.id,
          ruleName: rule.name,
          status: isBlock ? 'block' : 'hit',
          statusText: '命中 · 裁决',
          badgeClass: isBlock ? 'badge-danger' : 'badge-ok',
          facts,
          whyText: `全部条件满足，执行裁决动作：${rule.action} → ${rule.destination}`,
          latency: '0.3ms',
        });
      } else {
        traceSteps.push({
          ruleId: rule.id,
          ruleName: rule.name,
          status: 'skip',
          statusText: '未命中',
          badgeClass: 'badge-neutral',
          facts,
          whyText: '条件不满足，继续求值下一条规则',
          latency: '0.2ms',
        });
      }
    }

    simTraceSteps.value = traceSteps;

    if (matchedRule) {
      const isBlock = matchedRule.action.includes('拦截') || matchedRule.action.includes('404') || matchedRule.action.includes('限流');
      simVerdict.title = `命中 ${matchedRule.id} · ${matchedRule.action}`;
      simVerdict.badgeClass = isBlock ? 'badge-danger' : 'badge-ok';
      simVerdict.accentColor = isBlock ? 'var(--danger)' : 'var(--accent)';
      simVerdict.cardStyle = isBlock
        ? 'background: var(--danger-soft); border-color: color-mix(in srgb, var(--danger) 30%, transparent);'
        : 'background: var(--accent-soft); border-color: color-mix(in srgb, var(--accent) 30%, transparent);';
      simVerdict.actionText = `${matchedRule.action} → ${matchedRule.destination}`;
      simVerdict.detailText = `依据规则「${matchedRule.name}」判定，端到端执行耗时 0.4ms`;
    } else {
      simVerdict.title = '无规则命中 · 无规则兜底';
      simVerdict.badgeClass = 'badge-neutral';
      simVerdict.accentColor = 'var(--muted)';
      simVerdict.cardStyle = '';
      simVerdict.actionText = '无规则兜底 → 目标池 A';
      simVerdict.detailText = '所有启用规则均未命中，执行短链默认兜底动作放行至主目标池';
    }

    message.success('规则链模拟求值完成');
  }, 350);
}

function loadSampleBot() {
  simInput.url = 'https://example.com/promo?fbclid=IwAR27abc';
  simInput.ip = '157.240.1.35';
  simInput.ua = 'facebookexternalhit/1.1 (+http://www.facebook.com/externalhit_uatext.php)';
  simInput.lang = 'en-US,en;q=0.9';
  runSimulation();
  message.info('已载入示例并开始求值：Meta 审查爬虫');
}

function loadSampleDatacenter() {
  simInput.url = 'https://example.com/promo';
  simInput.ip = '52.95.245.14';
  simInput.ua = 'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36';
  simInput.lang = 'de-DE,de;q=0.9,en;q=0.8';
  runSimulation();
  message.info('已载入示例并开始求值：AWS 机房 IP');
}

function loadSampleMobile() {
  simInput.url = 'https://example.com/promo?ttclid=9d1f2a7c';
  simInput.ip = '189.45.71.13';
  simInput.ua = 'Mozilla/5.0 (iPhone; CPU iPhone OS 18_1 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/18.1 Mobile/15E148 Safari/604.1';
  simInput.lang = 'pt-BR,pt;q=0.9,en-US;q=0.8';
  runSimulation();
  message.info('已载入示例并开始求值：巴西真实移动访客');
}

// ==================== 名单库展开与操作 ====================
const expandedListId = ref<string | null>('L-01');
const quickItemInputs = reactive<Record<string, string>>({});

function toggleListExpand(id: string) {
  if (expandedListId.value === id) {
    expandedListId.value = null;
  } else {
    expandedListId.value = id;
  }
}

function removeListItem(list: ListItem, idx: number) {
  const item = list.items[idx];
  list.items.splice(idx, 1);
  message.info(`已从名单「${list.name}」中移除条目: ${item}`);
}

function addQuickItem(list: ListItem) {
  const val = (quickItemInputs[list.id] || '').trim();
  if (!val) return;

  const parts = val.split(/[\n,]+/).map((s) => s.trim()).filter(Boolean);
  let added = 0;
  parts.forEach((p) => {
    if (!list.items.includes(p)) {
      list.items.unshift(p);
      added++;
    }
  });

  quickItemInputs[list.id] = '';
  message.success(`已向「${list.name}」添加 ${added} 条新记录`);
}

// ==================== 导入 CIDR / CSV 弹窗 ====================
const showImportModal = ref(false);
const importForm = reactive({
  targetListId: 'L-01',
  mode: 'append' as 'append' | 'overwrite',
  text: '',
});

const parsedImportLines = computed(() => {
  return importForm.text
    .split(/[\n,]+/)
    .map((s) => s.trim())
    .filter(Boolean);
});

function handleConfirmImport() {
  const target = lists.value.find((l) => l.id === importForm.targetListId);
  if (!target) return;

  const newEntries = parsedImportLines.value;
  if (importForm.mode === 'overwrite') {
    target.items = [...newEntries];
  } else {
    newEntries.forEach((item) => {
      if (!target.items.includes(item)) target.items.push(item);
    });
  }

  message.success(`已成功向名单「${target.name}」导入 ${newEntries.length} 条数据`);
  showImportModal.value = false;
  importForm.text = '';
}

// ==================== 新建名单弹窗 ====================
const showNewListModal = ref(false);
const newListForm = reactive({
  id: 'L-08',
  name: '',
  type: 'IP 段',
  source: '自建 · 运营人工维护',
  initialItems: '',
});

watch(showNewListModal, (open) => {
  if (open) {
    const nextNum = lists.value.length + 1;
    newListForm.id = `L-${String(nextNum).padStart(2, '0')}`;
    newListForm.name = '';
    newListForm.type = 'IP 段';
    newListForm.source = '自建 · 运营人工维护';
    newListForm.initialItems = '';
  }
});

function handleCreateList() {
  const items = newListForm.initialItems
    .split(/[\n,]+/)
    .map((s) => s.trim())
    .filter(Boolean);

  const created: ListItem = {
    id: newListForm.id,
    name: newListForm.name.trim(),
    type: newListForm.type,
    source: newListForm.source.trim() || '自建',
    refRules: '—',
    items,
  };

  lists.value.push(created);
  expandedListId.value = created.id;
  showNewListModal.value = false;
  message.success(`名单「${created.name} (${created.id})」创建成功`);
}
</script>

<style scoped>
/* 局部补充或微调 */
.trace-step {
  transition: opacity 0.22s ease, transform 0.22s ease;
}
</style>
