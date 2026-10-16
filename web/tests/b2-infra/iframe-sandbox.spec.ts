/**
 * 自定义 HTML 预览 iframe 的 sandbox 一致性契约。
 *
 * 背景：线上（internal/httpapi/redirect.go 的 visitorErrorCSP）是
 * `sandbox allow-scripts allow-forms`，但 RuleFormView / RuleSimulatorView
 * 的预览却写成 `allow-same-origin`（恰好禁掉 allow-scripts），
 * 预览里脚本不执行、真机上却执行 —— 预览与真实行为的差异本身就是误导。
 * 本用例锁住「预览与线上同一组开关」，防止再次漂移。
 *
 * 视图组件体量大、且预览弹窗需要完整交互才能展开，这里直接校源码属性：
 * 断言的是 sandbox 值这一条不可变的契约，而不是渲染结果。
 */
import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';

import { describe, expect, it } from 'vitest';

/** 与线上 visitorErrorCSP 对齐的唯一合法值 */
const EXPECTED_SANDBOX = 'allow-scripts allow-forms';

const PREVIEW_VIEWS = [
  'src/views/rules/RuleFormView.vue',
  'src/views/rules/RuleSimulatorView.vue',
  'src/views/settings/ErrorPagesCard.vue',
];

// vitest 以 web/ 为项目根运行（见 vite.config.ts 的 test.include）
// 不用 new URL(..., import.meta.url)：vite 会把动态模板串当资源路径改写。
function readSource(relativePath: string): string {
  return readFileSync(resolve(process.cwd(), relativePath), 'utf8');
}

describe('自定义 HTML 预览 iframe sandbox', () => {
  for (const view of PREVIEW_VIEWS) {
    it(`${view} 使用与线上 CSP 一致的 sandbox`, () => {
      const source = readSource(view);

      expect(source).toContain(`sandbox="${EXPECTED_SANDBOX}"`);
      // allow-same-origin 会把预览 iframe 提升到本站来源，租户脚本即可读后台会话
      expect(source).not.toContain('sandbox="allow-same-origin"');
    });
  }
});
