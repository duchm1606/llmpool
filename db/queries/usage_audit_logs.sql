-- name: CreateUsageAuditLog :one
INSERT INTO usage_audit_logs (
    id,
    request_id,
    model,
    provider,
    credential_id,
    credential_type,
    credential_account_id,
    prompt_tokens,
    cached_tokens,
    completion_tokens,
    total_tokens,
    input_price_micros,
    output_price_micros,
    total_price_micros,
    status,
    error_message,
    started_at,
    completed_at,
    duration_ms,
    stream,
    created_at
)
VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9, $10,
    $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, NOW()
)
RETURNING *;

-- name: ListUsageAuditLogs :many
SELECT *
FROM usage_audit_logs
WHERE created_at >= @start_time::timestamptz
  AND created_at < @end_time::timestamptz
ORDER BY created_at DESC
LIMIT @limit_val::int
OFFSET @offset_val::int;

-- name: CountUsageAuditLogs :one
SELECT COUNT(*)
FROM usage_audit_logs
WHERE created_at >= @start_time::timestamptz
  AND created_at < @end_time::timestamptz;

-- name: GetUsageAuditLogByRequestID :one
SELECT *
FROM usage_audit_logs
WHERE request_id = $1
LIMIT 1;

-- name: ListUsageAuditLogsByModel :many
SELECT *
FROM usage_audit_logs
WHERE model = @model
  AND created_at >= @start_time::timestamptz
  AND created_at < @end_time::timestamptz
ORDER BY created_at DESC
LIMIT @limit_val::int
OFFSET @offset_val::int;

-- name: ListUsageAuditLogsByCredential :many
SELECT *
FROM usage_audit_logs
WHERE credential_id = @credential_id
  AND created_at >= @start_time::timestamptz
  AND created_at < @end_time::timestamptz
ORDER BY created_at DESC
LIMIT @limit_val::int
OFFSET @offset_val::int;

-- name: DeleteUsageAuditLogsBefore :execrows
DELETE FROM usage_audit_logs
WHERE created_at < $1;

-- name: AggregateUsageByModel :many
SELECT
    model,
    COUNT(*) as request_count,
    SUM(prompt_tokens) as total_prompt_tokens,
    SUM(cached_tokens) as total_cached_tokens,
    SUM(completion_tokens) as total_completion_tokens,
    SUM(total_tokens) as total_tokens,
    SUM(total_price_micros) as total_price_micros,
    COUNT(*) FILTER (WHERE status = 'done') as success_count,
    COUNT(*) FILTER (WHERE status = 'failed') as failed_count,
    COUNT(*) FILTER (WHERE status = 'canceled') as canceled_count
FROM usage_audit_logs
WHERE created_at >= @start_time::timestamptz
  AND created_at < @end_time::timestamptz
GROUP BY model
ORDER BY request_count DESC;

-- name: AggregateUsageByCredential :many
SELECT
    credential_id,
    credential_type,
    credential_account_id,
    COUNT(*) as request_count,
    SUM(prompt_tokens) as total_prompt_tokens,
    SUM(cached_tokens) as total_cached_tokens,
    SUM(completion_tokens) as total_completion_tokens,
    SUM(total_tokens) as total_tokens,
    SUM(total_price_micros) as total_price_micros,
    COUNT(*) FILTER (WHERE status = 'done') as success_count,
    COUNT(*) FILTER (WHERE status = 'failed') as failed_count,
    COUNT(*) FILTER (WHERE status = 'canceled') as canceled_count
FROM usage_audit_logs
WHERE created_at >= @start_time::timestamptz
  AND created_at < @end_time::timestamptz
GROUP BY credential_id, credential_type, credential_account_id
ORDER BY request_count DESC;

-- name: AggregateUsageHourly :many
SELECT
    date_trunc('hour', created_at) as hour,
    COUNT(*) as request_count,
    SUM(total_tokens) as total_tokens,
    SUM(total_price_micros) as total_price_micros,
    COUNT(*) FILTER (WHERE status = 'done') as success_count,
    COUNT(*) FILTER (WHERE status = 'failed') as failed_count
FROM usage_audit_logs
WHERE created_at >= @start_time::timestamptz
  AND created_at < @end_time::timestamptz
GROUP BY date_trunc('hour', created_at)
ORDER BY hour;

-- name: AggregateUsageDaily :many
SELECT
    date_trunc('day', created_at) as day,
    COUNT(*) as request_count,
    SUM(total_tokens) as total_tokens,
    SUM(total_price_micros) as total_price_micros,
    COUNT(*) FILTER (WHERE status = 'done') as success_count,
    COUNT(*) FILTER (WHERE status = 'failed') as failed_count
FROM usage_audit_logs
WHERE created_at >= @start_time::timestamptz
  AND created_at < @end_time::timestamptz
GROUP BY date_trunc('day', created_at)
ORDER BY day;

-- name: GetUsageOverview :one
SELECT
    COUNT(*) as total_requests,
    COALESCE(SUM(prompt_tokens), 0) as total_prompt_tokens,
    COALESCE(SUM(cached_tokens), 0) as total_cached_tokens,
    COALESCE(SUM(completion_tokens), 0) as total_completion_tokens,
    COALESCE(SUM(total_tokens), 0) as total_tokens,
    COALESCE(SUM(total_price_micros), 0) as total_price_micros,
    COUNT(*) FILTER (WHERE status = 'done') as success_count,
    COUNT(*) FILTER (WHERE status = 'failed') as failed_count,
    COUNT(*) FILTER (WHERE status = 'canceled') as canceled_count,
    COALESCE(AVG(duration_ms), 0) as avg_duration_ms
FROM usage_audit_logs
WHERE created_at >= @start_time::timestamptz
  AND created_at < @end_time::timestamptz;
