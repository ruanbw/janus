/**
 * trafficBreakdown 的契约测试。
 *
 * 关注点只有两个：
 *  1. 三个 UA 派生维度共用**一次**解析（模块注释承诺过，历史上没兑现）。
 *  2. 归类规则本身（爬虫优先、WebView 单列、权重累加）不被改动破坏。
 *
 * 「只解析一次」用计数器钉住，而不是靠注释提醒：直接统计 UAParser 的构造次数，
 * N 条 UA 必须恰好构造 N 次；三张图各跑一遍时会变成 3N，测试立刻红。
 */
const uaParserCalls = vi.hoisted(() => ({ count: 0 }));

vi.mock('ua-parser-js', async (importOriginal) => {
  const actual = await importOriginal<typeof import('ua-parser-js')>();
  return {
    ...actual,
    UAParser: class CountingUAParser extends actual.UAParser {
      constructor(ua?: string) {
        uaParserCalls.count += 1;
        super(ua);
      }
    },
  };
});

import { uaDistributions } from '@/views/overview/trafficBreakdown';
import type { FacetCount } from '@/types/api';

const UAS: FacetCount[] = [
  { value: 'Mozilla/5.0 (iPhone; CPU iPhone OS 17_0 like Mac OS X) AppleWebKit/605.1.15 Version/17.0 Mobile/15E148 Safari/604.1', count: 10 },
  { value: 'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0 Safari/537.36', count: 20 },
  { value: 'Mozilla/5.0 (compatible; Googlebot/2.1; +http://www.google.com/bot.html)', count: 5 },
];

describe('uaDistributions', () => {
  beforeEach(() => {
    uaParserCalls.count = 0;
  });

  it('三个维度合起来只解析一遍 UA（500 条 UA 时构造次数 = 条数，不是 3 倍）', () => {
    const many: FacetCount[] = Array.from({ length: 500 }, (_, i) => ({ value: UAS[i % UAS.length].value, count: i + 1 }));

    uaDistributions(many);

    expect(uaParserCalls.count).toBe(500);
  });

  it('单维度的合计与总数一致：三个维度的 count 之和都等于样本访问数', () => {
    const { device, os, browser } = uaDistributions(UAS);
    const total = UAS.reduce((sum, u) => sum + u.count, 0);

    expect(device.reduce((s, i) => s + i.count, 0)).toBe(total);
    expect(os.reduce((s, i) => s + i.count, 0)).toBe(total);
    expect(browser.reduce((s, i) => s + i.count, 0)).toBe(total);
  });

  it('空输入返回三组空数组，不产出占位行', () => {
    expect(uaDistributions([])).toEqual({ device: [], os: [], browser: [] });
  });
});

describe('uaDistributions 归类规则', () => {
  it('爬虫优先于移动端特征，权重按 count 累加', () => {
    const { device } = uaDistributions(UAS);

    // 爬虫 5 次走「爬虫 / 机器人」，不会被 iPhone UA 里的 mobile 特征带进移动端
    expect(device.find((d) => d.name === '爬虫 / 机器人')?.count).toBe(5);
    expect(device.find((d) => d.name === '移动端')?.count).toBe(10);
    expect(device.find((d) => d.name === '桌面端')?.count).toBe(20);
  });

  it('WebView / Safari / Firefox 分列，不塌进「其他」', () => {
    const { os, browser } = uaDistributions([
      { value: 'Mozilla/5.0 (iPhone; CPU iPhone OS 17_0 like Mac OS X) AppleWebKit/605.1.15 Mobile/15E148 MicroMessenger/8.0', count: 1 },
      { value: 'Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.0 Safari/605.1.15', count: 1 },
    ]);

    expect(os[0]?.name).toBe('iOS');
    expect(browser.find((b) => b.name === 'Safari')?.count).toBe(1);
    expect(browser.find((b) => b.name === '应用内内置')?.count).toBe(1);
  });
});