-- +goose Up
-- 0001_init.sql — Janus 初始表结构(见 spec 决策 #5)

CREATE TABLE tiers (
    id          BIGSERIAL PRIMARY KEY,
    name        TEXT NOT NULL UNIQUE,
    max_links   INT  NOT NULL,
    max_domains INT  NOT NULL
);

CREATE TABLE tenants (
    id             BIGSERIAL PRIMARY KEY,
    email          TEXT NOT NULL UNIQUE,
    password_hash  TEXT,              -- 超管首次登录前为 NULL
    tier_id        BIGINT NOT NULL REFERENCES tiers(id),
    status         TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending','active','banned')),
    slug           TEXT NOT NULL UNIQUE,
    is_super_admin BOOLEAN NOT NULL DEFAULT false,
    verified_at    TIMESTAMPTZ,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE domains (
    id             BIGSERIAL PRIMARY KEY,
    tenant_id      BIGINT NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    fqdn           TEXT NOT NULL UNIQUE,
    origin         TEXT NOT NULL CHECK (origin IN ('self','platform')),
    status         TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending','active','failed','stopped')),
    cert_status    TEXT NOT NULL DEFAULT 'pending' CHECK (cert_status IN ('pending','issued','failed')),
    dns_checked_at TIMESTAMPTZ,
    cert_probed_at TIMESTAMPTZ,
    activated_at   TIMESTAMPTZ,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_domains_tenant ON domains(tenant_id);
CREATE INDEX idx_domains_status ON domains(status);

CREATE TABLE links (
    id              BIGSERIAL PRIMARY KEY,
    tenant_id       BIGINT NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    code            TEXT NOT NULL,
    target_url      TEXT NOT NULL,
    redirect_status INT NOT NULL DEFAULT 302 CHECK (redirect_status IN (301,302)),
    status          TEXT NOT NULL DEFAULT 'enabled' CHECK (status IN ('enabled','disabled')),
    deleted_at      TIMESTAMPTZ,      -- 逻辑删除标记
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_links_tenant ON links(tenant_id);

-- 关联层唯一约束 (domain_id, code):同一域名下短码不重复(见 CONTEXT 短码)
CREATE TABLE link_domains (
    id        BIGSERIAL PRIMARY KEY,
    link_id   BIGINT NOT NULL REFERENCES links(id) ON DELETE CASCADE,
    domain_id BIGINT NOT NULL REFERENCES domains(id) ON DELETE CASCADE,
    code      TEXT NOT NULL,
    UNIQUE (domain_id, code)
);
CREATE INDEX idx_link_domains_link ON link_domains(link_id);
CREATE INDEX idx_link_domains_domain ON link_domains(domain_id);

CREATE TABLE visits (
    id         BIGSERIAL PRIMARY KEY,
    link_id    BIGINT NOT NULL REFERENCES links(id) ON DELETE CASCADE,
    domain_id  BIGINT NOT NULL REFERENCES domains(id),
    user_agent TEXT NOT NULL DEFAULT '',
    referer    TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_visits_link ON visits(link_id, created_at);
CREATE INDEX idx_visits_created ON visits(created_at);

CREATE TABLE sessions (
    id         BIGSERIAL PRIMARY KEY,
    tenant_id  BIGINT NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    token_hash TEXT NOT NULL UNIQUE,     -- SHA-256(token),明文仅存于 cookie
    csrf_token TEXT NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_sessions_tenant ON sessions(tenant_id);

CREATE TABLE email_tokens (
    id         BIGSERIAL PRIMARY KEY,
    tenant_id  BIGINT NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    token_hash TEXT NOT NULL UNIQUE,
    kind       TEXT NOT NULL CHECK (kind IN ('verify','reset')),
    expires_at TIMESTAMPTZ NOT NULL,
    used_at    TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- 种子数据:免费档(短链 100 / 域名 10);新租户默认免费档
INSERT INTO tiers (name, max_links, max_domains) VALUES ('free', 100, 10)
ON CONFLICT (name) DO NOTHING;
