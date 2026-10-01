/**
 * 流量结构分布：从后端聚合端点（GET /api/visits/overview）返回的**分桶计数**
 * 渲染来源 / 设备 / 系统 / 浏览器四张分布图，以及世界地图用的国家分布。
 *
 * 纯函数，不依赖路由与组件，便于单独验证。
 *
 * 数据为什么是「按值分桶」而不是「一行一次访问」：本模块过去接收的是
 * 「最近若干条短链 × 最近 50 条访问明细」这个样本，然后自己在浏览器里数。
 * 那样做有三个必然出错的地方：
 *   1. 口径漂移 —— 前端必须复刻一遍「只算 redirect/landing_view 且 success」
 *      的过滤，漏一个条件（历史上就漏过 action）就会把同一访客计两次，
 *      而同屏 KPI 走的是服务端已正确过滤的 link.visits，两套数字必然对不上。
 *   2. 抽样当全量 —— 点击量大的落地页，最近 50 行可能全是 click，
 *      分布图等于只统计了点击者。
 *   3. 覆盖不全 —— 短链列表每页上限 100 条，超过 100 条短链的租户只被统计了一部分。
 * 现在分桶与过滤都在 SQL 里完成（见 internal/store/visits.go 的
 * OverviewStatsForTenant），本模块只负责把「值 → 标签」这一步翻译好。
 */
import { UAParser } from 'ua-parser-js';

import { countryName } from '@/constants/countries';
import type { FacetCount, SourceCount } from '@/types/api';

export interface BreakdownItem {
  name: string;
  count: number;
  percent: number;
}

/** 一条 UA 解析出的三个维度，UA 只解析一次供三张图共用 */
interface UAFacets {
  device: string;
  os: string;
  browser: string;
}

const BOT_UA_RE = /bot|spider|crawl|curl|wget|python/i;

/**
 * 把一条 UA 串解析成设备 / 系统 / 浏览器三个标签。
 *
 * UA 只解析一次，三个维度共用同一个 UAParser 实例 —— 设备、系统、浏览器
 * 三次解析各跑一遍完整规则，在分布图要遍历全部 UA 串时是三倍的无谓开销。
 */
function facetsOfUA(ua: string): UAFacets {
  const parser = new UAParser(ua || '');
  return {
    device: deviceOf(parser.getDevice()?.type || '', ua),
    os: osOf(parser.getOS()?.name),
    browser: browserOf(parser.getBrowser()?.name),
  };
}

/**
 * 设备归类：爬虫判定放在最前面。
 *
 * 爬虫 UA 里常带 mobile/tablet 特征（伪装成移动端爬虫很常见），先判设备会把它们
 * 计进移动端，进而让「移动端占比」被爬虫流量抬高。
 */
function deviceOf(deviceType: string, ua: string): string {
  const userAgent = ua || '';
  if (BOT_UA_RE.test(userAgent)) return '爬虫 / 机器人';
  if (deviceType === 'mobile' || /mobile|iphone|android/i.test(userAgent)) return '移动端';
  if (deviceType === 'tablet' || /ipad/i.test(userAgent)) return '平板';
  return '桌面端';
}

function osOf(osName?: string | null): string {
  const name = osName || '';
  if (/ios/i.test(name)) return 'iOS';
  if (/android/i.test(name)) return 'Android';
  if (/windows/i.test(name)) return 'Windows';
  if (/mac/i.test(name)) return 'macOS';
  if (/linux/i.test(name)) return 'Linux';
  return '其他';
}

/**
 * 浏览器归类：与 ua-parser 的「其他 WebView」区别对待。
 *
 * 落到 else 的是各 App 内置 WebView（微信、抖音等）——它对短链运营来说和独立浏览器
 * 是两种投放来源，混在一起会看不清各渠道的真实环境。
 */
function browserOf(browserName?: string | null): string {
  const name = browserName || '';
  if (/chrome|chromium/i.test(name)) return 'Chrome / WebKit';
  if (/safari/i.test(name)) return 'Safari';
  if (/firefox/i.test(name)) return 'Firefox';
  return '应用内内置';
}

/**
 * 按固定标签集对**加权**的分桶计数求和后转占比，并按数量降序。
 *
 * 权重是这里唯一的改动点：过去喂进来的是「一行一次访问」，每行权重恒为 1；
 * 现在喂进来的是「某个 UA 串出现了 N 次」，必须按 N 累加，否则一台设备
 * 访问 500 次和另一台访问 1 次会被算成平手。
 */
function tallyWeighted(
	pairs: Array<{ label: string; count: number }>,
	labels: string[],
): BreakdownItem[] {
	const counts = new Map<string, number>(labels.map((l) => [l, 0]));
	let total = 0;
	for (const { label, count } of pairs) {
		counts.set(label, (counts.get(label) ?? 0) + count);
		total += count;
	}
	return labels
		.map((name) => ({
			name,
			count: counts.get(name) ?? 0,
			percent: total === 0 ? 0 : Math.round(((counts.get(name) ?? 0) / total) * 100),
		}))
		.sort((a, b) => b.count - a.count);
}

export interface CountryBreakdown extends BreakdownItem {
  /** ISO 3166-1 alpha-2 码（后端 visits.country 的原值），世界地图按它上色 */
  code: string;
}

/** 三个 UA 派生维度共用的一次解析结果，避免同一串 UA 被解析三遍 */
interface UAWeightedFacets {
  device: string;
  os: string;
  browser: string;
}

/**
 * 把「UA 串 → 出现次数」的分桶解析成四个维度的加权桶。
 *
 * 标签规则（爬虫优先、Chrome 归一、WebView 单列）只在这一处实现：
 * 后端只做 GROUP BY user_agent，不翻译标签 —— 在 Go 侧另写一套 UA 解析器
 * 只会制造第二个会漂移的口径，而 UA 串的数量级（几十到几百）远小于
 * 访问明细行数，传原始串既省带宽又不丢信息。
 */
function uaWeightedFacets(userAgents: FacetCount[]): {
  device: Array<{ label: string; count: number }>;
  os: Array<{ label: string; count: number }>;
  browser: Array<{ label: string; count: number }>;
} {
  const device: Array<{ label: string; count: number }> = [];
  const os: Array<{ label: string; count: number }> = [];
  const browser: Array<{ label: string; count: number }> = [];
  for (const { value, count } of userAgents) {
    const parsed = facetsOfUA(value);
    device.push({ label: parsed.device, count });
    os.push({ label: parsed.os, count });
    browser.push({ label: parsed.browser, count });
  }
  return { device, os, browser };
}

/** 设备 / 系统 / 浏览器的固定标签集（顺序即界面展示顺序的兜底） */
const DEVICE_LABELS = ['移动端', '桌面端', '平板', '爬虫 / 机器人'];
const OS_LABELS = ['iOS', 'Android', 'Windows', 'macOS', 'Linux', '其他'];
const BROWSER_LABELS = ['Chrome / WebKit', 'Safari', 'Firefox', '应用内内置'];

export function deviceDistribution(userAgents: FacetCount[]): BreakdownItem[] {
  if (userAgents.length === 0) return [];
  return tallyWeighted(uaWeightedFacets(userAgents).device, DEVICE_LABELS);
}

export function osDistribution(userAgents: FacetCount[]): BreakdownItem[] {
  if (userAgents.length === 0) return [];
  return tallyWeighted(uaWeightedFacets(userAgents).os, OS_LABELS);
}

export function browserDistribution(userAgents: FacetCount[]): BreakdownItem[] {
  if (userAgents.length === 0) return [];
  return tallyWeighted(uaWeightedFacets(userAgents).browser, BROWSER_LABELS);
}

/**
 * 来源分布：后端已按广告平台归类完成（见 store 的 overviewSourceExpr），
 * 这里只把「标签 → 占比」这一步做完，并补齐固定标签集里没出现的项。
 */
export function sourceDistribution(sources: SourceCount[]): BreakdownItem[] {
  if (sources.length === 0) return [];
  return tallyWeighted(
		sources.map((s) => ({ label: s.name, count: s.count })),
		['TikTok Ads', 'Meta Ads', 'Google Ads', '直接访问', '其他来源'],
	);
}

/**
 * 国家分布：直接取 `visits.country`（后端用离线 GeoIP 库解析的 ISO 3166-1 alpha-2 码）。
 *
 * 与其余四个维度不同，这里不做兜底归类：解析不出国家的访问（私网/回环地址、
 * 离线库未收录）既不进任何国家，也不塞进「其他来源」那种筐。「其他来源」是用户
 * 看得懂的语义，「未知」不是——那批访问的来源确实未知，把它摊到某个国家名下
 * 等于编数据；而地图上少一块颜色，本就是一个诚实的表达。
 */
export function countryDistribution(countries: FacetCount[]): CountryBreakdown[] {
  if (countries.length === 0) return [];
  const counts = new Map<string, number>();
  for (const { value, count } of countries) {
    const code = (value || '').trim().toUpperCase();
    if (!/^[A-Z]{2}$/.test(code)) continue;
    counts.set(code, (counts.get(code) ?? 0) + count);
  }
  // 分母是「能定位到国家的访问数」而不是样本总量，否则一个只覆盖海外流量的租户
  // 会因为私网/回环访问占多数而把所有颜色压到最低档。
  const total = [...counts.values()].reduce((sum, n) => sum + n, 0);
  if (total === 0) return [];

  return [...counts.entries()]
    .map(([code, count]) => ({
      code,
      name: countryName(code) || code,
      count,
      percent: Math.round((count / total) * 100),
    }))
    .sort((a, b) => b.count - a.count);
}
