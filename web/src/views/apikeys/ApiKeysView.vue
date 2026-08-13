<template>
  <div>
    <div class="page-header">
      <h2 class="page-title">API Key</h2>
      <a-button type="primary" @click="openCreate">
        <template #icon><PlusOutlined /></template>
        生成 API Key
      </a-button>
    </div>

    <a-alert
      class="tip-alert"
      type="info"
      show-icon
      message="API Key 用于程序化调用公开 REST API(Authorization: Bearer <key>)。密钥明文仅在生成时展示一次,请立即保存。"
    />

    <a-table
      :columns="columns"
      :data-source="keys"
      :loading="loading"
      row-key="id"
      :pagination="false"
    >
      <template #bodyCell="{ column, record }">
        <template v-if="column.key === 'createdAt'">
          {{ formatDateTime(record.createdAt) }}
        </template>
        <template v-else-if="column.key === 'action'">
          <a-popconfirm
            title="吊销后该 Key 立即失效,且无法恢复。确定吊销?"
            ok-text="吊销"
            :ok-button-props="{ danger: true }"
            cancel-text="取消"
            @confirm="onRevoke(record)"
          >
            <a-button size="small" type="text" danger>吊销</a-button>
          </a-popconfirm>
        </template>
      </template>
    </a-table>

    <!-- 生成 -->
    <a-modal
      v-model:open="createOpen"
      title="生成 API Key"
      :confirm-loading="creating"
      ok-text="生成"
      cancel-text="取消"
      @ok="onCreate"
    >
      <a-form layout="vertical">
        <a-form-item label="名称" required>
          <a-input
            v-model:value="newName"
            placeholder="例如:我的 CI 脚本"
            :maxlength="64"
            @press-enter="onCreate"
          />
        </a-form-item>
      </a-form>
    </a-modal>

    <!-- 明文仅展示一次 -->
    <a-modal
      v-model:open="keyOpen"
      title="API Key 已生成"
      :footer="null"
      :closable="true"
    >
      <a-alert
        type="warning"
        show-icon
        message="密钥明文仅展示这一次,关闭后将无法再次查看。请立即复制并妥善保存。"
      />
      <div class="key-box">
        <a-typography-text code copyable>{{ plainKey }}</a-typography-text>
      </div>
      <div class="key-actions">
        <a-button type="primary" @click="copyKey">复制密钥</a-button>
        <a-button @click="keyOpen = false">我已保存</a-button>
      </div>
    </a-modal>
  </div>
</template>

<script setup lang="ts">
import { onMounted, ref } from 'vue';
import { message } from 'ant-design-vue';
import { PlusOutlined } from '@ant-design/icons-vue';
import type { TableColumnsType } from 'ant-design-vue';

import { createApiKey, deleteApiKey, listApiKeys } from '@/api/apiKeys';
import { ApiError } from '@/types/api';
import type { ApiKey } from '@/types/api';
import { formatDateTime } from '@/utils/format';

const keys = ref<ApiKey[]>([]);
const loading = ref(false);

const createOpen = ref(false);
const creating = ref(false);
const newName = ref('');

const keyOpen = ref(false);
const plainKey = ref('');

const columns: TableColumnsType = [
  { title: '名称', key: 'name', dataIndex: 'name' },
  { title: '创建时间', key: 'createdAt', dataIndex: 'createdAt', width: 200 },
  { title: '操作', key: 'action', width: 120 },
];

async function load() {
  loading.value = true;
  try {
    keys.value = await listApiKeys();
  } catch (error) {
    if (error instanceof ApiError && error.status !== 401) {
      message.error(error.message);
    }
  } finally {
    loading.value = false;
  }
}

onMounted(load);

function openCreate() {
  newName.value = '';
  createOpen.value = true;
}

async function onCreate() {
  const name = newName.value.trim();
  if (!name) {
    message.warning('请输入名称');
    return;
  }
  creating.value = true;
  try {
    const created = await createApiKey({ name });
    plainKey.value = created.key ?? '';
    createOpen.value = false;
    keyOpen.value = true;
    await load();
  } catch (error) {
    if (error instanceof ApiError) message.error(error.message);
    else message.error('生成失败,请稍后重试');
  } finally {
    creating.value = false;
  }
}

async function copyKey() {
  try {
    await navigator.clipboard.writeText(plainKey.value);
    message.success('已复制到剪贴板');
  } catch {
    message.error('复制失败,请手动选择复制');
  }
}

async function onRevoke(record: ApiKey) {
  try {
    await deleteApiKey(record.id);
    message.success(`API Key「${record.name}」已吊销`);
    await load();
  } catch (error) {
    if (error instanceof ApiError) message.error(error.message);
    else message.error('吊销失败,请稍后重试');
  }
}
</script>

<style scoped>
.page-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 16px;
}

.page-title {
  margin: 0;
  font-size: 18px;
}

.tip-alert {
  margin-bottom: 16px;
}

.key-box {
  margin: 16px 0;
  padding: 12px;
  background: #fafafa;
  border: 1px dashed #d9d9d9;
  border-radius: 6px;
  word-break: break-all;
}

.key-actions {
  display: flex;
  gap: 8px;
}
</style>
