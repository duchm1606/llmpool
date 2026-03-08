ALTER TABLE usage_audit_logs
ADD COLUMN IF NOT EXISTS cached_tokens INTEGER NOT NULL DEFAULT 0;

CREATE INDEX IF NOT EXISTS idx_usage_audit_logs_cached_tokens ON usage_audit_logs(cached_tokens);
