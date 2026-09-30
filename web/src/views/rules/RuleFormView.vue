<script setup lang="ts">
/**
 * 规则表单：新建 /rules/new 与编辑 /rules/:id/edit 共用这一个组件。
 *
 * 编辑是表单操作，不该和列表挤在一个 tab 里——列表要能翻页、筛选、对比，
 * 表单要能专注改一条规则，状态互不污染，所以它是一条独立路由。
 */
import { computed, onMounted, reactive, ref } from 'vue';
import { useRoute, useRouter } from 'vue-router';
import {
  ArrowLeft,
  Ban,
  Check,
  CirclePlay,
  Filter,
  FlaskConical,
  Globe,
  Link2,
  Plus,
  Trash2,
} from '@lucide/vue';

import { listLinks } from '@/api/links';
import { createRule, deleteRule, getRule, updateRule, type RuleCreatePayload } from '@/api/rules';
import PageHeader from '@/components/PageHeader.vue';
import { confirm } from '@/components/ui/confirm';
import type { FormRule } from '@/components/ui/types';
import type { Rule, RuleCondition, RuleScope } from '@/types/api';
import { message } from '@/utils/toast';
import {
  ACTION_OPTIONS,
  FIELD_OPTIONS,
  LOGIC_OPTIONS,
  OPERATOR_OPTIONS,
  actionLabel,
  actionTagColor,
  fieldOption,
  operatorLabel,
} from './ruleMeta';

const route = useRoute();
const router = useRouter();

const LINK_CATALOG_PAGE_SIZE = 50;
let keySeq = 0;
function nextKey(prefix: string): string {
  keySeq += 1;
  return `${prefix}-${keySeq}`;
}

interface EditableCondition {
  key: string;
  field: string;
  operator: string;
  raw: string;
}

const loading = ref(false);
const saving = ref(false);
const formRef = ref<InstanceType<typeof import('@/components/ui/AppForm.vue')['default']> | null>(null);

const form = reactive({
  name: '',
  description: '',
  priority: 100,
  scope: 'global' as RuleScope,
  enabled: true,
  logic: 'all',
  action: 'pass',
  destination: '',
  linkIds: [] as number[],
  conditions: [] as EditableCondition[],
});

const validRuleId = computed(() => {
  const raw = route.params.id;
  const id = Number(Array.isArray(raw) ? raw[0] : raw);
  return Number.isInteger(id) && id > 0 ? id : undefined;
});
const isEdit = computed(() => route.name === 'rule-edit' && validRuleId.value !== undefined);

const headerDescription = computed(() =>
  isEdit.value
    ? `规则 #${validRuleId.value} 的配置。保存后立即按新条件参与裁决。`
    : '新建规则。规则在短链可用性之后裁决：按优先级升序逐条求值，首条命中即定案。',
);

const rules: Record<string, FormRule[]> = {
  name: [{ required: true, message: '请填写规则名称' }],
  destination: [
    {
      validator: (_rule, value) => {
        if (form.action !== 'redirect') return Promise.resolve();
        const url = String(value ?? '').trim();
        if (!url) return Promise.reject(new Error('动作为「重定向到指定 URL」时必须填写改写目标'));
        if (!/^https?:\/\/\S+$/i.test(url)) {
          return Promise.reject(new Error('改写目标必须是完整的 http(s) URL'));
        }
        return Promise.resolve();
      },
    },
  ],
};

// ---- 短链目录（作用域选择器） ----
const linkCatalog = ref<{ id: number; code: string; domains: string[] }[]>([]);
const linkCatalogPage = ref(0);
const linkCatalogTotal = ref(0);
const linkCatalogLoading = ref(false);

const linkCatalogHasMore = computed(() => linkCatalog.value.length < linkCatalogTotal.value);
const linkOptions = computed(() =>
  linkCatalog.value.map((l) => ({
    value: l.id,
    // 与后端 linkNames 的 `短码@域名` 保持同一形态（短码在租户内不唯一，必须带域名）
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
    linkCatalog.value = append
      ? [...linkCatalog.value, ...res.items.map((l) => ({ id: l.id, code: l.code, domains: l.domains }))]
      : res.items.map((l) => ({ id: l.id, code: l.code, domains: l.domains }));
  } catch (error) {
    message.error(error instanceof Error ? error.message : '加载短链列表失败，请稍后重试');
  } finally {
    linkCatalogLoading.value = false;
  }
}

// ---- 表单状态 ----
function newCondition(): EditableCondition {
  return { key: nextKey('cond'), field: 'ip', operator: 'in', raw: '' };
}

function resetForm() {
  form.name = '';
  form.description = '';
  form.priority = 100;
  form.scope = 'global';
  form.enabled = true;
  form.logic = 'all';
  form.action = 'pass';
  form.destination = '';
  form.linkIds = [];
  form.conditions = [newCondition()];
}

const fieldOptions = computed(() =>
  FIELD_OPTIONS.map((f) => ({
    value: f.value as string,
    label: f.pending ? `${f.label}（数据源待接入）` : f.label,
    // 数据源未接入的字段置灰：它是「能配不能跑」的死选项（spec D5）
    disabled: f.pending === true,
  })),
);
const operatorOptions = computed(() => OPERATOR_OPTIONS.map((o) => ({ value: o.value as string, label: o.label })));
const logicOptions = LOGIC_OPTIONS.map((l) => ({ value: l.value as string, label: l.label }));
const actionOptions = ACTION_OPTIONS.map((a) => ({
  value: a.value as string,
  label: a.label,
  desc: a.desc,
}));
const scopeOptions = [
  { value: 'global', label: '全局' },
  { value: 'links', label: '指定短链' },
];

/**
 * 回填已关联短链：直接用后端返回的完整 linkIds。
 * 不用短码反推——短码在租户内不唯一（唯一性是「同一域名下短码不重复」），
 * 同一短码可以属于不同短链，按短码猜出来的关联天然歧义。
 */
function applyRule(rule: Rule) {
  form.name = rule.name;
  form.description = rule.description;
  form.priority = rule.priority;
  form.scope = rule.scope;
  form.enabled = rule.enabled;
  form.logic = rule.logic;
  form.action = rule.action;
  form.destination = rule.destination;
  form.linkIds = (rule.linkIds || []).slice();
  form.conditions = (
    rule.conditions && rule.conditions.length > 0
      ? rule.conditions
      : [{ field: 'ip', operator: 'in', values: [] } as unknown as RuleCondition]
  ).map((c: RuleCondition) => ({
    key: nextKey('cond'),
    field: c.field,
    operator: c.operator,
    raw: (c.values || []).join(', '),
  }));
}

const summaryChips = computed(() =>
  form.conditions.map((cond) => {
    const opt = fieldOption(cond.field);
    const raw = cond.raw.trim();
    return {
      key: cond.key,
      text: `${opt?.label ?? cond.field} ${operatorLabel(cond.operator)} ${raw || '（空）'}`,
    };
  }),
);

const scopeSummaryText = computed(() =>
  form.scope === 'global' ? '本租户的全部' : `所关联的 ${form.linkIds.length} 条`,
);

function setScope(scope: string) {
  // 保留已选的 linkIds：切回「指定短链」时不用重选；真正保存为 global 时会提交空数组清理关联
  form.scope = scope as RuleScope;
}

function onFieldChange(cond: EditableCondition) {
  const opt = fieldOption(cond.field);
  if (opt) cond.operator = opt.defaultOp;
}

function addCondition() {
  form.conditions.push(newCondition());
}

function removeCondition(key: string) {
  form.conditions = form.conditions.filter((c) => c.key !== key);
  if (form.conditions.length === 0) form.conditions.push(newCondition());
}

function conditionHint(field: string): string {
  const opt = fieldOption(field);
  if (!opt) return '';
  return opt.pending
    ? '该字段的数据源尚未接入，现在保存也会恒不命中'
    : `取值示例：${opt.placeholder}${opt.hint ? ` · ${opt.hint}` : ''}`;
}

/** 表单态取值 → 后端契约：按换行/逗号/分号切分，丢弃空项 */
function buildConditions(): RuleCondition[] {
  return form.conditions.map((cond, idx) => {
    const opt = fieldOption(cond.field);
    if (!opt) {
      throw new Error(`第 ${idx + 1} 条条件的字段「${cond.field}」不在后端支持的 13 个字段内`);
    }
    if (opt.pending) {
      throw new Error(`第 ${idx + 1} 条条件使用了「${opt.label}」，该字段数据源尚未接入，无法保存`);
    }
    const op = OPERATOR_OPTIONS.find((o) => o.value === cond.operator);
    if (!op) {
      // 多为经 API 直写入的 duplicated：后端恒不命中，不在当前版本的可选范围里
      throw new Error(
        `第 ${idx + 1} 条条件的运算符「${cond.operator}」当前版本不可选（恒不命中，未接入数据源），请改为其他运算符`,
      );
    }
    // regex / gt / lt 不做多值切分：后端把 values 当 JSON 数组原样透传，
    // 按逗号/分号切会把 `^/promo{1,3}$` 撕成 `^/promo{1` 和 `3}$` 两条，
    // 两条都编译失败 → 条件被丢弃 → 规则静默失真。
    const single = cond.operator === 'regex' || cond.operator === 'gt' || cond.operator === 'lt';
    const values = single
      ? [cond.raw.trim()].filter(Boolean)
      : cond.raw
          .split(/[\n,;]+/)
          .map((s) => s.trim())
          .filter(Boolean);
    if (values.length === 0) {
      throw new Error(`第 ${idx + 1} 条条件（「${opt.label}」）的取值不能为空`);
    }
    return { field: opt.value, operator: op.value, values };
  });
}

function goBack() {
  router.push({ name: 'rules' });
}

function previewInSimulator() {
  if (validRuleId.value === undefined) return;
  router.push({ name: 'rule-simulator', query: { rule: String(validRuleId.value) } });
}

function handleDelete() {
  const id = validRuleId.value;
  if (id === undefined) return;
  confirm({
    title: `删除规则「${form.name || `#${id}`}」？`,
    content: '删除后不可恢复，正在命中这条规则的访问会立刻回落到后续规则或短链自身流程。',
    okText: '确认删除',
    cancelText: '取消',
    danger: true,
    onOk: async () => {
      try {
        await deleteRule(id);
        message.success('规则已删除');
        router.push({ name: 'rules' });
      } catch (error) {
        message.error(error instanceof Error ? error.message : '删除规则失败，请稍后重试');
      }
    },
  });
}

async function doSave(payload: RuleCreatePayload, linkIds: number[]) {
  saving.value = true;
  try {
    if (isEdit.value && validRuleId.value !== undefined) {
      await updateRule(validRuleId.value, payload);
      message.success(`规则「${payload.name}」已保存并生效`);
    } else {
      await createRule(payload);
      message.success(`规则「${payload.name}」已创建并生效`);
    }
    goBack();
  } catch (error) {
    message.error(error instanceof Error ? error.message : '保存规则失败，请稍后重试');
  } finally {
    saving.value = false;
  }
}

async function onSubmit() {
  const ok = await formRef.value?.validate();
  if (!ok) return;

  const name = form.name.trim();
  if (!name) {
    message.error('请填写规则名称');
    return;
  }

  let conditions: RuleCondition[];
  try {
    conditions = buildConditions();
  } catch (error) {
    message.error(error instanceof Error ? error.message : '条件配置有误，请检查');
    return;
  }

  // 编辑器与后端状态精确一致（关联直接回填自 rule.linkIds），因此始终提交完整的 linkIds：
  // 传即整体替换，不传则不动关联；scope=global 传空数组，与后端「切回全局即清空关联」的语义一致。
  const linkIds = form.scope === 'links' ? [...form.linkIds] : [];
  const payload: RuleCreatePayload = {
    name,
    description: form.description.trim(),
    priority: (() => {
      // 不能用 `Math.trunc(x) || 100`：priority=0 是 falsy，会被抣成 100。
      // 0 的语义正是「最先求值」（后端 priority 升序），静默改成 100 是行为变更。
      const p = Math.trunc(Number(form.priority));
      return Number.isFinite(p) && p >= 0 ? p : 100;
    })(),
    scope: form.scope,
    enabled: form.enabled,
    logic: form.logic as RuleCreatePayload['logic'],
    action: form.action as RuleCreatePayload['action'],
    destination: form.action === 'redirect' ? form.destination.trim() : '',
    conditions,
    linkIds,
  };

  // spec D2：零关联的 scoped 规则永远不命中，保存前必须让租户明确知道自己在做什么
  if (payload.scope === 'links' && linkIds.length === 0) {
    confirm({
      title: '该规则未关联任何短链，保存后不会命中',
      content: '作用域为「指定短链」但关联数为 0 的规则永远不会命中，也不会退化成全局规则。确定要保存吗？',
      okText: '仍然保存',
      cancelText: '去关联短链',
      onOk: () => doSave(payload, linkIds),
    });
    return;
  }

  await doSave(payload, linkIds);
}

/**
 * 编辑已有规则时，目录只加载了前几页，而 rule.linkIds 可能指向后面的短链。
 * 不补拉的话 AppSelect 会把缺失的项显示成裸 id（labelOf 退化为 String(value)），
 * 用户看不到是哪条短链，而保存时 linkIds 是整体替换 → 静默丢关联。
 * 逐页翻到全部命中或翻完为止；翻完仍未命中的（已删除/跨租户不可见）显式标出。
 */
async function ensureLinksLoaded(ids: number[]): Promise<void> {
  for (let guard = 0; guard < 20; guard += 1) {
    if (!linkCatalogHasMore.value) break;
    const missing = ids.filter((id) => !linkCatalog.value.some((l) => l.id === id));
    if (missing.length === 0) break;
    const before = linkCatalog.value.length;
    await loadLinkCatalog(true);
    if (linkCatalog.value.length === before) break;
  }
  const unresolved = ids.filter((id) => !linkCatalog.value.some((l) => l.id === id));
  if (unresolved.length > 0) {
    message.warning(
      `有 ${unresolved.length} 条已关联短链未能加载（可能已删除）：${unresolved.join('、')}`,
    );
  }
}

async function init() {
  if (isEdit.value && validRuleId.value !== undefined) {
    loading.value = true;
    try {
      // 列表接口不含 conditions 与完整 linkIds，必须取详情
      const detail = await getRule(validRuleId.value);
      applyRule(detail);
    } catch (error) {
      message.error(error instanceof Error ? error.message : '加载规则详情失败，请稍后重试');
      router.push({ name: 'rules' });
      return;
    } finally {
      loading.value = false;
    }
  } else {
    resetForm();
  }
  await loadLinkCatalog();
  if (form.linkIds.length > 0) {
    await ensureLinksLoaded(form.linkIds);
  }
}

onMounted(init);
</script>

<template>
  <div class="mx-auto max-w-6xl">
    <PageHeader :title="isEdit ? '编辑规则' : '新建规则'" :description="headerDescription">
      <template #actions>
        <AppButton @click="goBack">
          <template #icon><ArrowLeft :size="15" /></template>
          返回列表
        </AppButton>
      </template>
    </PageHeader>

    <AppSpin :spinning="loading">
      <AppForm ref="formRef" :model="form as unknown as Record<string, unknown>" :rules="rules">
        <div class="grid grid-cols-1 gap-6 lg:grid-cols-3">
          <!-- 左侧 2 列：表单主操作区 -->
          <div class="space-y-6 lg:col-span-2">
            <!-- 模块 1：基本信息 -->
            <AppCard :padding="false">
              <CardHeader>
                <CardTitle class="flex items-center gap-2">
                  <Filter :size="18" class="text-brand-600 dark:text-brand-400" />
                  基本信息
                </CardTitle>
                <CardDescription>名称用于列表识别，优先级决定它在裁决队列里的先后位置</CardDescription>
              </CardHeader>
              <CardContent class="space-y-5">
                <AppFormItem name="name" label="规则名称" extra="建议能一眼看出「拦什么、放什么」，例如「爬虫 UA 拦截」。">
                  <AppInput v-model="form.name" placeholder="例如：Googlebot 限流" :maxlength="60" />
                </AppFormItem>

                <AppFormItem name="description" label="规则说明" extra="可选。会随规则一起保存，便于后续交接。">
                  <AppTextarea
                    v-model="form.description"
                    :rows="2"
                    :maxlength="200"
                    placeholder="例如：拦掉主流搜索引擎的爬虫，避免污染访问统计"
                  />
                </AppFormItem>

                <div class="grid grid-cols-1 gap-5 sm:grid-cols-2">
                  <AppFormItem name="priority" label="优先级" extra="数字越小越先求值；命中即定案，后面的规则不再参与。">
                    <AppInputNumber v-model="form.priority" :min="0" :max="9999" :step="10" />
                  </AppFormItem>

                  <AppFormItem
                    name="enabled"
                    label="启用状态"
                    extra="停用后规则保留在列表里，但不参与求值。"
                  >
                    <label class="flex h-9 items-center">
                      <AppCheckbox :checked="form.enabled" @change="(v: boolean) => (form.enabled = v)">
                        {{ form.enabled ? '启用后立即参与裁决' : '当前停用' }}
                      </AppCheckbox>
                    </label>
                  </AppFormItem>
                </div>

                <AppFormItem
                  name="logic"
                  label="条件组合方式"
                  extra="「全部满足」需要每条条件都成立；「任一满足」只要有一条成立即可。"
                >
                  <AppRadioGroup v-model="form.logic" class="grid grid-cols-1 gap-3 sm:grid-cols-2">
                    <AppRadioCard
                      v-for="opt in logicOptions"
                      :key="opt.value"
                      :value="opt.value"
                      :title="opt.label"
                    />
                  </AppRadioGroup>
                </AppFormItem>
              </CardContent>
            </AppCard>

            <!-- 模块 2：作用域 -->
            <AppCard :padding="false">
              <CardHeader>
                <CardTitle class="flex items-center gap-2">
                  <Globe :size="18" class="text-brand-600 dark:text-brand-400" />
                  作用范围
                </CardTitle>
                <CardDescription>
                  规则只在作用范围内生效。关联是唯一写入口——短链表单里勾选规则改的是同一条关联数据。
                </CardDescription>
              </CardHeader>
              <CardContent class="space-y-5">
                <AppRadioGroup v-model="form.scope" class="grid grid-cols-1 gap-3 sm:grid-cols-2" @change="setScope">
                  <AppRadioCard
                    value="global"
                    title="全局"
                    description="对本租户的全部短链生效，访问任何短码都参与求值。"
                    :icon="Globe"
                  />
                  <AppRadioCard
                    value="links"
                    title="指定短链"
                    description="只对下面勾选的短链生效；一条都不勾等于永不命中。"
                    :icon="Link2"
                  />
                </AppRadioGroup>

                <template v-if="form.scope === 'links'">
                  <AppFormItem name="linkIds" label="关联短链" extra="短码在租户内不唯一，这里按「短码@域名」区分。">
                    <AppSelect
                      v-model="form.linkIds"
                      :options="linkOptions"
                      multiple
                      show-search
                      allow-clear
                      placeholder="选择要作用于此规则的短链"
                      :loading="linkCatalogLoading"
                    />
                  </AppFormItem>

                  <div class="flex items-center justify-between">
                    <span class="text-xs text-ink-faint">
                      已加载 {{ linkCatalog.length }} / {{ linkCatalogTotal }} 条短链
                    </span>
                    <AppButton
                      v-if="linkCatalogHasMore"
                      size="small"
                      :loading="linkCatalogLoading"
                      @click="loadLinkCatalog(true)"
                    >
                      加载更多
                    </AppButton>
                  </div>

                  <AppAlert v-if="form.linkIds.length === 0" type="warning" title="未关联短链 · 不会命中">
                    作用域为「指定短链」但关联数为 0 的规则永远不会命中，也不会退化成全局规则。
                  </AppAlert>
                </template>
              </CardContent>
            </AppCard>

            <!-- 模块 3：条件 -->
            <AppCard :padding="false">
              <CardHeader>
                <CardTitle class="flex items-center gap-2">
                  <Check :size="18" class="text-brand-600 dark:text-brand-400" />
                  命中条件
                </CardTitle>
                <CardDescription>
                  条件值直接写在这里：IP 支持 CIDR 网段，枚举用逗号分隔，正则选「正则匹配」。不需要额外的名单库。
                </CardDescription>
              </CardHeader>
              <CardContent class="space-y-4">
                <div
                  v-for="(cond, idx) in form.conditions"
                  :key="cond.key"
                  class="rounded-xl border border-line bg-surface-muted/40 p-4"
                >
                  <div class="mb-3 flex items-center justify-between">
                    <span class="text-xs font-semibold text-ink-soft">条件 {{ idx + 1 }}</span>
                    <AppButton size="small" type="text" danger @click="removeCondition(cond.key)">
                      <template #icon><Trash2 :size="14" /></template>
                      删除
                    </AppButton>
                  </div>

                  <div class="grid grid-cols-1 gap-3 sm:grid-cols-2">
                    <AppSelect
                      :model-value="cond.field"
                      :options="fieldOptions"
                      placeholder="选择字段"
                      @change="(v: string) => { cond.field = v; onFieldChange(cond); }"
                    />
                    <AppSelect
                      :model-value="cond.operator"
                      :options="operatorOptions"
                      placeholder="选择运算符"
                      @change="(v: string) => (cond.operator = v)"
                    />
                  </div>

                  <div class="mt-3">
                    <AppInput
                      :model-value="cond.raw"
                      placeholder="取值，逗号 / 换行分隔可写多个"
                      @update:model-value="(v: string) => (cond.raw = v)"
                    />
                  </div>

                  <p class="mt-2 text-xs text-ink-faint">{{ conditionHint(cond.field) }}</p>
                </div>

                <AppButton @click="addCondition">
                  <template #icon><Plus :size="15" /></template>
                  添加条件
                </AppButton>
              </CardContent>
            </AppCard>

            <!-- 模块 4：命中动作 -->
            <AppCard :padding="false">
              <CardHeader>
                <CardTitle class="flex items-center gap-2">
                  <CirclePlay :size="18" class="text-brand-600 dark:text-brand-400" />
                  命中动作
                </CardTitle>
                <CardDescription>规则命中后对这次访问的处理方式</CardDescription>
              </CardHeader>
              <CardContent class="space-y-5">
                <AppRadioGroup v-model="form.action" class="grid grid-cols-1 gap-3 sm:grid-cols-2">
                  <AppRadioCard
                    v-for="opt in actionOptions"
                    :key="opt.value"
                    :value="opt.value"
                    :title="opt.label"
                    :description="opt.desc"
                    :icon="opt.value === 'notfound' || opt.value === 'throttle' ? Ban : CirclePlay"
                  />
                </AppRadioGroup>

                <AppFormItem
                  v-if="form.action === 'redirect'"
                  name="destination"
                  label="改写目标 URL"
                  extra="该地址不参与短链目标池轮询，直接把访问改写到这里。"
                >
                  <AppInput v-model="form.destination" placeholder="https://example.com/black-friday" />
                </AppFormItem>
              </CardContent>
            </AppCard>
          </div>

          <!-- 右侧 1 列：实时摘要 + 提交 -->
          <div class="lg:col-span-1">
            <div class="sticky top-20 space-y-4">
              <AppCard :padding="false">
                <CardHeader>
                  <CardTitle>规则摘要</CardTitle>
                  <CardDescription>当访问者满足下面所有条件时</CardDescription>
                </CardHeader>
                <CardContent class="space-y-4">
                  <p class="text-[13px] leading-relaxed text-ink">
                    当作用于 <b class="text-ink">{{ scopeSummaryText }}</b> 短链时：
                  </p>

                  <div class="flex flex-wrap gap-1.5">
                    <span
                      v-for="chip in summaryChips"
                      :key="chip.key"
                      class="mono rounded-md bg-surface-muted px-2 py-1 text-xs text-ink-soft"
                    >
                      {{ chip.text }}
                    </span>
                    <span v-if="summaryChips.length === 0" class="text-xs text-ink-faint">尚未添加有效条件</span>
                  </div>

                  <div class="flex flex-wrap items-center gap-2">
                    <span class="text-xs text-ink-faint">则：</span>
                    <AppTag :color="actionTagColor(form.action as Rule['action'])">
                      {{ actionLabel(form.action as Rule['action']) }}
                    </AppTag>
                    <span v-if="form.action === 'redirect'" class="mono break-all text-xs text-ink-soft">
                      → {{ form.destination || '未填写目标 URL' }}
                    </span>
                    <span v-else class="text-xs text-ink-faint">→ 短链自身流程</span>
                  </div>

                  <dl class="grid grid-cols-2 gap-y-2 border-t border-line pt-4 text-[13px]">
                    <dt class="text-ink-faint">作用域</dt>
                    <dd class="text-right text-ink">
                      {{ form.scope === 'global' ? '全局' : `${form.linkIds.length} 条短链` }}
                    </dd>
                    <dt class="text-ink-faint">条件条数</dt>
                    <dd class="text-right text-ink">{{ form.conditions.length }} 条</dd>
                    <dt class="text-ink-faint">状态</dt>
                    <dd class="text-right text-ink">{{ form.enabled ? '已启用' : '已停用' }}</dd>
                  </dl>
                </CardContent>
              </AppCard>

              <AppCard>
                <div class="space-y-3">
                  <AppButton type="primary" block :loading="saving" @click="onSubmit">
                    {{ isEdit ? '保存规则' : '创建规则' }}
                  </AppButton>
                  <AppTooltip
                    :title="isEdit ? '用当前表单条件跑一遍模拟器' : '先保存，规则才有 id 可供模拟'"
                  >
                    <AppButton block :disabled="!isEdit" @click="previewInSimulator">
                      <template #icon><FlaskConical :size="15" /></template>
                      在模拟器中预览
                    </AppButton>
                  </AppTooltip>
                  <AppButton v-if="isEdit" type="text" danger block @click="handleDelete">
                    <template #icon><Trash2 :size="15" /></template>
                    删除规则
                  </AppButton>
                </div>
              </AppCard>
            </div>
          </div>
        </div>
      </AppForm>
    </AppSpin>
  </div>
</template>
