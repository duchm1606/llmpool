CREATE INDEX IF NOT EXISTS idx_usage_audit_logs_completed_at ON usage_audit_logs(completed_at);
CREATE INDEX IF NOT EXISTS idx_usage_audit_logs_completed_at_model ON usage_audit_logs(completed_at, model);
CREATE INDEX IF NOT EXISTS idx_usage_audit_logs_completed_at_credential ON usage_audit_logs(completed_at, credential_id);
