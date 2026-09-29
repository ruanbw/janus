-- 0009_rule_link_scope.sql — 规则表与规则-短链关联表
--
-- 为什么这么改:
--   后台「规则引擎」此前只是前端原型,规则只存在于租户级别,描述"访客画像 → 处置",
--   却没有任何东西把它和具体短链连起来——"仅对活动短链生效"这种最常见的诉求无法表达,
--   租户也无从知道"这条规则正在影响我的哪条短链"。本次让规则持有作用域(spec D1):
--   scope='global' 对租户全部短链生效,scope='links' 只对 rule_links 里显式列出的短链生效。
--   关联只存在一侧(规则侧是唯一写入口),短链表单里的勾选本质是改写某条 scoped 规则
--   与本短链的关联,两边看到的是同一份数据,不会漂移。
--
-- 关键不变式:
--   1. 租户隔离:rules.tenant_id 外键到 tenants,按 id 取/改/删规则必须带 tenant_id 条件;
--      UNIQUE (tenant_id, name) 保证规则名只在租户内唯一。
--   2. rule_links 两侧都 ON DELETE CASCADE:短链被物理删除时指向它的关联一并消失
--      (与既有 link_domains / visits 的处理一致);规则删除同理,不留孤儿关联行。
--   3. 零关联的 scoped 规则不兜底:D2 明确 scope='links' 且关联为空的规则永远不命中,
--      它不会退化成全局规则——静默兜底会把"我以为只影响活动短链"的规则打到全租户流量上。
--   4. 规则集合常驻内存按租户缓存(见 internal/rules 快照),单租户规则数上限 200
--      (Go 侧 MaxRulesPerTenant),防止快照无限膨胀;因此这里刻意不存任何"每次访问都要写"的
--      计数器列(如 hits_24h):那会给最高 QPS 的跳转链路加写放大,且计数器重启即丢、
--      与访问明细两套真相。"24h 命中"改为读时从 visits 按 rule_id 聚合。
--   5. 求值在跳转热路径上必须便宜(见 spec D7):本表所有列都是标量或 JSONB,
--      没有需要在请求期做联表查询的东西——快照一次性把整租户规则读进内存并整体原子替换。

CREATE TABLE rules (
    id          BIGSERIAL PRIMARY KEY,
    tenant_id   BIGINT NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    name        TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    priority    INT  NOT NULL DEFAULT 100,   -- 数字小者先评估
    scope       TEXT NOT NULL DEFAULT 'global' CHECK (scope IN ('global','links')),
    enabled     BOOLEAN NOT NULL DEFAULT true,
    logic       TEXT NOT NULL DEFAULT 'all' CHECK (logic IN ('all','any')),  -- 条件组之间的关系
    action      TEXT NOT NULL CHECK (action IN ('pass','redirect','notfound','throttle')),
    destination TEXT NOT NULL DEFAULT '',    -- action=redirect 时的改写目标
    conditions  JSONB NOT NULL DEFAULT '[]', -- [{field,operator,values:[]}]
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (tenant_id, name)
);
-- 覆盖索引:求值按 (租户, 优先级, id) 顺序扫,列表接口同序分页
CREATE INDEX idx_rules_tenant ON rules(tenant_id, priority, id);

CREATE TABLE rule_links (
    id      BIGSERIAL PRIMARY KEY,
    rule_id BIGINT NOT NULL REFERENCES rules(id) ON DELETE CASCADE,
    link_id BIGINT NOT NULL REFERENCES links(id)  ON DELETE CASCADE,
    UNIQUE (rule_id, link_id)
);
CREATE INDEX idx_rule_links_link ON rule_links(link_id);
