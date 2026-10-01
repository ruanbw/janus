-- +goose Up
-- 0006_multi_targets.sql — 短链多目标 URL(默认轮询)
CREATE TABLE link_targets (
    id       BIGSERIAL PRIMARY KEY,
    link_id  BIGINT NOT NULL REFERENCES links(id) ON DELETE CASCADE,
    url      TEXT NOT NULL,
    position INT NOT NULL DEFAULT 0,
    UNIQUE (link_id, position)
);
CREATE INDEX idx_link_targets_link ON link_targets(link_id);
INSERT INTO link_targets (link_id, url, position)
SELECT id, target_url, 0 FROM links WHERE deleted_at IS NULL;
ALTER TABLE links ADD COLUMN rr_index BIGINT NOT NULL DEFAULT 0;
ALTER TABLE links DROP COLUMN target_url;
