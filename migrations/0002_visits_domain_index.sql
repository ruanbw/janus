-- +goose Up
-- 0002_visits_domain_index.sql — visits 补 domain_id 索引:
-- 域名删除(detachInTx)按 domain_id 清 visits,此前是全表扫描 + 长锁。
CREATE INDEX IF NOT EXISTS idx_visits_domain ON visits(domain_id);

-- +goose Down
DROP INDEX IF EXISTS idx_visits_domain;
