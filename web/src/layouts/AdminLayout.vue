<template>
  <div class="min-h-screen bg-surface-muted" :style="sidebarStyle">
    <!-- ================= 桌面侧边栏(≥768px) ================= -->
    <aside
      class="fixed inset-y-0 left-0 z-30 hidden w-[var(--sidebar-w)] flex-col overflow-hidden bg-[var(--sidebar-bg)] transition-[width] duration-200 md:flex"
    >
      <div
        class="flex h-14 shrink-0 cursor-pointer select-none items-center gap-2.5 px-3.5"
        @click="router.push('/overview')"
      >
        <span
          class="flex h-7 w-7 shrink-0 items-center justify-center rounded-lg bg-ink font-mono text-[13px] font-bold text-surface shadow-xs"
        >
          C
        </span>
        <div v-if="!collapsed" class="flex flex-col leading-tight">
          <span class="text-[14.5px] font-bold tracking-tight text-white">CLOAK</span>
          <span class="font-mono text-[10px] tracking-wider text-slate-400 uppercase">Cloak Console</span>
        </div>
      </div>

      <nav class="flex-1 overflow-y-auto px-2.5 py-2">
        <template v-for="group in navGroups" :key="group.title">
          <p v-if="!collapsed" class="px-2.5 pt-3 pb-1.5 font-mono text-[10.5px] font-semibold tracking-wider text-slate-400 uppercase">
            {{ group.title }}
          </p>
          <button
            v-for="item in visibleItems(group)"
            :key="item.to"
            type="button"
            class="group mb-0.5 flex w-full items-center gap-2.5 rounded-lg px-2.5 py-2 text-[13px] transition-colors"
            :class="
              isActive(item.to)
                ? 'bg-brand-600 font-semibold text-white shadow-sm'
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

      <div v-if="!collapsed" class="border-t border-white/10 px-3.5 py-3">
        <div class="truncate font-mono text-[10.5px] tracking-wider text-slate-400 uppercase">
          租户 · {{ auth.tenant?.slug?.toUpperCase() || auth.tenant?.email?.split('@')[0]?.toUpperCase() || 'TENANT' }}
        </div>
        <div class="mt-2 flex items-center gap-2.5">
          <span class="flex h-7 w-7 items-center justify-center rounded-full bg-white/10 font-mono text-[11px] font-semibold text-slate-200">
            {{ avatarInitial }}
          </span>
          <div class="min-w-0 text-[12px] leading-tight text-slate-300">
            <div>{{ auth.isSuperAdmin ? '超级管理员' : '租户管理员' }}</div>
            <div class="font-mono text-[10px] text-slate-400">{{ auth.tenant?.tier?.name || '标准版' }}</div>
          </div>
        </div>
      </div>
      <div v-else class="pb-3 pt-2 text-center font-mono text-[10px] text-slate-400">
        C
      </div>
    </aside>

    <!-- ================= 移动端抽屉 ================= -->
    <Transition name="drawer-fade">
      <div v-if="isMobile && drawerOpen" class="fixed inset-0 z-40 md:hidden">
        <div class="absolute inset-0 bg-black/45" @click="drawerOpen = false" />
        <aside class="absolute inset-y-0 left-0 flex w-64 flex-col bg-[var(--sidebar-bg)] shadow-2xl">
          <div class="flex h-14 shrink-0 items-center justify-between px-4">
            <div class="flex cursor-pointer items-center gap-2.5" @click="go('/overview')">
              <span
                class="flex h-7 w-7 items-center justify-center rounded-lg bg-ink font-mono text-[13px] font-bold text-surface shadow-xs"
              >
                C
              </span>
              <div class="flex flex-col leading-tight">
                <span class="text-[14.5px] font-bold tracking-tight text-white">CLOAK</span>
                <span class="font-mono text-[10px] tracking-wider text-slate-400 uppercase">Cloak Console</span>
              </div>
            </div>
            <button type="button" class="text-slate-400 hover:text-white" @click="drawerOpen = false">
              <X :size="18" />
            </button>
          </div>
          <nav class="flex-1 overflow-y-auto px-2.5 py-2">
            <template v-for="group in navGroups" :key="group.title">
              <p class="px-2.5 pt-3 pb-1.5 font-mono text-[10.5px] font-semibold tracking-wider text-slate-400 uppercase">{{ group.title }}</p>
              <button
                v-for="item in visibleItems(group)"
                :key="item.to"
                type="button"
                class="mb-0.5 flex w-full items-center gap-2.5 rounded-lg px-2.5 py-2 text-[13px] transition-colors"
                :class="isActive(item.to) ? 'bg-brand-600 font-semibold text-white' : 'text-slate-400 hover:bg-white/5 hover:text-slate-100'"
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
    <!-- 左内边距与侧边栏宽度同源于 --sidebar-w(侧边栏宽度也是它)。
         这里刻意不用「静态类里写展开态的 padding-left 任意值 + 动态类切换收起态
         的同类任意值」的写法:两个同属性、同断点的任意值工具类会同时挂在元素上,
         最终生效值由样式表先后顺序决定而非 class 顺序,而 Tailwind 产物里展开态
         那条恒排在收起态之后,于是收起后左内边距仍是 224px,右侧宽度纹丝不动。
         改用 var() 后只剩唯一一条规则,与 class 顺序彻底解耦。 -->
    <div
      class="flex min-h-screen flex-col transition-[padding] duration-200 ease-out md:pl-[var(--sidebar-w)]"
    >
      <header class="sticky top-0 z-20 flex h-14 items-center justify-between border-b border-line bg-surface/90 px-4 backdrop-blur-md md:px-6">
        <div class="flex min-w-0 items-center gap-3">
          <button
            type="button"
            class="flex h-8 w-8 shrink-0 items-center justify-center rounded-lg text-ink-soft transition-colors hover:bg-surface-strong hover:text-ink"
            @click="onToggleSidebar"
          >
            <PanelLeftClose v-if="!collapsed && !isMobile" :size="17" />
            <PanelLeftOpen v-else-if="!isMobile" :size="17" />
            <Menu v-else :size="17" @click="drawerOpen = true" />
          </button>
          <div class="flex min-w-0 items-center gap-2">
            <span class="font-mono text-xs text-muted">CLOAK</span>
            <span class="text-muted">/</span>
            <h1 class="truncate text-[14.5px] font-semibold text-ink">{{ routeTitle }}</h1>
          </div>
        </div>

        <div class="flex shrink-0 items-center gap-3">
          <span class="badge badge-ok hidden md:inline-flex">
            <span class="dot dot-live"></span>
            系统就绪
          </span>
          <span v-if="auth.config?.serverIp" class="badge badge-neutral hidden lg:inline-flex font-mono">
            节点 IP · {{ auth.config.serverIp }}
          </span>
          <AppTag v-if="auth.tenant?.tier" color="cyan" class="hidden sm:inline-flex">
            等级 · {{ auth.tenant.tier.name }}
          </AppTag>

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

      <!-- 内容宽度跟随侧边栏伸缩:不加固定 max-width,否则宽屏下收缩侧边栏时右侧不会变宽 -->
      <main class="w-full flex-1 px-4 py-5 md:px-6">
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
  ChevronDown,
  Crown,
  FlaskConical,
  Globe,
  LayoutDashboard,
  Link2,
  LogOut,
  Menu,
  Moon,
  PanelLeftClose,
  PanelLeftOpen,
  Settings,
  Sliders,
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

// 侧边栏宽度的唯一真源:aside 宽度与主区域左内边距都读 --sidebar-w
const sidebarStyle = computed<Record<string, string>>(() => ({
  '--sidebar-w': collapsed.value ? '64px' : '224px',
}));
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
    title: '控制台',
    items: [
      { to: '/overview', label: '总览', icon: LayoutDashboard },
      { to: '/links', label: '短链与目标', icon: Link2 },
      { to: '/rules', label: '规则引擎', icon: Sliders },
      { to: '/rules/simulator', label: '规则模拟器', icon: FlaskConical },
    ],
  },
  {
    title: '配置',
    items: [
      { to: '/domains', label: '域名池', icon: Globe },
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

/** 单条菜单项是否命中当前路由（含 hash 精确匹配与子路径前缀） */
function matchesRoute(to: string): boolean {
  if (to.includes('#')) {
    const [path, hash] = to.split('#');
    return route.path === path && route.hash === '#' + hash;
  }
  return route.path === to || (to !== '/' && route.path.startsWith(to + '/'));
}

/**
 * 菜单高亮用「最长路径优先」：/rules/simulator 同时匹配 /rules 与自身，
 * 只认最长的那条，否则侧栏会同时点亮「规则引擎」和「规则模拟器」。
 * 子路由（/rules/new、/rules/:id/edit）仍归到「规则引擎」，与短链模块的父子高亮一致。
 */
const activeItemTo = computed(() => {
  const candidates = navGroups
    .flatMap((group) => visibleItems(group))
    .map((item) => item.to)
    .filter(matchesRoute)
    .sort((a, b) => b.length - a.length);
  return candidates[0] ?? '';
});

function isActive(to: string): boolean {
  return activeItemTo.value === to;
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
