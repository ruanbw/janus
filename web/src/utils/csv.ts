// CSV 导出工具

/**
 * CSV 公式注入(CSV Injection / Formula Injection)触发字符。
 *
 * Excel / Google Sheets 打开 CSV 时,会把以 `=` `+` `-` `@` 开头的单元格当公式求值
 * (还有以 Tab/CR 开头的 DDE 变体)。短码、目标 URL 等字段完全由租户输入控制,
 * 一旦带上 `=HYPERLINK(...)` / `=cmd|...` 就会在打开表格的同事机器上执行。
 *
 * 前置 `'` 是业界通行的缓解手段:Excel 会把该单元格当纯文本,不显示这个引号。
 * 注意前导空白/Tab/CR 之后仍以触发字符开头同样会被求值,故一并覆盖。
 */
const FORMULA_TRIGGER = /^[\s]*[=+\-@]/;

/**
 * 对可能被解释为公式的单元格内容前置 `'`,其余原样返回。
 * 只做值层面的清洗,不负责 CSV 引号转义(见 toCsvCell)。
 */
export function escapeCsvFormula(value: unknown): string {
  const text = value === null || value === undefined ? '' : String(value);
  return FORMULA_TRIGGER.test(text) ? `'${text}` : text;
}

/** 转成 CSV 单元格:先做公式注入防护,再用双引号包裹并转义内部引号 */
export function toCsvCell(value: unknown): string {
  return `"${escapeCsvFormula(value).replace(/"/g, '""')}"`;
}

/** 转成一行 CSV */
export function toCsvRow(cells: readonly unknown[]): string {
  return cells.map((cell) => toCsvCell(cell)).join(',');
}

/**
 * 生成 CSV 文本(含 UTF-8 BOM)。
 *
 * BOM 是给 Excel 的:没有它,Excel 会按本地代码页打开,中文表头与目标 URL 全是乱码。
 * 表头是固定字面量,保持不加引号以便阅读;数据行逐单元格转义(见 toCsvRow)。
 */
export function buildCsv(headers: readonly string[], rows: readonly (readonly unknown[])[]): string {
  return '\uFEFF' + [headers.join(','), ...rows.map((row) => toCsvRow(row))].join('\n');
}

/**
 * 触发一次文件下载。
 *
 * `revokeObjectURL` 必须延迟:click() 只是「开始下载」,浏览器此时还在解析 blob URL,
 * 同步释放会让部分实现(实测 Firefox)直接取消下载。1s 足够下载器接管句柄,
 * 又不会把页面生命周期里的 blob 长期攒着。
 */
export function downloadCsv(
  filename: string,
  headers: readonly string[],
  rows: readonly (readonly unknown[])[],
): void {
  const blob = new Blob([buildCsv(headers, rows)], { type: 'text/csv;charset=utf-8;' });
  const url = URL.createObjectURL(blob);
  const a = document.createElement('a');
  a.href = url;
  a.download = filename;
  a.click();
  window.setTimeout(() => URL.revokeObjectURL(url), DOWNLOAD_REVOKE_DELAY_MS);
}

/** blob URL 的释放延迟(见 downloadCsv 的注释) */
const DOWNLOAD_REVOKE_DELAY_MS = 1000;
