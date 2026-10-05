/**
 * 失败原因 → 界面文案。
 *
 * 取值与后端 store.VisitReason* 一一对应(见 internal/store/visits.go):
 * 短链自身不可用(link_* / no_target / landing_missing)排在规则求值之前;
 * rule_blocked / rule_throttled 是规则裁决的拦截,括号里直接写明访客看到的
 * 状态码 —— 「规则拦截」四个字不回答"访客到底看到了什么"。
 *
 * 访问明细页(LinkVisitsView)另有一份带条件摘要的原因文案:那边能给规则类的
 * 失败补上命中的具体条件,这边是总览的聚合口径,只到原因一级。
 */
const FAILURE_REASON_LABELS: Record<string, string> = {
  rule_blocked: '规则拦截（404）',
  rule_throttled: '规则限流（429）',
  link_disabled: '短链已停用',
  link_deleted: '短链已删除',
  no_target: '无可用目标',
  landing_missing: '落地页文件缺失',
};

/** 失败原因的中文文案;认不出的取值原样显示,不吞也不编。 */
export function failureReasonLabel(reason: string): string {
  return FAILURE_REASON_LABELS[reason] ?? reason;
}
