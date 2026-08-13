/// <reference types="vite/client" />

declare module '*.vue' {
  import type { DefineComponent } from 'vue';
  const component: DefineComponent<Record<string, never>, Record<string, never>, unknown>;
  export default component;
}

interface ImportMetaEnv {
  readonly VITE_API_PREFIX?: string;
  readonly VITE_PROXY_TARGET?: string;
  readonly VITE_PLATFORM_DOMAIN?: string;
}

interface ImportMeta {
  readonly env: ImportMetaEnv;
}
