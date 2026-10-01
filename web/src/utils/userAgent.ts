// User-Agent 解析工具(访问明细列表与总览的流量结构分布共用)
// 判别逻辑为原样搬迁,任何调整都会同时影响两个页面,故此处不做“顺手优化”。
import { UAParser } from 'ua-parser-js';
import type { TagColor } from '@/components/ui/AppTag.vue';

/** UA 解析结果 */
export interface ParsedUA {
  os: string;
  browser: string;
  deviceType: '移动端' | '桌面端' | '平板' | '爬虫机器人';
  deviceModel: string;
  isBot: boolean;
}

/** 爬虫/自动化客户端特征签名 */
const BOT_REGEX =
  /bot|spider|crawl|slurp|facebookexternalhit|curl|wget|python|httpclient|postman|apachebench|googlebot|bingbot|bytespider|yandex|duckduckbot|headless|phantomjs|selenium|puppeteer/i;

/**
 * 解析结果缓存:访问明细列表里同一批 User-Agent 会被反复解析(设备列、
 * 搜索过滤、每行渲染),这里按 UA 字符串缓存结果,避免重复构造 UAParser。
 */
const parseCache = new Map<string, ParsedUA>();

/** 缓存上限:列表翻页时 UA 高度重复,无需无限增长;超出后整体清空一次 */
const CACHE_LIMIT = 500;

/** 专业解析真实 User-Agent */
export function parseUserAgent(ua: string): ParsedUA {
  if (!ua || !ua.trim()) {
    return {
      os: '未知操作系统',
      browser: '未知浏览器',
      deviceType: '桌面端',
      deviceModel: '未知设备',
      isBot: false,
    };
  }

  const cached = parseCache.get(ua);
  if (cached) return cached;

  const parser = new UAParser(ua);
  const res = parser.getResult();
  const rawUa = ua.toLowerCase();

  const isBot = BOT_REGEX.test(rawUa) || (res.browser.name ? BOT_REGEX.test(res.browser.name) : false);

  let deviceType: ParsedUA['deviceType'] = '桌面端';
  if (isBot) {
    deviceType = '爬虫机器人';
  } else if (res.device.type === 'tablet' || /ipad|tablet/i.test(rawUa)) {
    deviceType = '平板';
  } else if (res.device.type === 'mobile' || /mobile|iphone|android/i.test(rawUa)) {
    deviceType = '移动端';
  } else {
    deviceType = '桌面端';
  }

  // 操作系统解析 (如 iOS 18, Windows 11, Android 14)
  const osName = res.os.name || '';
  const osVersion = res.os.version || '';
  let os = [osName, osVersion].filter(Boolean).join(' ').trim();
  if (osName === 'Windows' && rawUa.includes('windows nt 10.0') && rawUa.includes('windows 11')) {
    os = 'Windows 11';
  }
  if (!os) {
    os = isBot ? '服务器环境 (Bot)' : '未知操作系统';
  }

  // 浏览器解析 (如 Chrome 131, Safari, Firefox)
  const browserName = res.browser.name || '';
  const browserVer = res.browser.major || res.browser.version || '';
  let browser = [browserName, browserVer].filter(Boolean).join(' ').trim();
  if (!browser) {
    if (rawUa.includes('curl')) browser = 'cURL CLI';
    else if (rawUa.includes('facebookexternalhit')) browser = 'Facebook Crawler';
    else if (isBot) browser = 'Automated Agent';
    else browser = '未知浏览器';
  }

  // 真实设备型号
  const vendor = res.device.vendor || '';
  const model = res.device.model || '';
  let deviceModel = [vendor, model].filter(Boolean).join(' ').trim();
  if (!deviceModel) {
    if (deviceType === '爬虫机器人') deviceModel = '自动化节点';
    else if (deviceType === '桌面端') deviceModel = osName ? `${osName} PC` : 'PC 桌面';
    else deviceModel = deviceType;
  }

  const parsed: ParsedUA = {
    os,
    browser,
    deviceType,
    deviceModel,
    isBot,
  };

  if (parseCache.size >= CACHE_LIMIT) parseCache.clear();
  parseCache.set(ua, parsed);
  return parsed;
}

/** 设备类型对应的徽标配色(AppTag 的 color 预设,不是裸 class 名) */
export function getDeviceTagColor(deviceType: ParsedUA['deviceType']): TagColor {
  switch (deviceType) {
    case '移动端':
      return 'success';
    case '平板':
      return 'warning';
    case '爬虫机器人':
      return 'error';
    default:
      return 'default';
  }
}
