<template>
  <a-layout class="admin-layout">
    <!-- 侧边栏 -->
    <a-layout-sider
      v-model:collapsed="collapsed"
      collapsible
      :trigger="null"
      :width="224"
      :collapsed-width="64"
      theme="dark"
      class="sider"
    >
      <div class="sider-logo" @click="router.push('/domains')">
        <BrandMark :size="collapsed ? 30 : 32" />
        <span v-if="!collapsed" class="sider-wordmark">CLOAK 后台</span>
      </div>

      <a-menu theme="dark" mode="inline" :selected-keys="[activeKey]" @click="onMenuClick">
        <a-menu-item-group v-if="!collapsed" title="管理">
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
        </a-menu-item-group>

        <a-menu-item-group v-if="!collapsed" title="系统">
          <a-menu-item key="/account">
            <SettingOutlined />
            <span>账号设置</span>
          </a-menu-item>
          <a-menu-item v-if="auth.isSuperAdmin" key="/admin/tenants">
            <CrownOutlined />
            <span>平台管理</span>
          </a-menu-item>
        </a-menu-item-group>
      </a-menu>

      <div v-if="!collapsed" class="sider-version">CLOAK v0.1</div>
    </a-layout-sider>

    <!-- 主区域 -->
    <a-layout class="main-area">
      <a-layout-header class="header">
        <div class="header-left">
          <a-button type="text" class="collapse-btn" @click="collapsed = !collapsed">
            <template #icon>
              <MenuFoldOutlined v-if="!collapsed" />
              <MenuUnfoldOutlined v-else />
            </template>
          </a-button>
          <span class="header-title">{{ routeTitle }}</span>
        </div>

        <div class="header-right">
          <a-tag v-if="auth.tenant?.tier" class="tier-tag" color="cyan">
            等级 · {{ auth.tenant.tier.name }}
          </a-tag>

          <a-dropdown placement="bottomRight" :trigger="['click']">
            <div class="user-chip">
              <a-avatar :size="30" class="user-avatar">{{ avatarInitial }}</a-avatar>
              <span class="user-email">{{ auth.tenant?.email }}</span>
              <DownOutlined class="user-caret" />
            </div>
            <template #overlay>
              <a-menu @click="onUserMenu">
                <a-menu-item key="account">
                  <UserOutlined />
                  账号设置
                </a-menu-item>
                <a-menu-item-divider />
                <a-menu-item key="logout" danger>
                  <LogoutOutlined />
                  退出登录
                </a-menu-item>
              </a-menu>
            </template>
          </a-dropdown>
        </div>
      </a-layout-header>

      <a-layout-content class="content">
        <router-view v-slot="{ Component }">
          <transition name="page" mode="out-in">
            <component :is="Component" />
          </transition>
        </router-view>
      </a-layout-content>

      <a-layout-footer class="footer">CLOAK · 自托管短链服务</a-layout-footer>
    </a-layout>
  </a-layout>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue';
import { useRoute, useRouter } from 'vue-router';
import {
  BarChartOutlined,
  CrownOutlined,
  DownOutlined,
  GlobalOutlined,
  LinkOutlined,
  LogoutOutlined,
  MenuFoldOutlined,
  MenuUnfoldOutlined,
  SettingOutlined,
  UserOutlined,
} from '@ant-design/icons-vue';

import BrandMark from '@/components/BrandMark.vue';
import { useAuthStore } from '@/stores/auth';

const auth = useAuthStore();
const route = useRoute();
const router = useRouter();

const collapsed = ref(false);
const activeKey = computed(() => route.path);
const routeTitle = computed(() => (route.meta.title as string | undefined) ?? 'CLOAK 后台');

const avatarInitial = computed(() => {
  const email = auth.tenant?.email ?? '?';
  return email.charAt(0).toUpperCase();
});

function onMenuClick({ key }: { key: string | number }) {
  router.push(String(key));
}

async function onUserMenu({ key }: { key: string | number }) {
  if (key === 'logout') {
    await auth.logout();
    router.push('/login');
  } else if (key === 'account') {
    router.push('/account');
  }
}
</script>

<style scoped>
.admin-layout {
  min-height: 100vh;
}

/* ---------- 侧边栏 ---------- */
.sider {
  position: sticky;
  top: 0;
  height: 100vh;
  overflow: auto;
}

.sider-logo {
  height: 56px;
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 10px;
  cursor: pointer;
  user-select: none;
}

.sider-wordmark {
  color: #f8fafc;
  font-size: 15px;
  font-weight: 700;
  letter-spacing: 2px;
  white-space: nowrap;
}

.sider :deep(.ant-menu) {
  border-inline-end: none;
}

.sider-version {
  position: absolute;
  bottom: 14px;
  left: 0;
  right: 0;
  text-align: center;
  color: rgba(148, 163, 184, 0.45);
  font-size: 11px;
  letter-spacing: 1px;
}

/* ---------- 顶栏 ---------- */
.header {
  position: sticky;
  top: 0;
  z-index: 10;
  display: flex;
  align-items: center;
  justify-content: space-between;
  background: #fff;
  border-bottom: 1px solid #e6ebf1;
  box-shadow: 0 1px 3px rgb(15 23 42 / 4%);
}

.header-left {
  display: flex;
  align-items: center;
  gap: 8px;
}

.collapse-btn {
  color: #5b6b7c;
}

.header-title {
  font-size: 15px;
  font-weight: 600;
  color: #0f172a;
}

.header-right {
  display: flex;
  align-items: center;
  gap: 14px;
}

.tier-tag {
  margin-inline-end: 0;
}

.user-chip {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 4px 10px 4px 4px;
  border-radius: 20px;
  cursor: pointer;
  transition: background-color 0.2s;
}

.user-chip:hover {
  background: #f1f5f9;
}

.user-avatar {
  background: #0e7490;
  font-size: 13px;
  font-weight: 600;
}

.user-email {
  max-width: 220px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  font-size: 13px;
  color: #334155;
}

.user-caret {
  font-size: 10px;
  color: #8b98a5;
}

/* ---------- 内容区 ---------- */
.content {
  margin: 20px 24px 0;
  padding: 20px 24px 28px;
  background: #fff;
  border: 1px solid #e6ebf1;
  border-radius: 12px;
  min-height: calc(100vh - 56px - 20px - 56px);
}

.footer {
  text-align: center;
  padding: 14px 0;
  color: #8b98a5;
  font-size: 12px;
  background: transparent;
}

/* 页面切换过渡 */
.page-enter-active,
.page-leave-active {
  transition:
    opacity 0.18s ease,
    transform 0.18s ease;
}

.page-enter-from {
  opacity: 0;
  transform: translateY(6px);
}

.page-leave-to {
  opacity: 0;
}
</style>
