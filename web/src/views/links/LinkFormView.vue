<template>
  <div>
    <PageHeader :title="isEdit ? '编辑短链' : '创建短链'" :description="headerDescription">
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
          <!-- 左侧 2 列:表单主操作区 -->
          <div class="space-y-6 lg:col-span-2">
            <!-- 模块 1:基础与类型 -->
            <AppCard :padding="false">
              <CardHeader>
                <CardTitle class="flex items-center gap-2">
                  <Hash :size="18" class="text-brand" />
                  基本信息与类型
                </CardTitle>
                <CardDescription>
                  设置短链的公开标识短码与行为方式（创建后短码不可更改）
                </CardDescription>
              </CardHeader>
              <CardContent class="space-y-5">
                <AppFormItem name="code" label="短码" :extra="codeExtra">
                  <AppInput
                    v-model="form.code"
                    placeholder="留空自动生成"
                    :maxlength="SHORT_CODE_MAX_LENGTH"
                    :disabled="isEdit"
                  >
                    <template #prefix>
                      <Hash :size="15" />
                    </template>
                  </AppInput>
                </AppFormItem>

                <AppFormItem
                  name="linkType"
                  label="短链类型"
                  extra="跳转型直接重定向到目标 URL；落地页型先展示中间页面，访问者点击按钮经 SDK 计数后再到达目标。"
                >
                  <AppRadioGroup v-model="form.linkType" class="grid grid-cols-1 gap-3 sm:grid-cols-2">
                    <AppRadioCard
                      value="redirect"
                      title="跳转型"
                      description="访问即立即重定向到目标 URL，支持多目标轮询分发"
                      :icon="ExternalLink"
                    />
                    <AppRadioCard
                      value="landing"
                      title="落地页型"
                      description="访问先展示落地页，点击按钮经 JS SDK 计数后到达目标"
                      :icon="Layout"
                    />
                  </AppRadioGroup>
                </AppFormItem>
              </CardContent>
            </AppCard>

            <!-- 模块 2:路由目标与落地页配置 -->
            <AppCard :padding="false">
              <CardHeader>
                <CardTitle class="flex items-center gap-2">
                  <Link2 :size="18" class="text-brand" />
                  {{ form.linkType === 'redirect' ? '目标 URL 与跳转机制' : '落地页与最终目标' }}
                </CardTitle>
                <CardDescription>
                  {{
                    form.linkType === 'redirect'
                      ? '配置访问者重定向的目的地与 HTTP 缓存策略'
                      : '配置落地页来源以及落地页按钮点击后跳转的最终目标'
                  }}
                </CardDescription>
              </CardHeader>
              <CardContent class="space-y-6">
                <!-- 跳转型:重定向状态选择 -->
                <AppFormItem
                  v-if="form.linkType === 'redirect'"
                  name="redirectStatus"
                  label="重定向状态码"
                  extra="临时重定向(302)适合经常调整目标的场景；永久重定向(301)会被浏览器与搜索引擎深度缓存。"
                >
                  <AppRadioGroup v-model="form.redirectStatus" class="grid grid-cols-1 gap-3 sm:grid-cols-2">
                    <AppRadioCard
                      value="302"
                      title="302 临时重定向"
                      description="不缓存跳转，目标变更立即对所有访问者生效"
                      :icon="Zap"
                    >
                      <template #extra>
                        <AppTag color="ok">推荐</AppTag>
                      </template>
                    </AppRadioCard>
                    <AppRadioCard
                      value="301"
                      title="301 永久重定向"
                      description="浏览器与搜索引擎长久缓存，减轻服务器二次请求负载"
                      :icon="Clock"
                    />
                  </AppRadioGroup>
                </AppFormItem>

                <!-- 落地页型:来源切换与配置 -->
                <template v-if="form.linkType === 'landing'">
                  <AppFormItem
                    name="landingSource"
                    label="落地页来源"
                    extra="可直接指向外部托管的网页，或将静态站点压缩包直接上传至本平台托管。"
                  >
                    <AppRadioGroup v-model="form.landingSource" class="grid grid-cols-1 gap-3 sm:grid-cols-2">
                      <AppRadioCard
                        value="url"
                        title="URL 地址"
                        description="落地页直接指向一个外部在线地址，访问时先跳至该页面"
                        :icon="Globe"
                      />
                      <AppRadioCard
                        value="upload"
                        title="上传静态压缩包"
                        description="上传含 index.html 的 zip 包，由本平台托管在短链根路径"
                        :icon="UploadCloud"
                      />
                    </AppRadioGroup>
                  </AppFormItem>

                  <!-- 落地页 URL -->
                  <AppFormItem
                    v-if="form.landingSource === 'url'"
                    name="landingUrl"
                    label="落地页地址"
                    :required="form.linkType === 'landing'"
                    extra="访问此短链时首先呈现的在线落地页 URL，最长 4096 字符。"
                  >
                    <AppInput
                      v-model="form.landingUrl"
                      placeholder="https://example.com/landing"
                    >
                      <template #prefix>
                        <Globe :size="15" />
                      </template>
                    </AppInput>
                  </AppFormItem>

                  <!-- 落地页压缩包上传区域 -->
                  <AppFormItem
                    v-else
                    name="landingFile"
                    label="落地页静态压缩包"
                    extra="压缩包必须包含 index.html 作为页面入口。支持拖拽或点击上传，支持覆盖式热更新。"
                  >
                    <div
                      class="relative flex flex-col items-center justify-center rounded-xl border-2 border-dashed p-6 text-center transition-all"
                      :class="
                        isDragging
                          ? 'border-primary bg-primary/10'
                          : 'border-line-strong hover:border-brand bg-surface/40 hover:bg-surface'
                      "
                      @dragover.prevent="isDragging = true"
                      @dragleave.prevent="isDragging = false"
                      @drop.prevent="onDropZip"
                    >
                      <FileArchive :size="36" class="mb-3 text-brand" />
                      
                      <div class="mb-2 flex items-center gap-2">
                        <AppUpload accept=".zip" :before-upload="onSelectZip">
                          <AppButton size="small" :loading="landingUploading">
                            <template #icon><Upload :size="14" /></template>
                            {{ form.landingUploaded ? '重新选择压缩包' : '选择 zip 文件' }}
                          </AppButton>
                        </AppUpload>

                        <AppTag v-if="form.landingUploaded" color="ok">
                          <CheckCircle2 :size="12" class="mr-1" />
                          已托管压缩包
                        </AppTag>
                        <AppTag v-else color="default">未上传</AppTag>
                      </div>

                      <p class="text-xs text-ink-faint">
                        支持点击上方按钮或将 <span class="font-medium text-ink">.zip 压缩包</span> 拖拽到此处
                      </p>
                      <p class="mt-1 text-2xs text-ink-faint">
                        压缩包根目录必须包含 <code class="font-mono text-ink">index.html</code>，更新上传后即刻生效
                      </p>

                      <!-- 已选文件待上传提示 -->
                      <div
                        v-if="landingFile"
                        class="mt-3.5 flex w-full max-w-md items-center justify-between rounded-lg border border-brand/30 bg-brand/10 px-3.5 py-2 text-xs text-brand"
                      >
                        <div class="flex min-w-0 items-center gap-2 truncate">
                          <FileArchive :size="14" class="shrink-0" />
                          <span class="truncate font-medium">{{ landingFile.name }}</span>
                          <span class="text-2xs opacity-75">({{ formatFileSize(landingFile.size) }})</span>
                        </div>
                        <AppTag color="brand" class="shrink-0">待上传</AppTag>
                      </div>
                    </div>
                  </AppFormItem>
                </template>

                <!-- 目标 URL 动态列表卡片 -->
                <AppFormItem
                  name="targetUrls"
                  :label="form.linkType === 'landing' ? '落地页最终目标 URL' : '目标 URL 列表'"
                  extra="支持配置多个目标 URL，访问时按轮询（Round-Robin）顺序分发。支持任意协议（如 https://、http://、mailto:）。"
                >
                  <div class="space-y-2.5">
                    <div
                      v-for="(_, index) in form.targetUrls"
                      :key="index"
                      class="group flex items-center gap-2"
                    >
                      <!-- 序号徽章 -->
                      <span
                        class="flex h-8 w-8 shrink-0 items-center justify-center rounded-lg border border-line bg-surface-muted text-xs font-semibold text-ink-soft select-none"
                      >
                        {{ index + 1 }}
                      </span>

                      <!-- URL 输入框 -->
                      <div class="flex-1">
                        <AppInput
                          v-model="form.targetUrls[index]"
                          placeholder="https://example.com/target-page"
                        >
                          <template #prefix>
                            <Link2 :size="15" />
                          </template>
                        </AppInput>
                      </div>

                      <!-- 删除按钮 -->
                      <AppButton
                        v-if="form.targetUrls.length > 1"
                        type="text"
                        danger
                        class="shrink-0 text-ink-faint hover:text-err"
                        title="删除该目标"
                        @click="removeTargetUrl(index)"
                      >
                        <template #icon><Trash2 :size="16" /></template>
                      </AppButton>
                    </div>

                    <!-- 操作与分发提示行 -->
                    <div class="pt-1 flex flex-col gap-2 sm:flex-row sm:items-center sm:justify-between">
                      <AppButton
                        type="dashed"
                        class="w-full sm:w-auto"
                        @click="addTargetUrl"
                      >
                        <template #icon><Plus :size="15" /></template>
                        添加目标 URL
                      </AppButton>
                      <span
                        v-if="form.targetUrls.length > 1"
                        class="text-xs text-ink-faint flex items-center gap-1"
                      >
                        <RefreshCw :size="12" class="text-brand" />
                        已启用多地址轮询分发机制（共 {{ form.targetUrls.length }} 个目标）
                      </span>
                    </div>
                  </div>
                </AppFormItem>
              </CardContent>
            </AppCard>

            <!-- 模块 3:域名与状态 -->
            <AppCard :padding="false">
              <CardHeader>
                <CardTitle class="flex items-center gap-2">
                  <Globe :size="18" class="text-brand" />
                  关联域名与状态
                </CardTitle>
                <CardDescription>
                  指定承载该短链的一个或多个已激活域名，以及短链当前的服务状态
                </CardDescription>
              </CardHeader>
              <CardContent class="space-y-5">
                <AppFormItem
                  name="domainIds"
                  label="关联域名"
                  extra="仅已激活（active）的域名可关联短链。同一短码可在不同域名下独立指向不同目标，至少选择一个域名。"
                >
                  <AppSelect
                    v-model="form.domainIds"
                    multiple
                    placeholder="请选择关联域名"
                    :options="domainOptions"
                    :max-tag-count="4"
                  />
                </AppFormItem>

                <!-- 编辑模式状态切换 -->
                <AppFormItem
                  v-if="isEdit"
                  name="status"
                  label="短链服务状态"
                  extra="启用时正常解析跳转并记录访问统计；停用时保留短链配置但对外返回 404 未命中。"
                >
                  <AppRadioGroup v-model="form.status" class="grid grid-cols-1 gap-3 sm:grid-cols-2">
                    <AppRadioCard
                      value="enabled"
                      title="正常启用"
                      description="短链正常对外提供访问与跳转，记录访问明细"
                      :icon="CheckCircle2"
                    />
                    <AppRadioCard
                      value="disabled"
                      title="暂停停用"
                      description="短链保留配置但访问时返回 404，可随时重新启用"
                      :icon="PauseCircle"
                    />
                  </AppRadioGroup>
                </AppFormItem>
              </CardContent>
            </AppCard>

            <!-- 模块 4:适用规则(spec D3 指定的界面落点) -->
            <AppCard :padding="false">
              <CardHeader class="pb-4">
                <div class="flex items-start justify-between gap-3">
                  <div class="min-w-0 space-y-1">
                    <CardTitle class="flex items-center gap-2">
                      <ShieldCheck :size="18" class="text-brand" />
                      适用规则
                    </CardTitle>
                    <CardDescription>
                      规则在规则引擎侧声明作用域，这里的勾选改写的是同一份 rule_links 关联
                    </CardDescription>
                  </div>
                  <div class="flex shrink-0 items-center gap-2">
                    <AppTag v-if="globalRuleCount > 0" color="brand">全局 {{ globalRuleCount }}</AppTag>
                    <AppTag v-if="scopedCheckedCount > 0" color="ok">已选 {{ scopedCheckedCount }}</AppTag>
                    <AppButton
                      type="text"
                      class="text-ink-faint hover:text-ink"
                      :aria-label="rulesExpanded ? '收起适用规则' : '展开适用规则'"
                      :aria-expanded="rulesExpanded"
                      @click="rulesExpanded = !rulesExpanded"
                    >
                      <template #icon>
                        <ChevronDown
                          :size="16"
                          class="transition-transform duration-200"
                          :class="rulesExpanded ? 'rotate-180' : ''"
                        />
                      </template>
                      {{ rulesExpanded ? '收起' : '展开' }}
                    </AppButton>
                  </div>
                </div>
              </CardHeader>

              <CardContent v-if="rulesExpanded" class="space-y-3">
                <!-- 加载态 -->
                <div v-if="rulesLoading" class="flex items-center gap-2 py-4 text-xs text-ink-faint">
                  <Loader2 :size="14" class="animate-spin" />
                  正在加载规则列表...
                </div>

                <!-- 短链刚创建：还没有 id，关联无处可写，只做展示 -->
                <template v-else-if="!isEdit">
                  <AppAlert type="info" title="创建完成后可配置适用规则">
                    关联关系写在短链与规则之间，创建短链时无法写入。
                    下面列出当前租户的规则供你预先了解，<strong>创建后回到编辑页勾选即可生效</strong>。
                  </AppAlert>
                  <ul class="max-h-56 divide-y divide-line overflow-y-auto rounded-lg border border-line">
                    <li
                      v-for="row in ruleRows"
                      :key="row.id"
                      class="flex items-center gap-3 px-3 py-2"
                    >
                      <AppCheckbox :checked="row.scope === 'global'" disabled>
                        {{ row.name }}
                      </AppCheckbox>
                      <AppTag v-if="row.scope === 'global'" color="brand">全局</AppTag>
                      <AppTag v-else color="default">指定短链</AppTag>
                      <span class="ml-auto text-2xs text-ink-faint">
                        {{ row.scope === 'global' ? '对本短链恒生效' : '创建后可勾选' }}
                      </span>
                    </li>
                  </ul>
                </template>

                <!-- 编辑模式:可勾选的 scoped 规则 -->
                <template v-else>
                  <AppAlert v-if="ruleRows.length === 0" type="info" title="暂无规则">
                    还没有任何规则。请先到
                    <RouterLink to="/rules" class="font-medium text-brand underline">规则引擎</RouterLink>
                    新建规则，再回到这里关联。
                  </AppAlert>

                  <ul v-else class="max-h-64 divide-y divide-line overflow-y-auto rounded-lg border border-line">
                    <li
                      v-for="row in ruleRows"
                      :key="row.id"
                      class="flex items-center gap-3 px-3 py-2 transition-colors hover:bg-surface-muted/50"
                    >
                      <AppTooltip
                        :title="
                          row.scope === 'global'
                            ? '全局规则对所有短链生效，无法在单个短链上关闭'
                            : '勾选即建立关联，取消即删除'
                        "
                      >
                        <AppCheckbox
                          :checked="row.checked"
                          :disabled="row.scope === 'global' || row.updating"
                          @change="(val: boolean) => onToggleRule(row, val)"
                        >
                          <span class="text-xs text-ink">{{ row.name }}</span>
                        </AppCheckbox>
                      </AppTooltip>

                      <AppTag v-if="row.scope === 'global'" color="brand">全局</AppTag>
                      <AppTag v-else color="default">指定短链</AppTag>
                      <AppTag v-if="!row.enabled" color="warn">已停用</AppTag>

                      <span class="ml-auto flex shrink-0 items-center gap-2 text-2xs text-ink-faint">
                        <span class="font-mono">优先级 {{ row.priority }}</span>
                        <span class="font-mono">{{ actionLabel(row.action) }}</span>
                        <Loader2 v-if="row.updating" :size="12" class="animate-spin" />
                      </span>
                    </li>
                  </ul>

                  <p class="text-xs text-ink-faint">
                    勾选后立即生效（变更即刻写入关联，不需要保存表单）。
                    <span class="text-warn">「全局」规则对所有短链生效，无法在单个短链上关闭</span>，如需收窄请到规则引擎改作用域。
                  </p>
                </template>
              </CardContent>
            </AppCard>

            <!-- 表单底部操作栏 -->
            <div class="flex items-center justify-between rounded-xl border border-line bg-surface px-6 py-4">
              <span class="text-xs text-ink-faint">
                配置提交后立即生效
              </span>
              <div class="flex items-center gap-3">
                <AppButton @click="goBack">取消</AppButton>
                <AppButton
                  type="primary"
                  :loading="submitting"
                  @click="onSubmit"
                >
                  {{ isEdit ? '保存短链修改' : '立即创建短链' }}
                </AppButton>
              </div>
            </div>
          </div>

          <!-- 右侧 1 列:实时预览与指引侧栏 -->
          <div class="space-y-6 lg:col-span-1">
            <!-- 实时预览卡片 -->
            <AppCard :padding="false" class="sticky top-6">
              <CardHeader class="pb-3">
                <CardTitle class="flex items-center gap-2 text-sm font-semibold">
                  <Sparkles :size="16" class="text-brand" />
                  短链实时预览
                </CardTitle>
                <CardDescription>
                  根据当前填写的短码与所选域名实时生成短链完整地址
                </CardDescription>
              </CardHeader>
              <CardContent class="space-y-4">
                <!-- 拼接后的完整地址展示框 -->
                <div class="rounded-xl border border-line bg-surface-muted/60 p-3.5 space-y-2">
                  <div class="flex items-center justify-between text-2xs text-ink-faint">
                    <span>主访问地址</span>
                    <span v-if="selectedDomains.length > 1" class="font-medium text-brand">
                      +{{ selectedDomains.length - 1 }} 个备选域名
                    </span>
                  </div>
                  <div class="mono break-all text-xs font-semibold text-ink selection:bg-brand/30">
                    {{ previewUrl }}
                  </div>
                  <div class="pt-1 flex items-center justify-between">
                    <CopyText :text="previewUrl" class="text-xs text-brand hover:text-brand/75 font-medium">
                      复制完整地址
                    </CopyText>
                    <a
                      v-if="canOpenPreview"
                      :href="previewUrl"
                      target="_blank"
                      rel="noopener noreferrer"
                      class="inline-flex items-center gap-1 text-xs text-ink-soft hover:text-ink transition-colors"
                    >
                      <span>测试访问</span>
                      <ExternalLink :size="12" />
                    </a>
                  </div>
                </div>

                <!-- 配置清单摘要 -->
                <div class="space-y-2.5 border-t border-line pt-3.5 text-xs">
                  <div class="text-2xs font-medium uppercase tracking-wider text-ink-faint">
                    配置清单摘要
                  </div>

                  <div class="flex items-center justify-between">
                    <span class="text-ink-soft">短链类型</span>
                    <AppTag v-if="form.linkType === 'redirect'" color="brand">
                      跳转型 ({{ form.redirectStatus }})
                    </AppTag>
                    <AppTag v-else color="info">
                      落地页型 ({{ form.landingSource === 'url' ? 'URL' : '压缩包' }})
                    </AppTag>
                  </div>

                  <div class="flex items-center justify-between">
                    <span class="text-ink-soft">目标地址</span>
                    <span class="font-medium text-ink tabular-nums">
                      {{ validTargetsCount }} 个
                      <span v-if="validTargetsCount > 1" class="text-ink-faint font-normal">(轮询)</span>
                    </span>
                  </div>

                  <div class="flex items-center justify-between">
                    <span class="text-ink-soft">关联域名</span>
                    <span class="font-medium text-ink tabular-nums">
                      {{ form.domainIds.length }} 个域名
                    </span>
                  </div>

                  <div class="flex items-center justify-between">
                    <span class="text-ink-soft">适用规则</span>
                    <span class="font-medium text-ink tabular-nums">
                      {{ rulesLoading ? '加载中…' : (isEdit ? globalRuleCount + scopedCheckedCount : 0) }} 条
                      <span v-if="isEdit && globalRuleCount > 0" class="text-ink-faint font-normal">
                        (含全局 {{ globalRuleCount }} 条)
                      </span>
                    </span>
                  </div>

                  <div class="flex items-center justify-between">
                    <span class="text-ink-soft">短码模式</span>
                    <span class="font-medium text-ink">
                      {{ form.code.trim() ? '自定义短码' : '系统自动生成' }}
                    </span>
                  </div>

                  <div v-if="isEdit" class="flex items-center justify-between">
                    <span class="text-ink-soft">服务状态</span>
                    <AppTag :color="form.status === 'enabled' ? 'ok' : 'default'">
                      {{ form.status === 'enabled' ? '正常启用' : '暂停停用' }}
                    </AppTag>
                  </div>
                </div>

                <!-- 指南小卡片 -->
                <div class="rounded-lg border border-line-strong/60 bg-surface/50 p-3 text-2xs leading-relaxed text-ink-faint space-y-1.5">
                  <div class="flex items-center gap-1 font-medium text-ink-soft">
                    <Info :size="13" class="text-brand" />
                    温馨提示
                  </div>
                  <p>• 同一短码可在不同域名下指向不同目标，实现域名隔离。</p>
                  <p>• 推荐使用 302 临时重定向，方便未来根据需要随时修改目标。</p>
                  <p v-if="form.linkType === 'landing'">
                    • 落地页按钮须引用平台提供的 JS SDK，点击才能被回传统计并跳转至目标。
                  </p>
                </div>
              </CardContent>
            </AppCard>
          </div>
        </div>
      </AppForm>
    </AppSpin>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue';
import { useRoute, useRouter } from 'vue-router';
import {
  ArrowLeft,
  CheckCircle2,
  ChevronDown,
  Clock,
  ExternalLink,
  FileArchive,
  Globe,
  Hash,
  Info,
  Layout,
  Link2,
  Loader2,
  PauseCircle,
  Plus,
  RefreshCw,
  ShieldCheck,
  Trash2,
  Upload,
  UploadCloud,
  Zap,
} from '@lucide/vue';
import type { FormRule } from '@/components/app/types';

import { listDomains } from '@/api/domains';
import { createLink, getLink, updateLink, uploadLanding } from '@/api/links';
import { listLinkRules, ruleOptions, setLinkRules } from '@/api/rules';
import PageHeader from '@/components/PageHeader.vue';
import AppAlert from '@/components/app/AppAlert.vue';
import AppCheckbox from '@/components/app/AppCheckbox.vue';
import AppTag from '@/components/app/AppTag.vue';
import AppTooltip from '@/components/app/AppTooltip.vue';
import {
  AUTO_SHORT_CODE_LENGTH,
  SHORT_CODE_FORBIDDEN_PATTERN,
  SHORT_CODE_MAX_LENGTH,
  SHORT_CODE_PATTERN,
} from '@/constants/dict';
import { ApiError, getQuotaUsage } from '@/types/api';
import { useDirtyGuard } from '@/composables/useDirtyGuard';
import type {
  Domain,
  LandingSource,
  Link,
  LinkRule,
  LinkType,
  RedirectStatus,
  RuleAction,
  RuleScope,
} from '@/types/api';
import { message } from '@/utils/toast';

const route = useRoute();
const router = useRouter();

/** 仅“短链编辑”路由且 ID 为正整数时才是编辑模式;创建路由没有 id 参数,不能把 NaN 当编辑 */
const validLinkId = computed(() => {
  const raw = route.params.id;
  const id = typeof raw === 'string' ? Number(raw) : Number.NaN;
  return Number.isInteger(id) && id > 0 ? id : undefined;
});
const isEdit = computed(() => route.name === 'link-edit' && validLinkId.value !== undefined);

const headerDescription = computed(() =>
  isEdit.value
    ? '修改短链的目标、类型、落地页与关联域名；短码创建后不可修改'
    : '选择短链类型、目标与关联域名，提交后立即生效；短码可留空自动生成',
);

const formRef = ref();
const submitting = ref(false);
const loading = ref(false);
const landingFile = ref<File | null>(null);
const landingUploading = ref(false);
const isDragging = ref(false);

const link = ref<Link | null>(null);
const domains = ref<Domain[]>([]);

const form = reactive<{
  code: string;
  targetUrls: string[];
  domainIds: number[];
  redirectStatus: RedirectStatus;
  linkType: LinkType;
  landingSource: LandingSource;
  landingUrl: string;
  landingUploaded: boolean;
  status: 'enabled' | 'disabled';
}>({
  code: '',
  targetUrls: [''],
  domainIds: [],
  redirectStatus: '302',
  linkType: 'redirect',
  landingSource: 'url',
  landingUrl: '',
  landingUploaded: false,
  status: 'enabled',
});

// 表单脏检查与离开拦截
const initialFormJson = ref('');
const isFormDirty = () => {
  if (!initialFormJson.value) return false;
  return JSON.stringify(form) !== initialFormJson.value || landingFile.value !== null;
};
const { markClean } = useDirtyGuard({
  isDirty: isFormDirty,
  title: '放弃未保存的短链修改？',
  message: '当前填写的短链配置尚未保存，离开此页面将丢失修改。',
});

/** 域名状态备注(非激活域名置灰) */
const DOMAIN_STATUS_NOTE: Record<string, string> = {
  pending: '待激活',
  failed: '校验失败',
  stopped: '已停用',
};

/** 域名下拉:仅 active 域名可关联(后端 validateLinkDomains 要求 active) */
const domainOptions = computed(() =>
  domains.value.map((d) => {
    const active = d.status === 'active';
    return {
      value: d.id,
      label: active ? d.fqdn : d.fqdn + ' (' + (DOMAIN_STATUS_NOTE[d.status] ?? '不可用') + ')',
      disabled: !active,
    };
  }),
);

/** 当前选中的域名对象列表 */
const selectedDomains = computed(() =>
  domains.value.filter((d) => form.domainIds.includes(d.id)),
);

/** 预览短链的主域名 */
const primaryDomain = computed(() => {
  return selectedDomains.value[0]?.fqdn || 'your-domain.com';
});

/** 实时预览 URL */
const previewUrl = computed(() => {
  const codePart = form.code.trim() || (isEdit.value ? 'code' : '自动生成');
  return `https://${primaryDomain.value}/${codePart}`;
});

/** 是否可直接点击打开测试预览(非占位且已保存模式) */
const canOpenPreview = computed(() => {
  return isEdit.value && selectedDomains.value.length > 0 && form.code.trim().length > 0;
});

/** 有效的目标 URL 计数 */
const validTargetsCount = computed(() => {
  return form.targetUrls.filter((u) => u.trim().length > 0).length;
});

/** 短码说明:创建时说明生成规则,编辑时说明不可修改 */
const codeExtra = computed(() =>
  isEdit.value
    ? '短码创建后不可修改。完整短链地址为「https://<域名>/<短码>」，短码即地址最后一段。'
    : '留空则由系统自动生成 ' +
      AUTO_SHORT_CODE_LENGTH +
      ' 位随机短码。字符集仅含字母与数字，并去除易混淆字符 0/O/1/l/I。自定义短码最长 ' +
      SHORT_CODE_MAX_LENGTH +
      ' 位，创建后不可修改。',
);

const hasControlChars = (value: string) => /[\u0000-\u001f\u007f]/.test(value);

const rules: Record<string, FormRule[]> = {
  code: [
    {
      validator: (_rule, value: unknown) => {
        const v = value as string;
        if (!v) return Promise.resolve();
        if (!SHORT_CODE_PATTERN.test(v)) {
          return Promise.reject(new Error('短码仅允许字母与数字'));
        }
        if (SHORT_CODE_FORBIDDEN_PATTERN.test(v)) {
          return Promise.reject(new Error('短码不能包含易混淆字符 0/O/1/l/I'));
        }
        if (v.length > SHORT_CODE_MAX_LENGTH) {
          return Promise.reject(new Error('短码最长 ' + SHORT_CODE_MAX_LENGTH + ' 位'));
        }
        return Promise.resolve();
      },
    },
  ],
  targetUrls: [
    {
      // 标 required 才会显示红星；空值判定仍交给下面的自定义校验，
      // 因为 targetUrls 的初始值是 ['']（数组非空但首个元素为空），
      // 只用 isBlank 判不出「没填」这种真正要报错的情况。
      required: true,
      validator: (_rule, value: unknown) => {
        if (!Array.isArray(value) || value.length === 0) {
          return Promise.reject(new Error('请至少填写一个目标 URL'));
        }
        const urls = value as string[];
        for (let i = 0; i < urls.length; i++) {
          const url = (urls[i] ?? '').trim();
          if (!url) {
            return Promise.reject(new Error('第 ' + (i + 1) + ' 个目标 URL 不能为空'));
          }
          if (hasControlChars(url)) {
            return Promise.reject(new Error('目标 URL 不能包含控制字符(换行/制表符等)'));
          }
          if (url.length > 4096) {
            return Promise.reject(new Error('目标 URL 最长 4096 字符'));
          }
        }
        return Promise.resolve();
      },
    },
  ],
  landingUrl: [
    {
      validator: (_rule, value: unknown) => {
        if (form.linkType !== 'landing' || form.landingSource !== 'url') {
          return Promise.resolve();
        }
        const url = String(value ?? '').trim();
        if (!url) {
          return Promise.reject(new Error('请填写落地页地址'));
        }
        if (hasControlChars(url)) {
          return Promise.reject(new Error('落地页地址不能包含控制字符(换行/制表符等)'));
        }
        if (url.length > 4096) {
          return Promise.reject(new Error('落地页地址最长 4096 字符'));
        }
        return Promise.resolve();
      },
    },
  ],
  landingFile: [
    {
      // 「已有托管文件」与「本次选了新文件」二者居其一即可。
      // 后端在创建/编辑时也会拒 landing+upload 且无文件(issue 04),这里是同一条
      // 不变式的前置提示 —— 没有它,用户填完表单点保存才会撞上 400。
      validator: () => {
        if (form.linkType !== 'landing' || form.landingSource !== 'upload') {
          return Promise.resolve();
        }
        if (form.landingUploaded || landingFile.value) {
          return Promise.resolve();
        }
        return Promise.reject(
          new Error('请选择要上传的落地页压缩包(zip,须含 index.html)'),
        );
      },
    },
  ],
  domainIds: [
    {
      // 同上：空数组能被 isBlank 认出，这里给出更准确的文案
      required: true,
      message: '请至少选择一个关联域名',
      validator: (_rule, value: unknown) => {
        if (!Array.isArray(value) || value.length === 0) {
          return Promise.reject(new Error('请至少选择一个关联域名'));
        }
        return Promise.resolve();
      },
    },
  ],
};

/** 格式化文件大小 */
function formatFileSize(bytes: number): string {
  if (bytes < 1024) return bytes + ' B';
  if (bytes < 1024 * 1024) return (bytes / 1024).toFixed(1) + ' KB';
  return (bytes / (1024 * 1024)).toFixed(2) + ' MB';
}

/** 用短链详情回填表单(编辑模式) */
function applyLink(data: Link) {
  const fqdnSet = new Set(data.domains);
  form.code = data.code;
  form.targetUrls = data.targetUrls.length > 0 ? [...data.targetUrls] : [''];
  form.redirectStatus = data.redirectStatus;
  form.linkType = data.linkType || 'redirect';
  form.landingSource = data.landingSource || 'url';
  form.landingUrl = data.landingUrl || '';
  form.landingUploaded = data.landingUploaded === true;
  form.status = data.status;
  form.domainIds = domains.value.filter((d) => fqdnSet.has(d.fqdn)).map((d) => d.id);
  initialFormJson.value = JSON.stringify(form);
}

/** 创建模式的默认值 */
function applyDefaults() {
  form.code = '';
  form.targetUrls = [''];
  form.redirectStatus = '302';
  form.linkType = 'redirect';
  form.landingSource = 'url';
  form.landingUrl = '';
  form.landingUploaded = false;
  form.status = 'enabled';
  // 默认选中所有已激活域名
  form.domainIds = domains.value.filter((d) => d.status === 'active').map((d) => d.id);
  initialFormJson.value = JSON.stringify(form);
}

async function loadDomains() {
  try {
    domains.value = await listDomains();
  } catch {
    // 域名加载失败不阻塞表单页;下拉为空时校验会提示选择域名
  }
}

/* ==================== 适用规则(spec D3 界面落点) ==================== */
/** 勾选区的一行：全量规则 + 该短链当前的关联状态 */
interface RuleRow {
  id: number;
  name: string;
  scope: RuleScope;
  action: RuleAction;
  priority: number;
  enabled: boolean;
  /** 全局规则恒为 true；scoped 规则取决于是否已写入 rule_links */
  checked: boolean;
  updating: boolean;
}

const ACTION_LABELS: Record<RuleAction, string> = {
  pass: '放行',
  redirect: '重定向',
  notfound: '404',
  throttle: '限流 429',
};

function actionLabel(action: RuleAction): string {
  return ACTION_LABELS[action] ?? action;
}

const ruleRows = ref<RuleRow[]>([]);
const rulesLoading = ref(false);
/** 折叠默认收起：表单字段本就多，适用规则不展开时不占版面 */
const rulesExpanded = ref(false);

const globalRuleCount = computed(() => ruleRows.value.filter((r) => r.scope === 'global').length);
const scopedCheckedCount = computed(
  () => ruleRows.value.filter((r) => r.scope === 'links' && r.checked).length,
);

/** 全局规则在前、scoped 按优先级升序，与求值顺序一致 */
function sortRuleRows(rows: RuleRow[]): RuleRow[] {
  return [...rows].sort((a, b) => {
    if (a.scope !== b.scope) return a.scope === 'global' ? -1 : 1;
    return a.priority - b.priority || a.id - b.id;
  });
}

/**
 * 加载适用规则列表。
 * 全量规则来自 /api/rules/options；当前关联与全局继承项来自 /api/links/{id}/rules。
 * 新建模式下还没有 id，只能拿到全量规则（预置空态）。
 */
async function loadLinkRules(id: number | undefined) {
  rulesLoading.value = true;
  try {
    const [options, applied] = await Promise.all([
      ruleOptions(),
      id === undefined ? Promise.resolve<LinkRule[]>([]) : listLinkRules(id),
    ]);
    // 全局规则由服务端以 source=inherited 告知；scoped 以 source=scoped 为准
    const scopedIds = new Set(
      applied.filter((r) => r.source === 'scoped').map((r) => r.id),
    );
    const inheritedIds = new Set(
      applied.filter((r) => r.source === 'inherited').map((r) => r.id),
    );
    ruleRows.value = sortRuleRows(
      options.map((o) => ({
        id: o.id,
        name: o.name,
        scope: o.scope,
        action: o.action,
        priority: o.priority,
        enabled: o.enabled,
        checked: o.scope === 'global' || inheritedIds.has(o.id) || scopedIds.has(o.id),
        updating: false,
      })),
    );
  } catch (error) {
    if (error instanceof ApiError) message.error(error.message);
    else message.error('加载适用规则失败,请稍后重试');
    ruleRows.value = [];
  } finally {
    rulesLoading.value = false;
  }
}

/** 以服务端返回的关联为准重置勾选态（PUT 失败时不信任本地状态） */
function applyServerRules(items: LinkRule[]) {
  const scopedIds = new Set(items.filter((r) => r.source === 'scoped').map((r) => r.id));
  ruleRows.value = ruleRows.value.map((row) => ({
    ...row,
    checked: row.scope === 'global' || scopedIds.has(row.id),
    updating: false,
  }));
}

/**
 * 勾选变更即刻写入关联（PUT /api/links/{id}/rules 传完整的 scoped id 集合）。
 * 失败时回滚勾选态并 toast 提示。
 */
async function onToggleRule(row: RuleRow, next: boolean) {
  if (row.scope === 'global' || !link.value || row.updating) return;
  const prev = row.checked;
  row.checked = next;
  row.updating = true;
  try {
    const scopedIds = ruleRows.value
      .filter((r) => r.scope === 'links' && (r.id === row.id ? next : r.checked))
      .map((r) => r.id);
    const items = await setLinkRules(link.value.id, scopedIds);
    applyServerRules(items);
    message.success(next ? '已关联规则' : '已解除规则关联');
  } catch (error) {
    row.checked = prev;
    row.updating = false;
    if (error instanceof ApiError) message.error(error.message);
    else message.error('保存规则关联失败，请稍后重试');
  }
}

async function init() {
  await loadDomains();
  if (route.name === 'link-edit') {
    const id = validLinkId.value;
    if (id === undefined) {
      message.error('无效的短链 ID');
      router.push({ name: 'links' });
      return;
    }
    loading.value = true;
    try {
      const data = await getLink(id);
      link.value = data;
      applyLink(data);
    } catch (error) {
      if (error instanceof ApiError) message.error(error.message);
      else message.error('加载短链失败，请稍后重试');
      router.push({ name: 'links' });
      return;
    } finally {
      loading.value = false;
    }
  } else {
    applyDefaults();
  }

  // 适用规则区：编辑模式取本短链的实际关联，新建模式预置空态
  void loadLinkRules(link.value?.id);
}

onMounted(init);

/** 新增一行目标 URL */
function addTargetUrl() {
  form.targetUrls.push('');
}

/** 删除指定行目标 URL(至少保留 1 行) */
function removeTargetUrl(index: number) {
  if (form.targetUrls.length <= 1) return;
  form.targetUrls.splice(index, 1);
}

/** 选择 zip 文件 */
async function onSelectZip(file: File): Promise<boolean> {
  if (!file.name.toLowerCase().endsWith('.zip')) {
    message.error('仅支持 .zip 格式的压缩包');
    return false;
  }
  landingFile.value = file;
  if (isEdit.value && link.value) {
    await doUploadLanding(link.value.id);
  } else {
    message.info('落地页压缩包已选定，将在短链创建成功后自动上传');
  }
  return false;
}

/** 拖拽 zip 文件上传 */
function onDropZip(e: DragEvent) {
  isDragging.value = false;
  const file = e.dataTransfer?.files?.[0];
  if (!file) return;
  onSelectZip(file);
}

/** 上传已选 zip(替换式);返回是否成功,调用方据此决定能不能继续切类型 */
async function doUploadLanding(id: number): Promise<boolean> {
  if (!landingFile.value) return false;
  landingUploading.value = true;
  try {
    const updated = await uploadLanding(id, landingFile.value);
    form.landingUploaded = updated.landingUploaded === true;
    message.success('落地页压缩包已上传');
    return true;
  } catch (error) {
    if (error instanceof ApiError) message.error(error.message);
    else message.error('上传失败，请稍后重试');
    return false;
  } finally {
    landingUploading.value = false;
  }
}

type LinkPayload = {
  targetUrls: string[];
  domainIds: number[];
  redirectStatus?: RedirectStatus;
  linkType?: LinkType;
  landingSource?: LandingSource;
  landingUrl?: string;
};

async function onSubmit() {
  try {
    await formRef.value?.validate();
  } catch {
    return;
  }

  submitting.value = true;
  try {
    const payload: LinkPayload = {
      targetUrls: form.targetUrls.map((s) => s.trim()),
      domainIds: form.domainIds,
      linkType: form.linkType,
    };
    if (form.linkType === 'redirect') {
      payload.redirectStatus = form.redirectStatus;
    } else {
      payload.landingSource = form.landingSource;
      if (form.landingSource === 'url') {
        payload.landingUrl = form.landingUrl.trim();
      }
    }

    if (isEdit.value && link.value) {
      await updateLink(link.value.id, {
        ...payload,
        status: form.status,
      });
      message.success('短链已更新');
      // 编辑:上传来源且选了新文件 → 补一次替换式上传。
      // (选了文件时 onSelectZip 通常已经即时传过一次,这里是幂等重传兜底。)
      if (form.linkType === 'landing' && form.landingSource === 'upload' && landingFile.value) {
        await doUploadLanding(link.value.id);
      }
    } else {
      // upload 来源不能在创建请求里直接给:落地页文件按**短链 ID** 落盘,
      // 建链那一刻 id 还不存在,后端会拒(issue 04)。
      // 所以顺序是:先建一条可用的跳转型短链 → 上传压缩包 → 再 PATCH 定型为 landing+upload。
      // 关键是失败形态:上传失败时留下的是一条**能正常跳转**的短链,而不是
      // 一条 source=upload 却无文件、此后每次访问都记一行 landing_missing 的空壳。
      const usesUpload = form.linkType === 'landing' && form.landingSource === 'upload';
      const created = await createLink({
        targetUrls: payload.targetUrls,
        domainIds: payload.domainIds,
        redirectStatus: '302',
        linkType: 'redirect',
        code: form.code.trim() || undefined,
      });
      if (usesUpload) {
        if (!(await doUploadLanding(created.id))) {
          router.push({ name: 'links' });
          return;
        }
        await updateLink(created.id, { linkType: 'landing', landingSource: 'upload' });
      }
      markClean();
      message.success('短链已创建');
    }
    markClean();
    router.push({ name: 'links' });
  } catch (error) {
    if (error instanceof ApiError) {
      if (error.status === 403) {
        const usage = getQuotaUsage(error.details);
        if (usage) {
          message.error('短链配额超限:当前 ' + usage.links + '/' + usage.maxLinks + ' 条短链，已达上限');
        } else {
          message.error('短链配额超限:' + error.message);
        }
      } else if (error.status === 409) {
        message.error('短码冲突:' + error.message);
      } else {
        message.error(error.message);
      }
    } else {
      message.error('保存失败，请稍后重试');
    }
  } finally {
    submitting.value = false;
  }
}

function goBack() {
  router.push({ name: 'links' });
}
</script>
