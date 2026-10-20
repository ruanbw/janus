/**
 * `downloadCsv` 的契约：把「表格数据 → 用户下载到的文件」这一段落只实现一次。
 *
 * 被钉住的不变式（Given-When-Then）：
 *   Given 一组表头与行数据
 *   When  调用 downloadCsv
 *   Then  生成的 CSV 带 UTF-8 BOM、行内单元格做过公式注入防护、
 *         并且 blob URL 在下载开始后**延迟**释放
 *
 * 为什么要抽出来：短链列表与访问明细都要导出，两处各写一遍时，
 * 「BOM / 转义 / 延迟释放」这三条里迟早有一条只在其中一处成立 ——
 * 比如漏掉 BOM，Excel 打开中文全是乱码，而这种问题只有用户下载后才看得到。
 */
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

import { downloadCsv } from '@/utils/csv';

let createObjectURL: ReturnType<typeof vi.fn>;
let revokeObjectURL: ReturnType<typeof vi.fn>;
let anchorClick: ReturnType<typeof vi.spyOn>;
let originalCreate: typeof URL.createObjectURL;
let originalRevoke: typeof URL.revokeObjectURL;
let originalBlob: typeof Blob;
/** 捕获最终写进 Blob 的文本 */
let blobParts: BlobPart[] = [];

beforeEach(() => {
  blobParts = [];
  createObjectURL = vi.fn(() => 'blob:mock-url');
  revokeObjectURL = vi.fn();
  originalCreate = URL.createObjectURL;
  originalRevoke = URL.revokeObjectURL;
  originalBlob = globalThis.Blob;
  URL.createObjectURL = createObjectURL as unknown as typeof URL.createObjectURL;
  URL.revokeObjectURL = revokeObjectURL as unknown as typeof URL.revokeObjectURL;
  anchorClick = vi.spyOn(HTMLAnchorElement.prototype, 'click').mockImplementation(() => {});

  // 追踪 Blob 构造参数:比事后读 blob.text() 更直接,也不依赖 happy-dom 的实现细节
  const RealBlob = globalThis.Blob;
  globalThis.Blob = class extends RealBlob {
    constructor(parts: BlobPart[] = [], options?: BlobPropertyBag) {
      super(parts, options);
      blobParts = parts;
    }
  } as typeof Blob;
});

afterEach(() => {
  URL.createObjectURL = originalCreate;
  URL.revokeObjectURL = originalRevoke;
  anchorClick.mockRestore();
  globalThis.Blob = originalBlob;
});

const HEADERS = ['短码', '目标'];
const ROWS = [
  [' abc123', 'https://example.com/a'],
  ['=cmd|calc', '@SUM(A1)'],
];

/** 取回写进 Blob 的完整 CSV 文本 */
function csvText(): string {
  return blobParts.map((part) => String(part)).join('');
}

describe('downloadCsv', () => {
  it('Given 表头与行, When 导出, Then CSV 以 BOM 开头且表头不加引号', () => {
    downloadCsv('demo.csv', HEADERS, ROWS);

    const text = csvText();
    expect(text.startsWith('\uFEFF'), '缺 BOM 时 Excel 打开中文是乱码').toBe(true);
    expect(text).toContain('短码,目标');
  });

  it('Given 含公式触发的单元格, When 导出, Then 逐单元格做注入防护', () => {
    downloadCsv('demo.csv', HEADERS, ROWS);

    const body = csvText().slice(1); // 去掉 BOM
    const lines = body.split('\n');
    // 数据行里的 =cmd|calc 与 @SUM(A1) 都必须被前置单引号
    // (lines[0] 是表头,lines[1] 是普通行,lines[2] 才是公式行)
    expect(lines[2]).toContain(`"'=cmd|calc"`);
    expect(lines[2]).toContain(`"'@SUM(A1)"`);
    // 普通值原样（首尾空白也保留在引号里）
    expect(lines[1]).toContain(`" abc123"`);
  });

  it('Given 文件名, When 导出, Then 用文件名创建下载并触发一次点击', () => {
    downloadCsv('visits-20261021.csv', HEADERS, ROWS);

    expect(createObjectURL).toHaveBeenCalledTimes(1);
    expect(anchorClick).toHaveBeenCalledTimes(1);
  });

  it('Given 导出完成, When 立即检查, Then blob URL 还没被释放（下载器还没接管）', () => {
    downloadCsv('demo.csv', HEADERS, ROWS);

    // 同步 revoke 会让 Firefox 直接取消下载
    expect(revokeObjectURL).not.toHaveBeenCalled();
  });

  it('Given 只有表头没有数据行, When 导出, Then 仍产出可打开的 CSV（不生成空文件）', () => {
    downloadCsv('empty.csv', HEADERS, []);

    expect(csvText().slice(1)).toBe('短码,目标');
    expect(anchorClick).toHaveBeenCalledTimes(1);
  });
});
