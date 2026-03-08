DROP INDEX IF EXISTS idx_usage_audit_logs_cached_tokens;

ALTER TABLE usage_audit_logs
DROP COLUMN IF EXISTS cached_tokens;
