-- Janus 数据库初始化脚本(schema 唯一事实来源,见 AGENTS.md「数据库与 Schema 约定」)
--
-- 契约:
--   1. 整段可重复执行 —— 启动、重跑、并发多副本都必须安全,不破坏已有数据;
--   2. 不写增量迁移 —— 改表直接改本文件,改完 DROP 开发库重建即可;
--   3. 语句幂等 —— 建表/索引用 IF NOT EXISTS,函数用 OR REPLACE,触发器先 DROP;
--   4. 整段在一个事务里执行 —— 任何一句失败都不会留下半截 schema。

-- 1. 等级表 (Tiers)
-- 配额列的非负约束在表定义之后以命名约束补上(见紧跟的 DO 块),此处不再内联,
-- 避免同一列的 CHECK 出现两份(内联那份是自动命名,DO 块就判不出来)。
CREATE TABLE IF NOT EXISTS tiers (
    id          BIGSERIAL PRIMARY KEY,
    name        TEXT NOT NULL UNIQUE,
    max_links   INT  NOT NULL,
    max_domains INT  NOT NULL
);

-- 配额判定形如 `已用 < max_links`,负值会让任何租户恒超限(或者按实现绕开限制)。
-- CREATE TABLE IF NOT EXISTS 不会给已存在的表补约束,ADD CONSTRAINT 又没有
-- IF NOT EXISTS,故用 pg_constraint 判定后幂等添加。
-- 用 NOT VALID:只保证"此后写入不得为负",不因存量库里的历史脏值(如某档位被手工
-- 写成 -1)让整个初始化脚本失败、进程起不来 —— 启动路径不该被一条 Low 级加固拖垮。
DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'tiers_max_links_nonneg') THEN
        ALTER TABLE tiers ADD CONSTRAINT tiers_max_links_nonneg CHECK (max_links >= 0) NOT VALID;
    END IF;
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'tiers_max_domains_nonneg') THEN
        ALTER TABLE tiers ADD CONSTRAINT tiers_max_domains_nonneg CHECK (max_domains >= 0) NOT VALID;
    END IF;
END $$;

-- 2. 租户表 (Tenants)
CREATE TABLE IF NOT EXISTS tenants (
    id                   BIGSERIAL PRIMARY KEY,
    username             TEXT NOT NULL UNIQUE,
    password_hash        TEXT,
    tier_id              BIGINT NOT NULL REFERENCES tiers(id),
    status               TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending','active','banned')),
    slug                 TEXT NOT NULL UNIQUE,
    is_super_admin       BOOLEAN NOT NULL DEFAULT false,
    must_change_password BOOLEAN NOT NULL DEFAULT false,
    token_version        INT NOT NULL DEFAULT 1,
    custom_404_html      TEXT NOT NULL DEFAULT '',
    custom_429_html      TEXT NOT NULL DEFAULT '',
    created_at           TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- 3. 域名表 (Domains)
CREATE TABLE IF NOT EXISTS domains (
    id                      BIGSERIAL PRIMARY KEY,
    tenant_id               BIGINT NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    fqdn                    TEXT NOT NULL UNIQUE,
    description             TEXT NOT NULL DEFAULT '',
    origin                  TEXT NOT NULL CHECK (origin IN ('self','platform')),
    status                  TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending','active','failed','stopped','expired')),
    cert_status             TEXT NOT NULL DEFAULT 'pending' CHECK (cert_status IN ('pending','issued','failed')),
    verify_token            TEXT NOT NULL DEFAULT '',
    verify_token_created_at TIMESTAMPTZ,
    ownership_verified_at   TIMESTAMPTZ,
    dns_checked_at          TIMESTAMPTZ,
    cert_probed_at          TIMESTAMPTZ,
    activated_at            TIMESTAMPTZ,
    created_at              TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_domains_tenant ON domains(tenant_id);
CREATE INDEX IF NOT EXISTS idx_domains_status ON domains(status);
CREATE INDEX IF NOT EXISTS idx_domains_dns_retry ON domains(origin, COALESCE(dns_checked_at, created_at))
    WHERE status IN ('pending','failed');
CREATE INDEX IF NOT EXISTS idx_domains_cert_probe ON domains(COALESCE(cert_probed_at, created_at))
    WHERE status = 'active' AND cert_status IN ('pending','failed');

-- 4. 短链表 (Links)
CREATE TABLE IF NOT EXISTS links (
    id              BIGSERIAL PRIMARY KEY,
    tenant_id       BIGINT NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    code            TEXT NOT NULL,
    redirect_status INT NOT NULL DEFAULT 302 CHECK (redirect_status IN (301,302)),
    status          TEXT NOT NULL DEFAULT 'enabled' CHECK (status IN ('enabled','disabled')),
    link_type       TEXT NOT NULL DEFAULT 'redirect' CHECK (link_type IN ('redirect','landing')),
    landing_source  TEXT NOT NULL DEFAULT 'url' CHECK (landing_source IN ('url','upload')),
    landing_url     TEXT NOT NULL DEFAULT '',
    clicks          BIGINT NOT NULL DEFAULT 0,
    rr_index        BIGINT NOT NULL DEFAULT 0,
    rules_enabled   BOOLEAN NOT NULL DEFAULT true,
    metadata        JSONB NOT NULL DEFAULT '{}'::jsonb,  -- 外部工程业务自定义元数据(ADR-0014)
    deleted_at      TIMESTAMPTZ,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_links_tenant ON links(tenant_id);
-- 总览聚合(OverviewStatsForTenant 的 totals/previous/topLinks/facets)共用作用域:
--   FROM visits v JOIN links l ON l.id = v.link_id
--   WHERE l.tenant_id = ? AND l.deleted_at IS NULL AND v.created_at >= ?
-- 这条索引按租户取出"在册"短链集合(选择度最高的过滤条件在这里),
-- 与下面 visits(link_id, created_at) 配对成嵌套循环路径。
CREATE INDEX IF NOT EXISTS idx_links_tenant_live ON links(tenant_id, deleted_at);

-- 5. 短链多目标表 (Link Targets)
CREATE TABLE IF NOT EXISTS link_targets (
    id       BIGSERIAL PRIMARY KEY,
    link_id  BIGINT NOT NULL REFERENCES links(id) ON DELETE CASCADE,
    url      TEXT NOT NULL,
    position INT NOT NULL DEFAULT 0,
    UNIQUE (link_id, position)
);
CREATE INDEX IF NOT EXISTS idx_link_targets_link ON link_targets(link_id);

-- 6. 短链-域名关联表 (Link Domains)
-- 唯一性约束采用部分唯一索引：只对未删除短链生效 (WHERE NOT link_deleted)
--
-- 不变式(库内无触发器/外键可保证,靠写入侧契约 + 本条注释钉住):
--   link_domains.code 必须恒等于 links.code —— 它是 links.code 的冗余副本,
--   存在的唯一理由是让"同一域名下未删除短链的短码唯一"这个约束落在
--   link_domains 上(部分唯一索引 link_domains_live_code_uniq 只能建在这里)。
--   写入点只有两处且都在事务内(store/links.go 的 CreateLink 与还原短码路径),
--   两者都用该短链自己的 code;短码一经创建不可修改(spec),因此不会漂移。
--   新增任何写 link_domains 的路径都必须遵守这条;一旦两者不一致,热路径
--   lookupLink 会按 ld.code 命中失败(表现为短链 404),而索引仍以为未占用。
CREATE TABLE IF NOT EXISTS link_domains (
    id           BIGSERIAL PRIMARY KEY,
    link_id      BIGINT NOT NULL REFERENCES links(id) ON DELETE CASCADE,
    domain_id    BIGINT NOT NULL REFERENCES domains(id) ON DELETE CASCADE,
    code         TEXT NOT NULL,
    link_deleted BOOLEAN NOT NULL DEFAULT false
);
CREATE INDEX IF NOT EXISTS idx_link_domains_link ON link_domains(link_id);
CREATE INDEX IF NOT EXISTS idx_link_domains_domain ON link_domains(domain_id);
-- 热路径 lookupLink(短链跳转)按 (domain_id, code) 命中。部分唯一索引
-- link_domains_live_code_uniq 带 WHERE NOT link_deleted,而宽松口径必须同时看到
-- 未删除与历史软删两类行,planner 推不出可以只看未删除行,用不上它 ——
-- 缺这条非部分索引就是每次访问按 domain_id 全量扫短码。
CREATE INDEX IF NOT EXISTS idx_link_domains_domain_code ON link_domains(domain_id, code);
CREATE UNIQUE INDEX IF NOT EXISTS link_domains_live_code_uniq ON link_domains(domain_id, code) WHERE NOT link_deleted;

CREATE OR REPLACE FUNCTION link_domains_sync_link_deleted() RETURNS trigger AS $$
BEGIN
    UPDATE link_domains
       SET link_deleted = (NEW.deleted_at IS NOT NULL)
     WHERE link_id = NEW.id;
    RETURN NULL;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS trg_links_sync_link_deleted ON links;
CREATE TRIGGER trg_links_sync_link_deleted
    AFTER UPDATE OF deleted_at ON links
    FOR EACH ROW
    WHEN (OLD.deleted_at IS DISTINCT FROM NEW.deleted_at)
    EXECUTE FUNCTION link_domains_sync_link_deleted();

CREATE OR REPLACE FUNCTION link_domains_derive_link_deleted() RETURNS trigger AS $$
BEGIN
    SELECT (l.deleted_at IS NOT NULL) INTO NEW.link_deleted
      FROM links l WHERE l.id = NEW.link_id;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS trg_link_domains_derive_link_deleted ON link_domains;
CREATE TRIGGER trg_link_domains_derive_link_deleted
    BEFORE INSERT ON link_domains
    FOR EACH ROW
    EXECUTE FUNCTION link_domains_derive_link_deleted();

-- 7. 规则表 (Rules)
CREATE TABLE IF NOT EXISTS rules (
    id          BIGSERIAL PRIMARY KEY,
    tenant_id   BIGINT NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    name        TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    priority    INT  NOT NULL DEFAULT 100,
    scope       TEXT NOT NULL DEFAULT 'global' CHECK (scope IN ('global','links')),
    enabled     BOOLEAN NOT NULL DEFAULT true,
    logic       TEXT NOT NULL DEFAULT 'all' CHECK (logic IN ('all','any')),
    action      TEXT NOT NULL CHECK (action IN ('pass','redirect','notfound','throttle')),
    destination TEXT NOT NULL DEFAULT '',
    conditions  JSONB NOT NULL DEFAULT '[]',
    page_mode   VARCHAR(16) NOT NULL DEFAULT 'default' CHECK (page_mode IN ('default', 'custom')),
    custom_html TEXT NOT NULL DEFAULT '',
    rule_type   VARCHAR(20) NOT NULL DEFAULT 'visual',
    expression  TEXT NOT NULL DEFAULT '',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (tenant_id, name)
);
CREATE INDEX IF NOT EXISTS idx_rules_tenant ON rules(tenant_id, priority, id);

-- 8. 规则-短链关联表 (Rule Links)
CREATE TABLE IF NOT EXISTS rule_links (
    id      BIGSERIAL PRIMARY KEY,
    rule_id BIGINT NOT NULL REFERENCES rules(id) ON DELETE CASCADE,
    link_id BIGINT NOT NULL REFERENCES links(id) ON DELETE CASCADE,
    UNIQUE (rule_id, link_id)
);
CREATE INDEX IF NOT EXISTS idx_rule_links_link ON rule_links(link_id);

-- 9. 访问明细表 (Visits)
CREATE TABLE IF NOT EXISTS visits (
    id            BIGSERIAL PRIMARY KEY,
    link_id       BIGINT NOT NULL REFERENCES links(id) ON DELETE CASCADE,
    -- domain_id 与 link_id 同为"这条明细属于谁":必须是 CASCADE,否则删域名
    -- 要么抛 23503,要么依赖调用方记得先手工清 visits(两条外键语义自相矛盾)。
    domain_id     BIGINT NOT NULL REFERENCES domains(id) ON DELETE CASCADE,
    ip            TEXT NOT NULL DEFAULT '',
    user_agent    TEXT NOT NULL DEFAULT '',
    referer       TEXT NOT NULL DEFAULT '',
    action        TEXT NOT NULL DEFAULT 'redirect' CHECK (action IN ('redirect','landing_view','click')),
    outcome       TEXT NOT NULL DEFAULT 'success' CHECK (outcome IN ('success','failed')),
    reason        TEXT NOT NULL DEFAULT '',
    target_url    TEXT NOT NULL DEFAULT '',
    lang          TEXT NOT NULL DEFAULT '',
    country       TEXT NOT NULL DEFAULT '',
    is_datacenter BOOLEAN NOT NULL DEFAULT false,
    asn           TEXT NOT NULL DEFAULT '',
    rule_id       BIGINT REFERENCES rules(id) ON DELETE SET NULL,
    rule_action   TEXT NOT NULL DEFAULT '' CHECK (rule_action IN ('', 'pass', 'redirect', 'notfound', 'throttle')),
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_visits_created ON visits(created_at);
-- 总览聚合的 visits 侧:按 (link_id, created_at) 取"某条短链在窗口内的访问行"。
-- 列序刻意是 (link_id, created_at) 而不是 (created_at, link_id):所有总览查询都已经
-- 在手上有"本租户在册短链"这个集合(见 idx_links_tenant_live),需要的是逐条短链
-- 在时间窗口内的范围扫描;反过来按 created_at 打头会退化成扫全站窗口再回表判租户。
CREATE INDEX IF NOT EXISTS idx_visits_link_created ON visits(link_id, created_at);
CREATE INDEX IF NOT EXISTS idx_visits_link_action ON visits(link_id, action, id DESC);
CREATE INDEX IF NOT EXISTS idx_visits_link_id_desc ON visits(link_id, id DESC);
CREATE INDEX IF NOT EXISTS idx_visits_link_success ON visits(link_id)
    WHERE action IN ('redirect','landing_view') AND outcome = 'success';
CREATE INDEX IF NOT EXISTS idx_visits_rule ON visits(rule_id, created_at DESC)
    WHERE rule_id IS NOT NULL;
-- 域名删除(detachInTx)按 domain_id 清 visits,无此索引是全表扫描 + 长锁。
CREATE INDEX IF NOT EXISTS idx_visits_domain ON visits(domain_id);

-- 存量库的 visits_domain_id_fkey 是裸 REFERENCES(无 ON DELETE 语义),而 CREATE TABLE
-- IF NOT EXISTS 不会改写已存在的约束 —— 这里只在语义确实不对时重建它。
-- 用 confdeltype 判定(不重建已是 CASCADE 的约束):重建外键会全表校验,
-- 放在每次启动都执行的脚本里,无条件 DROP/ADD 会让大表每次启动都白扫一遍。
DO $$
BEGIN
    IF EXISTS (
        SELECT 1 FROM pg_constraint
         WHERE conname = 'visits_domain_id_fkey' AND confdeltype <> 'c'
    ) THEN
        ALTER TABLE visits DROP CONSTRAINT visits_domain_id_fkey;
        ALTER TABLE visits ADD CONSTRAINT visits_domain_id_fkey
            FOREIGN KEY (domain_id) REFERENCES domains(id) ON DELETE CASCADE;
    END IF;
END $$;

-- 10. 会话表 (Sessions)
CREATE TABLE IF NOT EXISTS sessions (
    id         BIGSERIAL PRIMARY KEY,
    tenant_id  BIGINT NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    token_hash TEXT NOT NULL UNIQUE,     -- SHA-256(token),明文仅存于 cookie
    csrf_token TEXT NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_sessions_tenant ON sessions(tenant_id);
CREATE INDEX IF NOT EXISTS idx_sessions_expires ON sessions(expires_at);

-- 11. (已删除)邮箱验证/重置/设置 Token 表 email_tokens
-- 身份模型从「邮箱 + 邮件验证」改为「username + 密码」后,注册即 active,
-- 验证/重置/首登 setup 三种 token 全部下线,表与索引一并从本文件移除。

-- 初始种子数据
INSERT INTO tiers (name, max_links, max_domains) VALUES ('free', 100, 10)
ON CONFLICT (name) DO NOTHING;
