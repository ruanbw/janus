/**
 * CSV 导出公式注入防护契约。
 *
 * 背景：短码/目标 URL 由租户输入控制，导出 CSV 时若以 [=+\-@]（含前导空白后）
 * 开头，Excel/Sheets 会当公式执行。修复：前置 `'` 强制按文本处理。
 */
import { describe, expect, it } from 'vitest';

import { escapeCsvFormula, toCsvCell, toCsvRow } from '@/utils/csv';

describe('escapeCsvFormula', () => {
  it('普通文本原样返回', () => {
    expect(escapeCsvFormula('abc')).toBe('abc');
    expect(escapeCsvFormula('https://example.com/a?b=1')).toBe('https://example.com/a?b=1');
    expect(escapeCsvFormula('落地页型')).toBe('落地页型');
  });

  it('以 = + - @ 开头的单元格前置单引号', () => {
    expect(escapeCsvFormula('=1+1')).toBe("'=1+1");
    expect(escapeCsvFormula('+SUM(A1:A9)')).toBe("'+SUM(A1:A9)");
    expect(escapeCsvFormula('-2+3')).toBe("'-2+3");
    expect(escapeCsvFormula('@SUM(A1)')).toBe("'@SUM(A1)");
    expect(escapeCsvFormula('=HYPERLINK("http://evil.test")')).toBe(
      '\'=HYPERLINK("http://evil.test")',
    );
    expect(escapeCsvFormula('=cmd|\' /c calc\'!A0')).toBe('\'=cmd|\' /c calc\'!A0');
  });

  it('前导空白/Tab/CR 之后是触发字符时同样前置单引号', () => {
    expect(escapeCsvFormula('  =1+1')).toBe("'  =1+1");
    expect(escapeCsvFormula('\t=1+1')).toBe("'\t=1+1");
    expect(escapeCsvFormula('\n@x')).toBe("'\n@x");
    expect(escapeCsvFormula('\r+1')).toBe("'\r+1");
  });

  it('空值/null/undefined 返回空串且不加前缀', () => {
    expect(escapeCsvFormula('')).toBe('');
    expect(escapeCsvFormula(null)).toBe('');
    expect(escapeCsvFormula(undefined)).toBe('');
  });

  it('已有引号的普通文本不触发公式防护(只由引号转义处理)', () => {
    expect(escapeCsvFormula('say "hi"')).toBe('say "hi"');
    expect(escapeCsvFormula('"=1+1"')).toBe('"=1+1"');
  });

  it('非字符串值先转字符串再判定', () => {
    expect(escapeCsvFormula(0)).toBe('0');
    expect(escapeCsvFormula(-5)).toBe("'-5");
  });
});

describe('toCsvCell', () => {
  it('公式内容前置单引号后整体加引号', () => {
    expect(toCsvCell('=1+1')).toBe('"\'=1+1"');
  });

  it('内部双引号被翻倍转义', () => {
    expect(toCsvCell('say "hi"')).toBe('"say ""hi"""');
    expect(toCsvCell('=HYPERLINK("x")')).toBe('"\'=HYPERLINK(""x"")"');
  });

  it('空值输出空单元格', () => {
    expect(toCsvCell(null)).toBe('""');
    expect(toCsvCell('')).toBe('""');
  });

  it('正常数字与日期不受影响', () => {
    expect(toCsvCell(12)).toBe('"12"');
    expect(toCsvCell('2026-01-01 00:00:00')).toBe('"2026-01-01 00:00:00"');
  });
});

describe('toCsvRow', () => {
  it('按列拼接并逐个单元格防护', () => {
    expect(toCsvRow(['abc', '=1+1', 3])).toBe('"abc","\'=1+1","3"');
  });
});
