// 品牌白标 (White-Label) 配置
// 支持通过环境变量 VITE_APP_BRAND_NAME, VITE_APP_BRAND_TITLE 等无侵入注入定制品牌

export interface BrandConfig {
  name: string;
  subTitle: string;
  footer: string;
  authSlogan: string;
}

export const brandConfig: BrandConfig = {
  name: import.meta.env.VITE_APP_BRAND_NAME || 'Janus',
  subTitle: import.meta.env.VITE_APP_BRAND_SUBTITLE || 'Janus Console',
  footer: import.meta.env.VITE_APP_BRAND_FOOTER || 'Janus · 自托管短链服务',
  authSlogan: import.meta.env.VITE_APP_BRAND_AUTH_SLOGAN || '每一次跳转，皆为裁决。',
};
