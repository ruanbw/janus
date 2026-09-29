<template>
  <div class="space-y-5 pb-10">
    <!-- 系统定位说明 -->
    <section class="panel">
      <div class="panel-bd">
        <div class="max-w-[78ch]">
          <p class="eyebrow">系统定位</p>
          <h2 class="text-[20px] font-[640] tracking-[-0.018em] text-ink">
            斗篷（cloaking）是一层「准入裁决」，不是一层「跳转」
          </h2>
          <p class="tiny muted mt-2">
            广告平台审查员、爬虫、同行与无效流量都会访问投放链接。斗篷在请求到达时先做访客画像，再用一组有序规则裁决：放行到真实落地页、返回白标页、限流，或直接丢弃。判断依据包括 IP 归属与网络属性、国家地区、设备与系统、浏览器、语言、Referrer 与设备指纹。下面的 4 个模块就是这条裁决链路的完整实现。
          </p>
        </div>
      </div>
    </section>

    <!-- KPI 卡片网格 -->
    <section class="kpi-grid">
      <div
        v-for="kpi in kpiList"
        :key="kpi.label"
        class="kpi"
      >
        <div class="kpi-k">{{ kpi.label }}</div>
        <div class="kpi-v">
          {{ kpi.value }}<span v-if="kpi.unit" class="text-[14px] text-muted font-normal">{{ kpi.unit }}</span>
        </div>
        <div class="kpi-sub">
          <span v-if="kpi.change" :class="kpi.changeType === 'up' ? 'up' : 'dn'">{{ kpi.change }}</span>
          {{ kpi.sub }}
        </div>

        <!-- 说明提示气泡 -->
        <button
          type="button"
          class="kpi-info"
          :aria-label="`指标说明：${kpi.label}`"
        >
          <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.9" stroke-linecap="round">
            <circle cx="12" cy="12" r="9" />
            <path d="M12 11.2v5.1M12 7.7h.01" />
          </svg>
          <span class="kpi-tip" role="tooltip">
            {{ kpi.tip }}
          </span>
        </button>
      </div>
    </section>

    <!-- 决策结果构成 + 拦截原因 TOP -->
    <section>
      <div class="cols-2 items-stretch">
        <!-- 今日决策结果构成 -->
        <div class="panel flex flex-col">
          <div class="panel-hd">
            <div>
              <h2>今日决策结果构成</h2>
              <p>每次访问只会落到一个结果，四类互斥，合计等于今日访问总量。</p>
            </div>
            <router-link to="/visit-stream" class="btn btn-sm">
              看逐条决策 →
            </router-link>
          </div>
          <div class="panel-bd flex-1">
            <div
              class="stackbar"
              role="img"
              aria-label="今日决策结果：真实落地页 120,071 次占 93.5%，白标页 843 次占 0.7%，限流 512 次占 0.4%，拦截丢弃 6,980 次占 5.4%"
            >
              <span class="bg-ink" style="width: 93.5%"></span>
              <span class="bg-warn" style="width: 0.7%"></span>
              <span class="bg-slate-400" style="width: 0.4%"></span>
              <span class="bg-err" style="width: 5.4%"></span>
            </div>

            <div class="legend">
              <span class="legend-item">
                <span class="legend-key bg-ink"></span>
                真实落地页
              </span>
              <span class="legend-item">
                <span class="legend-key bg-warn"></span>
                白标页
              </span>
              <span class="legend-item">
                <span class="legend-key bg-slate-400"></span>
                限流
              </span>
              <span class="legend-item">
                <span class="legend-key bg-err"></span>
                拦截丢弃
              </span>
            </div>

            <div class="bars mt-4">
              <div class="bar-row bar-row-lg">
                <span class="bar-lab">真实落地页</span>
                <span class="bar-track">
                  <span class="bar-fill" style="width: 93.5%"></span>
                </span>
                <span class="bar-val">120,071 · 93.5%</span>
              </div>
              <div class="bar-row bar-row-lg">
                <span class="bar-lab">白标页</span>
                <span class="bar-track">
                  <span class="bar-fill t-warn" style="width: 0.7%"></span>
                </span>
                <span class="bar-val">843 · 0.7%</span>
              </div>
              <div class="bar-row bar-row-lg">
                <span class="bar-lab">限流</span>
                <span class="bar-track">
                  <span class="bar-fill bg-slate-400" style="width: 0.4%"></span>
                </span>
                <span class="bar-val">512 · 0.4%</span>
              </div>
              <div class="bar-row bar-row-lg">
                <span class="bar-lab">拦截丢弃</span>
                <span class="bar-track">
                  <span class="bar-fill t-danger" style="width: 5.4%"></span>
                </span>
                <span class="bar-val">6,980 · 5.4%</span>
              </div>
            </div>
          </div>
          <div class="panel-ft">
            白标页与限流的占比是健康度信号：白标页高说明目标池库存不足，限流高说明频次阈值可能卡得过紧。
          </div>
        </div>

        <!-- 拦截原因 TOP -->
        <div class="panel flex flex-col">
          <div class="panel-hd">
            <div>
              <h2>拦截原因 TOP</h2>
              <p>拦截动作按触发它的条件归类，合计 7,492 次。</p>
            </div>
            <router-link to="/rules" class="btn btn-sm">
              调整规则 →
            </router-link>
          </div>
          <div class="panel-bd flex-1">
            <div class="bars">
              <div class="bar-row bar-row-lg">
                <span class="bar-lab">爬虫识别</span>
                <span class="bar-track">
                  <span class="bar-fill t-danger" style="width: 100%"></span>
                </span>
                <span class="bar-val">4,420 · 59%</span>
              </div>
              <div class="bar-row bar-row-lg">
                <span class="bar-lab">代理 / VPN 出口</span>
                <span class="bar-track">
                  <span class="bar-fill t-danger" style="width: 33.9%"></span>
                </span>
                <span class="bar-val">1,498 · 20%</span>
              </div>
              <div class="bar-row bar-row-lg">
                <span class="bar-lab">单 IP 频次超限</span>
                <span class="bar-track">
                  <span class="bar-fill t-danger" style="width: 27.1%"></span>
                </span>
                <span class="bar-val">1,199 · 16%</span>
              </div>
              <div class="bar-row bar-row-lg">
                <span class="bar-lab">名单命中</span>
                <span class="bar-track">
                  <span class="bar-fill t-danger" style="width: 8.5%"></span>
                </span>
                <span class="bar-val">375 · 5%</span>
              </div>
            </div>
          </div>
          <div class="panel-ft">
            同一 IP 可能同时符合多条拦截规则，按优先级只计入第一条命中的原因，因此各项相加等于拦截总数。
          </div>
        </div>
      </div>
    </section>

    <!-- 能力模块导航网格 -->
    <section class="panel">
      <div class="panel-hd">
        <div>
          <h2>能力模块</h2>
          <p>四个模块构成完整斗篷系统。点击进入可交互的高保真控制面板。</p>
        </div>
        <span class="badge badge-neutral mono">4 个模块</span>
      </div>
      <div class="panel-bd">
        <div class="mod-grid">
          <router-link to="/links" class="mod-card">
            <span class="mod-ico">
              <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.7" stroke-linecap="round" stroke-linejoin="round">
                <path d="M10 13a5 5 0 0 0 7 0l3-3a5 5 0 0 0-7-7l-1 1M14 11a5 5 0 0 0-7 0l-3 3a5 5 0 0 0 7 7l1-1" />
              </svg>
            </span>
            <h3>短链与目标</h3>
            <p>每条短链绑定一组规则、一个兜底动作与一个目标池；支持跳转型与落地页型、301/302、权重轮询、定时上下线。</p>
            <div class="mod-list">
              已实现部分 + 待补：<br />
              规则绑定 · 目标权重 · 排期 · Referrer 剥离 · 转化回传绑定
            </div>
            <span class="mod-go">进入模块</span>
          </router-link>

          <router-link to="/rules" class="mod-card">
            <span class="mod-ico">
              <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.7" stroke-linecap="round" stroke-linejoin="round">
                <path d="M4 6h16M4 12h10M4 18h7M17 15l3 3-3 3" />
              </svg>
            </span>
            <h3>规则引擎</h3>
            <p>按优先级串行裁决。条件维度覆盖 IP/CIDR/ASN、代理与机房、国家地区、设备型号与系统、浏览器、语言、时区、Referrer、指纹。</p>
            <div class="mod-list">
              规则编辑器 · 条件构造器 · 优先级 · 模拟器 · 冲突检测 · 命中率预估 · 名单库
            </div>
            <span class="mod-go">进入模块</span>
          </router-link>

          <router-link to="/visit-stream" class="mod-card">
            <span class="mod-ico">
              <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.7" stroke-linecap="round" stroke-linejoin="round">
                <path d="M3 12h4l3 8 4-16 3 8h4" />
              </svg>
            </span>
            <h3>访问决策流</h3>
            <p>每次访问的完整决策链留痕：画像字段、逐条规则求值结果、最终动作与落地 URL，可回放、可一键封禁。</p>
            <div class="mod-list">
              实时流 · 决策回放 · 多维过滤 · 封禁入名单 · 慢请求排查
            </div>
            <span class="mod-go">进入模块</span>
          </router-link>

          <router-link to="/insights" class="mod-card">
            <span class="mod-ico">
              <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.7" stroke-linecap="round" stroke-linejoin="round">
                <path d="M4 19V9M10 19V5M16 19v-7M22 19H2" />
              </svg>
            </span>
            <h3>数据洞察</h3>
            <p>放行/拦截结构、国家与设备分布、语言构成、各规则命中率与拦截贡献、规则改动后的回放沙盘、回传通道健康度。</p>
            <div class="mod-list">
              分流结构 · 多维分布 · 语言×国家交叉 · 规则命中 · 回放沙盘 · 回传延迟
            </div>
            <span class="mod-go">进入模块</span>
          </router-link>
        </div>
      </div>
    </section>

    <!-- 合规提示 -->
    <section class="panel">
      <div class="panel-bd">
        <div class="note">
          <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round">
            <circle cx="12" cy="12" r="9" />
            <path d="M12 8h.01M11 12h1v4h1" />
          </svg>
          <div>
            <b>合规提示。</b>斗篷的本质是「对不同访问者返回不同内容」。用于绕过平台审核或对审核员定向展示白标内容，可能违反 TikTok / Meta / Google 的广告政策并导致账户封禁。本系统按「流量准入控制 + 品牌合规」的正向用途设计：拦截爬虫与无效流量、地域与语言适配、转化归因。落地生产前请确认业务场景合规性。
          </div>
        </div>
      </div>
    </section>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import { useAuthStore } from '@/stores/auth';

const auth = useAuthStore();

interface KPIItem {
  label: string;
  value: string;
  unit?: string;
  change?: string;
  changeType?: 'up' | 'down';
  sub: string;
  tip: string;
}

const activeLinksCount = computed(() => {
  if (auth.tenant?.usage?.links) {
    return auth.tenant.usage.links.toLocaleString();
  }
  return '1,284';
});

const activeDomainsCount = computed(() => {
  if (auth.tenant?.usage?.domains) {
    return auth.tenant.usage.domains;
  }
  return 6;
});

const kpiList = computed<KPIItem[]>(() => [
  {
    label: '活跃短链',
    value: activeLinksCount.value,
    sub: `分布在 ${activeDomainsCount.value} 个已激活域名 · 8 条规则`,
    tip: '状态为「启用」、且至少绑定 1 个已激活域名的短链数量。已停用、或排期尚未开始的短链不计入。短链是斗篷的入口单位——投放链接、落地页入口都挂在一个短链上。',
  },
  {
    label: '今日访问',
    value: '128,406',
    change: '+12.4%',
    changeType: 'up',
    sub: '对比昨日同时段',
    tip: '今天 00:00 起所有域名收到的短链请求总数，含被拦截的请求。每一次请求都会触发一次裁决，无论最终放行还是丢弃，所以它不等于访客人数。',
  },
  {
    label: '放行率',
    value: '94.2%',
    sub: '拦截 7,492 次 · 爬虫占 59%',
    tip: '裁决结果为放行的访问占全部访问的比例，包含进入真实落地页和展示白标页两种。剩下的部分为限流与拦截。放行率过低会直接浪费广告预算，过高通常意味着规则写得太松。',
  },
  {
    label: '决策延迟 P50',
    value: '11',
    unit: 'ms',
    sub: 'P99 46ms · 规则求值 8 条',
    tip: '从收到请求到返回重定向或落地页的耗时中位数（P50 表示一半的请求比它更快）。这是斗篷链路自己吃掉的时间，必须远低于广告平台对跳转耗时的容忍上限。',
  },
]);
</script>
