/**
 * 环比文案:当前窗口的访问量与上一周期的同一个数比。
 *
 * 只看绝对值分不清"有没有量"和"在涨还是在跌",所以把对比做成一条文案,
 * 由 KPI 卡片直接展示。三种特殊态各有各的说法,不用数字硬凑:
 *   上一周期 0 → 当前 >0 是「新增」(除数为 0,涨跌幅没有定义);
 *   上一周期 0 → 当前 =0 是「持平」(两期都没量,谈不上增长);
 *   涨跌幅绝对值不足 0.05 个百分点 → 「持平」,避免 -0.0% 这种噪声。
 */
export function recentCompareText(current: number, previous: number): string {
  if (previous <= 0) return current > 0 ? '较上一周期 新增' : '较上一周期 持平';
  const percent = ((current - previous) / previous) * 100;
  if (Math.abs(percent) < 0.05) return '较上一周期 持平';
  return `较上一周期 ${percent > 0 ? '+' : ''}${percent.toFixed(1)}%`;
}
