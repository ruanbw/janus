import { beforeAll, describe, expect, it } from 'vitest';

/**
 * vite.config.ts 属于 tsconfig.node.json 的 composite 项目(tsconfig.json 以
 * project reference 引用它),从 tests/ 静态 import 会触发 TS6305
 * 「输出文件未构建」。这里用变量 specifier 让 TS 不做跨项目解析,
 * 运行时仍由 vite/vitest 正常加载真实配置文件。
 */
const configSpecifier = '../../vite.config';

interface AllowedHostsBuilder {
  (platformDomain: string, extraHosts: string): string[];
}

let buildAllowedHosts: AllowedHostsBuilder;

beforeAll(async () => {
  const mod: unknown = await import(/* @vite-ignore */ configSpecifier);
  const exported = mod as { buildAllowedHosts?: AllowedHostsBuilder };
  if (typeof exported.buildAllowedHosts !== 'function') {
    throw new Error('vite.config.ts 未导出 buildAllowedHosts');
  }
  buildAllowedHosts = exported.buildAllowedHosts;
});

/**
 * dev server 的 Host 白名单。vite 的判定逻辑是
 * 「`.${条目}` 与 hostname 相等,或 hostname 以它结尾」才放行,
 * 所以每一项都必须以子域后缀形式给出;`true` / `'*'` 等于关掉 DNS rebinding 防护。
 */
describe('vite dev server allowedHosts', () => {
  it('默认只放行平台域名及其子域', () => {
    expect(buildAllowedHosts('janus.test', '')).toEqual(['.janus.test']);
  });

  it('支持逗号分隔的额外白名单,每一项统一成子域后缀形式', () => {
    expect(buildAllowedHosts('janus.test', 'dev.example.com, .team.janus.test ,localhost')).toEqual([
      '.janus.test',
      '.dev.example.com',
      '.team.janus.test',
      '.localhost',
    ]);
  });

  it('不下发任何通配:不放宽 DNS rebinding 防护', () => {
    for (const hosts of [buildAllowedHosts('janus.test', ''), buildAllowedHosts('janus.test', '*')]) {
      expect(hosts).not.toContain(true);
      expect(hosts).not.toContain('*');
    }
  });

  it('额外白名单为空或仅空白时退化为只有平台域名', () => {
    expect(buildAllowedHosts('janus.test', ' , ')).toEqual(['.janus.test']);
  });

  it('重复声明平台域名不会产生冗余条目', () => {
    expect(buildAllowedHosts('janus.test', 'janus.test, .janus.test')).toEqual(['.janus.test']);
  });

  // 注:test 块 / resolve.alias / server.proxy 不在本组断言内。vite.config.ts 的
  // default export 在 vitest 运行时求值 fileURLToPath(new URL(...)) 会因
  // import.meta.url 非 file: 协议而抛错,无法直接调用。这三项的可用性由
  // `pnpm test:run` 自身跑绿来证明(vitest 需要 test 块,测试需要 @ 别名与 proxy)。
});