/**
 * 全局注册表与 `GlobalComponents` 声明的一致性守卫。
 *
 * 背景：`components/app/index.ts` 用 `app.component(name, component)` 全局注册了
 * 一批组件，而 view 里绝大多数**不写 import**（`ErrorPagesCard.vue` 的 `.vue` import
 * 数为 0）。`vue-tsc` 解析模板标签时只认「当前文件 import 过的符号」+
 * `GlobalComponents` 接口 —— 全局注册的组件若没在 `env.d.ts` 里声明，
 * 会被当成未知组件，**props 校验静默失效且不报任何错**。
 *
 * 这正是本次踩到的坑：删掉 `declare module '*.vue'` 通配声明后 vue-tsc 仍是 0 错误，
 * 就是因为漏了这一层。两份事实源（index.ts / env.d.ts）靠人工同步迟早会漂移，
 * 故加测试钉住。
 */
import { readFileSync } from 'node:fs';
import { fileURLToPath, URL } from 'node:url';

const read = (rel: string): string =>
  readFileSync(fileURLToPath(new URL(rel, import.meta.url)), 'utf8');

describe('全局注册表 ↔ GlobalComponents 一致性', () => {
  it('index.ts 注册的每个组件都在 env.d.ts 的 GlobalComponents 里有条目', () => {
    const index = read('../../src/components/app/index.ts');
    const env = read('../../src/env.d.ts');

    // 注册表字面量里的键
    const block = index.split('const components: Record<string, Component> = {')[1]?.split('\n};')[0];
    expect(block).toBeDefined();

    const registered = (block ?? '')
      .split('\n')
      .map((l) => l.trim().replace(/,$/, ''))
      .filter((l) => /^[A-Z][A-Za-z0-9]*$/.test(l));

    expect(registered.length).toBeGreaterThan(0);

    const gcBlock = env.split('export interface GlobalComponents {')[1]?.split('\n  }')[0];
    expect(gcBlock).toBeDefined();

    const declared = [...(gcBlock ?? '').matchAll(/^\s{4}([A-Za-z][A-Za-z0-9]*):/gm)].map((m) => m[1]!);

    const missing = registered.filter((n) => !declared.includes(n));
    // 缺一个就意味着该组件的 props 完全不过编译期检查，且没有任何报错提示
    expect(missing).toEqual([]);

    const extra = declared.filter((n) => !registered.includes(n));
    expect(extra).toEqual([]);
  });

  it('env.d.ts 不含 declare module "*.vue" 通配声明', () => {
    const env = read('../../src/env.d.ts');

    // 先剥掉注释与字符串字面量，否则解释「为什么不能加回来」的注释里
    // 提到的 `declare module '*.vue'` 会被自己这条断言误命中。
    const code = env
      .replace(/\/\*[\s\S]*?\*\//g, '')
      .replace(/\/\/[^\n]*/g, '');

    // 这条通配会把每个 SFC 的 props 统一声明成 Record<string, never>，
    // 「漏传必填 prop / prop 类型不匹配」两类错误在编译期一个都查不出来。
    expect(code).not.toMatch(/declare module ['"]\*\.vue['"]/);
  });

  it('env.d.ts 用 export {} 把文件变成模块，使 declare module "vue" 成为接口增强而非重声明', () => {
    const env = read('../../src/env.d.ts');

    // 少了 export {}，declare module 'vue' 会整体遮蔽真实 vue 类型，
    // 报出成百上千条 TS2305「Module '"vue"' has no exported member 'computed'」。
    expect(env).toMatch(/^\s*export \{\};/m);
    expect(env).toMatch(/declare module 'vue'\s*\{/);
  });
});