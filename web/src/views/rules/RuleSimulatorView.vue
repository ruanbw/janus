<script setup lang="ts">
/**
 * 规则模拟器：输入一次「假想访问」，由**后端**跑一遍规则链，
 * 输出访客画像、逐步决策链和最终裁决。独立路由，不属于任何一条规则的生命周期。
 *
 * 判定完全在后端：Simulate 与线上 Evaluate 共用同一套条件求值函数与同一套匹配顺序。
 * 曾经这里跑的是一份前端等价求值，正则（RE2 vs JS）与 GeoIP（浏览器没有数据源）
 * 两处必然偏差——那不是「近似」，而是会让人照着假结论去改规则。
 */
import { computed, onMounted, ref } from 'vue';
import { useRoute, useRouter } from 'vue-router';
import {
  ArrowLeft,
  CircleAlert,
  Eye,
  FlaskConical,
  Globe,
  Play,
  RotateCcw,
  User,
  X,
} from '@lucide/vue';

import { fetchTenantErrorPages } from '@/api/me';
import { getRule, simulateRules } from '@/api/rules';
import PageHeader from '@/components/PageHeader.vue';
import { COUNTRY_OPTIONS } from '@/constants/countries';
import type { Rule } from '@/types/api';
import { message } from '@/utils/toast';
import { actionTagColor } from './ruleMeta';
import {
  traceStepsFromServer,
  verdictFromServer,
  visitorFactsFromServer,
  visitorFieldViews,
  type SimInput,
  type TraceStep,
  type VerdictView,
  type VisitorFacts,
} from './ruleSim';

const route = useRoute();
const router = useRouter();

const isSimulating = ref(false);
const previewRuleId = ref<number | null>(null);
const traceSteps = ref<TraceStep[]>([]);
const profile = ref<VisitorFacts | null>(null);
const scopeNote = ref('');
const verdict = ref<VerdictView | null>(null);

const previewModalVisible = ref(false);
const previewModalTitle = ref('');
const previewModalHtml = ref('');
const loadingPreview = ref(false);

async function previewVisitorBlockedPage() {
  if (!verdict.value) return;
  const action = verdict.value.action;
  loadingPreview.value = true;
  try {
    previewModalTitle.value = action === 'notfound' ? '404 访客拦截页面预览' : '429 访客限流页面预览';

    // 1. 规则专属自定义页面。裁决只回传 ruleId,规则现取:
    //    裁决本身不需要把整条规则带回来。
    const ruleId = verdict.value.matchedRuleId;
    const rule: Rule | null = ruleId ? await getRule(ruleId) : null;
    if (rule?.pageMode === 'custom' && rule.customHtml) {
      previewModalHtml.value = rule.customHtml;
      previewModalVisible.value = true;
      return;
    }

    // 2. 租户全局自定义页面
    const ep = await fetchTenantErrorPages();
    if (action === 'notfound' && ep.custom404Html) {
      previewModalHtml.value = ep.custom404Html;
      previewModalVisible.value = true;
      return;
    }
    if (action === 'throttle' && ep.custom429Html) {
      previewModalHtml.value = ep.custom429Html;
      previewModalVisible.value = true;
      return;
    }

    // 3. 系统默认页面
    if (action === 'throttle') {
      previewModalHtml.value = `<!DOCTYPE html><html lang="zh-CN"><head><meta charset="utf-8"><title>429 - 请求过多</title><style>body{font-family:system-ui,-apple-system,sans-serif;display:flex;align-items:center;justify-content:center;height:100vh;margin:0;background:#0f172a;color:#f8fafc;text-align:center;}h1{font-size:5rem;margin:0;color:#f59e0b;}p{color:#94a3b8;font-size:1.1rem;}</style></head><body><div><h1>429</h1><p>请求过于频繁，请稍后再试</p></div></body></html>`;
    } else {
      previewModalHtml.value = `<!DOCTYPE html><html lang="zh-CN"><head><meta charset="utf-8"><title>404 - 页面未找到</title><style>body{font-family:system-ui,-apple-system,sans-serif;display:flex;align-items:center;justify-content:center;height:100vh;margin:0;background:#0f172a;color:#f8fafc;text-align:center;}h1{font-size:5rem;margin:0;color:#38bdf8;}p{color:#94a3b8;font-size:1.1rem;}</style></head><body><div><h1>404</h1><p>页面不存在或链接已失效</p></div></body></html>`;
    }
    previewModalVisible.value = true;
  } catch (error) {
    message.error(error instanceof Error ? error.message : '加载拦截页面失败');
  } finally {
    loadingPreview.value = false;
  }
}

const origin = typeof window !== 'undefined' ? window.location.origin : 'https://example.com';

const countryOptions = COUNTRY_OPTIONS.map((c) => ({ value: c.value, label: c.label }));

const simInput = ref<SimInput>({
  url: `${origin}/promo`,
  ip: '',
  ua: typeof navigator !== 'undefined' ? navigator.userAgent : '',
  lang: typeof navigator !== 'undefined' ? navigator.language : '',
  ref: '',
  country: '',
});

const fieldViews = computed(() => visitorFieldViews(profile.value));

const ruleOptions = computed(() =>
  previewRuleId.value === null
    ? []
    : [{ value: String(previewRuleId.value), label: `仅看规则 #${previewRuleId.value}` }],
);

function stepToneClass(status: TraceStep['status']): string {
  if (status === 'block') return 'border-err/40 bg-err/10';
  if (status === 'hit') return 'border-primary/50 bg-primary/10';
  if (status === 'disabled') return 'border-line bg-surface-muted opacity-70';
  return 'border-line bg-surface';
}

function verdictToneClass(): string {
  if (!verdict.value) return 'border-line bg-surface';
  if (!verdict.value.matched) return 'border-line bg-surface-muted';
  return verdict.value.blocking
    ? 'border-err/40 bg-err/10'
    : 'border-primary/50 bg-primary/10';
}

async function runSimulation() {
  isSimulating.value = true;
  try {
    // 切到后端求值后:画像、决策链、裁决全部来自 internal/rules 那一份实现,
      // 与线上访问同一套顺序。这里只做「后端结果 → 页面渲染形态」的映射。
      const res = await simulateRules({
        url: simInput.value.url,
        ip: simInput.value.ip,
        userAgent: simInput.value.ua,
        acceptLanguage: simInput.value.lang,
        referrer: simInput.value.ref,
        manualCountry: simInput.value.country,
        onlyRuleId: previewRuleId.value,
      });
      profile.value = visitorFactsFromServer(res.facts);
      scopeNote.value = res.scopeNote;
      traceSteps.value = traceStepsFromServer(res.steps, res.verdict);
      verdict.value = verdictFromServer(res.verdict);
      if (res.error) {
        // 后端求值期异常会被兜住并 fail-open(线上不 500),但模拟器要把它显示出来,
        // 否则用户会把「出错了」误读成「没命中」。
        message.warning(`后端求值有异常,结果可能不完整：${res.error}`);
      }
      message.success('规则链模拟求值完成');
  } catch (error) {
    message.error(error instanceof Error ? error.message : '模拟求值失败，请稍后重试');
  } finally {
    isSimulating.value = false;
  }
}

const SAMPLES: { label: string; input: SimInput }[] = [
  {
    label: 'Facebook 爬虫',
    input: {
      url: `${origin}/promo?fbclid=IwAR27abc`,
      ip: '157.240.1.35',
      country: 'US',
      ua: 'facebookexternalhit/1.1 (+http://www.facebook.com/externalhit_uatext.php)',
      lang: 'en-US,en;q=0.9',
      ref: 'https://www.facebook.com/',
    },
  },
  {
    label: '机房 IP + Chrome',
    input: {
      url: `${origin}/promo`,
      ip: '52.95.245.14',
      country: 'US',
      ua: 'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36',
      lang: 'de-DE,de;q=0.9,en;q=0.8',
      ref: 'https://news.ycombinator.com/',
    },
  },
  {
    label: '微信内 iPhone',
    input: {
      url: `${origin}/promo?utm_source=wechat`,
      ip: '189.45.71.13',
      country: 'BR',
      ua: 'Mozilla/5.0 (iPhone; CPU iPhone OS 18_1 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/18.1 Mobile/15E148 Safari/604.1',
      lang: 'pt-BR,pt;q=0.9,en-US;q=0.8',
      ref: 'https://www.google.com/',
    },
  },
];

function loadSample(sample: SimInput) {
  simInput.value = { ...sample };
  void runSimulation();
}

function resetInput() {
  simInput.value = {
    url: `${origin}/promo`,
    ip: '',
    ua: typeof navigator !== 'undefined' ? navigator.userAgent : '',
    lang: typeof navigator !== 'undefined' ? navigator.language : '',
    ref: '',
    country: '',
  };
}

function goBack() {
  router.push({ name: 'rules' });
}

onMounted(async () => {
  const ruleRaw = route.query.rule;
  const ruleId = Number(Array.isArray(ruleRaw) ? ruleRaw[0] : ruleRaw);
  previewRuleId.value = Number.isInteger(ruleId) && ruleId > 0 ? ruleId : null;

  // 短链访问明细页的「用此访客在模拟器打开」会带 ip / ua / referrer / url / lang / country 进来，
  // 直接替用户填好并跑一次。url 必带：模拟器靠它里的短码定位短链，
  // 缺了就找不到短链，于是全部 scope='links' 规则被判「不适用」。
  // country 也带：那条链路上后端已经查出了真实国家码，带过来回放才和当时的裁决对得上。
  const pick = (key: string): string => {
    const v = route.query[key];
    return String(Array.isArray(v) ? v[0] ?? '' : v ?? '');
  };
  const ip = pick('ip');
  const ua = pick('ua');
  const referrer = pick('referrer');
  const url = pick('url');
  const lang = pick('lang');
  const country = pick('country');
  if (ip) simInput.value.ip = ip;
  if (ua) simInput.value.ua = ua;
  if (referrer) simInput.value.ref = referrer;
  if (url) simInput.value.url = url;
  if (lang) simInput.value.lang = lang;
  if (country) simInput.value.country = country;
  if (ip || ua || referrer || url) await runSimulation();
});
</script>

<template>
  <div>
    <!-- 内容型页面与短链列表、总览一致：占满内容区，不居中限宽 -->
    <PageHeader
      title="规则模拟器"
      description="用一次假想访问跑遍规则链，看清每条规则为什么命中或被跳过。改动规则前先在这里验一遍，比上线后翻访问日志快得多。"
    >
      <template #actions>
        <AppButton @click="goBack">
          <template #icon><ArrowLeft :size="15" /></template>
          返回规则列表
        </AppButton>
      </template>
    </PageHeader>

    <div class="mb-4">
      <AppAlert type="info" title="与线上同一套求值">
        判定由后端 <code class="mono">internal/rules</code> 完成，与真实访问同一套条件语义与匹配顺序（首条命中即停）。
        国家码由后端离线库按 IP 解析；<code class="mono">asn</code> 尚无数据源，依赖它的条件恒不命中。
      </AppAlert>
    </div>

    <div class="grid grid-cols-1 gap-6 xl:grid-cols-5">
      <!-- 左：输入 -->
      <div class="space-y-6 xl:col-span-2">
        <AppCard :padding="false">
          <CardHeader>
            <CardTitle class="flex items-center gap-2">
              <User :size="18" class="text-brand-600 dark:text-brand-400" />
              访客输入
            </CardTitle>
            <CardDescription>留空的字段按「取不到值」处理，依赖它的条件会判不成立</CardDescription>
          </CardHeader>
          <CardContent class="space-y-4">
            <div>
              <label class="mb-1.5 block text-sm font-medium text-ink">访问 URL</label>
              <AppInput v-model="simInput.url" placeholder="https://go.example.com/promo" />
            </div>
            <div>
              <label class="mb-1.5 block text-sm font-medium text-ink">访客 IP</label>
              <AppInput v-model="simInput.ip" placeholder="203.0.113.7" />
            </div>
            <div>
              <label class="mb-1.5 block text-sm font-medium text-ink">国家 / 地区</label>
              <AppSelect
                v-model="simInput.country"
                :options="countryOptions"
                show-search
                allow-clear
                placeholder="留空 = 由后端按 IP 解析"
              />
              <p class="mt-1 text-2xs leading-relaxed text-ink-faint">
                ISO 国家码。留空由后端离线库按访客 IP 解析，与真实裁决一致；
                手填则覆盖 GeoIP 结果，用于验证「假如他来自这个国家」。
              </p>
            </div>
            <div>
              <label class="mb-1.5 block text-sm font-medium text-ink">User-Agent</label>
              <AppTextarea v-model="simInput.ua" :rows="2" placeholder="Mozilla/5.0 …" />
            </div>
            <div class="grid grid-cols-1 gap-3 sm:grid-cols-2">
              <div>
                <label class="mb-1.5 block text-sm font-medium text-ink">Accept-Language</label>
                <AppInput v-model="simInput.lang" placeholder="zh-CN,zh;q=0.9" />
              </div>
              <div>
                <label class="mb-1.5 block text-sm font-medium text-ink">Referrer</label>
                <AppInput v-model="simInput.ref" placeholder="https://www.google.com/" />
              </div>
            </div>

            <div class="flex flex-wrap gap-2">
              <AppButton type="primary" :loading="isSimulating" @click="runSimulation">
                <template #icon><Play :size="15" /></template>
                运行模拟
              </AppButton>
              <AppButton :disabled="isSimulating" @click="resetInput">
                <template #icon><RotateCcw :size="15" /></template>
                重置
              </AppButton>
            </div>

            <div v-if="ruleOptions.length > 0" class="rounded-lg bg-surface-muted px-3 py-2 text-xs text-ink-soft">
              预览范围限定为 <span class="mono">#{{ previewRuleId }}</span>，其余规则不参与本次求值。
            </div>
          </CardContent>
        </AppCard>

        <AppCard :padding="false">
          <CardHeader>
            <CardTitle class="flex items-center gap-2">
              <FlaskConical :size="18" class="text-brand-600 dark:text-brand-400" />
              示例访客
            </CardTitle>
            <CardDescription>载入后立即运行，用来快速验证「爬虫 / 机房 IP / 微信内」三类典型流量</CardDescription>
          </CardHeader>
          <CardContent class="space-y-2">
            <AppButton
              v-for="s in SAMPLES"
              :key="s.label"
              variant="outline"
              class="w-full justify-start font-normal text-left h-auto py-2 px-3 text-ink"
              @click="loadSample(s.input)"
            >
              {{ s.label }}
            </AppButton>
          </CardContent>
        </AppCard>
      </div>

      <!-- 右：画像 + 决策链 + 裁决 -->
      <div class="space-y-6 xl:col-span-3">
        <AppCard :padding="false">
          <CardHeader>
            <CardTitle class="flex items-center gap-2">
              <Globe :size="18" class="text-brand-600 dark:text-brand-400" />
              访客画像
              <span class="rounded-md bg-surface-muted px-1.5 py-0.5 text-2xs text-ink-faint">13 字段</span>
            </CardTitle>
            <CardDescription>{{ scopeNote || '运行模拟后，这里显示从请求解析出的可判定字段' }}</CardDescription>
          </CardHeader>
          <CardContent>
            <div class="grid gap-px overflow-hidden rounded-lg border border-line bg-line sm:grid-cols-2">
              <div
                v-for="f in fieldViews"
                :key="f.field"
                class="flex min-w-0 flex-col gap-0.5 bg-surface px-3 py-2"
              >
                <span class="text-xs text-ink-faint">{{ f.label }}</span>
                <span class="mono truncate text-sm text-ink" :class="f.value === '—' ? 'text-ink-faint' : ''" :title="f.value">
                  {{ f.value }}
                </span>
                <span v-if="f.pending" class="text-2xs text-warn">{{ f.note }}</span>
              </div>
            </div>
          </CardContent>
        </AppCard>

        <AppCard :padding="false">
          <CardHeader>
            <CardTitle>决策链</CardTitle>
            <CardDescription>按优先级升序逐条求值，首条命中即定案；「已跳过」的规则根本没被求值</CardDescription>
          </CardHeader>
          <CardContent class="space-y-3">
            <p v-if="traceSteps.length === 0" class="text-sm text-ink-faint">
              还没有运行过模拟。填好左侧输入后点「运行模拟」。
            </p>

            <div
              v-for="step in traceSteps"
              :key="step.key"
              class="rounded-lg border px-4 py-3"
              :class="stepToneClass(step.status)"
            >
              <div class="flex flex-wrap items-center gap-2">
                <span class="mono text-xs text-ink-faint">#{{ step.ruleId }}</span>
                <span class="text-sm font-semibold text-ink">{{ step.ruleName }}</span>
                <AppTag
                  :color="step.status === 'block' ? 'red' : step.status === 'hit' ? 'green' : 'default'"
                >
                  {{ step.statusText }}
                </AppTag>
              </div>
              <p class="mt-1.5 text-sm leading-relaxed text-ink-soft">{{ step.whyText }}</p>
              <ul v-if="step.facts.length > 0" class="mt-2 space-y-1">
                <li
                  v-for="(fact, i) in step.facts"
                  :key="i"
                  class="mono flex items-start gap-2 text-xs"
                  :class="fact.hit ? 'text-ink' : 'text-ink-faint'"
                >
                  <CircleAlert :size="12" class="mt-0.5 shrink-0" :class="fact.hit ? 'text-ok' : 'text-ink-faint'" />
                  <span class="break-all">{{ fact.text }}</span>
                </li>
              </ul>
            </div>
          </CardContent>
        </AppCard>

        <AppCard v-if="verdict" :padding="false">
          <CardHeader>
            <CardTitle>裁决结果</CardTitle>
            <CardDescription>这次访问最终会怎么走</CardDescription>
          </CardHeader>
          <CardContent>
            <div class="rounded-lg border px-4 py-4" :class="verdictToneClass()">
              <div class="flex flex-wrap items-center gap-2">
                <span class="text-xs font-semibold text-ink-faint">裁决结果</span>
                <AppTag :color="verdict.matched ? actionTagColor(verdict.action as Rule['action']) : 'default'">
                  {{ verdict.title }}
                </AppTag>
              </div>
              <div class="mt-2 text-lg font-semibold text-ink">{{ verdict.actionText }}</div>
              <p class="mt-1 text-sm leading-relaxed text-ink-soft">{{ verdict.detailText }}</p>

              <!-- 404 / 429 访客页面预览操作 -->
              <div
                v-if="verdict.action === 'notfound' || verdict.action === 'throttle'"
                class="mt-3 pt-3 border-t border-line/60 flex items-center justify-between"
              >
                <span class="text-xs text-ink-faint">
                  访客端将收到 HTTP {{ verdict.action === 'notfound' ? '404' : '429' }} 网页响应
                </span>
                <AppButton size="small" :loading="loadingPreview" @click="previewVisitorBlockedPage">
                  <template #icon><Eye :size="14" /></template>
                  预览访客端拦截页面
                </AppButton>
              </div>
            </div>
          </CardContent>
        </AppCard>
      </div>
    </div>

    <!-- 访客拦截页面沙箱预览弹窗 -->
    <Teleport to="body">
      <div
        v-if="previewModalVisible"
        class="fixed inset-0 z-[100] flex items-center justify-center bg-black/60 p-4 backdrop-blur-xs"
        @click.self="previewModalVisible = false"
      >
        <div class="flex h-[85vh] w-full max-w-4xl flex-col rounded-xl border border-line bg-surface shadow-2xl overflow-hidden">
          <div class="flex items-center justify-between border-b border-line px-5 py-3 bg-surface-muted/50">
            <div class="flex items-center gap-2">
              <Eye :size="16" class="text-brand-600 dark:text-brand-400" />
              <span class="text-sm font-semibold text-ink">{{ previewModalTitle }}</span>
              <span class="text-xs text-ink-faint">已开启 sandbox 安全隔离</span>
            </div>
            <AppButton
              size="icon"
              variant="ghost"
              aria-label="关闭预览"
              @click="previewModalVisible = false"
            >
              <X :size="18" />
            </AppButton>
          </div>
          <div class="flex-1 p-3 bg-line/20">
            <iframe
              :srcdoc="previewModalHtml"
              sandbox="allow-same-origin"
              class="h-full w-full rounded border border-line bg-background shadow-xs"
            />
          </div>
        </div>
      </div>
    </Teleport>
  </div>
</template>
