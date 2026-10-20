/**
 * 批量导入的逐行解析（纯逻辑，不碰网络）。
 *
 * 抽出来的理由：解析规则（分隔符、补协议、重复短码、行号）与"逐条 POST"是两件事，
 * 混在视图里时，任何一条规则都只能靠手工粘贴文本去验证。这里保持纯函数，
 * 视图只负责展示结果与发请求。
 */

/** 一条待导入条目；失败项带上能直接展示给用户的原因 */
export type BatchImportItem =
  | { ok: true; lineNo: number; code?: string; url: string }
  | { ok: false; lineNo: number; line: string; source: string; reason: string };

/** 短码在租户内不唯一（唯一性只在同一域名下），但同一批里重复必然自相冲突 */
export function parseBatchLines(text: string): BatchImportItem[] {
  const items: BatchImportItem[] = [];
  /** 短码 → 首次出现的行号，用于重复提示 */
  const seenCode = new Map<string, number>();

  const rawLines = text.split('\n');
  for (let i = 0; i < rawLines.length; i += 1) {
    const raw = rawLines[i].trim();
    if (raw === '') continue;
    const lineNo = i + 1;

    // 空格 / 逗号 / 制表符都当分隔符：从表格里复制出来的内容常是后两种
    const parts = raw.split(/[\s,]+/).filter(Boolean);
    if (parts.length === 0) continue;

    if (parts.length > 2) {
      items.push({
        ok: false,
        lineNo,
        line: raw,
        source: raw,
        reason: '每行最多两段（短码 目标URL），多余内容无法解析',
      });
      continue;
    }

    let code: string | undefined;
    let url: string;
    if (parts.length === 2) {
      code = parts[0];
      url = parts[1];
    } else {
      url = parts[0];
    }

    // 已带协议但不在白名单（javascript:/data:/file: …）：必须显式拒绝。
    // 放在补协议之前，否则会被拼成 "https://javascript:alert(1)" 而报成"格式非法"。
    const scheme = /^([a-z][a-z0-9+.-]*):/i.exec(url);
    if (scheme && !/^https?$/i.test(scheme[1])) {
      items.push({
        ok: false,
        lineNo,
        line: raw,
        source: url,
        reason: `仅支持 http/https 协议（收到 ${scheme[1]}:）`,
      });
      continue;
    }

    if (!/^https?:\/\//i.test(url)) {
      // 只有一段且不是 URL 时，用户很可能只写了短码：直接说清楚，
      // 而不是补成 "https://deal-a" 再报"URL 非法"（那条错误指向的是拼出来的值）
      if (code === undefined && !url.includes('.')) {
        items.push({
          ok: false,
          lineNo,
          line: raw,
          source: url,
          reason: '缺少目标 URL（只有短码时请写成「短码 目标URL」）',
        });
        continue;
      }
      url = 'https://' + url;
    }

    let parsed: URL;
    try {
      parsed = new URL(url);
    } catch {
      items.push({ ok: false, lineNo, line: raw, source: url, reason: 'URL 格式非法' });
      continue;
    }
    // 与后端 validTargetURL 同一白名单：javascript:/data:/file: 会被写进 Location 头
    if (parsed.protocol !== 'http:' && parsed.protocol !== 'https:') {
      items.push({
        ok: false,
        lineNo,
        line: raw,
        source: url,
        reason: '仅支持 http/https 协议',
      });
      continue;
    }

    if (code !== undefined) {
      const first = seenCode.get(code);
      if (first !== undefined) {
        items.push({
          ok: false,
          lineNo,
          line: raw,
          source: code,
          reason: `短码重复（与第 ${first} 行相同）`,
        });
        continue;
      }
      seenCode.set(code, lineNo);
      items.push({ ok: true, lineNo, code, url });
      continue;
    }

    items.push({ ok: true, lineNo, url });
  }

  return items;
}
