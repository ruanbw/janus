-- +goose Up
-- 0001_init.sql — Janus 初始数据库表结构与索引

-- 1. 等级表 (Tiers)
CREATE TABLE tiers (
    id          BIGSERIAL PRIMARY KEY,
    name        TEXT NOT NULL UNIQUE,
    max_links   INT  NOT NULL,
    max_domains INT  NOT NULL
);

-- 2. 租户表 (Tenants)
CREATE TABLE tenants (
    id              BIGSERIAL PRIMARY KEY,
    email           TEXT NOT NULL UNIQUE,
    password_hash   TEXT,              -- 超管首次登录前为 NULL
    tier_id         BIGINT NOT NULL REFERENCES tiers(id),
    status          TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending','active','banned')),
    slug            TEXT NOT NULL UNIQUE,
    is_super_admin  BOOLEAN NOT NULL DEFAULT false,
    token_version   INT NOT NULL DEFAULT 1,
    custom_404_html TEXT NOT NULL DEFAULT '',
    custom_429_html TEXT NOT NULL DEFAULT '',
    verified_at     TIMESTAMPTZ,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- 3. 域名表 (Domains)
CREATE TABLE domains (
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
CREATE INDEX idx_domains_tenant ON domains(tenant_id);
CREATE INDEX idx_domains_status ON domains(status);
CREATE INDEX idx_domains_dns_retry ON domains(origin, COALESCE(dns_checked_at, created_at))
    WHERE status IN ('pending','failed');
CREATE INDEX idx_domains_cert_probe ON domains(COALESCE(cert_probed_at, created_at))
    WHERE status = 'active' AND cert_status IN ('pending','failed');

-- 4. 短链表 (Links)
CREATE TABLE links (
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
    deleted_at      TIMESTAMPTZ,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_links_tenant ON links(tenant_id);

-- 5. 短链多目标表 (Link Targets)
CREATE TABLE link_targets (
    id       BIGSERIAL PRIMARY KEY,
    link_id  BIGINT NOT NULL REFERENCES links(id) ON DELETE CASCADE,
    url      TEXT NOT NULL,
    position INT NOT NULL DEFAULT 0,
    UNIQUE (link_id, position)
);
CREATE INDEX idx_link_targets_link ON link_targets(link_id);

-- 6. 短链-域名关联表 (Link Domains)
-- 唯一性约束采用部分唯一索引：只对未删除短链生效 (WHERE NOT link_deleted)
CREATE TABLE link_domains (
    id           BIGSERIAL PRIMARY KEY,
    link_id      BIGINT NOT NULL REFERENCES links(id) ON DELETE CASCADE,
    domain_id    BIGINT NOT NULL REFERENCES domains(id) ON DELETE CASCADE,
    code         TEXT NOT NULL,
    link_deleted BOOLEAN NOT NULL DEFAULT false
);
CREATE INDEX idx_link_domains_link ON link_domains(link_id);
CREATE INDEX idx_link_domains_domain ON link_domains(domain_id);
CREATE UNIQUE INDEX link_domains_live_code_uniq ON link_domains(domain_id, code) WHERE NOT link_deleted;

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION link_domains_sync_link_deleted() RETURNS trigger AS $$
BEGIN
    UPDATE link_domains
       SET link_deleted = (NEW.deleted_at IS NOT NULL)
     WHERE link_id = NEW.id;
    RETURN NULL;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

CREATE TRIGGER trg_links_sync_link_deleted
    AFTER UPDATE OF deleted_at ON links
    FOR EACH ROW
    WHEN (OLD.deleted_at IS DISTINCT FROM NEW.deleted_at)
    EXECUTE FUNCTION link_domains_sync_link_deleted();

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION link_domains_derive_link_deleted() RETURNS trigger AS $$
BEGIN
    SELECT (l.deleted_at IS NOT NULL) INTO NEW.link_deleted
      FROM links l WHERE l.id = NEW.link_id;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

CREATE TRIGGER trg_link_domains_derive_link_deleted
    BEFORE INSERT ON link_domains
    FOR EACH ROW
    EXECUTE FUNCTION link_domains_derive_link_deleted();

-- 7. 规则表 (Rules)
CREATE TABLE rules (
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
CREATE INDEX idx_rules_tenant ON rules(tenant_id, priority, id);

-- 8. 规则-短链关联表 (Rule Links)
CREATE TABLE rule_links (
    id      BIGSERIAL PRIMARY KEY,
    rule_id BIGINT NOT NULL REFERENCES rules(id) ON DELETE CASCADE,
    link_id BIGINT NOT NULL REFERENCES links(id) ON DELETE CASCADE,
    UNIQUE (rule_id, link_id)
);
CREATE INDEX idx_rule_links_link ON rule_links(link_id);

-- 9. 访问明细表 (Visits)
CREATE TABLE visits (
    id            BIGSERIAL PRIMARY KEY,
    link_id       BIGINT NOT NULL REFERENCES links(id) ON DELETE CASCADE,
    domain_id     BIGINT NOT NULL REFERENCES domains(id),
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
CREATE INDEX idx_visits_created ON visits(created_at);
CREATE INDEX idx_visits_link_action ON visits(link_id, action, id DESC);
CREATE INDEX idx_visits_link_id_desc ON visits(link_id, id DESC);
CREATE INDEX idx_visits_link_success ON visits(link_id)
    WHERE action IN ('redirect','landing_view') AND outcome = 'success';
CREATE INDEX idx_visits_rule ON visits(rule_id, created_at DESC)
    WHERE rule_id IS NOT NULL;

-- 10. 会话表 (Sessions)
CREATE TABLE sessions (
    id         BIGSERIAL PRIMARY KEY,
    tenant_id  BIGINT NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    token_hash TEXT NOT NULL UNIQUE,     -- SHA-256(token),明文仅存于 cookie
    csrf_token TEXT NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_sessions_tenant ON sessions(tenant_id);
CREATE INDEX idx_sessions_expires ON sessions(expires_at);

-- 11. 邮箱验证/重置/设置 Token 表 (Email Tokens)
CREATE TABLE email_tokens (
    id         BIGSERIAL PRIMARY KEY,
    tenant_id  BIGINT NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    token_hash TEXT NOT NULL UNIQUE,
    kind       TEXT NOT NULL CHECK (kind IN ('verify','reset','setup')),
    expires_at TIMESTAMPTZ NOT NULL,
    used_at    TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_email_tokens_expires ON email_tokens(expires_at);

-- 初始种子数据
INSERT INTO tiers (name, max_links, max_domains) VALUES ('free', 100, 10)
ON CONFLICT (name) DO NOTHING;
