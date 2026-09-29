<template>
  <div class="flex flex-col gap-5 pb-10" data-od-id="insights-view">
    <!-- ==================== 顶部页头与系统说明 ==================== -->
    <header class="panel" data-od-id="topbar-insights">
      <div class="panel-hd">
        <div>
          <div class="eyebrow">CLOAK / 数据洞察</div>
          <h1 class="text-xl font-bold tracking-tight text-ink md:text-2xl mt-0.5">数据洞察</h1>
          <p class="topbar-sub">
            分流结构、多维分布、规则集健康度。目标不是看总数，而是找出「放行了却不该放行」的那部分。
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

    <!-- ==================== Tab 1: 流量结构 (flow) ==================== -->
    <section v-show="currentTab === 'flow'" data-tabpanel="flow" data-od-id="traffic-mix" class="flex flex-col gap-4">
      <!-- 4 个 KPI 卡片 -->
      <div class="kpi-grid">
        <div
          v-for="kpi in flowKpis"
          :key="kpi.id"
          class="kpi"
          :data-tip="kpi.tip"
        >
          <div class="kpi-k">{{ kpi.label }}</div>
          <div class="kpi-v">{{ kpi.value }}</div>
          <div class="kpi-sub">
            <span v-if="kpi.change" :class="kpi.changeType === 'up' ? 'up' : 'dn'">{{ kpi.change }}</span>
            {{ kpi.sub }}
          </div>
          <!-- 右上角 ⓘ 悬浮气泡 -->
          <button
            type="button"
            class="kpi-info"
            :aria-label="`指标说明：${kpi.label}`"
            :aria-describedby="kpi.id"
          >
            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.9" stroke-linecap="round">
              <circle cx="12" cy="12" r="9" />
              <path d="M12 11.2v5.1M12 7.7h.01" />
            </svg>
            <span class="kpi-tip" :id="kpi.id" role="tooltip">
              {{ kpi.tip }}
            </span>
          </button>
        </div>
      </div>

      <!-- 国家/地区分布 & 流量来源 -->
      <div class="cols-2 items-stretch">
        <!-- 国家 / 地区分布 -->
        <div class="panel flex flex-col">
          <div class="panel-hd">
            <div>
              <h2>国家 / 地区分布</h2>
              <p>放行访问按访客真实 IP 归属地拆分。</p>
            </div>
          </div>
          <div class="panel-bd flex-1">
            <div class="bars">
              <div
                v-for="item in countryDistribution"
                :key="item.country"
                class="bar-row"
              >
                <span class="bar-lab">{{ item.country }}</span>
                <span class="bar-track">
                  <span
                    class="bar-fill"
                    :class="item.isAccent ? 't-accent' : ''"
                    :style="{ width: item.percent }"
                  ></span>
                </span>
                <span class="bar-val">{{ item.percent }}</span>
              </div>
            </div>
          </div>
        </div>

        <!-- 流量来源 -->
        <div class="panel flex flex-col">
          <div class="panel-hd">
            <div>
              <h2>流量来源</h2>
              <p>按 Referrer 与 UTM 参数归因到广告平台。</p>
            </div>
          </div>
          <div class="panel-bd flex-1">
            <!-- 堆积条 (stackbar) -->
            <div
              class="stackbar"
              role="img"
              aria-label="流量来源：TikTok 41.3%，Meta 33.8%，Google 18.2%，其他 6.7%"
            >
              <span style="width: 41.3%; background: var(--fg)"></span>
              <span style="width: 33.8%; background: var(--accent)"></span>
              <span style="width: 18.2%; background: color-mix(in srgb, var(--fg) 55%, var(--surface))"></span>
              <span style="width: 6.7%; background: var(--border)"></span>
            </div>

            <!-- 图例 -->
            <div class="legend">
              <span class="legend-item">
                <span class="legend-key" style="background: var(--fg)"></span>
                TikTok Ads 41.3%
              </span>
              <span class="legend-item">
                <span class="legend-key" style="background: var(--accent)"></span>
                Meta Ads 33.8%
              </span>
              <span class="legend-item">
                <span class="legend-key" style="background: color-mix(in srgb, var(--fg) 55%, var(--surface))"></span>
                Google Ads 18.2%
              </span>
              <span class="legend-item">
                <span class="legend-key" style="background: var(--border)"></span>
                直接 / 其它 6.7%
              </span>
            </div>

            <!-- 明细条形图 -->
            <div class="bars mt-4">
              <div
                v-for="source in sourceDistribution"
                :key="source.name"
                class="bar-row"
              >
                <span class="bar-lab">{{ source.name }}</span>
                <span class="bar-track">
                  <span class="bar-fill" :style="{ width: source.percent }"></span>
                </span>
                <span class="bar-val">{{ source.percent }}</span>
              </div>
            </div>
          </div>
        </div>
      </div>

      <!-- 设备类型、操作系统、访客语言分布网格 -->
      <div class="cols-3 items-stretch">
        <!-- 设备类型 -->
        <div class="panel flex flex-col">
          <div class="panel-hd">
            <div>
              <h2>设备类型</h2>
            </div>
          </div>
          <div class="panel-bd flex-1">
            <div class="bars">
              <div
                v-for="device in deviceDistribution"
                :key="device.name"
                class="bar-row"
              >
                <span class="bar-lab">{{ device.name }}</span>
                <span class="bar-track">
                  <span
                    class="bar-fill"
                    :class="device.isAccent ? 't-accent' : ''"
                    :style="{ width: device.percent }"
                  ></span>
                </span>
                <span class="bar-val">{{ device.percent }}</span>
              </div>
            </div>
          </div>
        </div>

        <!-- 操作系统 -->
        <div class="panel flex flex-col">
          <div class="panel-hd">
            <div>
              <h2>操作系统</h2>
            </div>
          </div>
          <div class="panel-bd flex-1">
            <div class="bars">
              <div
                v-for="os in osDistribution"
                :key="os.name"
                class="bar-row"
              >
                <span class="bar-lab">{{ os.name }}</span>
                <span class="bar-track">
                  <span
                    class="bar-fill"
                    :class="os.isAccent ? 't-accent' : ''"
                    :style="{ width: os.percent }"
                  ></span>
                </span>
                <span class="bar-val">{{ os.percent }}</span>
              </div>
            </div>
          </div>
        </div>

        <!-- 访客语言 -->
        <div class="panel flex flex-col">
          <div class="panel-hd">
            <div>
              <h2>访客语言</h2>
            </div>
          </div>
          <div class="panel-bd flex-1">
            <div class="bars">
              <div
                v-for="lang in languageDistribution"
                :key="lang.name"
                class="bar-row"
              >
                <span class="bar-lab">{{ lang.name }}</span>
                <span class="bar-track">
                  <span
                    class="bar-fill"
                    :class="lang.isAccent ? 't-accent' : ''"
                    :style="{ width: lang.percent }"
                  ></span>
                </span>
                <span class="bar-val">{{ lang.percent }}</span>
              </div>
            </div>
          </div>
        </div>
      </div>

      <!-- 访客语言与「语言 × 国家 交叉热力矩阵」高密分析表格 -->
      <div class="panel">
        <div class="panel-hd">
          <div>
            <h2>语言 × 国家 交叉</h2>
            <p>语言与 IP 地区一致说明流量真实；差异过大可能是代理或机器人。</p>
          </div>
          <span class="badge badge-ok mono">一致率 89.4%</span>
        </div>
        <div class="tbl-wrap">
          <table class="tbl">
            <thead>
              <tr>
                <th>语言</th>
                <th class="num">BR</th>
                <th class="num">US</th>
                <th class="num">CA</th>
                <th class="num">GB</th>
                <th class="num">DE</th>
                <th>判断</th>
              </tr>
            </thead>
            <tbody>
              <tr
                v-for="row in crossMatrixData"
                :key="row.lang"
              >
                <td class="mono font-semibold">{{ row.lang }}</td>
                <td class="num" :class="row.lang === 'pt-BR' ? 'font-semibold text-brand-600 bg-brand-500/10' : (row.lang === 'es-ES' ? 'font-medium text-amber-600 bg-amber-500/10' : '')">
                  {{ row.br }}
                </td>
                <td class="num" :class="row.lang === 'en-US' ? 'font-semibold text-brand-600 bg-brand-500/10' : (row.lang === 'ru-RU' ? 'font-medium text-err bg-err/10' : '')">
                  {{ row.us }}
                </td>
                <td class="num" :class="row.lang === 'en-US' ? 'font-semibold text-brand-600 bg-brand-500/10' : ''">
                  {{ row.ca }}
                </td>
                <td class="num">{{ row.gb }}</td>
                <td class="num">{{ row.de }}</td>
                <td>
                  <span
                    class="badge"
                    :class="{
                      'badge-ok': row.statusType === 'ok',
                      'badge-warn': row.statusType === 'warn',
                      'badge-danger': row.statusType === 'danger'
                    }"
                  >
                    {{ row.status }}
                  </span>
                </td>
              </tr>
            </tbody>
          </table>
        </div>
        <div class="panel-ft">
          表内只列出前 4 种主要语言；其余 43,689 次访问分散在另外 22 种语言中，合计与「放行访问 120,914」对齐。
        </div>
      </div>
    </section>

    <!-- ==================== Tab 2: 规则表现 (rules) ==================== -->
    <section v-show="currentTab === 'rules'" data-tabpanel="rules" data-od-id="rule-performance" class="flex flex-col gap-4">
      <div class="two-col items-start">
        <!-- 左侧：各规则命中占比条形图 (R-001 ~ R-008) -->
        <div class="panel flex flex-col">
          <div class="panel-hd">
            <div>
              <h2>各规则命中占比</h2>
              <p>首条命中即裁决。8 条规则合计 74.4%，其余 25.6% 未命中任何绑定规则，直接走短链自身的无规则兜底。</p>
            </div>
            <router-link to="/rules" class="btn btn-sm">
              去规则引擎 →
            </router-link>
          </div>
          <div class="panel-bd">
            <div class="bars">
              <div
                v-for="rule in ruleHitDistribution"
                :key="rule.code"
                class="bar-row"
              >
                <span class="bar-lab">{{ rule.code }} {{ rule.name }}</span>
                <span class="bar-track">
                  <span
                    class="bar-fill"
                    :class="rule.isAccent ? 't-accent' : ''"
                    :style="{ width: rule.percent }"
                  ></span>
                </span>
                <span class="bar-val">{{ rule.percent }}</span>
              </div>
            </div>
          </div>
        </div>

        <!-- 右侧：「需要处理的问题」卡片 -->
        <div class="panel flex flex-col">
          <div class="panel-hd">
            <div>
              <h2>需要处理的问题</h2>
              <p>规则集健康度检查结果。</p>
            </div>
            <span class="badge badge-warn mono">4 项</span>
          </div>
          <div class="panel-bd stack-sm">
            <div class="note">
              <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" class="text-amber-500 shrink-0">
                <path d="M12 9v4M12 17h.01M10.3 3.9 1.8 18a2 2 0 0 0 1.7 3h17a2 2 0 0 0 1.7-3L13.7 3.9a2 2 0 0 0-3.4 0z" />
              </svg>
              <div>
                <b>R-005 零命中。</b>条件被 R-001 完全覆盖，建议删除或调整优先级。
              </div>
            </div>
            <div class="note">
              <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" class="text-amber-500 shrink-0">
                <path d="M12 9v4M12 17h.01M10.3 3.9 1.8 18a2 2 0 0 0 1.7 3h17a2 2 0 0 0 1.7-3L13.7 3.9a2 2 0 0 0-3.4 0z" />
              </svg>
              <div>
                <b>R-001 与 R-004 重叠。</b>BR + pt-BR 的流量两个规则都能命中，建议用「单一归属」模式避免统计双算。
              </div>
            </div>
            <div class="note">
              <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" class="text-sky-500 shrink-0">
                <circle cx="12" cy="12" r="9" />
                <path d="M12 8h.01M11 12h1v4h1" />
              </svg>
              <div>
                <b>兜底 R-008 占比 6.6% 偏高。</b>说明有 8,431 次访问没能被任何一条规则描述，建议补规则或收紧白标页。
              </div>
            </div>
            <div class="note">
              <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" class="text-sky-500 shrink-0">
                <circle cx="12" cy="12" r="9" />
                <path d="M12 8h.01M11 12h1v4h1" />
              </svg>
              <div>
                <b>R-002 拦截了 2,810 次疑似正常 UA。</b>名单 L-02 覆盖过宽，建议按 UA 精确匹配替代 IP 段匹配。
              </div>
            </div>
          </div>
        </div>
      </div>

      <!-- 「规则改动后的历史回放」沙盘卡片 -->
      <div class="panel">
        <div class="panel-hd">
          <div>
            <h2>规则改动后的历史回放</h2>
            <p>把最近 7 天真实访问按新规则重跑一次，看放行率会怎么变——上线前的沙盘。</p>
          </div>
          <button
            type="button"
            class="btn btn-sm"
            :class="replayStatus === 'done' ? '' : 'btn-primary'"
            :disabled="replayStatus === 'running'"
            id="replayBtn"
            @click="handleTriggerReplay"
          >
            <RefreshCw v-if="replayStatus === 'running'" class="animate-spin" :size="13" />
            {{ replayBtnText }}
          </button>
        </div>

        <!-- 沙盘结论提示横幅 -->
        <div v-if="replayStatus === 'done'" class="px-4 pt-3">
          <div class="flex items-center justify-between gap-3 p-3 rounded-lg bg-surface-soft border border-line text-xs">
            <div class="flex items-center gap-2 text-ink">
              <span class="dot bg-accent"></span>
              <span class="font-medium">沙盘仿真结论：</span>
              <span class="text-muted">基于近 7 天 128,406 次真实访问重算，预期放行率 94.2%（与线上持平），误拦截风险评估 0.00%，规则集合规安全。</span>
            </div>
            <span class="badge badge-ok mono">无漂移风险</span>
          </div>
        </div>

        <div class="tbl-wrap">
          <table class="tbl" id="replayTbl">
            <thead>
              <tr>
                <th>规则</th>
                <th>动作</th>
                <th class="num">当前放行</th>
                <th class="num">改动后放行</th>
                <th class="num">差异</th>
                <th>评估</th>
              </tr>
            </thead>
            <tbody>
              <tr
                v-for="row in replayData"
                :key="row.rule"
              >
                <td class="mono font-semibold">{{ row.rule }}</td>
                <td>
                  <span
                    class="badge"
                    :class="row.actionType === 'ok' ? 'badge-ok' : 'badge-neutral'"
                  >
                    {{ row.action }}
                  </span>
                </td>
                <td class="num">{{ row.currentAllow }}</td>
                <td class="num">{{ row.simulatedAllow }}</td>
                <td class="num">{{ row.diff }}</td>
                <td>
                  <span
                    class="badge"
                    :class="row.assessmentType === 'warn' ? 'badge-warn' : 'badge-neutral'"
                  >
                    {{ row.assessment }}
                  </span>
                </td>
              </tr>
            </tbody>
          </table>
        </div>
        <div class="panel-ft">
          重跑在独立沙箱执行，不影响线上裁决；结果显示后再决定是否发布。
        </div>
      </div>
    </section>

    <!-- ==================== Tab 3: 回传健康度 (postback) ==================== -->
    <section v-show="currentTab === 'postback'" data-tabpanel="postback" id="postback" data-od-id="postback-health" class="flex flex-col gap-4">
      <!-- 4 个 KPI 卡片 -->
      <div class="kpi-grid">
        <div
          v-for="kpi in postbackKpis"
          :key="kpi.id"
          class="kpi"
          :data-tip="kpi.tip"
        >
          <div class="kpi-k">{{ kpi.label }}</div>
          <div class="kpi-v">
            {{ kpi.value }}<span v-if="kpi.unit" class="text-[13px] text-muted font-normal">{{ kpi.unit }}</span>
          </div>
          <div class="kpi-sub">{{ kpi.sub }}</div>
          <!-- 右上角 ⓘ 悬浮气泡 -->
          <button
            type="button"
            class="kpi-info"
            :aria-label="`指标说明：${kpi.label}`"
            :aria-describedby="kpi.id"
          >
            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.9" stroke-linecap="round">
              <circle cx="12" cy="12" r="9" />
              <path d="M12 11.2v5.1M12 7.7h.01" />
            </svg>
            <span class="kpi-tip" :id="kpi.id" role="tooltip">
              {{ kpi.tip }}
            </span>
          </button>
        </div>
      </div>

      <!-- 回传通道状态高密表格 -->
      <div class="panel">
        <div class="panel-hd">
          <div>
            <h2>回传通道</h2>
            <p>每个平台的事件队列、凭证状态与最近错误。</p>
          </div>
          <button
            type="button"
            class="btn btn-sm"
            :disabled="isRetryingAll"
            @click="handleRetryAllFailures"
          >
            <RefreshCw v-if="isRetryingAll" class="animate-spin" :size="13" />
            {{ isRetryingAll ? '正在重试…' : '重试全部失败' }}
          </button>
        </div>

        <div class="tbl-wrap">
          <table class="tbl">
            <thead>
              <tr>
                <th>通道</th>
                <th>标识</th>
                <th>事件</th>
                <th class="num">今日</th>
                <th class="num">成功率</th>
                <th class="num">P50 延迟</th>
                <th>状态</th>
                <th></th>
              </tr>
            </thead>
            <tbody>
              <tr
                v-for="channel in postbackChannels"
                :key="channel.id"
              >
                <td class="font-medium text-ink">{{ channel.name }}</td>
                <td class="mono tiny text-muted">{{ channel.identifier }}</td>
                <td class="tiny text-muted max-w-[240px] truncate" :title="channel.events">
                  {{ channel.events }}
                </td>
                <td class="num">{{ channel.todayCount }}</td>
                <td class="num">{{ channel.successRate }}</td>
                <td class="num">{{ channel.p50Latency }}</td>
                <td>
                  <span
                    class="badge"
                    :class="channel.statusType === 'ok' ? 'badge-ok' : 'badge-warn'"
                  >
                    {{ channel.status }}
                  </span>
                </td>
                <td class="shrink">
                  <div class="flex items-center gap-1.5 justify-end">
                    <button
                      v-if="channel.id === 'google-ads'"
                      type="button"
                      class="btn btn-sm text-xs py-0.5 px-2"
                      title="重试该通道待处理失败"
                      @click="handleRetryChannel(channel)"
                    >
                      重试
                    </button>
                    <button
                      type="button"
                      class="btn btn-sm text-xs py-0.5 px-2"
                      @click="openChannelConfig(channel)"
                    >
                      配置
                    </button>
                  </div>
                </td>
              </tr>
            </tbody>
          </table>
        </div>
        <div class="panel-ft">
          回传事件按短链上配置的事件列表发送；被规则拦截的访问不产生回传，避免污染平台归因。
        </div>
      </div>

      <!-- 近 14 日趋势图表 / 数据概览卡片 -->
      <div class="panel">
        <div class="panel-hd">
          <div>
            <h2>近 14 日趋势</h2>
            <p>放行、拦截与转化三条线。</p>
          </div>
          <div class="flex items-center gap-2 text-xs text-muted">
            <span class="inline-flex items-center gap-1.5">
              <span class="h-2 w-2 rounded-xs bg-ink"></span>
              放行访问 (均值 105k)
            </span>
            <span class="inline-flex items-center gap-1.5">
              <span class="h-2 w-2 rounded-xs bg-accent"></span>
              今日峰值 (128k)
            </span>
          </div>
        </div>
        <div class="panel-bd">
          <div class="bars">
            <div
              v-for="trend in trendData"
              :key="trend.date"
              class="bar-row"
            >
              <span class="bar-lab">{{ trend.date }}</span>
              <span class="bar-track">
                <span
                  class="bar-fill"
                  :class="trend.isAccent ? 't-accent' : ''"
                  :style="{ width: trend.percent }"
                ></span>
              </span>
              <span class="bar-val">{{ trend.value }}</span>
            </div>
          </div>

          <!-- 近 14 日多维汇总指标 -->
          <div class="mt-6 pt-5 border-t border-line grid grid-cols-1 sm:grid-cols-3 gap-4">
            <div class="p-3 rounded-lg bg-surface-soft border border-line">
              <div class="font-mono text-xs text-muted">14 日累计放行访问</div>
              <div class="font-mono text-xl font-semibold text-ink mt-1">1,482,900</div>
              <div class="text-xs text-muted mt-0.5">占总流量 94.1%</div>
            </div>
            <div class="p-3 rounded-lg bg-surface-soft border border-line">
              <div class="font-mono text-xs text-muted">14 日累计防御拦截</div>
              <div class="font-mono text-xl font-semibold text-ink mt-1">91,240</div>
              <div class="text-xs text-muted mt-0.5">拦截率 5.9% · 爬虫为主</div>
            </div>
            <div class="p-3 rounded-lg bg-surface-soft border border-line">
              <div class="font-mono text-xs text-muted">14 日累计转化回传</div>
              <div class="font-mono text-xl font-semibold text-ink mt-1">2,198,340</div>
              <div class="text-xs text-brand-600 font-medium mt-0.5">综合成功率 99.2%</div>
            </div>
          </div>
        </div>
      </div>
    </section>

    <!-- ==================== 通道配置与诊断弹窗 ==================== -->
    <Transition name="fade">
      <div
        v-if="isConfigModalOpen && activeChannel"
        class="fixed inset-0 z-50 flex items-center justify-center bg-black/45 backdrop-blur-xs p-4"
        @click.self="closeChannelConfig"
      >
        <div class="panel w-full max-w-lg shadow-2xl animate-in fade-in zoom-in-95 duration-150">
          <div class="panel-hd">
            <div>
              <h2>通道配置 · {{ activeChannel.name }}</h2>
              <p>通道凭证状态、上报事件映射与最近诊断日志。</p>
            </div>
            <button
              type="button"
              class="icon-btn"
              aria-label="关闭弹窗"
              @click="closeChannelConfig"
            >
              <X :size="15" />
            </button>
          </div>

          <div class="panel-bd space-y-4 text-xs">
            <!-- 通道基础信息 -->
            <div class="grid grid-cols-2 gap-3 p-3 rounded-lg bg-surface-soft border border-line">
              <div>
                <span class="text-muted block">通道标识 / Pixel ID</span>
                <span class="font-mono font-semibold text-ink text-[13px] mt-0.5 block">
                  {{ activeChannel.identifier }}
                </span>
              </div>
              <div>
                <span class="text-muted block">通道运行状态</span>
                <span
                  class="badge mt-1"
                  :class="activeChannel.statusType === 'ok' ? 'badge-ok' : 'badge-warn'"
                >
                  {{ activeChannel.status }}
                </span>
              </div>
            </div>

            <!-- 凭证与协议 -->
            <div class="space-y-1.5">
              <label class="font-medium text-ink">API 访问令牌 / 凭证</label>
              <div class="flex items-center gap-2">
                <input
                  type="password"
                  readonly
                  value="EAAGNO41x98BAZCV9482kjsfd98234jksdf87"
                  class="input mono text-xs grow bg-surface-muted"
                />
                <button
                  type="button"
                  class="btn btn-sm"
                  :disabled="isTestingConnection"
                  @click="testChannelConnection"
                >
                  <RefreshCw v-if="isTestingConnection" class="animate-spin" :size="13" />
                  {{ isTestingConnection ? '测试中…' : '测试连通性' }}
                </button>
              </div>
              <p class="text-[11px] text-muted">凭证状态：有效 · 上次通过校验于 14 分钟前</p>
            </div>

            <!-- 上报事件配置 -->
            <div class="space-y-1.5">
              <label class="font-medium text-ink">当前上报生效事件</label>
              <div class="flex flex-wrap gap-1.5 p-2 rounded-lg bg-surface-soft border border-line">
                <span
                  v-for="ev in activeChannel.events.split(', ')"
                  :key="ev"
                  class="badge badge-neutral font-mono text-[11px]"
                >
                  {{ ev }}
                </span>
              </div>
            </div>

            <!-- 最近错误诊断 -->
            <div class="space-y-1.5">
              <label class="font-medium text-ink">最近队列诊断</label>
              <div
                class="p-2.5 rounded-lg border text-[11px] font-mono leading-relaxed"
                :class="activeChannel.id === 'google-ads' ? 'bg-amber-500/10 border-amber-500/30 text-amber-700 dark:text-amber-300' : 'bg-surface-soft border-line text-muted'"
              >
                <div v-if="activeChannel.id === 'google-ads'">
                  [WARN] 12 条待处理失败 · HTTP 400 INVALID_CONVERSION_VALUE<br />
                  详情：Google Enhanced Conversions 要求货币代码必填，部分回传未带 currency 字段，已排入重试。
                </div>
                <div v-else>
                  [OK] 最近 2,000 次请求响应正常 · HTTP 200 OK · 平均耗时 {{ activeChannel.p50Latency }} · 无重发队列积压
                </div>
              </div>
            </div>
          </div>

          <div class="panel-ft flex items-center justify-between">
            <span class="text-xs text-muted">更新于 3 分钟前</span>
            <div class="btn-row">
              <button
                type="button"
                class="btn btn-sm"
                @click="closeChannelConfig"
              >
                取消
              </button>
              <button
                type="button"
                class="btn btn-sm btn-primary"
                @click="saveChannelConfig"
              >
                保存配置
              </button>
            </div>
          </div>
        </div>
      </div>
    </Transition>
  </div>
</template>

<script setup lang="ts">
import { onMounted, ref, watch } from 'vue';
import { useRoute, useRouter } from 'vue-router';
import { RefreshCw, X } from '@lucide/vue';
import { message } from '@/utils/toast';

// ==================== 选项卡控制与 URL / 本地存储同步 ====================
export type InsightsTab = 'flow' | 'rules' | 'postback';

const TAB_STORAGE_KEY = 'cloak.in.tab';

const route = useRoute();
const router = useRouter();

const currentTab = ref<InsightsTab>('flow');

/**
 * 从 route.hash 或 route.query 解析目标 Tab
 */
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

/**
 * 切换选项卡并持久化至 localStorage 与 URL hash
 */
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

onMounted(() => {
  // 1. 优先使用 URL 中指定的 hash 或 query
  const routeTab = resolveTabFromRoute();
  if (routeTab) {
    currentTab.value = routeTab;
    try {
      localStorage.setItem(TAB_STORAGE_KEY, routeTab);
    } catch {
      // 忽略
    }
    return;
  }

  // 2. 其次读取 localStorage 记忆
  try {
    const saved = localStorage.getItem(TAB_STORAGE_KEY) as InsightsTab | null;
    if (saved === 'flow' || saved === 'rules' || saved === 'postback') {
      currentTab.value = saved;
      return;
    }
  } catch {
    // 忽略
  }

  // 3. 兜底默认流量结构
  currentTab.value = 'flow';
});

// 监听路由 hash 或 query 变化 (如侧边栏直接点击 /insights#postback 导航时)
watch(
  () => [route.hash, route.query.tab],
  () => {
    const routeTab = resolveTabFromRoute();
    if (routeTab && routeTab !== currentTab.value) {
      switchTab(routeTab, false);
    }
  }
);

// ==================== Tab 1: 流量结构数据 ====================
interface FlowKpiItem {
  id: string;
  label: string;
  value: string;
  sub: string;
  change?: string;
  changeType?: 'up' | 'dn';
  tip: string;
}

const flowKpis: FlowKpiItem[] = [
  {
    id: 'kpi-tip-flow-1',
    label: '总访问',
    value: '128,406',
    sub: '对比昨日',
    change: '+12.4%',
    changeType: 'up',
    tip: '所选时间范围内到达任意短链的请求总数，含被拦截的请求。它是判断规则宽严的基准分母——所有占比型指标都除以它。',
  },
  {
    id: 'kpi-tip-flow-2',
    label: '放行访问',
    value: '120,914',
    sub: '占 94.2%',
    tip: '裁决结果为放行的访问数，包含进入真实落地页和展示白标页两种。只有这部分访客会真正看到广告对应的页面，也是广告平台会计入点击的来源。',
  },
  {
    id: 'kpi-tip-flow-3',
    label: '拦截',
    value: '7,492',
    sub: '占 5.8% · 爬虫为主',
    tip: '被规则判定为不允许的访问，直接返回 403 或丢弃，不做任何跳转。占比突然升高通常是规则写严了，或者爬虫与攻击流量抬头。',
  },
  {
    id: 'kpi-tip-flow-4',
    label: '白标页展示',
    value: '843',
    sub: '占放行访问 0.7%',
    tip: '放行但目标池里没有可用真实落地页时，返回的替代页（通常是去品牌的通用页）。它属于放行访问的子集。持续偏高说明目标池库存不足或轮询失败。',
  },
];

const countryDistribution = [
  { country: '美国 US', percent: '34.1%', isAccent: true },
  { country: '巴西 BR', percent: '22.7%', isAccent: false },
  { country: '加拿大 CA', percent: '11.4%', isAccent: false },
  { country: '英国 GB', percent: '8.2%', isAccent: false },
  { country: '德国 DE', percent: '6.0%', isAccent: false },
  { country: '其他 11 国', percent: '17.6%', isAccent: false },
];

const sourceDistribution = [
  { name: 'TikTok Ads', percent: '41.3%' },
  { name: 'Meta Ads', percent: '33.8%' },
  { name: 'Google Ads', percent: '18.2%' },
  { name: '直接 / 其它', percent: '6.7%' },
];

const deviceDistribution = [
  { name: '移动端', percent: '78.4%', isAccent: true },
  { name: '桌面端', percent: '19.1%', isAccent: false },
  { name: '平板', percent: '2.5%', isAccent: false },
];

const osDistribution = [
  { name: 'iOS', percent: '46.2%', isAccent: true },
  { name: 'Android', percent: '32.2%', isAccent: false },
  { name: 'Windows', percent: '18.3%', isAccent: false },
  { name: 'macOS', percent: '3.3%', isAccent: false },
];

const languageDistribution = [
  { name: 'en-US', percent: '48.5%', isAccent: false },
  { name: 'pt-BR / pt-PT', percent: '22.7%', isAccent: true },
  { name: 'es-ES', percent: '9.1%', isAccent: false },
  { name: 'de-DE', percent: '6.0%', isAccent: false },
  { name: '其他', percent: '13.7%', isAccent: false },
];

interface MatrixRow {
  lang: string;
  br: string;
  us: string;
  ca: string;
  gb: string;
  de: string;
  status: string;
  statusType: 'ok' | 'warn' | 'danger';
}

const crossMatrixData: MatrixRow[] = [
  {
    lang: 'pt-BR',
    br: '27,318',
    us: '412',
    ca: '96',
    gb: '41',
    de: '18',
    status: '语言与地区一致',
    statusType: 'ok',
  },
  {
    lang: 'en-US',
    br: '1,204',
    us: '38,442',
    ca: '13,806',
    gb: '2,910',
    de: '1,188',
    status: '正常',
    statusType: 'ok',
  },
  {
    lang: 'es-ES',
    br: '206',
    us: '88',
    ca: '22',
    gb: '19',
    de: '14',
    status: 'BR 流量却声明西语，疑似改写',
    statusType: 'warn',
  },
  {
    lang: 'ru-RU',
    br: '0',
    us: '41',
    ca: '0',
    gb: '0',
    de: '0',
    status: '非目标市场，已被 R-003 拦截',
    statusType: 'danger',
  },
];

// ==================== Tab 2: 规则表现数据与沙盘历史回放 ====================
interface RuleHitItem {
  code: string;
  name: string;
  percent: string;
  isAccent?: boolean;
}

const ruleHitDistribution: RuleHitItem[] = [
  { code: 'R-001', name: '目标市场', percent: '40.8%', isAccent: true },
  { code: 'R-004', name: '语言分流', percent: '22.3%', isAccent: false },
  { code: 'R-008', name: '兜底白标', percent: '6.6%', isAccent: false },
  { code: 'R-002', name: '爬虫拦截', percent: '2.8%', isAccent: false },
  { code: 'R-003', name: '代理拦截', percent: '0.9%', isAccent: false },
  { code: 'R-007', name: '频次风控', percent: '0.8%', isAccent: false },
  { code: 'R-006', name: '内部测试', percent: '0.3%', isAccent: false },
  { code: 'R-005', name: '型号白名单', percent: '0%', isAccent: false },
];

interface ReplayRow {
  rule: string;
  action: string;
  actionType: 'ok' | 'neutral';
  currentAllow: string;
  simulatedAllow: string;
  diff: string;
  assessment: string;
  assessmentType: 'neutral' | 'warn';
}

const replayData: ReplayRow[] = [
  {
    rule: 'R-001',
    action: '放行',
    actionType: 'ok',
    currentAllow: '52,411',
    simulatedAllow: '52,411',
    diff: '0',
    assessment: '无变化',
    assessmentType: 'neutral',
  },
  {
    rule: 'R-004',
    action: '放行',
    actionType: 'ok',
    currentAllow: '28,603',
    simulatedAllow: '28,603',
    diff: '0',
    assessment: '无变化',
    assessmentType: 'neutral',
  },
  {
    rule: 'R-005',
    action: '已停用',
    actionType: 'neutral',
    currentAllow: '0',
    simulatedAllow: '0',
    diff: '0',
    assessment: '被 R-001 覆盖，可删',
    assessmentType: 'warn',
  },
];

// 沙盘重跑状态交互
const replayStatus = ref<'idle' | 'running' | 'done'>('idle');
const replayBtnText = ref('用当前规则重跑近 7 天');

function handleTriggerReplay() {
  replayStatus.value = 'running';
  replayBtnText.value = '回放中…';

  window.setTimeout(() => {
    replayStatus.value = 'done';
    replayBtnText.value = '回放完成：无显著差异';
    message.success('历史回放完成：沙盒校验通过，线上放行率稳定无异常漂移');
  }, 1600);

  window.setTimeout(() => {
    // 恢复按钮可点击状态，保留结论
    if (replayStatus.value === 'done') {
      // 允许再次点击重测
    }
  }, 3000);
}

// ==================== Tab 3: 回传健康度数据 ====================
interface PostbackKpiItem {
  id: string;
  label: string;
  value: string;
  unit?: string;
  sub: string;
  tip: string;
}

const postbackKpis: PostbackKpiItem[] = [
  {
    id: 'kpi-tip-pb-1',
    label: '回传成功率',
    value: '99.1%',
    sub: '失败 75 次 · 已自动重试 3 次',
    tip: '推送到各平台的事件中收到成功响应的比例。失败事件会自动进入重试队列；长期低于 95% 会让广告平台侧的转化归因失真，反过来影响投放模型的学习。',
  },
  {
    id: 'kpi-tip-pb-2',
    label: '回传延迟 P50',
    value: '1.4',
    unit: 's',
    sub: 'P99 9.2s · 超 8s 需关注',
    tip: '回传请求从发出到平台返回结果的耗时中位数（P50 表示一半的事件比它更快）。延迟过高时，平台可能把事件判定为超时直接丢弃。',
  },
  {
    id: 'kpi-tip-pb-3',
    label: '今日回传',
    value: '171,204',
    sub: 'View · Click · Purchase',
    tip: '今天推送给广告平台的事件总条数（ViewContent / Click / Purchase）。这是回传队列的吞吐指标，用来看回传跟不跟得上访问量，不是转化数。',
  },
  {
    id: 'kpi-tip-pb-4',
    label: '待处理失败',
    value: '12',
    sub: '主要为 4xx 凭证过期',
    tip: '已经重试到上限、仍未成功的回传事件数。多数是 access token 过期或平台侧限流，处理前先看下方回传通道表里的「最近错误」。',
  },
];

interface PostbackChannelItem {
  id: string;
  name: string;
  identifier: string;
  events: string;
  todayCount: string;
  successRate: string;
  p50Latency: string;
  status: '正常' | '部分失败';
  statusType: 'ok' | 'warn';
}

const postbackChannels = ref<PostbackChannelItem[]>([
  {
    id: 'tiktok',
    name: 'TikTok Events API',
    identifier: 'C1A9…7Q',
    events: 'ViewContent, Click, Purchase',
    todayCount: '70,714',
    successRate: '99.4%',
    p50Latency: '1.2s',
    status: '正常',
    statusType: 'ok',
  },
  {
    id: 'meta',
    name: 'Meta Conversions API',
    identifier: '8842…1',
    events: 'ViewContent, InitiatedCheckout, Purchase',
    todayCount: '57,802',
    successRate: '99.2%',
    p50Latency: '1.6s',
    status: '正常',
    statusType: 'ok',
  },
  {
    id: 'google-ads',
    name: 'Google Ads',
    identifier: 'AW-7712…',
    events: 'PageView, Lead, Purchase',
    todayCount: '34,110',
    successRate: '98.6%',
    p50Latency: '2.1s',
    status: '部分失败',
    statusType: 'warn',
  },
  {
    id: 'custom-webhook',
    name: '自有回调',
    identifier: 'api.north…/conv',
    events: '全事件',
    todayCount: '8,578',
    successRate: '100%',
    p50Latency: '0.3s',
    status: '正常',
    statusType: 'ok',
  },
]);

const isRetryingAll = ref(false);

function handleRetryAllFailures() {
  isRetryingAll.value = true;
  window.setTimeout(() => {
    isRetryingAll.value = false;
    message.success('已触发重试 12 条待处理失败事件，任务已进入回传队列');
  }, 900);
}

function handleRetryChannel(channel: PostbackChannelItem) {
  message.info(`正在为通道 [${channel.name}] 重新投递 12 条失败事件…`);
  window.setTimeout(() => {
    message.success(`[${channel.name}] 失败事件已重新投递，等待远端 ACK`);
  }, 600);
}

// 通道配置弹窗
const isConfigModalOpen = ref(false);
const activeChannel = ref<PostbackChannelItem | null>(null);
const isTestingConnection = ref(false);

function openChannelConfig(channel: PostbackChannelItem) {
  activeChannel.value = { ...channel };
  isConfigModalOpen.value = true;
}

function closeChannelConfig() {
  isConfigModalOpen.value = false;
  activeChannel.value = null;
}

function testChannelConnection() {
  isTestingConnection.value = true;
  window.setTimeout(() => {
    isTestingConnection.value = false;
    message.success(`通道 [${activeChannel.value?.name}] 连通性测试通过，远端返回 200 OK (耗时 184ms)`);
  }, 700);
}

function saveChannelConfig() {
  message.success(`通道 [${activeChannel.value?.name}] 配置保存成功`);
  closeChannelConfig();
}

// 近 14 日趋势图表数据
const trendData = [
  { date: '12-06', percent: '72%', value: '92,140', isAccent: false },
  { date: '12-08', percent: '78%', value: '99,870', isAccent: false },
  { date: '12-10', percent: '83%', value: '106,402', isAccent: false },
  { date: '12-12', percent: '79%', value: '101,033', isAccent: false },
  { date: '12-14', percent: '88%', value: '112,908', isAccent: false },
  { date: '12-16', percent: '100%', value: '128,406', isAccent: true },
];
</script>

<style scoped>
.fade-enter-active,
.fade-leave-active {
  transition: opacity 0.16s ease;
}

.fade-enter-from,
.fade-leave-to {
  opacity: 0;
}
</style>
