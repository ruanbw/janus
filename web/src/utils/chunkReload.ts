/**
 * chunk 自动重载计数（共享实现）。
 *
 * 为什么集中在这里：这份逻辑此前在 main.ts 与 router/index.ts 各有一份逐字拷贝，
 * 而拷贝出来的两份**已经漂移**，漂移方向全是对用户不利的：main.ts 那一份在计数不可
 * 用或用尽时什么都不做，用户只能盯着白屏，而 router 那一份会提示「请刷新重试」。
 *
 * 注意各自的**入参保持不同**是对的，不要合并成同一个：unhandledrejection 兜底只知道
 * 「页面坏了」，用 pathname 计数（同一页面不同查询串算同一页）且只能 reload；router
 * 场景手上有 to.fullPath，用 fullPath 计数并 assign 过去（reload 会丢掉 hash 与查询串）。
 * 两边语义本来就不同，强行统一才是 bug。
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
 * 处理一次 chunk 加载失败：判断该自动重载还是提示用户手动重试。
 *
 * @param targetPath 决定计数 key
 * @param reload 由调用方决定**怎么**重载 —— 两边语义不同，不能替它们统一：
 *   unhandledrejection 兜底只知道「页面坏了」，只能 reload 当前页；而 router 场景下
 *   手上有明确的 to.fullPath，assign 过去才精确（reload 会丢掉 hash 与查询串）。
 * @returns 是否已安排自动重载（false 表示已提示用户手动重试）
 */
export function handleChunkLoadFailure(targetPath: string, reload: () => void): boolean {
  const reloadKey = chunkReloadKey(targetPath);
  const reloadCount = readChunkReloadCount(reloadKey);
  if (reloadCount === null) {
    // sessionStorage 不可用：无法判断是否重载过，绝不自动重载
    message.error('页面资源加载失败，请检查网络连接后刷新重试');
    return false;
  }
  if (reloadCount < MAX_AUTO_RELOAD) {
    writeChunkReloadCount(reloadKey, reloadCount + 1);
    reload();
    return true;
  }
  // 已尝试自动刷新仍未成功，提示手动重试，避免死循环重载
  clearChunkReloadCount(reloadKey);
  message.error('页面资源加载失败，请检查网络连接后刷新重试');
  return false;
}
