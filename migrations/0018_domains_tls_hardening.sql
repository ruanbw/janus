-- +goose Up
-- 0018_domains_tls_hardening.sql — 域名归属证明(TXT 挑战)与后台任务可调度性
--
-- 1. domains 增加 TXT 挑战 token 与归属证明时间(激活不再只是"A 记录指向本机"的快照)
-- 2. domains.status 增加终态 expired:超过最长等待仍未完成归属证明的域名退出重试扫描
-- 3. 归一化历史 fqdn(小写 + 去掉根标签尾点),否则 a.com 与 a.com. 在 UNIQUE 索引下
--    是两行,同一个真实主机能被两个租户各绑一份、各计一份配额
-- 4. 为后台扫描补索引:dns_checked_at / cert_probed_at 从"写了没人读"变成调度依据

-- ---------- 1 & 2. 列与状态机 ----------

ALTER TABLE domains
  ADD COLUMN verify_token TEXT NOT NULL DEFAULT '',
  ADD COLUMN verify_token_created_at TIMESTAMPTZ,
  ADD COLUMN ownership_verified_at TIMESTAMPTZ;

COMMENT ON COLUMN domains.verify_token IS
  '当前归属挑战 token:提交者须在 _cloak-verify.<fqdn> 发布它,校验通过即被消费(不可重放);保留值供 active 域名低频复检比对';
COMMENT ON COLUMN domains.ownership_verified_at IS
  '最近一次通过 TXT 挑战证明域名归属的时间;NULL = 从未按新规则验证(存量 active 域名,不做降级)';
COMMENT ON COLUMN domains.verify_token_created_at IS
  '挑战签发时间;超过 CLOAK_DNS_MAX_AGE 仍未验证通过 -> status=expired(终态,退出重试扫描)';

-- expired 是终态:pending/failed 会被扫描无限重试,超期后必须离开扫描集合,
-- 否则每 5 分钟无条件写一次库(写放大 + 无终态)。租户可显式 recheck 复活。
ALTER TABLE domains DROP CONSTRAINT domains_status_check;
ALTER TABLE domains ADD CONSTRAINT domains_status_check
  CHECK (status IN ('pending','active','failed','stopped','expired'));

-- ---------- 3. 归一化历史数据 ----------

-- 归一化会把两组行折叠成同一 fqdn,必须先消重,否则撞 UNIQUE 索引导致迁移失败。
-- 同一主机被绑定多次时保留 id 最小的那条(最早建立的关联得以保留),删掉其余:
--   visits 先删(visits.domain_id 无 ON DELETE,会挡住域名删除);
--   link_domains 由 domains 的 ON DELETE CASCADE 连带清除,短链本身保留。
-- +goose StatementBegin
DO $$
DECLARE
  dupes BIGINT;
BEGIN
  SELECT count(*) INTO dupes FROM (
    SELECT id, row_number() OVER (
      PARTITION BY lower(rtrim(fqdn, '.')) ORDER BY id) AS rn
      FROM domains
  ) d WHERE d.rn > 1;
  IF dupes > 0 THEN
    RAISE WARNING
      '0018: 归一化 fqdn 前发现 % 条重复记录(同一主机被绑定多次,通常源于尾点未剥离),已保留 id 最小的记录并删除其余',
      dupes;
  END IF;
END $$;
-- +goose StatementEnd

DELETE FROM visits WHERE domain_id IN (
  SELECT id FROM (
    SELECT id, row_number() OVER (
      PARTITION BY lower(rtrim(fqdn, '.')) ORDER BY id) AS rn
      FROM domains
  ) d WHERE d.rn > 1);

DELETE FROM domains WHERE id IN (
  SELECT id FROM (
    SELECT id, row_number() OVER (
      PARTITION BY lower(rtrim(fqdn, '.')) ORDER BY id) AS rn
      FROM domains
  ) d WHERE d.rn > 1);

UPDATE domains SET fqdn = lower(rtrim(fqdn, '.'));

-- ---------- 4. 后台扫描索引 ----------

-- 重试队列:origin='self' 且 status IN (pending,failed),按"上次校验时间 + 退避"取到期行。
CREATE INDEX idx_domains_dns_retry
  ON domains(origin, COALESCE(dns_checked_at, created_at))
  WHERE status IN ('pending','failed');

-- 证书探活:active 且证书未签发,按上次探活时间排序并带 LIMIT(每轮有界)。
CREATE INDEX idx_domains_cert_probe
  ON domains(COALESCE(cert_probed_at, created_at))
  WHERE status = 'active' AND cert_status IN ('pending','failed');

-- +goose Down
-- 注意:fqdn 归一化与重复记录清除不可逆,Down 不恢复数据,只回退结构。
DELETE FROM domains WHERE status = 'expired';

DROP INDEX idx_domains_cert_probe;
DROP INDEX idx_domains_dns_retry;

ALTER TABLE domains DROP CONSTRAINT domains_status_check;
ALTER TABLE domains ADD CONSTRAINT domains_status_check
  CHECK (status IN ('pending','active','failed','stopped'));

ALTER TABLE domains
  DROP COLUMN ownership_verified_at,
  DROP COLUMN verify_token_created_at,
  DROP COLUMN verify_token;
