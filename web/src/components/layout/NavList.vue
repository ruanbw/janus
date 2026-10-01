<template>
  <nav aria-label="主导航" class="flex-1 overflow-y-auto px-2.5 py-2">
    <template v-for="group in groups" :key="group.title">
      <p
        v-if="showLabels"
        class="px-2.5 pt-3 pb-1.5 font-mono text-2xs font-semibold tracking-wider text-sidebar-ink/70 uppercase"
      >
        {{ group.title }}
      </p>
      <!-- 用 RouterLink 而非 button + router.push:保留 cmd/中键新标签页打开、
           状态栏 URL 预览,并借 aria-current 向读屏软件表达「当前页」 -->
      <RouterLink
        v-for="item in group.items"
        :key="item.to"
        :to="item.to"
        :title="showLabels ? undefined : item.label"
        :aria-current="isActive(item.to) ? 'page' : undefined"
        class="mb-0.5 flex items-center gap-2.5 rounded-lg px-2.5 py-2 text-xs transition-colors"
        :class="
          isActive(item.to)
            ? 'bg-primary font-semibold text-primary-foreground shadow-sm'
            : 'text-sidebar-ink hover:bg-accent hover:text-sidebar-ink-active'
        "
      >
        <component :is="item.icon" :size="16" class="shrink-0" :class="showLabels ? '' : 'mx-auto'" />
        <span v-if="showLabels">{{ item.label }}</span>
      </RouterLink>
    </template>
  </nav>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import { useRoute } from 'vue-router';
import {
  Crown,
  FlaskConical,
  Globe,
  LayoutDashboard,
  Link2,
  Settings,
  Sliders,
} from '@lucide/vue';

import { useAuthStore } from '@/stores/auth';

interface NavItem {
  to: string;
  label: string;
  icon: unknown;
  /** 仅超级管理员可见 */
  superAdmin?: boolean;
}

interface NavGroup {
  title: string;
  items: NavItem[];
}

const props = withDefaults(defineProps<{ collapsed?: boolean }>(), { collapsed: false });

const auth = useAuthStore();
const route = useRoute();

/** 收起态只留图标;移动抽屉始终展开 */
const showLabels = computed(() => !props.collapsed);

// 菜单配置是本组件的私有实现:AdminLayout 只需知道「渲染导航」,
// 不必知道有哪些分组、哪条要权限、哪条是高亮。
const NAV_GROUPS: NavGroup[] = [
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
    items: [{ to: '/domains', label: '域名池', icon: Globe }],
  },
  {
    title: '系统',
    items: [
      { to: '/account', label: '账号设置', icon: Settings },
      { to: '/admin/tenants', label: '平台管理', icon: Crown, superAdmin: true },
    ],
  },
];

/** 按权限过滤,并丢掉过滤后变空的分组(避免只剩一个空标题) */
const groups = computed<NavGroup[]>(() =>
  NAV_GROUPS.map((group) => ({
    title: group.title,
    items: group.items.filter((item) => item.superAdmin !== true || auth.isSuperAdmin),
  })).filter((group) => group.items.length > 0),
);

/** 单条菜单项是否命中当前路由(含 hash 精确匹配与子路径前缀) */
function matchesRoute(to: string): boolean {
  if (to.includes('#')) {
    const [path, hash] = to.split('#');
    return route.path === path && route.hash === '#' + hash;
  }
  return route.path === to || (to !== '/' && route.path.startsWith(to + '/'));
}

/**
 * 菜单高亮用「最长路径优先」:/rules/simulator 同时匹配 /rules 与自身,
 * 只认最长的那条,否则侧栏会同时点亮「规则引擎」和「规则模拟器」。
 * 子路由(/rules/new、/rules/:id/edit)仍归到「规则引擎」,与短链模块的父子高亮一致。
 */
const activeItemTo = computed(() => {
  const candidates = groups.value
    .flatMap((group) => group.items)
    .map((item) => item.to)
    .filter(matchesRoute)
    .sort((a, b) => b.length - a.length);
  return candidates[0] ?? '';
});

function isActive(to: string): boolean {
  return activeItemTo.value === to;
}
</script>
