// 前端启动配置 API(后台,会话鉴权)
import { get } from '@/utils/request';

import type { AppConfig } from '@/types/api';

/** 获取启动配置:服务器 IP/平台域名/当前租户配额(每个租户返回各自的 usage) */
export function fetchConfig(): Promise<AppConfig> {
  return get<AppConfig>('/config');
}
