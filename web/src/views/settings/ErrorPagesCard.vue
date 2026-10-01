<template>
  <AppCard :padding="false">
    <CardHeader>
      <div class="flex items-center justify-between">
        <div>
          <CardTitle class="flex items-center gap-2">
            <FileCode :size="18" class="text-brand-600 dark:text-brand-400" />
            访客端错误页面 (404 / 429)
          </CardTitle>
          <CardDescription>
            当短链不存在、已停用或被规则处置拦截时，向公网访客展示的自适应响应页面。支持系统默认或自定义上传独立 HTML。
          </CardDescription>
        </div>
        <AppButton
          type="primary"
          :loading="saving"
          @click="onSave"
        >
          保存配置
        </AppButton>
      </div>
    </CardHeader>

    <CardContent class="space-y-6 p-6">
      <!-- 提示条 -->
      <AppAlert
        type="info"
        show-icon
        title="三级页面决议机制说明"
        message="页面决议优先级为：单条规则专属页面 > 租户全局自定义页面 > 系统默认自适应页面。单页文件大小上限为 512 KB。管理后台与 API 端点不受影响。"
      />

      <!-- Tab 切换 404 与 429 -->
      <div class="flex border-b border-line" role="tablist">
        <button
          type="button"
          role="tab"
          :aria-selected="activeTab === '404'"
          class="flex items-center gap-2 border-b-2 px-4 py-2.5 text-sm font-medium transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500"
          :class="activeTab === '404'
            ? 'border-brand-600 text-brand-600 dark:border-brand-400 dark:text-brand-400'
            : 'border-transparent text-ink-soft hover:text-ink'"
          @click="activeTab = '404'"
        >
          <FileQuestion :size="16" />
          404 Not Found 页面
          <AppTag v-if="mode404 === 'custom'" color="blue" size="small">自定义</AppTag>
        </button>
        <button
          type="button"
          role="tab"
          :aria-selected="activeTab === '429'"
          class="flex items-center gap-2 border-b-2 px-4 py-2.5 text-sm font-medium transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500"
          :class="activeTab === '429'
            ? 'border-brand-600 text-brand-600 dark:border-brand-400 dark:text-brand-400'
            : 'border-transparent text-ink-soft hover:text-ink'"
          @click="activeTab = '429'"
        >
          <ClockAlert :size="16" />
          429 Too Many Requests 页面
          <AppTag v-if="mode429 === 'custom'" color="blue" size="small">自定义</AppTag>
        </button>
      </div>

      <!-- 404 配置区 -->
      <div v-show="activeTab === '404'" class="space-y-4">
        <div class="flex items-center gap-4">
          <span class="text-xs font-semibold text-ink">响应模式：</span>
          <AppRadioGroup v-model="mode404" class="flex gap-4">
            <AppRadio value="default">系统默认自适应 404 页面</AppRadio>
            <AppRadio value="custom">租户全局自定义 404 HTML</AppRadio>
          </AppRadioGroup>
        </div>

        <div v-if="mode404 === 'custom'" class="space-y-3 rounded-xl border border-line bg-surface-muted/30 p-4">
          <div class="flex flex-wrap items-center justify-between gap-2">
            <div class="flex items-center gap-2">
              <input
                ref="fileInput404"
                type="file"
                accept=".html,.htm"
                class="hidden"
                @change="handleFileUpload($event, '404')"
              />
              <AppButton size="small" @click="triggerUpload('404')">
                <template #icon><Upload :size="14" /></template>
                上传 .html 文件
              </AppButton>
              <AppButton size="small" :disabled="!html404" @click="openPreview(html404, '404')">
                <template #icon><Eye :size="14" /></template>
                沙箱预览
              </AppButton>
            </div>
            <div class="text-xs" :class="isOverLimit(html404) ? 'text-err font-bold' : 'text-ink-faint'">
              大小: {{ formatSize(html404) }} / 512 KB
            </div>
          </div>

          <AppTextarea
            v-model="html404"
            :rows="10"
            placeholder="<!DOCTYPE html><html><head><title>404 Not Found</title></head><body>...</body></html>"
            class="font-mono text-xs leading-relaxed"
          />
        </div>
      </div>

      <!-- 429 配置区 -->
      <div v-show="activeTab === '429'" class="space-y-4">
        <div class="flex items-center gap-4">
          <span class="text-xs font-semibold text-ink">响应模式：</span>
          <AppRadioGroup v-model="mode429" class="flex gap-4">
            <AppRadio value="default">系统默认自适应 429 页面</AppRadio>
            <AppRadio value="custom">租户全局自定义 429 HTML</AppRadio>
          </AppRadioGroup>
        </div>

        <div v-if="mode429 === 'custom'" class="space-y-3 rounded-xl border border-line bg-surface-muted/30 p-4">
          <div class="flex flex-wrap items-center justify-between gap-2">
            <div class="flex items-center gap-2">
              <input
                ref="fileInput429"
                type="file"
                accept=".html,.htm"
                class="hidden"
                @change="handleFileUpload($event, '429')"
              />
              <AppButton size="small" @click="triggerUpload('429')">
                <template #icon><Upload :size="14" /></template>
                上传 .html 文件
              </AppButton>
              <AppButton size="small" :disabled="!html429" @click="openPreview(html429, '429')">
                <template #icon><Eye :size="14" /></template>
                沙箱预览
              </AppButton>
            </div>
            <div class="text-xs" :class="isOverLimit(html429) ? 'text-err font-bold' : 'text-ink-faint'">
              大小: {{ formatSize(html429) }} / 512 KB
            </div>
          </div>

          <AppTextarea
            v-model="html429"
            :rows="10"
            placeholder="<!DOCTYPE html><html><head><title>429 Too Many Requests</title></head><body>...</body></html>"
            class="font-mono text-xs leading-relaxed"
          />
        </div>
      </div>
    </CardContent>

    <!-- 预览弹窗 (Teleport 到 body，保证完全沙箱隔离与层级覆盖) -->
    <Teleport to="body">
      <div
        v-if="previewVisible"
        class="fixed inset-0 z-[100] flex items-center justify-center bg-black/60 p-4 backdrop-blur-xs"
        @click.self="previewVisible = false"
      >
        <div class="flex h-[85vh] w-full max-w-4xl flex-col rounded-xl border border-line bg-surface shadow-2xl overflow-hidden">
          <div class="flex items-center justify-between border-b border-line px-5 py-3 bg-surface-muted/50">
            <div class="flex items-center gap-2">
              <Eye :size="16" class="text-brand-600 dark:text-brand-400" />
              <span class="text-sm font-semibold text-ink">访客拦截页面沙箱预览 ({{ previewTitle }})</span>
              <span class="text-xs text-ink-faint">已开启 sandbox 安全隔离</span>
            </div>
            <AppButton
              size="icon"
              variant="ghost"
              aria-label="关闭预览"
              @click="previewVisible = false"
            >
              <X :size="18" />
            </AppButton>
          </div>
          <div class="flex-1 p-3 bg-line/20">
            <iframe
              :srcdoc="previewContent"
              sandbox="allow-same-origin"
              class="h-full w-full rounded border border-line bg-background shadow-xs"
            />
          </div>
        </div>
      </div>
    </Teleport>
  </AppCard>
</template>

<script setup lang="ts">
import { onMounted, ref } from 'vue';
import { ClockAlert, Eye, FileCode, FileQuestion, Upload, X } from '@lucide/vue';

import { fetchTenantErrorPages, updateTenantErrorPages } from '@/api/me';
import { message } from '@/utils/toast';

const MAX_BYTES = 512 * 1024; // 512KB

const activeTab = ref<'404' | '429'>('404');
const mode404 = ref<'default' | 'custom'>('default');
const mode429 = ref<'default' | 'custom'>('default');
const html404 = ref('');
const html429 = ref('');
const saving = ref(false);

const fileInput404 = ref<HTMLInputElement | null>(null);
const fileInput429 = ref<HTMLInputElement | null>(null);

const previewVisible = ref(false);
const previewTitle = ref('');
const previewContent = ref('');

function byteLength(str: string): number {
  return new Blob([str]).size;
}

function formatSize(str: string): string {
  const bytes = byteLength(str);
  if (bytes < 1024) return `${bytes} B`;
  return `${(bytes / 1024).toFixed(1)} KB`;
}

function isOverLimit(str: string): boolean {
  return byteLength(str) > MAX_BYTES;
}

function triggerUpload(type: '404' | '429') {
  if (type === '404') fileInput404.value?.click();
  else fileInput429.value?.click();
}

function handleFileUpload(event: Event, type: '404' | '429') {
  const input = event.target as HTMLInputElement;
  const file = input.files?.[0];
  if (!file) return;

  if (file.size > MAX_BYTES) {
    message.error(`文件大小 (${(file.size / 1024).toFixed(1)} KB) 超过 512 KB 限制`);
    input.value = '';
    return;
  }

  const reader = new FileReader();
  reader.onload = (e) => {
    const text = String(e.target?.result ?? '');
    if (type === '404') {
      html404.value = text;
      mode404.value = 'custom';
    } else {
      html429.value = text;
      mode429.value = 'custom';
    }
    message.success(`${file.name} 载入成功`);
  };
  reader.onerror = () => {
    message.error('读取文件失败');
  };
  reader.readAsText(file);
  input.value = '';
}

function openPreview(content: string, type: '404' | '429') {
  previewTitle.value = type === '404' ? '404 Not Found' : '429 Too Many Requests';
  previewContent.value = content || '<h2 style="font-family:sans-serif;text-align:center;margin-top:40px;">（内容为空）</h2>';
  previewVisible.value = true;
}

async function loadData() {
  try {
    const res = await fetchTenantErrorPages();
    html404.value = res.custom404Html || '';
    mode404.value = res.custom404Html ? 'custom' : 'default';
    html429.value = res.custom429Html || '';
    mode429.value = res.custom429Html ? 'custom' : 'default';
  } catch (error) {
    message.error(error instanceof Error ? error.message : '加载错误页面配置失败');
  }
}

async function onSave() {
  const payload404 = mode404.value === 'custom' ? html404.value.trim() : '';
  const payload429 = mode429.value === 'custom' ? html429.value.trim() : '';

  if (mode404.value === 'custom' && isOverLimit(payload404)) {
    message.error('404 页面 HTML 超过 512 KB 上限');
    return;
  }
  if (mode429.value === 'custom' && isOverLimit(payload429)) {
    message.error('429 页面 HTML 超过 512 KB 上限');
    return;
  }

  saving.value = true;
  try {
    const res = await updateTenantErrorPages({
      custom404Html: payload404,
      custom429Html: payload429,
    });
    html404.value = res.custom404Html;
    mode404.value = res.custom404Html ? 'custom' : 'default';
    html429.value = res.custom429Html;
    mode429.value = res.custom429Html ? 'custom' : 'default';
    message.success('错误页面配置已更新，即刻生效');
  } catch (error) {
    message.error(error instanceof Error ? error.message : '保存失败');
  } finally {
    saving.value = false;
  }
}

onMounted(loadData);
</script>
