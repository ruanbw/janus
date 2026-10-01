-- +goose Up
-- 0015_tenant_token_version_setup_token.sql — JWT 可吊销 + 邮箱 token 新增 setup 用途

-- 1) tenants.token_version:JWT 吊销开关。
--    之前已签发的 Bearer JWT 在 TTL(默认 24h)内无法作废:改密/重置密码只删
--    sessions 表,而 authenticate 走 JWT 分支时根本不查 sessions,jwt.go 生成的
--    jti 也从未被服务端读过。把当前版本号写进 JWT 载荷,认证时比对,不一致即 401。
--    存量行取默认值 1;本迁移上线前签发的 JWT 没有该声明(解析为 0)→ 全部失效,
--    这正是想要的行为(部署一次即可作废所有历史 token)。
ALTER TABLE tenants
    ADD COLUMN token_version INT NOT NULL DEFAULT 1;

-- 2) email_tokens.kind 放开 'setup':超管首次设置密码的一次性 token
--    (原来只有 'verify'/'reset',超管登录完全跳过 bcrypt 比对 = 认证旁路)。
ALTER TABLE email_tokens
    DROP CONSTRAINT IF EXISTS email_tokens_kind_check;

ALTER TABLE email_tokens
    ADD CONSTRAINT email_tokens_kind_check CHECK (kind IN ('verify','reset','setup'));
