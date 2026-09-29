-- 0010_visit_rule.sql — 访问明细记下本次访问的规则裁决
--
-- 为什么这么改:
--   规则(spec「规则与短链关联」)已经在跳转链路上裁决,租户迟早会问"这条访问为什么
--   去了那个地址"。裁决结果此前只体现在响应码上,一旦过去就无法还原:
--   一条 404 明细看不出是短链没了还是被规则拦了,一条 302 也看不出目标是被规则改写的。
--   这里把命中了哪条规则、给出了什么处置落到每一行明细上。
--
-- 关键不变式:
--   1. rule_id 外键 ON DELETE SET NULL:规则删了,历史明细保留(rule_id 置空)。
--      不级联删除——访问明细是历史事实,不能因为配置清理而消失。
--   2. 校验方式:库内 CHECK + 应用层白名单校验(both,不是二选一)。
--      CHECK 保证绕过应用层(手写 SQL/数据修复脚本)也写不进非法值,应用层校验保证
--      写入前就能给出可读错误而不是 500。历史行为空串(无规则参与)。
--   3. 访问次数口径不变:仍是 action IN ('redirect','landing_view') AND outcome='success'。
--      被规则拦下的访问是 outcome='failed'(reason 为 rule_blocked / rule_throttled),
--      不灌水访问量;规则改写目标的访问仍是 success(确实重定向出去了)。
--   4. 热路径零写入:本次没有、以后也不在 rules 上加命中计数器。求值命中只落这一行明细,
--      "24h 命中"由规则列表接口按 rule_id 读时聚合(spec D9),不给最高 QPS 的跳转链路加写放大。

-- 上线注意:ADD COLUMN 带常量默认值在 PG 11+ 是仅改目录的操作(不重写表);
-- 唯一一次全表校验是外键的 VALIDATE,而历史行的 rule_id 全为 NULL,校验立即通过。
-- 整段 DDL 仍会短暂持有 ACCESS EXCLUSIVE 锁(与 0008 同量级),访问明细表很大的部署
-- 建议在低峰执行。
ALTER TABLE visits
  ADD COLUMN rule_id BIGINT REFERENCES rules(id) ON DELETE SET NULL,
  ADD COLUMN rule_action TEXT NOT NULL DEFAULT ''
    CHECK (rule_action IN ('', 'pass', 'redirect', 'notfound', 'throttle'));

-- 「24h 命中」的读时聚合(spec D9):按 rule_id 分组 + 24h 窗口。
-- 部分索引:只为"命中过规则的行"建索引,不命中规则的访问(绝大多数)不写这个索引,
-- 因此热路径的 INSERT 几乎不因它变慢,而后台规则页不必扫 90 天的全量明细。
CREATE INDEX idx_visits_rule ON visits(rule_id, created_at DESC) WHERE rule_id IS NOT NULL;
