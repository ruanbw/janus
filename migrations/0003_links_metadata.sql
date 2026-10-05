-- +goose Up
-- 0003_links_metadata.sql — links 表增加 metadata JSONB 字段 (ADR-0014):
-- 允许外部工程/高级版挂载业务自定义元数据（如 Cloak 阈值、防审探针配置、隐形反代目标等）。
ALTER TABLE links ADD COLUMN IF NOT EXISTS metadata JSONB NOT NULL DEFAULT '{}'::jsonb;

-- +goose Down
ALTER TABLE links DROP COLUMN IF EXISTS metadata;
