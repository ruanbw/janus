// 常用格式化工具
import dayjs from 'dayjs';

/** 时间格式化:2026-08-13 18:30:00 */
export function formatDateTime(value?: string | null): string {
  if (!value) return '-';
  const d = dayjs(value);
  if (!d.isValid()) return '-';
  return d.format('YYYY-MM-DD HH:mm:ss');
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
