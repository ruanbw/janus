/// <reference types="vite/client" />

declare module '*.vue' {
  import type { DefineComponent } from 'vue';
  const component: DefineComponent<Record<string, never>, Record<string, never>, unknown>;
  export default component;
}

/**
 * world-atlas 的国界 TopoJSON 用环境声明挡在类型推断之外。
 *
 * `countries-110m.json` 有一百多 KB，`resolveJsonModule` 会把它整个当字面量类型展开，
 * `vue-tsc` 因此多花两秒多（实测 6.8s → 9.3s）。这里只声明「有个 default 导出」，
 * 真实结构由 `views/overview/worldMap.ts` 按 TopoJSON 规范自行收窄。
 */
declare module 'world-atlas/countries-110m.json' {
  const topology: unknown;
  export default topology;
}

interface ImportMetaEnv {
  readonly VITE_API_PREFIX?: string;
  readonly VITE_PROXY_TARGET?: string;
  readonly VITE_PLATFORM_DOMAIN?: string;
}

interface ImportMeta {
  readonly env: ImportMetaEnv;
}
