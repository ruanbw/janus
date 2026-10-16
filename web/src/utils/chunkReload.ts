/**
 * chunk 自动重载计数（共享实现）。
 *
 * 为什么集中在这里：这份逻辑此前在 main.ts 与 router/index.ts 各有一份逐字拷贝，
 * 而拷贝出来的两份**已经漂移**，且漂移的方向全是对用户不利的：
 *
 *   - key 不一致：一份用 location.pathname，一份用 route.fullPath。同一个页面在两条
 *     路径上各算各的账 → 计数互相看不见 → 自动重载被放行两次才被拦住。
 *   - 失败静默：main.ts 那一份在计数不可用或已用尽时什么都不做，用户只能盯着白屏，
 *     而 router 那一份会提示「请刷新重试」。
 *
 * 发版后旧 chunk 被清掉是唯一真正会命中这条路径的典型场景，而它的表现恰恰是白屏 ——
 * 兜底代码自己再静默失效，等于没有兜底。
 */
import { message } from '@/utils/toast';

/** 同一语义只能有一个 key：任何调用方都必须用它，不得各自拼字符串 */
export function chunkReloadKey(targetPath: string): string {
  return `chunk_reload_${targetPath}`;
}

/** 只允许自动重载一次：宁可让用户手动刷新，也不要无限重载循环 */
const MAX_AUTO_RELOAD = 1;

/**
 * 读自动重载计数。sessionStorage 在受限环境（隐私模式 / 企业策略 / 扩展拦截）下会抛
 * SecurityError —— 这正是兜底代码最不该自己炸掉的地方，返回 null 让调用方跳过自动
 * 重载（拿不到计数就无法保证不重复重载，绝不无限重试）。
 */
export function readChunkReloadCount(reloadKey: string): number | null {
  try {
    return parseInt(sessionStorage.getItem(reloadKey) || '0', 10);
  } catch {
    return null;
  }
}

/** 受限存储下也必须能执行的重载计数写入 */
export function writeChunkReloadCount(reloadKey: string, count: number): void {
  try {
    sessionStorage.setItem(reloadKey, String(count));
  } catch {
    // 忽略 sessionStorage 访问限制异常
  }
}

/** 受限存储下也必须能执行的重载计数清除 */
export function clearChunkReloadCount(reloadKey: string): void {
  try {
    sessionStorage.removeItem(reloadKey);
  } catch {
    // 忽略 sessionStorage 访问限制异常
  }
}

/**
 * 处理一次 chunk 加载失败：可以自动重载就重载，否则提示用户手动重试。
 *
 * @returns 是否已安排自动重载（false 表示已提示用户手动重试）
 */
export function handleChunkLoadFailure(targetPath: string): boolean {
  const reloadKey = chunkReloadKey(targetPath);
  const reloadCount = readChunkReloadCount(reloadKey);
  if (reloadCount === null) {
    // sessionStorage 不可用：无法判断是否重载过，绝不自动重载
    message.error('页面资源加载失败，请检查网络连接后刷新重试');
    return false;
  }
  if (reloadCount < MAX_AUTO_RELOAD) {
    writeChunkReloadCount(reloadKey, reloadCount + 1);
    window.location.assign(targetPath);
    return true;
  }
  // 已尝试自动刷新仍未成功，提示手动重试，避免死循环重载
  clearChunkReloadCount(reloadKey);
  message.error('页面资源加载失败，请检查网络连接后刷新重试');
  return false;
}
