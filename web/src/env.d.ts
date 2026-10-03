/// <reference types="vite/client" />

/**
 * 这里**不能**有 `declare module '*.vue'` 通配声明。
 *
 * 那条通配会把每个 SFC 的 props 统一声明成 `DefineComponent<Record<string, never>, ...>`，
 * 于是「漏传必填 prop」「prop 类型不匹配」两类错误在编译期一个都查不出来，
 * 只能等运行时以 `[Vue warn]: Missing required prop` 暴露。
 *
 * 删掉它之后 `vue-tsc` 直接对 SFC 做真实的 props 推导（tsconfig 的 include 已经覆盖
 * `src` 下的 `.vue`，实测无需 `vueCompilerOptions` 之类的额外配置）。
 *
 * 但光删通配还不够：`components/app/index.ts` 通过 `app.component(name, component)`
 * **全局注册**了 40 个组件（`app.use(UI)`，见 `main.ts`），而模板里绝大多数地方
 * 不写 import。`vue-tsc` 解析模板标签时只认「当前文件 import 过的符号」+
 * `GlobalComponents` 接口，全局注册的组件两边都不沾 → 会被当成未知组件，
 * props 校验等于没开。所以下面把注册表显式声明成 `GlobalComponents`。
 */
export {};

declare module 'vue' {
  export interface GlobalComponents {
    AppAlert: (typeof import('@/components/app/AppAlert.vue'))['default'];
    AppButton: (typeof import('@/components/app/AppButton.vue'))['default'];
    AppCard: (typeof import('@/components/app/AppCard.vue'))['default'];
    CardHeader: (typeof import('@/components/app/CardHeader.vue'))['default'];
    CardTitle: (typeof import('@/components/app/CardTitle.vue'))['default'];
    CardDescription: (typeof import('@/components/app/CardDescription.vue'))['default'];
    CardContent: (typeof import('@/components/app/CardContent.vue'))['default'];
    CardFooter: (typeof import('@/components/app/CardFooter.vue'))['default'];
    AppCheckbox: (typeof import('@/components/app/AppCheckbox.vue'))['default'];
    AppDescriptions: (typeof import('@/components/app/AppDescriptions.vue'))['default'];
    AppDescriptionsItem: (typeof import('@/components/app/AppDescriptionsItem.vue'))['default'];
    AppDialog: (typeof import('@/components/app/AppDialog.vue'))['default'];
    AppDivider: (typeof import('@/components/app/AppDivider.vue'))['default'];
    AppEmpty: (typeof import('@/components/app/AppEmpty.vue'))['default'];
    AppForm: (typeof import('@/components/app/AppForm.vue'))['default'];
    AppFormItem: (typeof import('@/components/app/AppFormItem.vue'))['default'];
    AppInput: (typeof import('@/components/app/AppInput.vue'))['default'];
    AppInputNumber: (typeof import('@/components/app/AppInputNumber.vue'))['default'];
    AppModal: (typeof import('@/components/app/AppModal.vue'))['default'];
    AppPopconfirm: (typeof import('@/components/app/AppPopconfirm.vue'))['default'];
    AppProgress: (typeof import('@/components/app/AppProgress.vue'))['default'];
    AppRadio: (typeof import('@/components/app/AppRadio.vue'))['default'];
    AppRadioGroup: (typeof import('@/components/app/AppRadioGroup.vue'))['default'];
    AppRadioCard: (typeof import('@/components/app/AppRadioCard.vue'))['default'];
    AppResult: (typeof import('@/components/app/AppResult.vue'))['default'];
    AppSelect: (typeof import('@/components/app/AppSelect.vue'))['default'];
    AppSkeleton: (typeof import('@/components/app/AppSkeleton.vue'))['default'];
    AppSpace: (typeof import('@/components/app/AppSpace.vue'))['default'];
    AppSpin: (typeof import('@/components/app/AppSpin.vue'))['default'];
    AppSwitch: (typeof import('@/components/app/AppSwitch.vue'))['default'];
    AppTable: (typeof import('@/components/app/AppTable.vue'))['default'];
    AppTabs: (typeof import('@/components/app/AppTabs.vue'))['default'];
    AppTabsList: (typeof import('@/components/app/AppTabsList.vue'))['default'];
    AppTabsTrigger: (typeof import('@/components/app/AppTabsTrigger.vue'))['default'];
    AppTabsContent: (typeof import('@/components/app/AppTabsContent.vue'))['default'];
    AppTag: (typeof import('@/components/app/AppTag.vue'))['default'];
    AppTextarea: (typeof import('@/components/app/AppTextarea.vue'))['default'];
    AppTooltip: (typeof import('@/components/app/AppTooltip.vue'))['default'];
    AppUpload: (typeof import('@/components/app/AppUpload.vue'))['default'];
    CopyText: (typeof import('@/components/app/CopyText.vue'))['default'];
  }
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

declare module 'ipaddr.js' {
  export interface IPv4 {
    range(): string;
    kind(): 'ipv4';
  }
  export interface IPv6 {
    range(): string;
    kind(): 'ipv6';
    isIPv4MappedAddress(): boolean;
    toIPv4Address(): IPv4;
  }
  export function isValid(addr: string): boolean;
  export function parse(addr: string): IPv4 | IPv6;
}

interface ImportMetaEnv {
  readonly VITE_API_PREFIX?: string;
  readonly VITE_PROXY_TARGET?: string;
  readonly VITE_PLATFORM_DOMAIN?: string;
}

interface ImportMeta {
  readonly env: ImportMetaEnv;
}
