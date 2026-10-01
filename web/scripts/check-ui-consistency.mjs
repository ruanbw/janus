#!/usr/bin/env node

/**
 * check-ui-consistency.mjs
 * 
 * 自动化 UI 规范与一致性门禁检查脚本：
 * 1. legacy 类名引用检查（.btn, .panel*, .badge*, .tbl*, .switch, .kpi*, 等）
 * 2. legacy 令牌引用检查（var(--fg), var(--muted), var(--border), 等）
 * 3. 未定义 CSS 变量检查（提取所有 var(--*) 与 main.css 定义比对，支持白名单）
 * 4. 默认调色板泄漏检查（slate-*, cyan-*, amber-*, emerald-*，除白名单外）
 * 5. 任意字号类检查（text-[Npx]）
 * 6. 业务视图中裸 HTML 原语检查（<button, <input, <select, <table>）
 * 7. main.css 中自定义 .truncate 规则检查（禁止污染 Tailwind 原生 truncate）
 * 8. dist 产物中动效变体规则存在性检查
 */

import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const __filename = fileURLToPath(import.meta.url);
const __dirname = path.dirname(__filename);
const webRoot = path.resolve(__dirname, '..');
const srcDir = path.resolve(webRoot, 'src');
const mainCssPath = path.resolve(srcDir, 'styles/main.css');

let totalErrors = 0;

function reportError(checkName, file, line, col, message) {
  totalErrors++;
  const relPath = path.relative(webRoot, file);
  console.error(`  ❌ [${checkName}] ${relPath}:${line}:${col} - ${message}`);
}

function walkDir(dir, filterFn, fileList = []) {
  if (!fs.existsSync(dir)) return fileList;
  const entries = fs.readdirSync(dir, { withFileTypes: true });
  for (const entry of entries) {
    const fullPath = path.join(dir, entry.name);
    if (entry.isDirectory()) {
      walkDir(fullPath, filterFn, fileList);
    } else if (filterFn(fullPath)) {
      fileList.push(fullPath);
    }
  }
  return fileList;
}

console.log('🔍 开始执行 UI 样式一致性与门禁检查...\n');

// 1. 检查 main.css 中是否存在未包层的 .truncate 覆盖
console.log('1. 检查 main.css 是否存在自定义 .truncate 覆盖...');
if (fs.existsSync(mainCssPath)) {
  const mainCssContent = fs.readFileSync(mainCssPath, 'utf8');
  const lines = mainCssContent.split('\n');
  lines.forEach((line, idx) => {
    if (/^\s*\.truncate\s*\{/.test(line)) {
      reportError('CustomTruncate', mainCssPath, idx + 1, 1, '禁止在 main.css 中自定义覆盖 .truncate');
    }
  });
}

// 2. 检查 legacy 类在 Vue 文件中的引用
console.log('2. 检查 legacy 组件与工具类在 src/**/*.vue 中的残留...');
const legacyClasses = [
  'btn', 'btn-primary', 'btn-sm', 'btn-ghost', 'btn-danger', 'btn-danger-solid', 'btn-ghost-danger', 'btn-row',
  'badge', 'badge-ok', 'badge-warn', 'badge-danger', 'badge-neutral',
  'panel', 'panel-hd', 'panel-bd', 'panel-ft',
  'tbl', 'tbl-wrap',
  'switch',
  'kpi', 'kpi-k', 'kpi-v', 'kpi-sub', 'kpi-info', 'kpi-tip',
  'bars', 'bar-row', 'bar-row-lg', 'bar-lab', 'bar-track', 'bar-fill', 'bar-val', 'stackbar', 'legend', 'legend-item', 'legend-key',
  'tiny', 'micro', 'muted', 'num', 'linkish', 'empty', 'field', 'input-icon', 'toolbar', 'seg-filter', 'cols-2',
  'dot', 'dot-live', 'chip', 'note', 'icon-btn', 'row-between'
];
const legacyClassRegex = new RegExp(`\\b(${legacyClasses.join('|')})\\b`);

const vueFiles = walkDir(srcDir, (p) => p.endsWith('.vue'));

vueFiles.forEach((file) => {
  const content = fs.readFileSync(file, 'utf8');
  const lines = content.split('\n');
  lines.forEach((line, idx) => {
    // 检查 class="..." 或 class='...' 或 :class="..."
    const classMatches = line.matchAll(/class=["']([^"']+)["']/g);
    for (const match of classMatches) {
      const classAttr = match[1];
      const tokens = classAttr.split(/\s+/);
      tokens.forEach((token) => {
        if (legacyClasses.includes(token)) {
          reportError('LegacyClass', file, idx + 1, match.index + 1, `使用了遗留类: "${token}"`);
        }
      });
    }
  });
});

// 3. 检查 legacy 变量引用
console.log('3. 检查 Tech-Utility 遗产变量 (var(--fg), var(--muted), etc.) 残留...');
// 注意:--muted / --accent / --border 已从黑名单移除——它们现在是 shadcn 语义层的正式令牌
// (--muted 表头/悬浮行底、--accent 菜单 hover、--border 控件描边),不再是 Tech-Utility 遗产。
const legacyTokens = [
  '--fg', '--fg-soft', '--accent-soft',
  '--danger-soft', '--warn-soft', '--line-soft', '--r', '--rl',
  '--fs-micro', '--fs-meta', '--fs-h1', '--fs-h2', '--fs-h3', '--fs-body', '--side-w', '--bg'
];
const legacyTokenRegex = new RegExp(`var\\((${legacyTokens.join('|')})\\)`);

const codeFiles = walkDir(srcDir, (p) => p.endsWith('.vue') || p.endsWith('.css') || p.endsWith('.ts'));

codeFiles.forEach((file) => {
  const content = fs.readFileSync(file, 'utf8');
  const lines = content.split('\n');
  lines.forEach((line, idx) => {
    const match = line.match(legacyTokenRegex);
    if (match) {
      reportError('LegacyToken', file, idx + 1, match.index + 1, `使用了已废弃的 CSS 变量: "${match[1]}"`);
    }
  });
});

// 4. 未定义 CSS 变量检查（提取全部 var(--*) 并与 main.css 声明比对）
console.log('4. 检查未定义的 CSS 变量引用...');
const definedVars = new Set();
if (fs.existsSync(mainCssPath)) {
  const mainCssContent = fs.readFileSync(mainCssPath, 'utf8');
  const varDeclMatches = mainCssContent.matchAll(/--([a-zA-Z0-9_-]+)\s*:/g);
  for (const match of varDeclMatches) {
    definedVars.add(`--${match[1]}`);
  }
}

// 补充白名单（运行时或第三方库注入或 Tailwind 内部变量）
const allowedVarPrefixes = [
  '--reka-',
  '--tw-',
  '--radix-',
  '--color-',
  '--radius-',
  '--spacing',
  '--font-',
  '--text-',
  '--leading-',
  '--sidebar-'
];
const allowedExactVars = new Set([
  '--sidebar-w',
  '--value'
]);

codeFiles.forEach((file) => {
  const content = fs.readFileSync(file, 'utf8');
  const lines = content.split('\n');
  lines.forEach((line, idx) => {
    const varMatches = line.matchAll(/var\(\s*(--[a-zA-Z0-9_-]+)/g);
    for (const match of varMatches) {
      const varName = match[1];
      if (definedVars.has(varName)) continue;
      if (allowedExactVars.has(varName)) continue;
      if (allowedVarPrefixes.some((p) => varName.startsWith(p))) continue;
      reportError('UndefinedCSSVar', file, idx + 1, match.index + 1, `引用了未在 main.css 中定义的 CSS 变量: "${varName}"`);
    }
  });
});

// 5. 默认调色板泄漏检查 (slate-*, cyan-*, amber-*, emerald-*)
console.log('5. 检查默认调色板泄漏 (slate, cyan, amber, emerald)...');
const paletteRegex = /\b(text|bg|border|fill|stroke)-(slate|cyan|amber|emerald)-\d+\b/;
const paletteAllowList = [
  'components/ui/AppTag.vue',      // 预设标签支持多色
  'components/BrandMark.vue',      // 品牌 Logo SVG
  'views/rules/RuleSimulatorView.vue' // 访客请求特征模板代码
];

vueFiles.forEach((file) => {
  const relPath = path.relative(srcDir, file).replace(/\\/g, '/');
  if (paletteAllowList.some((allowed) => relPath.endsWith(allowed))) return;

  const content = fs.readFileSync(file, 'utf8');
  const lines = content.split('\n');
  lines.forEach((line, idx) => {
    const match = line.match(paletteRegex);
    if (match) {
      reportError('PaletteLeak', file, idx + 1, match.index + 1, `发现未经收敛的默认颜色类: "${match[0]}"`);
    }
  });
});

// 6. 检查任意字号 text-[Npx]
console.log('6. 检查任意字号类 text-[Npx]...');
vueFiles.forEach((file) => {
  const content = fs.readFileSync(file, 'utf8');
  const lines = content.split('\n');
  lines.forEach((line, idx) => {
    const match = line.match(/text-\[[0-9.]*px\]/);
    if (match) {
      reportError('ArbitraryFontSize', file, idx + 1, match.index + 1, `存在非标准字号: "${match[0]}"，请使用 text-2xs / text-xs / text-sm 等`);
    }
  });
});

// 7. 业务视图中裸 HTML 原语检查 (<button, <input, <select, <table>)
console.log('7. 检查业务视图中的裸 HTML 原语 (<button, <input, <select, <table>)...');
const businessViews = walkDir(path.resolve(srcDir, 'views'), (p) => p.endsWith('.vue'));

businessViews.forEach((file) => {
  const content = fs.readFileSync(file, 'utf8');
  
  // 仅在 <template> ... </template> 区域内检查
  const templateMatch = content.match(/<template>([\s\S]*?)<\/template>/);
  if (!templateMatch) return;
  const templateContent = templateMatch[1];
  const templateOffset = content.indexOf('<template>') + '<template>'.length;

  // 匹配所有 <button ...>, <input ...>, <select ...>, <table ...> 标签（支持跨行）
  const tagRegex = /<(button|input|select|table)\b([^>]*)>/gis;
  let match;
  while ((match = tagRegex.exec(templateContent)) !== null) {
    const tagName = match[1].toLowerCase();
    const attrs = match[2];

    // 白名单 1: 隐藏的文件上传 input (<input type="file" ... class="hidden" ...>)
    if (tagName === 'input' && attrs.includes('type="file"') && attrs.includes('hidden')) {
      continue;
    }
    // 白名单 2: 语义化 Tab 选项卡按钮 (<button ... role="tab" ...>)
    if (tagName === 'button' && attrs.includes('role="tab"')) {
      continue;
    }

    // 计算行列号
    const charIndex = templateOffset + match.index;
    const linesBefore = content.substring(0, charIndex).split('\n');
    const lineNum = linesBefore.length;
    const colNum = linesBefore[linesBefore.length - 1].length + 1;

    reportError('RawHTMLPrimitive', file, lineNum, colNum, `在业务视图中发现了裸 HTML 原语: "<${tagName}>"，请使用 App* 组件替代`);
  }
});

// 8. 动效变体规则在构建产物中的存在性检查
console.log('8. 检查 dist 产物中是否包含动效变体编译规则...');
const distAssetsDir = path.resolve(webRoot, 'dist/assets');
if (fs.existsSync(distAssetsDir)) {
  const cssFiles = fs.readdirSync(distAssetsDir).filter((f) => f.endsWith('.css'));
  let foundDelayedOpen = false;
  let foundClosedAnimateOut = false;

  for (const cf of cssFiles) {
    const cssContent = fs.readFileSync(path.join(distAssetsDir, cf), 'utf8');
    // 检查 animate-in 与 delayed-open / closed 变体规则
    if (cssContent.includes('animate-in') && (cssContent.includes('delayed-open') || cssContent.includes('data-state=delayed-open'))) {
      foundDelayedOpen = true;
    }
    if (cssContent.includes('animate-out') && (cssContent.includes('data-state=closed') || cssContent.includes('state=closed'))) {
      foundClosedAnimateOut = true;
    }
  }

  if (!foundDelayedOpen) {
    totalErrors++;
    console.error('  ❌ [AnimationVariants] dist 产物中未找到 data-[state=delayed-open]:animate-in 编译规则');
  }
  if (!foundClosedAnimateOut) {
    totalErrors++;
    console.error('  ❌ [AnimationVariants] dist 产物中未找到 data-[state=closed]:animate-out 编译规则');
  }
} else {
  console.log('  ⚠️ dist 目录尚未构建，跳过产物检查（可在 pnpm build 后复验）');
}

console.log('\n----------------------------------------');
if (totalErrors === 0) {
  console.log('✅ UI 样式一致性门禁检查通过！0 违规项。');
  process.exit(0);
} else {
  console.error(`❌ UI 样式一致性检查发现 ${totalErrors} 处违规，请按上方提示修正！`);
  process.exit(1);
}
