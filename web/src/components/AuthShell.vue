<template>
  <div class="auth-shell">
    <!-- 品牌侧(≥992px 显示) -->
    <aside class="auth-aside">
      <div class="aside-brand">
        <BrandMark :size="34" />
        <span class="aside-wordmark">CLOAK</span>
      </div>

      <div class="aside-hero">
        <h1 class="aside-title">自托管短链服务</h1>
        <p class="aside-desc">
          管理你的域名、短链与访问统计。域名激活后自动签发并续期 HTTPS 证书。
        </p>
        <ul class="aside-features">
          <li>
            <SafetyCertificateOutlined class="feat-icon" />
            <span>证书自动签发 · 到期自动续期</span>
          </li>
          <li>
            <DeploymentUnitOutlined class="feat-icon" />
            <span>同一短码可在不同域名下指向不同目标</span>
          </li>
          <li>
            <LineChartOutlined class="feat-icon" />
            <span>访问记录与统计,支持程序化 API</span>
          </li>
        </ul>
      </div>

      <div class="aside-footer">CLOAK · 自托管短链服务</div>
    </aside>

    <!-- 表单侧 -->
    <main class="auth-main">
      <div class="auth-card-wrap">
        <div class="mobile-brand">
          <BrandMark :size="28" />
          <span>CLOAK</span>
        </div>

        <div v-if="showHeading && $slots.title" class="auth-heading">
          <h2 class="auth-title"><slot name="title" /></h2>
          <p v-if="$slots.subtitle" class="auth-subtitle"><slot name="subtitle" /></p>
        </div>

        <div class="auth-body">
          <slot />
        </div>
      </div>
    </main>
  </div>
</template>

<script setup lang="ts">
import {
  DeploymentUnitOutlined,
  LineChartOutlined,
  SafetyCertificateOutlined,
} from '@ant-design/icons-vue';

import BrandMark from './BrandMark.vue';

withDefaults(
  defineProps<{
    /** 是否展示标题区(结果页等场景可关闭) */
    showHeading?: boolean;
  }>(),
  { showHeading: true },
);
</script>

<style scoped>
.auth-shell {
  min-height: 100vh;
  display: flex;
  background: #f3f5f7;
}

/* ---------- 品牌侧 ---------- */
.auth-aside {
  display: none;
  flex: 1 1 46%;
  max-width: 560px;
  padding: 40px 48px;
  position: relative;
  flex-direction: column;
  overflow: hidden;
  background:
    radial-gradient(620px 420px at 88% -8%, rgba(14, 116, 144, 0.45), transparent 62%),
    radial-gradient(520px 400px at -12% 112%, rgba(34, 211, 238, 0.2), transparent 60%),
    #0b1220;
}

.auth-aside::before {
  content: '';
  position: absolute;
  inset: 0;
  background-image:
    linear-gradient(rgba(148, 163, 184, 0.06) 1px, transparent 1px),
    linear-gradient(90deg, rgba(148, 163, 184, 0.06) 1px, transparent 1px);
  background-size: 44px 44px;
  pointer-events: none;
}

.aside-brand {
  display: flex;
  align-items: center;
  gap: 12px;
  position: relative;
}

.aside-wordmark {
  color: #f8fafc;
  font-size: 20px;
  font-weight: 700;
  letter-spacing: 3px;
}

.aside-hero {
  position: relative;
  margin: auto 0;
  padding: 48px 0;
}

.aside-title {
  color: #f8fafc;
  font-size: 30px;
  font-weight: 700;
  line-height: 1.3;
  margin: 0 0 12px;
}

.aside-desc {
  color: rgba(203, 213, 225, 0.72);
  font-size: 14px;
  line-height: 1.8;
  margin: 0 0 32px;
  max-width: 400px;
}

.aside-features {
  list-style: none;
  margin: 0;
  padding: 0;
  display: flex;
  flex-direction: column;
  gap: 16px;
}

.aside-features li {
  display: flex;
  align-items: center;
  gap: 12px;
  color: rgba(226, 232, 240, 0.85);
  font-size: 13.5px;
}

.feat-icon {
  color: #22d3ee;
  font-size: 17px;
  flex-shrink: 0;
}

.aside-footer {
  position: relative;
  color: rgba(148, 163, 184, 0.55);
  font-size: 12px;
  letter-spacing: 1px;
}

/* ---------- 表单侧 ---------- */
.auth-main {
  flex: 1 1 54%;
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 40px 24px;
}

.auth-card-wrap {
  width: 100%;
  max-width: 420px;
}

.mobile-brand {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 10px;
  margin-bottom: 28px;
  font-size: 18px;
  font-weight: 700;
  letter-spacing: 2px;
  color: #0f172a;
}

.auth-heading {
  margin-bottom: 28px;
  text-align: center;
}

.auth-title {
  margin: 0 0 8px;
  font-size: 24px;
  font-weight: 700;
  color: #0f172a;
}

.auth-subtitle {
  margin: 0;
  font-size: 13.5px;
  line-height: 1.7;
  color: #8b98a5;
}

@media (min-width: 992px) {
  .auth-aside {
    display: flex;
  }

  .mobile-brand {
    display: none;
  }

  .auth-heading {
    text-align: left;
  }
}
</style>
