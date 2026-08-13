<template>
  <a-layout style="min-height: 100vh">
    <a-layout-sider v-model:collapsed="collapsed" collapsible :width="220" theme="dark">
      <div class="logo">
        <span v-if="!collapsed">CLOAK 后台</span>
        <span v-else>CL</span>
      </div>
      <a-menu
        theme="dark"
        mode="inline"
        :selected-keys="[activeKey]"
        @click="onMenuClick"
      >
        <a-menu-item key="/domains">
          <GlobalOutlined />
          <span>域名</span>
        </a-menu-item>
        <a-menu-item key="/links">
          <LinkOutlined />
          <span>短链</span>
        </a-menu-item>
        <a-menu-item key="/stats">
          <BarChartOutlined />
          <span>统计</span>
        </a-menu-item>
        <a-menu-item key="/api-keys">
          <KeyOutlined />
          <span>API Key</span>
        </a-menu-item>
        <a-menu-item key="/account">
          <SettingOutlined />
          <span>账号设置</span>
        </a-menu-item>
        <a-menu-item v-if="auth.isSuperAdmin" key="/admin/tenants">
          <CrownOutlined />
          <span>平台管理</span>
        </a-menu-item>
      </a-menu>
    </a-layout-sider>

    <a-layout>
      <a-layout-header class="header">
        <div class="header-right">
          <a-tag color="blue">等级:{{ auth.tenant?.tier?.name ?? '-' }}</a-tag>
          <span class="email">{{ auth.tenant?.email }}</span>
          <a-button type="text" @click="onLogout">
            <template #icon><LogoutOutlined /></template>
            退出登录
          </a-button>
        </div>
      </a-layout-header>

      <a-layout-content class="content">
        <router-view />
      </a-layout-content>
    </a-layout>
  </a-layout>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue';
import { useRoute, useRouter } from 'vue-router';
import {
  BarChartOutlined,
  CrownOutlined,
  GlobalOutlined,
  KeyOutlined,
  LinkOutlined,
  LogoutOutlined,
  SettingOutlined,
} from '@ant-design/icons-vue';

import { useAuthStore } from '@/stores/auth';

const auth = useAuthStore();
const route = useRoute();
const router = useRouter();

const collapsed = ref(false);
const activeKey = computed(() => route.path);

function onMenuClick({ key }: { key: string | number }) {
  router.push(String(key));
}

async function onLogout() {
  await auth.logout();
  router.push('/login');
}
</script>

<style scoped>
.logo {
  height: 56px;
  display: flex;
  align-items: center;
  justify-content: center;
  color: #fff;
  font-size: 16px;
  font-weight: 600;
  letter-spacing: 1px;
}

.header {
  background: #fff;
  padding: 0 24px;
  display: flex;
  justify-content: flex-end;
  align-items: center;
  box-shadow: 0 1px 4px rgb(0 21 41 / 8%);
}

.header-right {
  display: flex;
  align-items: center;
  gap: 12px;
}

.email {
  color: rgba(0, 0, 0, 0.65);
}

.content {
  margin: 16px;
  padding: 16px;
  background: #fff;
  border-radius: 8px;
  min-height: calc(100vh - 88px);
}
</style>
