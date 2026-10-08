-- +goose NO TRANSACTION
-- +goose Up
-- 0020_visits_domain_index.sql — visits 补 domain_id 索引:
-- 域名删除(detachInTx)按 domain_id 清 visits,此前是全表扫描 + 长锁。
--
-- 版本号说明:原为 0002。5d8ec86 把旧 0001–0019 合并为 0001_init 后复用了 2/3 号,
-- 而合并前迁移过的库 goose_db_version 里已有 2/3 号(旧 0002_cleanup_indexes 等),
-- goose 会把本文件当作已应用而静默跳过。故改号到旧最大版本 19 之上。
-- 曾以 0002 号应用过本迁移的新库会以 20 号再执行一次,IF NOT EXISTS 保证幂等。
--
-- visits 是高频写入表,用 CONCURRENTLY 建索引避免持有阻塞 INSERT 的 SHARE 锁;
-- CONCURRENTLY 不能在事务内执行,所以本文件声明 NO TRANSACTION。
-- 注意:若并发建索引中途失败会残留 INVALID 索引,IF NOT EXISTS 会跳过它,
-- 需人工 DROP INDEX CONCURRENTLY idx_visits_domain 后重跑迁移。
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_visits_domain ON visits(domain_id);

-- +goose Down
DROP INDEX CONCURRENTLY IF EXISTS idx_visits_domain;
