/**
 * 流量结构分布：从访问明细样本聚合出来源 / 设备 / 系统 / 浏览器四张分布图。
 *
 * 纯函数，不依赖路由与组件，便于单独验证。数据来源是「最近若干条短链的最近若干条
 * 访问明细」这个**样本**，不是全量访问，所以调用方必须把样本量一并展示出来——
 * 否则 10 条短链的占比会被读成整个租户的结构。
 */
import { UAParser } from 'ua-parser-js';

import type { Visit } from '@/types/api';

export interface BreakdownItem {
  name: string;
  count: number;
  percent: number;
}

/** 单条访问明细解析出的四个维度，UA 只解析一次供四张图共用 */
interface VisitFacets {
  source: string;
  device: string;
  os: string;
  browser: string;
}

const BOT_UA_RE = /bot|spider|crawl|curl|wget|python/i;

/** 来源归类：优先匹配主流广告平台 referrer，未携带来源的流量归入直接访问 */
function sourceOf(referer: string): string {
  const ref = referer.toLowerCase();
  if (ref.includes('tiktok')) return 'TikTok Ads';
  if (ref.includes('facebook') || ref.includes('instagram') || ref.includes('meta')) return 'Meta Ads';
  if (ref.includes('google')) return 'Google Ads';
  if (!ref || ref === '-') return '直接访问';
  return '其他来源';
}

/**
 * 设备归类：爬虫判定放在最前面。
 *
 * 爬虫 UA 里常带 mobile/tablet 特征（伪装成移动端爬虫很常见），先判设备会把它们
 * 计进移动端，进而让「移动端占比」被爬虫流量抬高。
 */
function deviceOf(deviceType: string, ua: string): string {
  if (BOT_UA_RE.test(ua)) return '爬虫 / 机器人';
  if (deviceType === 'mobile' || /mobile|iphone|android/i.test(ua)) return '移动端';
  if (deviceType === 'tablet' || /ipad/i.test(ua)) return '平板';
  return '桌面端';
}

function osOf(osName: string): string {
  if (/ios/i.test(osName)) return 'iOS';
  if (/android/i.test(osName)) return 'Android';
  if (/windows/i.test(osName)) return 'Windows';
  if (/mac/i.test(osName)) return 'macOS';
  if (/linux/i.test(osName)) return 'Linux';
  return '其他';
}

/**
 * 浏览器归类：与 ua-parser 的「其他 WebView」区别对待。
 *
 * 落到 else 的是各 App 内置 WebView（微信、抖音等）——它对短链运营来说和独立浏览器
 * 是两种投放来源，混在一起会看不清各渠道的真实环境。
 */
function browserOf(browserName: string): string {
  if (/chrome|chromium/i.test(browserName)) return 'Chrome / WebKit';
  if (/safari/i.test(browserName)) return 'Safari';
  if (/firefox/i.test(browserName)) return 'Firefox';
  return '应用内内置';
}

function facetsOf(v: Visit): VisitFacets {
  const ua = v.userAgent || '';
  const parser = new UAParser(ua);
  return {
    source: sourceOf(v.referer || ''),
    device: deviceOf(parser.getDevice().type || '', ua),
    os: osOf(parser.getOS().name || ''),
    browser: browserOf(parser.getBrowser().name || ''),
  };
}

/** 按固定标签集计数后转成占比，并按数量降序 */
function tally(values: string[], labels: string[]): BreakdownItem[] {
  const counts = new Map<string, number>(labels.map((l) => [l, 0]));
  for (const value of values) counts.set(value, (counts.get(value) ?? 0) + 1);
  const total = values.length;
  return labels
    .map((name) => ({
      name,
      count: counts.get(name) ?? 0,
      percent: total === 0 ? 0 : Math.round(((counts.get(name) ?? 0) / total) * 100),
    }))
    .sort((a, b) => b.count - a.count);
}

export function sourceDistribution(visits: Visit[]): BreakdownItem[] {
  if (visits.length === 0) return [];
  return tally(
    visits.map((v) => facetsOf(v).source),
    ['TikTok Ads', 'Meta Ads', 'Google Ads', '直接访问', '其他来源'],
  );
}

export function deviceDistribution(visits: Visit[]): BreakdownItem[] {
  if (visits.length === 0) return [];
  return tally(
    visits.map((v) => facetsOf(v).device),
    ['移动端', '桌面端', '平板', '爬虫 / 机器人'],
  );
}

export function osDistribution(visits: Visit[]): BreakdownItem[] {
  if (visits.length === 0) return [];
  return tally(
    visits.map((v) => facetsOf(v).os),
    ['iOS', 'Android', 'Windows', 'macOS', 'Linux', '其他'],
  );
}

export function browserDistribution(visits: Visit[]): BreakdownItem[] {
  if (visits.length === 0) return [];
  return tally(
    visits.map((v) => facetsOf(v).browser),
    ['Chrome / WebKit', 'Safari', 'Firefox', '应用内内置'],
  );
}