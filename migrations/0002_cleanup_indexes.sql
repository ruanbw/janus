-- +goose Up
-- 0002_cleanup_indexes.sql — 后台清理任务索引
-- worker 定期清理过期会话/访问记录(见 internal/domain/worker.go);
-- 按 expires_at 删除,索引避免全表扫描。
-- email_tokens 的过期清理待 store 侧补充 DeleteExpiredEmailTokens 后启用(索引先行,幂等)。

CREATE INDEX IF NOT EXISTS idx_sessions_expires ON sessions(expires_at);
CREATE INDEX IF NOT EXISTS idx_email_tokens_expires ON email_tokens(expires_at);
