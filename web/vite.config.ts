/// <reference types="vitest/config" />
import { fileURLToPath, URL } from 'node:url';

import tailwindcss from '@tailwindcss/vite';
import vue from '@vitejs/plugin-vue';
import { defineConfig, loadEnv } from 'vite';

/**
 * 推导 dev server 的 Host 白名单。
 *
 * vite 的判定逻辑(见 vite/dist/node/chunks/dep-*.js)是「`.${域名}` 与 hostname 相等
 * 或hostname 以它结尾」才放行,所以每一项都以子域后缀形式给出;
 * 绝不下发 `true` / `'*'` —— 那等于关掉 DNS rebinding 防护。
 *
 * 额外白名单通过 VITE_EXTRA_ALLOWED_HOSTS 提供(逗号分隔),让团队按自己的
 * 本地域名访问 dev server 时不必改仓库文件,同时不扩大默认放行面。
 */
export function buildAllowedHosts(platformDomain: string, extraHosts: string): string[] {
  const platform = platformDomain.trim();
  const extras = extraHosts
    .split(',')
    .map((host) => host.trim().replace(/^\./, ''))
    .filter((host) => host.length > 0 && host !== '*');
  return [`.${platform}`, ...extras.filter((host) => host !== platform).map((host) => `.${host}`)];
}

// Vite 配置:本地开发 dev server 5173,/api 代理到 Go 后端(本机 8080)。
// 支持两种访问入口:
//   - http://localhost:5173(localhost 直连)
//   - http://<域名>:5173(如 app.janus.test,经 SwitchHosts/hosts 指向 127.0.0.1;
//     需 server.allowedHosts 放行,见下方 VITE_PLATFORM_DOMAIN)
export default defineConfig(({ mode }) => {
  const env = loadEnv(mode, process.cwd(), '');
  const proxyTarget = env.VITE_PROXY_TARGET || 'http://localhost:8080';
  const platformDomain = env.VITE_PLATFORM_DOMAIN || 'janus.test';
  const extraAllowedHosts = env.VITE_EXTRA_ALLOWED_HOSTS || '';

  return {
    plugins: [vue(), tailwindcss()],
    resolve: {
      alias: {
        '@': fileURLToPath(new URL('./src', import.meta.url)),
      },
    },
    server: {
      host: true,
      port: 5173,
      // Vite 默认拒绝非 localhost 的 Host(防 DNS rebinding,返回 403);
      // 放行平台域名及其子域,使 SwitchHosts 配置的 app.janus.test 等可访问 dev server;
      // 其余本地域名走 VITE_EXTRA_ALLOWED_HOSTS(逗号分隔)
      allowedHosts: buildAllowedHosts(platformDomain, extraAllowedHosts),
      proxy: {
        // 会话 cookie 与 CSRF cookie 均来自后端,代理必须透传 Set-Cookie
        '/api': {
          target: proxyTarget,
          changeOrigin: true,
        },
      },
    },
    build: {
      outDir: 'dist',
      sourcemap: false,
    },
    test: {
      // 复用上方 vue() 插件,vitest 才能编译 .vue SFC
      globals: true,
      environment: 'happy-dom',
      setupFiles: ['./vitest.setup.ts'],
      include: ['tests/**/*.{test,spec}.ts'],
      // 测试位于 tests/,别名与 dev 一致指向 src/
      alias: {
        '@': fileURLToPath(new URL('./src', import.meta.url)),
      },
    },
  };
});
