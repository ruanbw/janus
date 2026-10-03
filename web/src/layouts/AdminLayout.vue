<template>
  <div class="bg-surface-muted" :style="sidebarStyle">
    <!-- ================= 桌面侧边栏 =================
         显隐与宽度全部由 CSS 承担:显隐用 md:,宽度用 --sidebar-w,
         JS 不参与断点判断,因此不存在「CSS 已切换、JS 还没跟上」的状态。 -->
    <aside
      aria-label="侧边栏"
      class="fixed inset-y-0 left-0 z-30 hidden w-[var(--sidebar-w)] flex-col overflow-hidden border-r border-line bg-[var(--sidebar-bg)] transition-[width] duration-200 ease-out motion-reduce:transition-none md:flex"
    >
      <SidebarBrand :collapsed="collapsed" />
      <NavList :collapsed="collapsed" />

      <div
        class="border-t border-line px-3.5 py-3"
        :class="collapsed ? 'flex justify-center' : ''"
        :title="collapsed ? auth.tenant?.email : undefined"
      >
        <div
          v-if="!collapsed"
          class="truncate font-mono text-2xs tracking-wider text-sidebar-ink uppercase"
        >
          租户 · {{ tenantName }}
        </div>
        <div class="flex items-center gap-2.5" :class="collapsed ? '' : 'mt-2'">
          <span
            class="flex h-7 w-7 shrink-0 items-center justify-center rounded-full bg-surface-strong font-mono text-2xs font-semibold text-sidebar-ink-active"
          >
            {{ avatarInitial }}
          </span>
          <div v-if="!collapsed" class="min-w-0 text-xs leading-tight text-sidebar-ink-active">
            <div>{{ auth.isSuperAdmin ? '超级管理员' : '租户管理员' }}</div>
            <div class="font-mono text-2xs text-sidebar-ink">{{ auth.tenant?.tier?.name || '标准版' }}</div>
          </div>
        </div>
      </div>
    </aside>

    <!-- ================= 移动端抽屉 =================
         modal=false + 不锁焦点:抽屉的显隐由 md: 决定,而 JS 并不知道当前断点。
         若开启焦点陷阱,用户在手机上打开抽屉后转屏到 ≥768px,焦点会被锁进
         display:none 的面板里。关闭键改由 reka 的 Esc / 点外部处理,无需自写。 -->
    <DialogRoot :open="drawerOpen" :modal="false" @update:open="onDrawerOpenChange">
      <DialogPortal>
        <DialogOverlay class="drawer-overlay fixed inset-0 z-40 bg-black/45 md:hidden" />
        <DialogContent
          class="drawer-panel fixed inset-y-0 left-0 z-50 flex w-[var(--sidebar-w)] max-w-[85vw] flex-col bg-[var(--sidebar-bg)] shadow-2xl outline-none md:hidden"
          @open-auto-focus="onDrawerOpenAutoFocus"
        >
          <div class="flex h-14 shrink-0 items-center justify-between gap-2">
            <SidebarBrand />
            <DialogClose as-child>
              <button
                type="button"
                aria-label="关闭菜单"
                class="flex h-8 w-8 shrink-0 items-center justify-center rounded-lg text-sidebar-ink transition-colors hover:bg-accent hover:text-sidebar-ink-active"
              >
                <X :size="18" />
              </button>
            </DialogClose>
          </div>
          <NavList :on-item-click="() => (drawerOpen = false)" />
          <DialogTitle class="sr-only">主导航</DialogTitle>
        </DialogContent>
      </DialogPortal>
    </DialogRoot>

    <!-- ================= 主区域 ================= -->
    <!-- 左内边距与侧边栏宽度同源于 --sidebar-w(侧边栏宽度也是它)。
         这里刻意不用「静态类里写展开态的 padding-left 任意值 + 动态类切换收起态
         的同类任意值」的写法:两个同属性、同断点的任意值工具类会同时挂在元素上,
         最终生效值由样式表先后顺序决定而非 class 顺序,而 Tailwind 产物里展开态
         那条恒排在收起态之后,于是收起后左内边距仍是 224px,右侧宽度纹丝不动。
         改用 var() 后只剩唯一一条规则,与 class 顺序彻底解耦。
         两条 transition 的时长与缓动也刻意一致,否则侧边栏边缘与内容区边缘
         会在动画中途分叉。 -->
    <div
      ref="mainRegion"
      :aria-hidden="backgroundHidden ? 'true' : undefined"
      class="flex min-h-screen flex-col transition-[padding] duration-200 ease-out motion-reduce:transition-none md:pl-[var(--sidebar-w)]"
    >
      <header
        class="sticky top-0 z-20 flex h-14 items-center justify-between border-b border-line bg-surface/90 px-4 backdrop-blur-md md:px-6"
      >
        <div class="flex min-w-0 items-center gap-3">
          <!-- 移动端:开抽屉。桌面端由 md:hidden 摘掉,无需 JS 区分。 -->
          <button
            type="button"
            aria-label="打开菜单"
            class="flex h-8 w-8 shrink-0 items-center justify-center rounded-lg text-ink-soft transition-colors hover:bg-surface-strong hover:text-ink md:hidden"
            @click="drawerOpen = true"
          >
            <Menu :size="17" />
          </button>
          <!-- 桌面端:折叠/展开侧边栏。移动端由 hidden 摘掉。 -->
          <button
            type="button"
            :aria-label="collapsed ? '展开侧边栏' : '收起侧边栏'"
            :aria-expanded="!collapsed"
            class="hidden h-8 w-8 shrink-0 items-center justify-center rounded-lg text-ink-soft transition-colors hover:bg-surface-strong hover:text-ink md:flex"
            @click="collapsed = !collapsed"
          >
            <PanelLeftClose v-if="!collapsed" :size="17" />
            <PanelLeftOpen v-else :size="17" />
          </button>
          <div class="flex min-w-0 items-center gap-2">
            <span class="font-mono text-xs text-ink-faint">Janus</span>
            <span class="text-ink-faint">/</span>
            <h1 class="truncate text-sm font-semibold text-ink">{{ routeTitle }}</h1>
          </div>
        </div>

        <div class="flex shrink-0 items-center gap-3">
          <ThemeSwitcher />

          <DropdownMenuRoot>
            <DropdownMenuTrigger as-child>
              <button
                type="button"
                class="flex items-center gap-2 rounded-full py-1 pr-1.5 pl-1 transition-colors hover:bg-surface-strong"
              >
                <span
                  class="flex h-7 w-7 items-center justify-center rounded-full bg-primary text-xs font-semibold text-primary-foreground"
                >
                  {{ avatarInitial }}
                </span>
                <span class="hidden max-w-44 truncate text-sm text-ink sm:block">
                  {{ auth.tenant?.email }}
                </span>
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
                  class="flex cursor-pointer items-center gap-2 rounded-lg px-2.5 py-2 text-sm text-ink outline-none data-[highlighted]:bg-accent data-[highlighted]:text-accent-foreground"
                  @select="go('/account')"
                >
                  <User :size="14" />
                  账号设置
                </DropdownMenuItem>
                <DropdownMenuSeparator class="my-1 h-px bg-line" />
                <DropdownMenuItem
                  class="flex cursor-pointer items-center gap-2 rounded-lg px-2.5 py-2 text-sm text-destructive outline-none data-[highlighted]:bg-destructive/10"
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

      <!-- 内容宽度跟随侧边栏伸缩:不加固定 max-width,否则宽屏下收缩侧边栏时右侧不会变宽。
           页面底部留白由这里单点给出,视图层不再各自挂 pb-* ——
           过去只有总览与短链页带 pb-10,其余八页没有,翻页时页脚与内容的间距会跳。 -->
      <main class="w-full flex-1 px-4 pt-5 pb-10 md:px-6">
        <!-- 错误边界只包住页面本身:页面抛错时顶栏与侧边栏仍可用,能直接切走 -->
        <ErrorBoundary v-slot="{ attempt }" :reset-key="route.fullPath">
          <router-view v-slot="{ Component }">
            <transition name="page" mode="out-in" @leave="onPageLeave">
              <component :is="Component" :key="route.fullPath + '_' + attempt" />
            </transition>
          </router-view>
        </ErrorBoundary>
      </main>

      <footer class="pb-5 text-center text-xs text-ink-faint">Janus · 自托管短链服务</footer>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref, watch } from 'vue';
import { useRoute, useRouter } from 'vue-router';
import { ChevronDown, LogOut, Menu, PanelLeftClose, PanelLeftOpen, User, X } from '@lucide/vue';
import {
  DialogClose,
  DialogContent,
  DialogOverlay,
  DialogPortal,
  DialogRoot,
  DialogTitle,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuPortal,
  DropdownMenuRoot,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@/components/ui';

import ErrorBoundary from '@/components/layout/ErrorBoundary.vue';
import NavList from '@/components/layout/NavList.vue';
import SidebarBrand from '@/components/layout/SidebarBrand.vue';
import ThemeSwitcher from '@/components/layout/ThemeSwitcher.vue';
import { useAuthStore } from '@/stores/auth';

/** 收起态与展开态的侧边栏宽度,是 aside 宽度与主区域左内边距的唯一真源 */
const SIDEBAR_W_EXPANDED = '224px';
const SIDEBAR_W_COLLAPSED = '64px';
/** 与 web/src/styles/main.css 的 --sidebar-w 兜底值保持一致(首屏渲染前生效) */
const COLLAPSE_STORAGE_KEY = 'janus:sidebar-collapsed';

const auth = useAuthStore();
const route = useRoute();

// 页面过渡在后台标签页里会卡死,这里接管 leave 收尾。
//
// 根因:浏览器冻结隐藏标签页的动画帧。Vue 的 Transition 在 onLeave 里先
// nextFrame()(双层 requestAnimationFrame),再在**该回调内部**调 whenTransitionEnds
// 收尾 —— 隐藏标签页里 rAF 一个都不跑,旧页面就永久停在 page-leave-active。
// mode="out-in" 要等它走完才挂载新页面,于是「标题已切、内容还是上一页」的
// 白屏,刷新才恢复。自动化与截图工具同样会踩(它们一般不激活标签页)。
//
// 两条走不通的路(都实测过):
//   - :duration 压到 0:whenTransitionEnds 认显式 duration,但它整个位于
//     nextFrame 回调里,rAF 不跑就到不了那里,依旧卡在 page-leave-active;
//   - :css="false":旧元素被移除后新元素不挂载,<main> 直接变空(kids:0)。
//
// 所以接管 @leave 自己收尾。三个约束:
//   1. 声明 @leave 即等于 hasExplicitCallback,done 必须自己调,否则过渡永不结束;
//   2. 必须走 setTimeout(异步),不能同步调 —— mode="out-in" 在 leave 开始时把
//      state.isLeaving 置真并渲染 emptyPlaceholder,要等 afterLeave 把它置回假才
//      instance.update() 挂载新页面,同步 done 会重入同一次更新,新组件被判成
//      「仍在 leaving」而丢弃(实测 kids:0);
//   3. 可见时也要调 done,只是等满 CSS 动画,行为与改动前一致。
const tabHidden = ref(false);
const syncVisibility = (): void => {
  tabHidden.value = document.hidden;
};

// 与 main.css 的 .page-leave-active(0.18s)对齐。
const PAGE_TRANSITION_MS = 180;

function onPageLeave(_el: Element, done: () => void): void {
  window.setTimeout(done, tabHidden.value ? 0 : PAGE_TRANSITION_MS);
}

onMounted(() => {
  syncVisibility();
  document.addEventListener('visibilitychange', syncVisibility);
});

onUnmounted(() => {
  document.removeEventListener('visibilitychange', syncVisibility);
});

const router = useRouter();

const collapsed = ref(readCollapsed());
const drawerOpen = ref(false);

/**
 * 主区域是否已 aria-hidden。
 *
 * 刻意**不**直接绑 drawerOpen:点「打开菜单」时按钮先成为 document.activeElement,
 * 同一轮 patch 若就把 aria-hidden="true" 写到按钮的祖先上,Chrome 会拦截这次写入
 * 并报「Blocked aria-hidden on an element because its descendant retained focus」。
 * 抽屉虽经 DialogPortal 传送到 body(在 aria-hidden 之外),但 reka-ui 的 FocusScope
 * 要等它 watchEffect 里 `await nextTick()` 之后才调 dispatchMountAutoFocus 搬焦点,
 * 补不上这个窗口。
 *
 * 时机改由抽屉自己的 openAutoFocus 事件给定:那一刻焦点正要搬进面板,
 * 而 Vue 对本 ref 的 patch 排在后面的微任务里,真正写上 aria-hidden 时
 * 焦点已在该子树之外。见 onDrawerOpenAutoFocus 里的兜底 blur。
 */
const backgroundHidden = ref(false);
/** 主区域容器,仅用于判断焦点是否仍落在其中 */
const mainRegion = ref<HTMLElement | null>(null);

function onDrawerOpenChange(open: boolean): void {
  drawerOpen.value = open;
  // 关闭时同步撤销:reka 稍后才把焦点还给汉堡按钮,那时 aria-hidden 必须已经摘掉
  if (!open) backgroundHidden.value = false;
}

function onDrawerOpenAutoFocus(): void {
  // 兜底:若调用方 preventDefault 掉 reka 的自动聚焦,焦点会滞留在主区域内。
  // 先把它交出去(随后仍由 FocusScope 搬进面板),让「打 aria-hidden 时子树内无焦点」
  // 不依赖 reka-ui 的内部时序。
  const active = document.activeElement;
  if (active instanceof HTMLElement && mainRegion.value?.contains(active)) {
    active.blur();
  }
  backgroundHidden.value = true;
}

const sidebarStyle = computed<Record<string, string>>(() => ({
  '--sidebar-w': collapsed.value ? SIDEBAR_W_COLLAPSED : SIDEBAR_W_EXPANDED,
}));

// 折叠状态属于用户偏好,和主题一样应当跨刷新存活。
// 移动端不受影响:那里的侧边栏由 md: 摘掉,collapsed 只决定 ≥768px 时的观感。
function readCollapsed(): boolean {
  try {
    return localStorage.getItem(COLLAPSE_STORAGE_KEY) === '1';
  } catch {
    return false; // 隐私模式 / 存储被禁用时降级为默认展开
  }
}

watch(collapsed, (value) => {
  try {
    localStorage.setItem(COLLAPSE_STORAGE_KEY, value ? '1' : '0');
  } catch {
    /* 写入失败不影响本次会话,忽略 */
  }
});

watch(
  () => route.fullPath,
  () => {
    onDrawerOpenChange(false);
  },
);

const routeTitle = computed(() => (route.meta.title as string | undefined) ?? 'Janus 后台');

const tenantName = computed(
  () =>
    auth.tenant?.slug?.toUpperCase() ||
    auth.tenant?.email?.split('@')[0]?.toUpperCase() ||
    'TENANT',
);

const avatarInitial = computed(() => {
  const email = auth.tenant?.email ?? '?';
  return email.charAt(0).toUpperCase();
});

function go(to: string): void {
  router.push(to);
}

async function onLogout(): Promise<void> {
  await auth.logout();
  router.push('/login');
}
</script>

<style>
/* 移动端抽屉面板传送至 <body>,需要全局规则覆盖 shadcn 弹窗原语的默认居中与内边距 */
.drawer-panel.drawer-panel {
  top: 0 !important;
  left: 0 !important;
  transform: none !important;
  translate: none !important;
  padding: 0 !important;
  gap: 0 !important;
  border-radius: 0 !important;
  border-top: none !important;
  border-bottom: none !important;
  border-left: none !important;
}
</style>

<style scoped>
/* reka 的 Presence 依据 data-state 决定卸载时机,退出动画走 CSS 即可 */
.drawer-overlay[data-state='open'] {
  animation: drawer-fade-in 0.2s ease;
}

.drawer-overlay[data-state='closed'] {
  animation: drawer-fade-out 0.2s ease;
}

.drawer-panel[data-state='open'] {
  animation: drawer-slide-in 0.2s ease;
}

.drawer-panel[data-state='closed'] {
  animation: drawer-slide-out 0.2s ease;
}

@keyframes drawer-fade-in {
  from {
    opacity: 0;
  }
}

@keyframes drawer-fade-out {
  to {
    opacity: 0;
  }
}

@keyframes drawer-slide-in {
  from {
    transform: translateX(-100%);
  }
}

@keyframes drawer-slide-out {
  to {
    transform: translateX(-100%);
  }
}

@media (prefers-reduced-motion: reduce) {
  .drawer-overlay[data-state],
  .drawer-panel[data-state] {
    animation: none;
  }
}
</style>
