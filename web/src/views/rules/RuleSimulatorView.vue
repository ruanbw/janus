<script setup lang="ts">
/**
 * 规则模拟器：输入一次「假想访问」，按与后端一致的语义跑一遍规则链，
 * 输出访客画像、逐步决策链和最终裁决。独立路由，不属于任何一条规则的生命周期。
 *
 * 关键前提：这是**前端等价求值**，不是服务端执行结果。正则（RE2 vs JS）、
 * 地理字段（无数据源）两处必然存在差异，页面上必须明说而不是让用户自己发现。
 */
import { computed, onMounted, ref } from 'vue';
import { useRoute, useRouter } from 'vue-router';
import {
  ArrowLeft,
  CircleAlert,
  FlaskConical,
  Globe,
  Play,
  RotateCcw,
  User,
} from '@lucide/vue';

import PageHeader from '@/components/PageHeader.vue';
import type { Rule } from '@/types/api';
import { message } from '@/utils/toast';
import { actionTagColor } from './ruleMeta';
import { buildDecisionTrace, verdictOf, type Verdict } from './ruleTrace';
import { visitorFieldViews, type SimInput, type TraceStep, type VisitorFacts } from './ruleSim';

const route = useRoute();
const router = useRouter();

const isSimulating = ref(false);
const previewRuleId = ref<number | null>(null);
const traceSteps = ref<TraceStep[]>([]);
const profile = ref<VisitorFacts | null>(null);
const scopeNote = ref('');
const verdict = ref<Verdict | null>(null);

const origin = typeof window !== 'undefined' ? window.location.origin : 'https://example.com';

const simInput = ref<SimInput>({
  url: `${origin}/promo`,
  ip: '',
  ua: typeof navigator !== 'undefined' ? navigator.userAgent : '',
  lang: typeof navigator !== 'undefined' ? navigator.language : '',
  ref: '',
});

const fieldViews = computed(() => visitorFieldViews(profile.value));

const ruleOptions = computed(() =>
  previewRuleId.value === null
    ? []
    : [{ value: String(previewRuleId.value), label: `仅看规则 #${previewRuleId.value}` }],
);

function stepToneClass(status: TraceStep['status']): string {
  if (status === 'block') return 'border-err/40 bg-danger-soft';
  if (status === 'hit') return 'border-brand-500/40 bg-accent-soft';
  return 'border-line bg-surface';
}

function verdictToneClass(): string {
  if (!verdict.value) return 'border-line bg-surface';
  if (!verdict.value.matched) return 'border-line bg-surface-muted';
  return verdict.value.blocking
    ? 'border-err/40 bg-danger-soft'
    : 'border-brand-500/40 bg-accent-soft';
}

async function runSimulation() {
  isSimulating.value = true;
  try {
    // 求值逻辑在 ruleTrace.ts：规则模拟器与短链访问明细页共用同一份实现，
    // 两处各算一次必然漂移，而漂移一次的决策链会让人照着假结论改规则。
    const trace = await buildDecisionTrace(simInput.value, { onlyRuleId: previewRuleId.value });
    profile.value = trace.facts;
    scopeNote.value = trace.scopeNote;
    traceSteps.value = trace.steps;
    if (trace.skippedForDetail > 0) {
      message.warning(
        `${trace.skippedForDetail} 条规则未取到条件（未配置条件或详情接口失败），未参与本次求值`,
      );
    }
    verdict.value = verdictOf(trace);
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
  };
}

function goBack() {
  router.push({ name: 'rules' });
}

onMounted(async () => {
  const ruleRaw = route.query.rule;
  const ruleId = Number(Array.isArray(ruleRaw) ? ruleRaw[0] : ruleRaw);
  previewRuleId.value = Number.isInteger(ruleId) && ruleId > 0 ? ruleId : null;

  // 短链访问明细页的「用此访客在模拟器打开」会带 ip / ua / referrer / url / lang 进来，
  // 直接替用户填好并跑一次。url 必带：模拟器靠它里的短码定位短链，
  // 缺了就找不到短链，于是全部 scope='links' 规则被判「不适用」。
  const pick = (key: string): string => {
    const v = route.query[key];
    return String(Array.isArray(v) ? v[0] ?? '' : v ?? '');
  };
  const ip = pick('ip');
  const ua = pick('ua');
  const referrer = pick('referrer');
  const url = pick('url');
  const lang = pick('lang');
  if (ip) simInput.value.ip = ip;
  if (ua) simInput.value.ua = ua;
  if (referrer) simInput.value.ref = referrer;
  if (url) simInput.value.url = url;
  if (lang) simInput.value.lang = lang;
  if (ip || ua || referrer || url) await runSimulation();
});
</script>

<template>
  <!-- 内容型页面与短链列表、总览一致：占满内容区，不居中限宽 -->
  <div>
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
      <AppAlert type="warning" title="这是前端等价求值预览，不是服务端执行结果">
        求值语义与后端 <code class="mono">internal/rules</code> 对齐，但两处必然有偏差：正则是 RE2（大小写敏感），这里用 JS 近似；
        国家 / ASN 字段尚无数据源，依赖它们的条件恒不命中。真实结果以线上访问记录为准。
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
            <button
              v-for="s in SAMPLES"
              :key="s.label"
              type="button"
              class="w-full rounded-lg border border-line px-3 py-2 text-left text-[13px] text-ink transition-colors hover:border-brand-500 hover:bg-accent-soft"
              @click="loadSample(s.input)"
            >
              {{ s.label }}
            </button>
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
              <span class="rounded-md bg-surface-muted px-1.5 py-0.5 text-[11px] text-ink-faint">13 字段</span>
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
                <span class="mono truncate text-[13px] text-ink" :class="f.value === '—' ? 'text-ink-faint' : ''" :title="f.value">
                  {{ f.value }}
                </span>
                <span v-if="f.pending" class="text-[11px] text-warn">{{ f.note }}</span>
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
            <p v-if="traceSteps.length === 0" class="text-[13px] text-ink-faint">
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
                <span class="text-[13px] font-semibold text-ink">{{ step.ruleName }}</span>
                <AppTag
                  :color="step.status === 'block' ? 'red' : step.status === 'hit' ? 'green' : 'default'"
                >
                  {{ step.statusText }}
                </AppTag>
              </div>
              <p class="mt-1.5 text-[13px] leading-relaxed text-ink-soft">{{ step.whyText }}</p>
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
              <p class="mt-1 text-[13px] leading-relaxed text-ink-soft">{{ verdict.detailText }}</p>
            </div>
          </CardContent>
        </AppCard>
      </div>
    </div>
  </div>
</template>
