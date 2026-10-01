<template>
  <div class="auth-shell flex min-h-screen bg-surface-muted">
    <!-- 品牌侧(≥992px 显示) -->
    <aside class="auth-aside relative w-[46%] max-w-[560px] flex-col overflow-hidden px-12 py-10">
      <div class="relative z-10 flex items-center gap-3">
        <BrandMark :size="34" />
        <span class="text-xl font-bold tracking-[3px] text-white">CLOAK</span>
      </div>

      <div class="relative z-10 my-auto py-12">
        <h1 class="mb-3 text-3xl font-bold leading-[1.3] text-white">自托管短链服务</h1>
        <p class="mb-8 max-w-[400px] text-sm leading-[1.8] text-white/75">
          管理你的域名、短链与访问统计。域名激活后自动签发并续期 HTTPS 证书。
        </p>
        <ul class="flex flex-col gap-4">
          <li class="flex items-center gap-3 text-sm text-white/85">
            <ShieldCheck :size="17" class="shrink-0 text-accent-400" />
            <span>证书自动签发 · 到期自动续期</span>
          </li>
          <li class="flex items-center gap-3 text-sm text-white/85">
            <Boxes :size="17" class="shrink-0 text-accent-400" />
            <span>同一短码可在不同域名下指向不同目标</span>
          </li>
          <li class="flex items-center gap-3 text-sm text-white/85">
            <LineChart :size="17" class="shrink-0 text-accent-400" />
            <span>访问记录与统计,支持程序化 API</span>
          </li>
        </ul>
      </div>

      <div class="relative z-10 text-xs tracking-wider text-white/50">CLOAK · 自托管短链服务</div>
    </aside>

    <!-- 表单侧 -->
    <main class="relative flex flex-1 items-center justify-center bg-surface px-6 py-10">
      <!-- 登录前也允许切换主题,跟随系统的用户不必先登录 -->
      <div class="absolute top-4 right-4">
        <ThemeSwitcher />
      </div>

      <div class="w-full max-w-[420px]">
        <div class="auth-mobile-brand mb-7 flex items-center justify-center gap-2.5 text-lg font-bold tracking-[2px] text-ink">
          <BrandMark :size="28" />
          <span>CLOAK</span>
        </div>

        <div v-if="showHeading && $slots.title" class="auth-heading mb-7 text-center">
          <h2 class="mb-2 text-2xl font-bold text-ink"><slot name="title" /></h2>
          <p v-if="$slots.subtitle" class="text-sm leading-[1.7] text-ink-faint"><slot name="subtitle" /></p>
        </div>

        <div class="auth-body">
          <slot />
        </div>
      </div>
    </main>
  </div>
</template>

<script setup lang="ts">
import { Boxes, LineChart, ShieldCheck } from '@lucide/vue';

import BrandMark from './BrandMark.vue';
import ThemeSwitcher from './layout/ThemeSwitcher.vue';

withDefaults(
  defineProps<{
    /** 是否展示标题区(结果页等场景可关闭) */
    showHeading?: boolean;
  }>(),
  { showHeading: true },
);
</script>

<style scoped>
.auth-aside {
  display: none;
  background:
    radial-gradient(620px 420px at 88% -8%, color-mix(in srgb, var(--color-brand-600) 45%, transparent), transparent 62%),
    radial-gradient(520px 400px at -12% 112%, color-mix(in srgb, var(--color-brand-400) 20%, transparent), transparent 60%),
    var(--sidebar-bg-deep);
}

.auth-aside::before {
  content: '';
  position: absolute;
  inset: 0;
  background-image:
    linear-gradient(color-mix(in srgb, var(--line) 6%, transparent) 1px, transparent 1px),
    linear-gradient(90deg, color-mix(in srgb, var(--line) 6%, transparent) 1px, transparent 1px);
  background-size: 44px 44px;
  pointer-events: none;
}

@media (min-width: 992px) {
  .auth-aside {
    display: flex;
  }

  .auth-mobile-brand {
    display: none;
  }

  .auth-heading {
    text-align: left;
  }
}
</style>
