-- +goose Up
-- 0030_domain_claims_recheck_failures.sql — 域名抢注防护与归属复检容错。
--
-- 编号说明:旧的 schema_migrations 历史里出现过被合并(squash)掉的版本号,
-- 而并行分支也在 0020 段新增迁移;这里刻意跳到 0030,避免与任何已用/在途版本号撞车
-- (goose 只认数字部分,撞号会让其中一个迁移被静默跳过)。

-- 1. 域名认领挑战 (Domain Claims)
--
-- domains.fqdn 是全局 UNIQUE:任何租户都能先插一条 pending 行占住别人的域名
-- (pending → expired → 手动复活,无限续期),真正的域名所有者永远加不进来。
-- 这里不放宽 UNIQUE(读路径 —— 跳转、授权端点、错误页 —— 全都依赖"一个 FQDN
-- 至多一行"),而是给"被占住但从未证明归属"的域名开一条认领通道:
-- 认领方拿到一枚自己的 TXT 挑战,发布到 _janus-verify.<fqdn> 并通过校验后,
-- 在事务内驱逐旧行、为认领方建行并直接激活。挑战 token 存在这张表里
-- (按 租户 + 域名 唯一),与 domains.verify_token 互不干扰。
CREATE TABLE IF NOT EXISTS domain_claims (
    id           BIGSERIAL PRIMARY KEY,
    tenant_id    BIGINT NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    fqdn         TEXT NOT NULL,
    verify_token TEXT NOT NULL,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (tenant_id, fqdn)
);
CREATE INDEX IF NOT EXISTS idx_domain_claims_created ON domain_claims(created_at);

-- 2. 归属复检连续失败计数
--
-- 原实现一次 24h 复检失败(含 DNS 抖动)就把成熟域名降级下线。
-- 现在要求连续 N 次失败才降级;任何一次成功或降级本身都会清零。
ALTER TABLE domains ADD COLUMN IF NOT EXISTS recheck_failures INT NOT NULL DEFAULT 0;

-- 3. 长期 expired 行的回收扫描(worker 按 dns_checked_at 选取)
CREATE INDEX IF NOT EXISTS idx_domains_expired_gc ON domains(COALESCE(dns_checked_at, created_at))
    WHERE status = 'expired' AND origin = 'self';

-- +goose Down
DROP INDEX IF EXISTS idx_domains_expired_gc;
ALTER TABLE domains DROP COLUMN IF EXISTS recheck_failures;
DROP TABLE IF EXISTS domain_claims;
