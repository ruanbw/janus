-- +goose Up
-- 0019_visit_stats_indexes.sql — 访问计数聚合的覆盖索引 + 删掉一个冗余索引
--
-- 为什么改:
--
-- 1. 计数查询缺 outcome 维度的索引。
--    访问计数是 `WHERE link_id = ? AND action IN ('redirect','landing_view')
--    AND outcome = 'success'`(CountVisitsByLink / fillLinksMeta 两处口径一致)。
--    而 0008 建的 idx_visits_link_action 是 (link_id, action, id DESC),
--    不含 outcome,所以 outcome='success' 这个过滤必须回堆逐行判断。
--    而这条 count(*) 在两个地方每次都会跑:短链列表页(每页 100 条,一次批量聚合)
--    与单链详情页;代价随保留期内明细总量线性增长。
--
-- 2. idx_visits_link 是重复维护。
--    0008 的注释写的是"替换 idx_visits_link 的低基数前缀",但同一条迁移里
--    并没有 DROP INDEX,idx_visits_link(link_id, created_at) 至今仍在,
--    每插一行 visits 就白维护一个重复前缀。
--    已确认全仓没有查询依赖它的 created_at 排序:
--      - 访问列表按 v.id DESC 排(见 ListVisitsByLink),不走 created_at;
--      - 保留期清理是 `WHERE created_at < ? ORDER BY id LIMIT ?`,
--        过滤走 idx_visits_created(created_at),排序靠主键,不需要 link_id 前缀;
--      - 规则页的 24h 命中走 idx_visits_rule(rule_id, created_at DESC)。
--    所以这里可以安全删除。
--
-- 计数聚合的**部分索引**:只为计入访问量的那两类行建索引。
-- 排除 click 与 failed 有三重收益:
--   - 索引只覆盖 redirect/landing_view 且 outcome='success' 的行,
--     体积比全表索引小得多(点击量大或扫描器多的租户尤其明显);
--   - click 与 failed 行(热路径上是占比不小的一部分)不必写这个索引,
--     INSERT 成本随之下降 —— 而 visits 是全站写入最热的表;
--   - count(*) 的全部过滤条件都落在索引里,可直接 Index Only Scan。
-- 这也是"总览聚合端点"(GET /api/visits/overview)能在一次 SQL 里出各维度
-- 分布的前提 —— 那几条 GROUP BY 走的都是这个索引。
CREATE INDEX idx_visits_link_success ON visits(link_id)
  WHERE action IN ('redirect','landing_view') AND outcome = 'success';

-- 访问列表在**不传 action** 时的排序支撑。
-- idx_visits_link_action 的第二列是 action:按 (link_id, id DESC) 排序时,
-- 同一 link_id 下的行按 action 分组存放,整体不满足 id 有序,
-- 所以 PostgreSQL 只能额外排一遍。加一条 (link_id, id DESC) 让默认
-- (不带 action 过滤)的那条列表查询也能走索引有序输出。
CREATE INDEX idx_visits_link_id_desc ON visits(link_id, id DESC);

-- 删除被 0008 取代却一直没删的低基数重复索引(理由见文件头)。
DROP INDEX IF EXISTS idx_visits_link;
