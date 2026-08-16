<template>
  <div class="min-h-screen bg-surface-muted">
    <!-- ================= 桌面侧边栏(≥768px) ================= -->
    <aside
      class="fixed inset-y-0 left-0 z-30 hidden flex-col overflow-hidden bg-[var(--sidebar-bg)] transition-[width] duration-200 md:flex"
      :class="collapsed ? 'w-[64px]' : 'w-[224px]'"
    >
      <div
        class="flex h-14 shrink-0 cursor-pointer select-none items-center justify-center gap-2.5"
        @click="router.push('/domains')"
      >
        <BrandMark :size="collapsed ? 30 : 32" />
        <span v-if="!collapsed" class="text-[15px] font-bold tracking-[2px] text-white">CLOAK 后台</span>
      </div>

      <nav class="flex-1 overflow-y-auto px-2.5 py-2">
        <template v-for="group in navGroups" :key="group.title">
          <p v-if="!collapsed" class="px-2.5 pt-3 pb-1.5 text-[11px] font-medium tracking-wider text-slate-500">
            {{ group.title }}
          </p>
          <button
            v-for="item in visibleItems(group)"
            :key="item.to"
            type="button"
            class="group mb-0.5 flex w-full items-center gap-2.5 rounded-lg px-2.5 py-2 text-[13px] transition-colors"
            :class="
              isActive(item.to)
                ? 'bg-brand-600 font-medium text-white shadow-sm'
                : 'text-slate-400 hover:bg-white/5 hover:text-slate-100'
            "
            :title="collapsed ? item.label : undefined"
            @click="router.push(item.to)"
          >
            <component :is="item.icon" :size="16" class="shrink-0" :class="collapsed ? 'mx-auto' : ''" />
            <span v-if="!collapsed">{{ item.label }}</span>
          </button>
        </template>
      </nav>

      <div class="shrink-0 pb-4 pt-2 text-center text-[11px] tracking-wider text-slate-600">
        <span v-if="!collapsed">CLOAK v0.1</span>
      </div>
    </aside>

    <!-- ================= 移动端抽屉 ================= -->
    <Transition name="drawer-fade">
      <div v-if="isMobile && drawerOpen" class="fixed inset-0 z-40 md:hidden">
        <div class="absolute inset-0 bg-black/45" @click="drawerOpen = false" />
        <aside class="absolute inset-y-0 left-0 flex w-64 flex-col bg-[var(--sidebar-bg)] shadow-2xl">
          <div class="flex h-14 shrink-0 items-center justify-between px-4">
            <div class="flex cursor-pointer items-center gap-2.5" @click="go('/domains')">
              <BrandMark :size="28" />
              <span class="text-[15px] font-bold tracking-[2px] text-white">CLOAK 后台</span>
            </div>
            <button type="button" class="text-slate-400 hover:text-white" @click="drawerOpen = false">
              <X :size="18" />
            </button>
          </div>
          <nav class="flex-1 overflow-y-auto px-2.5 py-2">
            <template v-for="group in navGroups" :key="group.title">
              <p class="px-2.5 pt-3 pb-1.5 text-[11px] font-medium tracking-wider text-slate-500">{{ group.title }}</p>
              <button
                v-for="item in visibleItems(group)"
                :key="item.to"
                type="button"
                class="mb-0.5 flex w-full items-center gap-2.5 rounded-lg px-2.5 py-2 text-[13px] transition-colors"
                :class="isActive(item.to) ? 'bg-brand-600 font-medium text-white' : 'text-slate-400 hover:bg-white/5 hover:text-slate-100'"
                @click="go(item.to)"
              >
                <component :is="item.icon" :size="16" class="shrink-0" />
                {{ item.label }}
              </button>
            </template>
          </nav>
        </aside>
      </div>
    </Transition>

    <!-- ================= 主区域 ================= -->
    <div class="flex min-h-screen flex-col md:pl-[224px]" :class="collapsed ? 'md:pl-[64px]' : ''">
      <header class="sticky top-0 z-20 flex h-14 items-center justify-between border-b border-line bg-surface px-4 shadow-sm md:px-6">
        <div class="flex min-w-0 items-center gap-2">
          <button
            type="button"
            class="flex h-8 w-8 shrink-0 items-center justify-center rounded-lg text-ink-soft transition-colors hover:bg-surface-strong hover:text-ink"
            @click="onToggleSidebar"
          >
            <PanelLeftClose v-if="!collapsed && !isMobile" :size="17" />
            <PanelLeftOpen v-else-if="!isMobile" :size="17" />
            <Menu v-else :size="17" @click="drawerOpen = true" />
          </button>
          <h1 class="truncate text-[15px] font-semibold text-ink">{{ routeTitle }}</h1>
        </div>

        <div class="flex shrink-0 items-center gap-2.5">
          <AppTag v-if="auth.tenant?.tier" color="cyan">等级 · {{ auth.tenant.tier.name }}</AppTag>

          <button
            type="button"
            class="flex h-8 w-8 items-center justify-center rounded-lg text-ink-soft transition-colors hover:bg-surface-strong hover:text-ink"
            :title="theme.isDark ? '切换到浅色模式' : '切换到深色模式'"
            @click="theme.toggle()"
          >
            <Sun v-if="theme.isDark" :size="16" />
            <Moon v-else :size="16" />
          </button>

          <DropdownMenuRoot>
            <DropdownMenuTrigger as-child>
              <button
                type="button"
                class="flex items-center gap-2 rounded-full py-1 pr-1.5 pl-1 transition-colors hover:bg-surface-strong"
              >
                <span class="flex h-7 w-7 items-center justify-center rounded-full bg-brand-600 text-[12px] font-semibold text-white">
                  {{ avatarInitial }}
                </span>
                <span class="hidden max-w-44 truncate text-[13px] text-ink sm:block">{{ auth.tenant?.email }}</span>
                <ChevronDown :size="12" class="text-ink-faint" />
              </button>
            </DropdownMenuTrigger>
            <DropdownMenuPortal>
              <DropdownMenuContent
                :side-offset="6"
                align="end"
                class="z-[75] min-w-40 rounded-xl border border-line bg-surface p-1.5 shadow-xl"
              >
                <DropdownMenuItem
                  class="flex cursor-pointer items-center gap-2 rounded-lg px-2.5 py-2 text-[13px] text-ink outline-none data-[highlighted]:bg-brand-50 data-[highlighted]:text-brand-700 dark:data-[highlighted]:bg-brand-500/15 dark:data-[highlighted]:text-brand-300"
                  @select="go('/account')"
                >
                  <User :size="14" />
                  账号设置
                </DropdownMenuItem>
                <DropdownMenuSeparator class="my-1 h-px bg-line" />
                <DropdownMenuItem
                  class="flex cursor-pointer items-center gap-2 rounded-lg px-2.5 py-2 text-[13px] text-err outline-none data-[highlighted]:bg-err/10"
                  @select="onLogout"
                >
                  <LogOut :size="14" />
                  退出登录
                </DropdownMenuItem>
              </DropdownMenuContent>
            </DropdownMenuPortal>
          </DropdownMenuRoot>
        </div>
      </header>

      <main class="mx-auto w-full max-w-[1440px] flex-1 px-4 py-5 md:px-6">
        <router-view v-slot="{ Component }">
          <transition name="page" mode="out-in">
            <component :is="Component" :key="route.fullPath" />
          </transition>
        </router-view>
      </main>

      <footer class="pb-5 text-center text-xs text-ink-faint">CLOAK · 自托管短链服务</footer>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue';
import { useRoute, useRouter } from 'vue-router';
import {
  BarChart3,
  ChevronDown,
  Crown,
  Globe,
  Link2,
  LogOut,
  Menu,
  Moon,
  PanelLeftClose,
  PanelLeftOpen,
  Settings,
  Sun,
  User,
  X,
} from '@lucide/vue';
import {
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuPortal,
  DropdownMenuRoot,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from 'reka-ui';

import BrandMark from '@/components/BrandMark.vue';
import { useAuthStore } from '@/stores/auth';
import { useThemeStore } from '@/stores/theme';

interface NavItem {
  to: string;
  label: string;
  icon: unknown;
  superAdmin?: boolean;
}

interface NavGroup {
  title: string;
  items: NavItem[];
}

const auth = useAuthStore();
const theme = useThemeStore();
const route = useRoute();
const router = useRouter();

const collapsed = ref(false);
const drawerOpen = ref(false);
const isMobile = ref(window.matchMedia('(max-width: 767px)').matches);

const mql = window.matchMedia('(max-width: 767px)');
function onMqlChange(event: MediaQueryListEvent): void {
  isMobile.value = event.matches;
  if (event.matches) drawerOpen.value = false;
}
onMounted(() => {
  mql.addEventListener('change', onMqlChange);
});
onBeforeUnmount(() => {
  mql.removeEventListener('change', onMqlChange);
});

watch(
  () => route.fullPath,
  () => {
    drawerOpen.value = false;
  },
);

const navGroups: NavGroup[] = [
  {
    title: '管理',
    items: [
      { to: '/domains', label: '域名', icon: Globe },
      { to: '/links', label: '短链', icon: Link2 },
      { to: '/stats', label: '统计', icon: BarChart3 },
    ],
  },
  {
    title: '系统',
    items: [
      { to: '/account', label: '账号设置', icon: Settings },
      { to: '/admin/tenants', label: '平台管理', icon: Crown, superAdmin: true },
    ],
  },
];

function visibleItems(group: NavGroup): NavItem[] {
  return group.items.filter((item) => item.superAdmin !== true || auth.isSuperAdmin);
}

function isActive(to: string): boolean {
  return route.path === to || route.path.startsWith(to + '/');
}

const routeTitle = computed(() => (route.meta.title as string | undefined) ?? 'CLOAK 后台');

const avatarInitial = computed(() => {
  const email = auth.tenant?.email ?? '?';
  return email.charAt(0).toUpperCase();
});

function onToggleSidebar(): void {
  if (isMobile.value) {
    drawerOpen.value = true;
  } else {
    collapsed.value = !collapsed.value;
  }
}

function go(to: string): void {
  router.push(to);
}

async function onLogout(): Promise<void> {
  await auth.logout();
  router.push('/login');
}
</script>

<style scoped>
.drawer-fade-enter-active,
.drawer-fade-leave-active {
  transition: opacity 0.2s ease;
}

.drawer-fade-enter-from,
.drawer-fade-leave-to {
  opacity: 0;
}
</style>
