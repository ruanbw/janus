-- +goose Up
-- 0021_links_metadata.sql — links 表增加 metadata JSONB 字段 (ADR-0014):
-- 允许外部工程/高级版挂载业务自定义元数据（如 Cloak 阈值、防审探针配置、隐形反代目标等）。
--
-- 版本号说明:原为 0003,与合并前旧库已记录的 3 号版本冲突会被 goose 静默跳过,
-- 故改号为 21(见 0020_visits_domain_index.sql)。ADD COLUMN IF NOT EXISTS 保证重复执行幂等。
ALTER TABLE links ADD COLUMN IF NOT EXISTS metadata JSONB NOT NULL DEFAULT '{}'::jsonb;

-- +goose Down
ALTER TABLE links DROP COLUMN IF EXISTS metadata;
