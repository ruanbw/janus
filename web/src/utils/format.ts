// 常用格式化工具
import dayjs from 'dayjs';

/** 时间格式化:2026-08-13 18:30:00 */
export function formatDateTime(value?: string | null): string {
  if (!value) return '-';
  const d = dayjs(value);
  if (!d.isValid()) return '-';
  return d.format('YYYY-MM-DD HH:mm:ss');
}

/** 只要时分秒,如 "18:30:00"。
 *  列表里同一天的记录排在一起，年份与日期是噪声，悬停时再给完整时间（见 formatDateTime）。 */
export function formatClock(value?: string | null): string {
  if (!value) return '-';
  const d = dayjs(value);
  if (!d.isValid()) return '-';
  return d.format('HH:mm:ss');
}

/** 相对时间,如 "3 分钟前" */
export function formatRelative(value?: string | null): string {
  if (!value) return '-';
  const d = dayjs(value);
  if (!d.isValid()) return '-';
  const diff = Date.now() - d.valueOf();
  if (diff < 60_000) return '刚刚';
  if (diff < 3_600_000) return `${Math.floor(diff / 60_000)} 分钟前`;
  if (diff < 86_400_000) return `${Math.floor(diff / 3_600_000)} 小时前`;
  return formatDateTime(value);
}

/** 截断过长的目标 URL 展示 */
export function truncateText(text: string, max = 48): string {
  if (text.length <= max) return text;
  return `${text.slice(0, max)}…`;
}

// 地区名走浏览器内置的 CLDR 数据(零依赖、零打包体积、自动跟随 ICU 更新)。
// 单独包一层是因为 Intl.DisplayNames 在个别环境(旧 Node、老浏览器)上不存在,
// 直接用会把整页渲染打挂 —— 展示层不值得为一次翻译冒这个险。
let regionNames: Intl.DisplayNames | null | undefined;
function displayNames(): Intl.DisplayNames | null {
  if (regionNames === undefined) {
    try {
      regionNames = new Intl.DisplayNames(['zh-CN'], { type: 'region' });
    } catch {
      regionNames = null;
    }
  }
  return regionNames;
}

/** ISO 3166-1 alpha-2 国家码 → 「美国（US）」；查不到时给「—」。 */
export function formatCountry(code?: string | null): string {
  const c = (code || '').trim().toUpperCase();
  if (!c) return '—';
  const names = displayNames();
  // 不是两字母的码(脏数据、占位值)原样返回:猜一个名字比暴露数据问题更糟
  if (c.length !== 2 || !names) return c;
  try {
    const name = names.of(c);
    return name && name !== c ? `${name}（${c}）` : c;
  } catch {
    return c;
  }
}
