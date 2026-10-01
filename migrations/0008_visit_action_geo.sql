-- +goose Up
-- 0008_visit_action_geo.sql — 访问明细补动作/结果/目标与地理占位字段
-- 一次"访问"此前只能表示"重定向成功",无法回答"这次到底触发了什么动作、成没成功、
-- 为什么失败、最终到了哪里";落地页点击此前只是 links.clicks 上的裸计数器,没有明细。
-- 这里把动作与结果显式落到 visits 行上,点击也在此落行(action='click')。
-- 关键不变式:link.visits 的口径 = action IN ('redirect','landing_view') 且 outcome='success' 的行数,
-- 点击行与失败行都不得灌水访问量(CountVisitsByLink 与 fillLinksMeta 两处聚合同时过滤)。

-- 本次触发的动作:redirect(跳转型一次访问)/ landing_view(落地页型一次访问)/ click(落地页按钮回传)
ALTER TABLE visits
  ADD COLUMN action     TEXT NOT NULL DEFAULT 'redirect' CHECK (action IN ('redirect','landing_view','click')),
  ADD COLUMN outcome    TEXT NOT NULL DEFAULT 'success' CHECK (outcome IN ('success','failed')),
  ADD COLUMN reason     TEXT NOT NULL DEFAULT '',   -- 仅 outcome='failed' 时非空
  ADD COLUMN target_url TEXT NOT NULL DEFAULT '',   -- 本次动作最终抵达的地址
  ADD COLUMN lang       TEXT NOT NULL DEFAULT '';   -- Accept-Language 首标签

-- 地理占位:country 已由后端内嵌的离线 GeoIP 库(ip2region)填充(查不到时为空);
-- asn / is_datacenter 仍无数据源,恒为空/假 —— 要「拦截机房 IP」得另开一个决策
ALTER TABLE visits
  ADD COLUMN country      TEXT NOT NULL DEFAULT '',
  ADD COLUMN is_datacenter BOOLEAN NOT NULL DEFAULT FALSE,
  ADD COLUMN asn          TEXT NOT NULL DEFAULT '';

-- 历史行回填(必须在加列之后执行):迁移前所有行都是"访问",
-- 落地页型短链的旧访问语义上是落地页视图,归为 landing_view
UPDATE visits v SET action = 'landing_view'
  FROM links l
 WHERE l.id = v.link_id AND l.link_type = 'landing' AND v.action = 'redirect';

-- 访问列表按 (link_id, action) 倒序分页筛选的覆盖索引(替换 idx_visits_link 的低基数前缀)
CREATE INDEX idx_visits_link_action ON visits(link_id, action, id DESC);
