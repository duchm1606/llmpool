-- Usage audit logs for tracking API requests
CREATE TABLE IF NOT EXISTS usage_audit_logs (
    id TEXT PRIMARY KEY,
    request_id TEXT NOT NULL,
    model TEXT NOT NULL,
    provider TEXT NOT NULL,
    credential_id TEXT NOT NULL,
    credential_type TEXT NOT NULL,
    credential_account_id TEXT NOT NULL,
    
    -- Token usage
    prompt_tokens INTEGER NOT NULL DEFAULT 0,
    completion_tokens INTEGER NOT NULL DEFAULT 0,
    total_tokens INTEGER NOT NULL DEFAULT 0,
    
    -- Pricing (stored in microdollars for precision: $0.01 = 10000)
    input_price_micros BIGINT NOT NULL DEFAULT 0,
    output_price_micros BIGINT NOT NULL DEFAULT 0,
    total_price_micros BIGINT NOT NULL DEFAULT 0,
    
    -- Status: done, canceled, failed
    status TEXT NOT NULL DEFAULT 'done',
    error_message TEXT,
    
    -- Timing
    started_at TIMESTAMPTZ NOT NULL,
    completed_at TIMESTAMPTZ NOT NULL,
    duration_ms INTEGER NOT NULL DEFAULT 0,
    
    -- Metadata
    stream BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Indexes for dashboard queries
CREATE INDEX IF NOT EXISTS idx_usage_audit_logs_created_at ON usage_audit_logs(created_at);
CREATE INDEX IF NOT EXISTS idx_usage_audit_logs_request_id ON usage_audit_logs(request_id);
CREATE INDEX IF NOT EXISTS idx_usage_audit_logs_model ON usage_audit_logs(model);
CREATE INDEX IF NOT EXISTS idx_usage_audit_logs_provider ON usage_audit_logs(provider);
CREATE INDEX IF NOT EXISTS idx_usage_audit_logs_credential_id ON usage_audit_logs(credential_id);
CREATE INDEX IF NOT EXISTS idx_usage_audit_logs_status ON usage_audit_logs(status);

-- Composite index for time-based aggregations
CREATE INDEX IF NOT EXISTS idx_usage_audit_logs_created_at_model ON usage_audit_logs(created_at, model);
CREATE INDEX IF NOT EXISTS idx_usage_audit_logs_created_at_credential ON usage_audit_logs(created_at, credential_id);
