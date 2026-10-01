<template>
  <div data-od-id="links-view">
    <PageHeader
      title="短链列表"
      description="管理租户名下的短链，支持类型过滤、状态切换与出口多目标轮询配置。"
    >
      <template #actions>
        <AppButton size="sm" variant="outline" :loading="loading" title="刷新短链列表" @click="loadData">
          <template #icon><RefreshCw :size="13" /></template>
          刷新
        </AppButton>
        <AppButton v-if="!recycleBin" size="sm" variant="outline" @click="openBatchModal">
          <template #icon><Upload :size="13" /></template>
          批量导入
        </AppButton>
        <AppButton size="sm" variant="outline" :disabled="links.length === 0" @click="exportCsv">
          <template #icon><FileDown :size="13" /></template>
          导出 CSV
        </AppButton>
        <AppButton v-if="!recycleBin" size="sm" type="primary" @click="goCreate">
          <template #icon><Plus :size="14" /></template>
          新建短链
        </AppButton>
      </template>
    </PageHeader>

    <!-- 视图切换:短链 / 回收站。回收站是软删短链唯一的入口 ——
         没有它,删掉的短链既看不见、也无法找回,短码与配额就等于被永久锁死。 -->
    <AppTabs :model-value="viewTab" class="mb-4" @update:model-value="onSwitchTab">
      <AppTabsList variant="line">
        <AppTabsTrigger value="active" variant="line">短链</AppTabsTrigger>
        <AppTabsTrigger value="deleted" variant="line">
          回收站<template v-if="deletedTotal > 0">（{{ deletedTotal }}）</template>
        </AppTabsTrigger>
      </AppTabsList>
    </AppTabs>

    <!-- 批量操作条:有选中项时出现 -->
    <div
      v-if="selectedCount > 0"
      class="mb-4 flex flex-wrap items-center justify-between gap-3 rounded-lg border border-brand/30 bg-brand/10 px-3 py-2"
      data-od-id="link-batch-bar"
    >
      <div class="flex items-center gap-2 text-xs text-ink-soft">
        <CheckSquare :size="14" />
        <span>
          已选
          <strong class="font-mono text-ink">{{ selectedCount }}</strong>
          / {{ filteredLinks.length }} 条(当前页筛选结果)
        </span>
      </div>
      <div class="flex items-center gap-2">
        <AppButton size="sm" variant="outline" :disabled="batchOperating" @click="clearSelection">
          <template #icon><X :size="12" /></template>
          清空选择
        </AppButton>
        <AppButton
          v-if="!recycleBin"
          size="sm"
          variant="outline"
          class="border-err/60 text-err hover:border-err hover:text-err"
          :disabled="batchOperating"
          title="批量逻辑删除:记录与历史访问明细保留,「域名/短码」不再对外重定向"
          @click="handleBatchDelete"
        >
          <template #icon><Trash2 :size="12" /></template>
          批量逻辑删除
        </AppButton>
        <AppButton
          v-if="recycleBin"
          size="sm"
          variant="outline"
          :disabled="batchOperating"
          title="批量还原:短链回到正常列表,短码重新占用"
          @click="handleBatchRestore"
        >
          <template #icon><RotateCcw :size="12" /></template>
          批量还原
        </AppButton>
        <AppButton
          size="sm"
          variant="destructive"
          :disabled="batchOperating"
          :title="
            recycleBin
              ? '批量彻底删除:物理移除回收站里的短链及全部历史访问明细,并清理其落地页文件,不可撤销'
              : '批量彻底删除:物理移除短链及全部历史访问明细,不可撤销'
          "
          @click="handleBatchPurge"
        >
          <template #icon><Flame :size="12" /></template>
          批量彻底删除
        </AppButton>
      </div>
    </div>

    <!-- 搜索与筛选工具栏(回收站只有搜索有意义:类型/状态对已删除的短链是历史快照) -->
    <div
      class="mb-4 grid gap-3"
      :class="recycleBin ? 'sm:grid-cols-1' : 'sm:grid-cols-2 lg:grid-cols-[minmax(0,1fr)_180px_180px_auto]'"
    >
      <AppInput
        v-model="keyword"
        allow-clear
        :aria-label="recycleBin ? '搜索已删除的短链' : '搜索短链'"
        :placeholder="recycleBin ? '搜索已删除短链的短码、域名或目标 URL…' : '搜索短码、域名、目标 URL 或规则名…'"
      >
        <template #prefix><Search :size="15" class="text-ink-faint" /></template>
      </AppInput>
      <AppSelect
        v-if="!recycleBin"
        v-model="typeFilter"
        :options="TYPE_FILTER_OPTIONS"
        aria-label="按类型过滤"
      />
      <AppSelect
        v-if="!recycleBin"
        v-model="statusFilter"
        :options="STATUS_FILTER_OPTIONS"
        aria-label="按状态过滤"
      />
      <AppButton
        v-if="hasActiveFilter"
        size="sm"
        variant="ghost"
        class="justify-self-start text-ink-soft"
        @click="resetFilters"
      >
        <template #icon><X :size="13" /></template>
        清空过滤
      </AppButton>
    </div>

    <!-- 表格数据区 -->
    <AppTable
      :columns="columns"
      :data-source="tableData"
      :loading="loading"
      :scroll="{ x: 1180 }"
      row-key="id"
    >
      <!-- 表头：首列放全选框,其余列回落到 column.title -->
      <template #header="{ column }">
        <AppCheckbox
          v-if="column.key === 'select'"
          :model-value="allSelected"
          :indeterminate="someSelected"
          :disabled="filteredLinks.length === 0"
          aria-label="全选当前页短链"
          @change="toggleSelectAll"
        />
        <template v-else>{{ column.title }}</template>
      </template>

      <template #empty>
        <AppEmpty
          :description="
            recycleBin
              ? deletedTotal === 0
                ? '回收站是空的'
                : '未找到符合当前搜索条件的已删除短链'
              : links.length === 0
                ? '暂无短链记录，请点击下方按钮创建第一条短链'
                : '未找到符合当前筛选条件的短链记录'
          "
        />
        <div class="mt-3 flex justify-center">
          <AppButton v-if="!recycleBin && links.length === 0" size="sm" type="primary" @click="goCreate">
            <template #icon><Plus :size="14" /></template>
            新建短链
          </AppButton>
          <AppButton v-else-if="!recycleBin" size="sm" @click="resetFilters">重置过滤条件</AppButton>
        </div>
      </template>

      <template #cell="{ column, record }">
        <!-- 多选 -->
        <template v-if="column.key === 'select'">
          <AppCheckbox
            :model-value="isSelected(toLink(record).id)"
            :aria-label="'选中短链 ' + toLink(record).code"
            @change="toggleSelectOne(toLink(record).id)"
          />
        </template>

        <!-- 短链链接:每个关联域名一行完整短链(同短码可被多条域名承载) -->
        <template v-else-if="column.key === 'link'">
          <div class="flex flex-col gap-[3px]">
            <div
              v-for="url in linkUrls(toLink(record))"
              :key="url"
              class="flex min-w-0 items-center gap-1.5"
            >
              <a
                :href="url"
                target="_blank"
                rel="noopener noreferrer"
                class="-my-1 truncate py-1 font-mono text-xs underline underline-offset-2 decoration-line-strong hover:decoration-ink"
                :style="{ maxWidth: 'min(100%, 450px)' }"
                :title="url + '（新标签页打开）'"
                @click.stop
              >
                {{ url }}
              </a>
              <AppButton
                size="icon"
                variant="ghost"
                class="size-6 text-ink-faint hover:text-ink"
                :title="'复制 ' + url"
                @click.stop="copyText(url)"
              >
                <template #icon><Copy :size="12" /></template>
              </AppButton>
            </div>
          </div>
        </template>

        <!-- 类型 -->
        <template v-else-if="column.key === 'type'">
          <AppTag color="default">
            {{
              toLink(record).linkType === 'landing'
                ? '落地页型'
                : `跳转型 · ${toLink(record).redirectStatus || '302'}`
            }}
          </AppTag>
        </template>

        <!-- 出口目标 URL:每个目标独占一行,序号即轮询顺序。自动适应宽度,超长悬停 tooltip -->
        <template v-else-if="column.key === 'targets'">
          <div class="flex flex-col gap-[3px]">
            <div v-if="(toLink(record).targetUrls || []).length > 1" class="text-xs text-ink-soft">
              {{ toLink(record).targetUrls.length }} 个目标 · 轮询分发
            </div>
            <div
              v-for="(url, i) in toLink(record).targetUrls || []"
              :key="url + '-' + i"
              class="flex min-w-0 items-center gap-1.5"
            >
              <span
                v-if="(toLink(record).targetUrls || []).length > 1"
                class="shrink-0 rounded-xs border border-line bg-surface-strong/70 px-1 font-mono text-2xs leading-[1.4] text-ink-soft"
                :title="'轮询顺序第 ' + (i + 1) + ' 位'"
              >
                {{ i + 1 }}
              </span>
              <AppTooltip :title="url" class="max-w-md whitespace-normal break-all">
                <span class="min-w-0 flex-1 truncate font-mono text-xs">{{ url }}</span>
              </AppTooltip>
            </div>
            <span v-if="(toLink(record).targetUrls || []).length === 0" class="text-xs text-ink-soft">
              —
            </span>
          </div>
        </template>

        <!-- 访问 / 点击:访问 = 跳转 / 落地页视图,点击 = 落地页按钮回传(仅落地页型有意义) -->
        <template v-else-if="column.key === 'visits'">
          <AppButton
            variant="link"
            size="sm"
            class="metric-link h-auto px-1 py-0.5 text-xs"
            :title="'访问 = 跳转 / 落地页视图次数(点击行不计入);点击 = 落地页按钮经 SDK 回传的次数。查看短链「' + toLink(record).code + '」的访问明细'"
            :aria-label="'查看短链 ' + toLink(record).code + ' 的访问明细'"
            @click="goVisits(toLink(record))"
          >
            <span class="metric-part metric-visits" title="访问次数:跳转 / 落地页视图">
              {{ (toLink(record).visits || 0).toLocaleString() }}
            </span>
            <span class="metric-sep" aria-hidden="true">/</span>
            <span
              class="metric-part metric-clicks"
              :class="{ 'is-empty': toLink(record).linkType !== 'landing' }"
              :title="toLink(record).linkType === 'landing' ? '落地页点击次数(保留期内,与访问量同源同期)' : '非落地页型短链无点击统计'"
            >
              {{
                toLink(record).linkType === 'landing'
                  ? (toLink(record).clickVisits || 0).toLocaleString()
                  : '—'
              }}
            </span>
            <ChevronRight :size="12" class="metric-arrow" aria-hidden="true" />
          </AppButton>
        </template>

        <!-- 状态：开关与文案同格。原先这里是「状态」徽标 + 「启用」开关两列,
             但两者读的都是 link.status,扫一行得看两处才确认得了状态。 -->
        <template v-else-if="column.key === 'status'">
          <!-- 回收站:已删除的短链没有"启用/停用"可言,只标出删除时间 -->
          <span
            v-if="recycleBin"
            class="whitespace-nowrap text-xs text-ink-faint"
            :title="'删除于 ' + formatDateTime(toDeletedLink(record).deletedAt)"
          >
            已删除
          </span>
          <div v-else class="flex items-center gap-2 whitespace-nowrap">
            <AppSwitch
              :model-value="toLink(record).status === 'enabled'"
              :disabled="statusUpdatingId === toLink(record).id"
              :aria-label="
                (toLink(record).status === 'enabled' ? '停用短链 ' : '启用短链 ') +
                toLink(record).code
              "
              @change="onToggleLinkStatus(toLink(record))"
            />
            <span
              class="text-xs whitespace-nowrap"
              :class="toLink(record).status === 'enabled' ? 'text-ink' : 'text-ink-faint'"
            >
              {{ toLink(record).status === 'enabled' ? '已启用' : '已停用' }}
            </span>
          </div>
        </template>

        <!-- 规则：短链维度开关 + 适用规则摘要 -->
        <template v-else-if="column.key === 'rules'">
          <span v-if="recycleBin" class="text-xs text-ink-faint">—</span>
          <div v-else class="flex items-center gap-2 whitespace-nowrap">
            <AppSwitch
              :model-value="toLink(record).rulesEnabled"
              :disabled="rulesUpdatingId === toLink(record).id"
              :title="
                toLink(record).ruleCount === 0
                  ? '未关联规则'
                  : toLink(record).rulesEnabled
                    ? '点击停用当前短链的规则'
                    : '点击启用当前短链的规则'
              "
              :aria-label="
                (toLink(record).rulesEnabled ? '停用短链规则 ' : '启用短链规则 ') +
                toLink(record).code
              "
              @change="onToggleLinkRules(toLink(record))"
            />
            <template v-if="!toLink(record).rulesEnabled">
              <span
                class="text-xs whitespace-nowrap text-ink-faint"
                :title="
                  toLink(record).ruleCount > 0
                    ? '当前短链规则已停用（适用 ' +
                      (toLink(record).ruleNames || []).join('、') +
                      (toLink(record).ruleCount > (toLink(record).ruleNames || []).length
                        ? ' 等 ' + toLink(record).ruleCount + ' 条'
                        : '') +
                      '）'
                    : '未关联规则'
                "
              >
                已停用{{ toLink(record).ruleCount > 0 ? ` (${toLink(record).ruleCount} 条)` : '' }}
              </span>
            </template>
            <template v-else-if="toLink(record).ruleCount === 0">
              <span class="text-xs whitespace-nowrap text-ink-soft">未关联规则</span>
            </template>
            <template v-else>
              <div
                class="flex items-center gap-1 overflow-hidden"
                style="max-width: 160px"
                :title="
                  '适用规则：' +
                  (toLink(record).ruleNames || []).join('、') +
                  (toLink(record).ruleCount > (toLink(record).ruleNames || []).length
                    ? ' 等 ' + toLink(record).ruleCount + ' 条'
                    : '')
                "
              >
                <AppTag
                  v-for="name in visibleRuleNames(toLink(record))"
                  :key="name"
                  color="default"
                  class="max-w-[75px] truncate text-2xs"
                  :title="name"
                >
                  {{ name }}
                </AppTag>
                <span
                  v-if="toLink(record).ruleCount > visibleRuleNames(toLink(record)).length"
                  class="shrink-0 font-mono text-2xs text-ink-faint"
                  :title="'还有 ' + (toLink(record).ruleCount - visibleRuleNames(toLink(record)).length) + ' 条规则'"
                >
                  +{{ toLink(record).ruleCount - visibleRuleNames(toLink(record)).length }}
                </span>
                <span
                  v-if="inheritedRuleCount(toLink(record)) > 0 && (toLink(record).rules || []).length === 0"
                  class="shrink-0 text-2xs text-ink-faint"
                  title="全局规则对所有短链生效"
                >
                  · {{ inheritedRuleCount(toLink(record)) }} 条全局
                </span>
              </div>
            </template>
          </div>
        </template>

        <!-- 操作 -->
        <template v-else-if="column.key === 'actions'">
          <!-- 回收站:只剩「还原」与「彻底删除」两条路径(编辑/状态开关对已删除记录无意义) -->
          <div v-if="recycleBin" class="flex items-center gap-1">
            <AppButton size="sm" variant="ghost" title="还原短链" @click="handleRestoreLink(toLink(record))">
              <template #icon><RotateCcw :size="12" /></template>
              还原
            </AppButton>
            <AppButton
              size="sm"
              variant="ghost"
              class="text-err hover:bg-err/10 hover:text-err"
              title="彻底删除（物理删除全部数据并清理落地页文件）"
              @click="handlePurgeLink(toLink(record))"
            >
              <template #icon><Flame :size="12" /></template>
              彻底删除
            </AppButton>
          </div>
          <div v-else class="flex items-center gap-1">
            <AppButton size="sm" variant="ghost" title="编辑短链" @click="goEdit(toLink(record))">
              <template #icon><Pencil :size="12" /></template>
              编辑
            </AppButton>
            <AppButton
              size="sm"
              variant="ghost"
              class="text-err hover:bg-err/10 hover:text-err"
              title="逻辑删除（保留记录与历史数据）"
              @click="handleDeleteLink(toLink(record))"
            >
              <template #icon><Trash2 :size="12" /></template>
              删除
            </AppButton>
            <AppButton
              size="sm"
              variant="ghost"
              class="text-err opacity-60 hover:bg-err/10 hover:text-err hover:opacity-100"
              title="彻底清除（物理删除全部数据）"
              @click="handlePurgeLink(toLink(record))"
            >
              彻底删除
            </AppButton>
          </div>
        </template>
      </template>
    </AppTable>

    <!-- 底栏与真实分页 -->
    <div class="mt-4 flex flex-wrap items-center justify-between gap-3">
      <div class="flex items-center gap-3 text-xs text-ink-soft">
        <span>
          共 <strong class="font-mono text-ink">{{ total }}</strong>
          {{ recycleBin ? '条已删除短链' : '条短链' }}
        </span>
      </div>
      <div class="flex items-center gap-2.5">
        <div class="flex items-center gap-1.5 text-xs text-ink-soft">
          <span>每页</span>
          <AppSelect
            v-model="pageSize"
            size="sm"
            class="w-[76px]"
            :options="PAGE_SIZE_OPTIONS"
            @change="onPageSizeChange"
          />
          <span>条</span>
        </div>
        <div class="flex items-center gap-1.5">
          <AppButton
            size="sm"
            variant="outline"
            :disabled="page <= 1 || loading"
            @click="goToPage(page - 1)"
          >
            上一页
          </AppButton>
          <span class="px-1 font-mono text-xs text-ink-soft">
            {{ page }} / {{ totalPages }}
          </span>
          <AppButton
            size="sm"
            variant="outline"
            :disabled="page >= totalPages || loading"
            @click="goToPage(page + 1)"
          >
            下一页
          </AppButton>
        </div>
      </div>
    </div>

    <!-- ==================== 模态框: 批量导入短链 ==================== -->
    <AppModal
      v-model:open="showBatchModal"
      title="批量导入短链"
      description="每行一条短链，支持「短码 目标URL」或仅「目标URL」自动生成短码。"
      size="md"
    >
      <div class="flex flex-col gap-3">
        <AppFormItem label="指定承载域名" name="batchDomain">
          <AppSelect v-model="batchDomainId" :options="batchDomainOptions" placeholder="请选择承载域名" />
        </AppFormItem>
        <AppFormItem
          label="短链行列表"
          name="batchInput"
          extra="每行一条，短码与 URL 用空格分隔；若只有 URL 则由后端自动生成短码"
        >
          <AppTextarea
            v-model="batchText"
            :rows="6"
            class="font-mono"
            placeholder="deal-a https://example.com/target-a&#10;deal-b https://example.com/target-b&#10;https://example.com/target-c"
          />
        </AppFormItem>
      </div>

      <template #footer>
        <span class="mr-auto text-xs text-ink-soft">有效行数：{{ parsedBatchLinesCount }}</span>
        <AppButton size="sm" variant="outline" :disabled="batchImporting" @click="showBatchModal = false">
          取消
        </AppButton>
        <AppButton
          size="sm"
          type="primary"
          :disabled="batchImporting || parsedBatchLinesCount === 0"
          @click="executeBatchImport"
        >
          {{ batchImporting ? '导入中...' : '开始导入' }}
        </AppButton>
      </template>
    </AppModal>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, onUnmounted, reactive, ref } from 'vue';
import { useRoute, useRouter } from 'vue-router';
import dayjs from 'dayjs';
import {
  ArrowDown,
  ArrowUp,
  CheckSquare,
  ChevronRight,
  Copy,
  FileDown,
  Flame,
  Pencil,
  Plus,
  RefreshCw,
  RotateCcw,
  Search,
  Trash2,
  Upload,
  X,
} from '@lucide/vue';

import { listDomains } from '@/api/domains';
import {
  batchDeleteLinks,
  batchPurgeLinks,
  createLink,
  deleteLink,
  getLink,
  listDeletedLinks,
  listLinks,
  purgeLink,
  restoreLink,
  updateLink,
  uploadLanding,
} from '@/api/links';
import PageHeader from '@/components/PageHeader.vue';
import type { TableColumn } from '@/components/app/types';
import { confirm } from '@/components/app/confirm';

import { ApiError } from '@/types/api';
import type { Domain, Link, LinkStatus } from '@/types/api';
import { formatDateTime } from '@/utils/format';
import { message } from '@/utils/toast';

import type { DeletedLink } from '@/api/links';

const route = useRoute();
const router = useRouter();

// 配额统一在账号设置展示，本页面不再依赖 auth store。

const DOMAIN_STATUS_NOTE: Record<string, string> = {
  pending: 'DNS 待验证',
  active: '已激活',
  failed: '校验失败',
  stopped: '已停用',
};

// ==================== 响应式基础数据 ====================
const links = ref<Link[]>([]);
const domains = ref<Domain[]>([]);
const loading = ref(false);
const total = ref(0);
const page = ref(1);
const pageSize = ref(20);

const keyword = ref('');
const typeFilter = ref<string>('all');
const statusFilter = ref<string>('all');
const statusUpdatingId = ref<number | null>(null);

// ==================== 回收站视图 ====================
// 软删的短链在正常列表里完全不可见,于是"短码还能不能用""配额还剩多少"
// 这些问题没有任何 UI 答案。回收站就是那个答案:能看见、能还原、能彻底删除。
const viewTab = ref<'active' | 'deleted'>('active');
const recycleBin = computed(() => viewTab.value === 'deleted');
const deletedTotal = ref(0);

/** 回收站行的类型化视图(后端仅在 includeDeleted 时回传 deletedAt) */
function toDeletedLink(record: Record<string, unknown>): DeletedLink {
  return record as unknown as DeletedLink;
}

// ==================== 规则列: 短链级别规则开关与摘要 ====================
const RULE_NAME_VISIBLE = 2;
const rulesUpdatingId = ref<number | null>(null);

function visibleRuleNames(link: Link): string[] {
  return (link.ruleNames || []).slice(0, RULE_NAME_VISIBLE);
}

/** 继承自全局的规则条数 */
function inheritedRuleCount(link: Link): number {
  return Math.max(link.ruleCount - (link.rules || []).length, 0);
}

async function onToggleLinkRules(link: Link): Promise<void> {
  if (rulesUpdatingId.value === link.id) return;
  const next = !link.rulesEnabled;
  const label = next ? '启用' : '停用';

  rulesUpdatingId.value = link.id;
  try {
    const updated = await updateLink(link.id, { rulesEnabled: next });
    link.rulesEnabled = updated.rulesEnabled;
    message.success(`短链「${link.code}」规则已${label}`);
  } catch (error) {
    if (error instanceof ApiError) {
      message.error(error.message);
    } else {
      message.error(`${label}规则失败，请稍后重试`);
    }
  } finally {
    rulesUpdatingId.value = null;
  }
}

const totalPages = computed(() => Math.max(1, Math.ceil(total.value / pageSize.value)));

const activeDomains = computed(() => {
  return domains.value.filter((d) => d.status === 'active');
});

// ==================== 表格列定义与工具栏选项 ====================
const columns: TableColumn[] = [
  { key: 'select', title: '', width: 48 },
  { key: 'link', title: '短链链接' },
  { key: 'type', title: '类型', width: 96 },
  { key: 'targets', title: '出口目标 URL' },
  { key: 'visits', title: '访问 / 点击', width: 140 },
  { key: 'status', title: '状态', width: 118 },
  { key: 'rules', title: '规则', width: 220 },
  { key: 'actions', title: '操作', width: 280, fixed: 'right' },
];

const TYPE_FILTER_OPTIONS = [
  { value: 'all', label: '全部类型' },
  { value: 'redirect', label: '跳转型' },
  { value: 'landing', label: '落地页型' },
];

const STATUS_FILTER_OPTIONS = [
  { value: 'all', label: '全部状态' },
  { value: 'enabled', label: '已启用' },
  { value: 'disabled', label: '已停用' },
];

const PAGE_SIZE_OPTIONS = [10, 20, 50, 100].map((size) => ({ value: size, label: String(size) }));

const hasActiveFilter = computed(() => {
  if (recycleBin.value) return keyword.value.trim() !== '';
  return keyword.value.trim() !== '' || typeFilter.value !== 'all' || statusFilter.value !== 'all';
});

/**
 * AppTable 的行记录是 `Record<string, unknown>`,单元格插槽里统一过一道,
 * 免得每处都写 `record.code as string`(与 RulesListView / DomainsView 一致)。
 */
function toLink(record: Record<string, unknown>): Link {
  return record as unknown as Link;
}

const filteredLinks = computed(() => {
  const q = keyword.value.trim().toLowerCase();

  return links.value.filter((l) => {
    const codeMatch = l.code.toLowerCase().includes(q);
    const domainMatch = l.domains?.some((d) => d.toLowerCase().includes(q)) ?? false;
    const targetMatch = l.targetUrls?.some((u) => u.toLowerCase().includes(q)) ?? false;
    const ruleMatch = l.ruleNames?.some((n) => n.toLowerCase().includes(q)) ?? false;
    const okKeyword = !q || codeMatch || domainMatch || targetMatch || ruleMatch;

    // 回收站不套用类型/状态过滤:两者对已删除的短链只是一个历史快照,
    // 按它们筛会让"回收站里还剩几条"变得难以解释。
    if (recycleBin.value) return okKeyword;

    const tf = typeFilter.value;
    const sf = statusFilter.value;
    let okType = true;
    if (tf === 'redirect') okType = l.linkType === 'redirect';
    else if (tf === 'landing') okType = l.linkType === 'landing';

    let okStatus = true;
    if (sf === 'enabled') okStatus = l.status === 'enabled';
    else if (sf === 'disabled') okStatus = l.status === 'disabled';

    return okKeyword && okType && okStatus;
  });
});

/** AppTable 的 dataSource(与 RulesListView / DomainsView 同一套转法) */
const tableData = computed(() => filteredLinks.value as unknown as Record<string, unknown>[]);

// ==================== 多选与批量删除 ====================
const selectedIds = ref<number[]>([]);
const batchOperating = ref(false);

const selectedCount = computed(() => selectedIds.value.length);

/** 可选范围 = 当前页 + 当前筛选结果(与列表所见一致) */
const selectableIds = computed(() => filteredLinks.value.map((l) => l.id));

const allSelected = computed(
  () => selectableIds.value.length > 0 && selectableIds.value.every((id) => selectedIds.value.includes(id)),
);

const someSelected = computed(() => selectedCount.value > 0 && !allSelected.value);

function isSelected(id: number): boolean {
  return selectedIds.value.includes(id);
}

function toggleSelectOne(id: number) {
  selectedIds.value = isSelected(id)
    ? selectedIds.value.filter((x) => x !== id)
    : [...selectedIds.value, id];
}

function toggleSelectAll() {
  selectedIds.value = allSelected.value ? [] : [...selectableIds.value];
}

function clearSelection() {
  selectedIds.value = [];
}

/** 选中短码摘要(超长截断),用于删除确认文案 */
function selectedCodesSummary(limit = 8): string {
  const codes = filteredLinks.value.filter((l) => isSelected(l.id)).map((l) => l.code);
  if (codes.length === 0) return '';
  const head = codes.slice(0, limit).join('、');
  return codes.length > limit ? `${head} 等 ${codes.length} 条` : head;
}

/** 批量操作后统一收尾:清空选择、重载列表。ids 为确认时快照的选中项。 */
async function afterBatchDone(
  ids: number[],
  ops: () => Promise<{ deleted: number }>,
  action: string,
) {
  batchOperating.value = true;
  try {
    const res = await ops();
    const skipped = ids.length - res.deleted;
    message.success(
      `已${action} ${res.deleted} 条短链` + (skipped > 0 ? `,${skipped} 条不存在或已被删除` : ''),
    );
    clearSelection();
    await loadData();
  } catch (err) {
    if (err instanceof ApiError) message.error(err.message);
    else message.error(`批量${action}失败,请稍后重试`);
  } finally {
    batchOperating.value = false;
  }
}

function handleBatchDelete() {
  if (selectedIds.value.length === 0) return;
  const ids = [...selectedIds.value];
  const summary = selectedCodesSummary();
  confirm({
    title: `逻辑删除选中的 ${ids.length} 条短链?`,
    content:
      '删除为逻辑删除：短链记录与历史访问明细保留，但「域名/短码」将不再对外重定向。' +
      (summary ? `\n涉及短码：${summary}` : ''),
    okText: `确认逻辑删除 ${ids.length} 条`,
    cancelText: '取消',
    danger: true,
    onOk: () => afterBatchDone(ids, () => batchDeleteLinks(ids), '逻辑删除'),
  });
}

function handleBatchPurge() {
  if (selectedIds.value.length === 0) return;
  const ids = [...selectedIds.value];
  const summary = selectedCodesSummary();
  confirm({
    title: `彻底清除选中的 ${ids.length} 条短链?`,
    content:
      '警告：彻底清除为物理删除！将永久移除这些短链及全部历史访问明细，此操作不可撤销！' +
      (summary ? `\n涉及短码：${summary}` : ''),
    okText: `确认彻底删除 ${ids.length} 条`,
    cancelText: '取消',
    danger: true,
    onOk: () => afterBatchDone(ids, () => batchPurgeLinks(ids), '彻底删除'),
  });
}

// ==================== 模态框: 批量导入 ====================
const showBatchModal = ref(false);
const batchText = ref('');
const batchDomainId = ref<number | undefined>(undefined);
const batchImporting = ref(false);

const parsedBatchLinesCount = computed(() => {
  return batchText.value
    .split('\n')
    .map((s) => s.trim())
    .filter(Boolean).length;
});

const batchDomainOptions = computed(() =>
  domains.value.map((d) => ({
    value: d.id,
    label: `${d.fqdn} (${d.origin === 'self' ? '自有' : '默认'})${
      d.status !== 'active' ? ` - ${DOMAIN_STATUS_NOTE[d.status] || '未激活'}` : ''
    }`,
    disabled: d.status !== 'active',
  })),
);

// ==================== 数据加载 ====================
async function loadData() {
  loading.value = true;
  try {
    if (recycleBin.value) {
      const res = await listDeletedLinks(page.value, pageSize.value);
      links.value = res.items;
      total.value = res.total;
      deletedTotal.value = res.total;
      // 选中项可能已不在当前页/筛选结果内,丢弃以免后续批量操作打到陈旧 id
      clearSelection();
    } else {
      const [linksRes, domainsRes, trashRes] = await Promise.all([
        listLinks({ page: page.value, pageSize: pageSize.value }),
        listDomains(),
        // 回收站角标:只取 total,所以 pageSize=1 即可(避免把整页回收站记录拉下来)
        listDeletedLinks(1, 1),
      ]);

      links.value = linksRes.items;
      total.value = linksRes.total;
      domains.value = domainsRes;
      deletedTotal.value = trashRes.total;

      // 批量删除后当前页可能被删空:回退一页再取,避免停在空白页
      if (links.value.length === 0 && page.value > 1) {
        page.value -= 1;
        loading.value = false;
        return loadData();
      }
      // 选中项可能已不在当前页/筛选结果内,丢弃以免后续批量操作打到陈旧 id
      clearSelection();

      if (domainsRes.length > 0 && !batchDomainId.value) {
        const active = domainsRes.find((d) => d.status === 'active');
        batchDomainId.value = active ? active.id : domainsRes[0].id;
      }
    }

  } catch (error) {
    if (error instanceof ApiError) {
      message.error(error.message);
    } else {
      message.error('加载短链数据失败');
    }
    links.value = [];
    total.value = 0;
  } finally {
    loading.value = false;
  }
}

/** 切换短链 / 回收站视图:回到第一页、清空选择与搜索词(旧筛选对新视图无意义) */
function onSwitchTab(value: string | number): void {
  const next = value === 'deleted' ? 'deleted' : 'active';
  if (next === viewTab.value) return;
  viewTab.value = next;
  page.value = 1;
  keyword.value = '';
  clearSelection();
  void loadData();
}

// ==================== 列表与表格操作 ====================
function getPrimaryDomain(link: Link): string {
  if (link.domains && link.domains.length > 0) {
    return link.domains[0];
  }
  return domains.value[0]?.fqdn || '—';
}

/** 短链完整 URL 列表:每个关联域名一行(无关联域名时回退到平台默认域名)。 */
function linkUrls(link: Link): string[] {
  const hosts =
    link.domains && link.domains.length > 0 ? link.domains : [getPrimaryDomain(link)];
  return hosts.map((host) => `https://${host}/${link.code}`);
}

function getLinkCtr(link: Link): string {
  if (link.linkType !== 'landing') return '—';
  if (!link.visits || link.visits === 0) return '—';
  // 分子取 clickVisits(visits 表口径)而不是 clicks(links.clicks 永久计数器):
  // 分母 visits 受保留期清理,用永久计数器做分子会在清理后虚高到 100% 以上。
  if (!link.clickVisits || link.clickVisits === 0) return '0.00%';
  return `${((link.clickVisits / link.visits) * 100).toFixed(2)}%`;
}

async function copyText(text: string) {
  try {
    await navigator.clipboard.writeText(text);
    message.success('已复制: ' + text);
  } catch {
    message.error('复制失败，请手动复制');
  }
}

async function onToggleLinkStatus(link: Link) {
  if (statusUpdatingId.value === link.id) return;
  const nextStatus: LinkStatus = link.status === 'enabled' ? 'disabled' : 'enabled';
  const label = nextStatus === 'disabled' ? '停用' : '启用';

  statusUpdatingId.value = link.id;
  try {
    const updated = await updateLink(link.id, { status: nextStatus });
    link.status = updated.status;
    message.success(`短链「${link.code}」已${label}`);
  } catch (error) {
    if (error instanceof ApiError) {
      message.error(error.message);
    } else {
      message.error(`${label}失败，请稍后重试`);
    }
  } finally {
    statusUpdatingId.value = null;
  }
}

function handleDeleteLink(link: Link) {
  confirm({
    title: `逻辑删除短链「${link.code}」?`,
    content: '删除为逻辑删除：短链记录与历史访问明细保留，但「域名/短码」将不再对外重定向。',
    okText: '确认逻辑删除',
    cancelText: '取消',
    danger: true,
    onOk: async () => {
      try {
        await deleteLink(link.id);
        message.success(`短链「${link.code}」已逻辑删除`);
        await loadData();
      } catch (err) {
        if (err instanceof ApiError) message.error(err.message);
        else message.error('删除短链失败');
      }
    },
  });
}

function handlePurgeLink(link: Link) {
  confirm({
    title: `彻底清除短链「${link.code}」?`,
    content: '警告：彻底清除为物理删除！将永久移除该短链及全部历史访问明细，此操作不可撤销！',
    okText: '彻底物理删除',
    cancelText: '取消',
    danger: true,
    onOk: async () => {
      try {
        await purgeLink(link.id);
        message.success(`短链「${link.code}」已彻底清除`);
        await loadData();
      } catch (err) {
        if (err instanceof ApiError) message.error(err.message);
        else message.error('彻底删除短链失败');
      }
    },
  });
}

function handleRestoreLink(link: Link) {
  confirm({
    title: `还原短链「${link.code}」?`,
    content:
      '还原后短链重新对外跳转,并重新占用该域名下的短码。' +
      '若该短码已被其它短链占用，还原会失败。',
    okText: '确认还原',
    cancelText: '取消',
    onOk: async () => {
      try {
        await restoreLink(link.id);
        message.success(`短链「${link.code}」已还原`);
        await loadData();
      } catch (err) {
        if (err instanceof ApiError) message.error(err.message);
        else message.error('还原短链失败');
      }
    },
  });
}

/** 批量还原:逐条 PATCH,汇总成功数与"短码被占用"导致的失败数。 */
async function handleBatchRestore(): Promise<void> {
  if (selectedIds.value.length === 0) return;
  const ids = [...selectedIds.value];

  batchOperating.value = true;
  try {
    const results = await Promise.allSettled(ids.map((id) => restoreLink(id)));
    const restored = results.filter((r) => r.status === 'fulfilled').length;
    // 409 = 短码已被另一条存活短链占用,是可预期的业务冲突而非故障,单独计数提示
    const conflicts = results.filter(
      (r) => r.status === 'rejected' && (r.reason as ApiError)?.status === 409,
    ).length;

    clearSelection();
    await loadData();
    if (restored > 0) {
      message.success(
        `已还原 ${restored} 条短链` + (conflicts > 0 ? `,${conflicts} 条因短码被占用未能还原` : ''),
      );
    } else {
      message.error(conflicts > 0 ? '所选短链的短码均已被占用,无法还原' : '还原失败,请稍后重试');
    }
  } catch (err) {
    if (err instanceof ApiError) message.error(err.message);
    else message.error('批量还原失败,请稍后重试');
  } finally {
    batchOperating.value = false;
  }
}

function exportCsv() {
  const headers = [
    '短码',
    '承载域名',
    '类型',
    '重定向状态码',
    '目标URL',
    '访问次数',
    '点击次数',
    'CTR',
    '状态',
    '创建时间',
  ];
  const rows = filteredLinks.value.map((l) => [
    l.code,
    (l.domains || []).join('; '),
    l.linkType === 'landing' ? '落地页型' : '跳转型',
    l.redirectStatus || '302',
    (l.targetUrls || []).join('; '),
    l.visits || 0,
    l.linkType === 'landing' ? l.clickVisits || 0 : 0,
    getLinkCtr(l),
    l.status === 'enabled' ? '启用' : '停用',
    formatDateTime(l.createdAt),
  ]);

  const csvContent = [
    headers.join(','),
    ...rows.map((r) => r.map((c) => `"${String(c).replace(/"/g, '""')}"`).join(',')),
  ].join('\n');

  const blob = new Blob(['\uFEFF' + csvContent], { type: 'text/csv;charset=utf-8;' });
  const url = URL.createObjectURL(blob);
  const a = document.createElement('a');
  a.href = url;
  a.download = `links-export-${dayjs().format('YYYYMMDD-HHmmss')}.csv`;
  a.click();
  URL.revokeObjectURL(a.href);
  message.success(`已导出 ${rows.length} 条短链记录`);
}

function resetFilters() {
  keyword.value = '';
  typeFilter.value = 'all';
  statusFilter.value = 'all';
}

function goToPage(p: number) {
  if (p < 1 || p > totalPages.value || p === page.value) return;
  page.value = p;
  loadData();
}

function onPageSizeChange() {
  page.value = 1;
  loadData();
}

// ==================== 独立页面路由导航 ====================
function goCreate() {
  router.push('/links/new');
}

function goEdit(link: Link) {
  router.push(`/links/${link.id}/edit`);
}

/** 跳转到该短链的访问明细列表(列表页唯一入口) */
function goVisits(link: Link) {
  router.push(`/links/${link.id}/visits`);
}

// ==================== 模态框操作: 批量导入 ====================
function openBatchModal() {
  batchText.value = '';
  const firstActive = activeDomains.value[0] || domains.value[0];
  if (firstActive) {
    batchDomainId.value = firstActive.id;
  }
  showBatchModal.value = true;
}

async function executeBatchImport() {
  const lines = batchText.value
    .split('\n')
    .map((l) => l.trim())
    .filter(Boolean);

  if (lines.length === 0) return;
  if (!batchDomainId.value) {
    message.error('请选择承载域名');
    return;
  }

  batchImporting.value = true;
  let successCount = 0;
  let failCount = 0;
  const errors: string[] = [];

  for (const line of lines) {
    const parts = line.split(/[\s,]+/).filter(Boolean);
    let code: string | undefined = undefined;
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
      new URL(url);
    } catch {
      failCount++;
      errors.push(`URL 非法: ${url}`);
      continue;
    }

    try {
      await createLink({
        code: code || undefined,
        targetUrls: [url],
        domainIds: [batchDomainId.value],
        redirectStatus: '302',
        linkType: 'redirect',
      });
      successCount++;
    } catch (err) {
      failCount++;
      if (err instanceof ApiError) {
        errors.push(`${code || url}: ${err.message}`);
      }
    }
  }

  batchImporting.value = false;
  showBatchModal.value = false;
  batchText.value = '';

  if (successCount > 0) {
    message.success(`成功导入 ${successCount} 条短链${failCount > 0 ? `，失败 ${failCount} 条` : ''}`);
    await loadData();
  } else {
    message.error(`导入失败：全部 ${failCount} 条均未成功 (${errors[0] || '未知错误'})`);
  }
}

// ==================== 键盘事件与路由监听 ====================
function onKeydown(e: KeyboardEvent) {
  if (e.key === 'Escape') {
    if (showBatchModal.value) {
      showBatchModal.value = false;
    }
  }
}

onMounted(async () => {
  window.addEventListener('keydown', onKeydown);
  await loadData();

  if (route.query.new === '1') {
    goCreate();
  } else if (route.query.edit) {
    const editIdOrCode = String(route.query.edit);
    const found = links.value.find(
      (l) => String(l.id) === editIdOrCode || l.code === editIdOrCode,
    );
    if (found) {
      goEdit(found);
    } else {
      const numId = Number(editIdOrCode);
      if (!isNaN(numId) && numId > 0) {
        router.push(`/links/${numId}/edit`);
      }
    }
  }
});

onUnmounted(() => {
  window.removeEventListener('keydown', onKeydown);
});
</script>

<style scoped>
/* 「访问 / 点击」合并列的可点击数字:访问数用前景主色,点击数用次级色;
   非落地页型无点击数据,弱化为破折号。hover 下划线 + 箭头右移提示可点。 */
.metric-link {
  display: inline-flex;
  align-items: center;
  gap: 3px;
  padding: 2px 2px 2px 0;
  border: 0;
  background: transparent;
  color: inherit;
  font: inherit;
  cursor: pointer;
  border-radius: 4px;
}

.metric-part {
  text-decoration: underline;
  text-decoration-color: transparent;
  text-underline-offset: 3px;
  transition: text-decoration-color 0.14s ease;
}

.metric-visits {
  color: var(--ink);
}

.metric-clicks {
  color: var(--ink-soft);
}

.metric-clicks.is-empty {
  color: var(--ink-faint);
  text-decoration: none;
}

.metric-sep {
  color: var(--ink-faint);
}

.metric-arrow {
  color: var(--ink-faint);
  opacity: 0;
  transform: translateX(-2px);
  transition:
    opacity 0.14s ease,
    transform 0.14s ease;
}

.metric-link:hover .metric-part {
  text-decoration-color: color-mix(in srgb, var(--ink) 55%, transparent);
}

.metric-link:hover .metric-visits {
  color: var(--brand);
}

.metric-link:hover .metric-arrow {
  opacity: 1;
  transform: translateX(0);
}

.metric-link:focus-visible {
  outline: 2px solid var(--brand);
  outline-offset: 2px;
}
</style>
