import { fileURLToPath, URL } from 'node:url';

import vue from '@vitejs/plugin-vue';
import { defineConfig, loadEnv } from 'vite';

// Vite 配置:本地开发 dev server 5173,/api 代理到 Go 后端(本机 8081,非 8080)。
// 支持两种访问入口:
//   - http://localhost:5173(localhost 直连)
//   - http://<域名>:5173(如 app.cloak.test,经 SwitchHosts/hosts 指向 127.0.0.1;
//     需 server.allowedHosts 放行,见下方 VITE_PLATFORM_DOMAIN)
export default defineConfig(({ mode }) => {
  const env = loadEnv(mode, process.cwd(), '');
  const proxyTarget = env.VITE_PROXY_TARGET || 'http://localhost:8081';
  const platformDomain = env.VITE_PLATFORM_DOMAIN || 'cloak.test';

  return {
    plugins: [vue()],
    resolve: {
      alias: {
        '@': fileURLToPath(new URL('./src', import.meta.url)),
      },
    },
    server: {
      host: true,
      port: 5173,
      // Vite 默认拒绝非 localhost 的 Host(防 DNS rebinding,返回 403);
      // 放行平台域名及其子域,使 SwitchHosts 配置的 app.cloak.test 等可访问 dev server
      allowedHosts: [`.${platformDomain}`],
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
  };
});
