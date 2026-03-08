package usage

import (
	"context"
	"fmt"
	"time"

	domainusage "github.com/duchoang/llmpool/internal/domain/usage"
	usecaseusage "github.com/duchoang/llmpool/internal/usecase/usage"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

// PostgresRepository implements AuditRepository using PostgreSQL.
type PostgresRepository struct {
	pool *pgxpool.Pool
}

// NewPostgresRepository creates a new PostgreSQL repository.
func NewPostgresRepository(pool *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{pool: pool}
}

// Create stores a new audit log.
func (r *PostgresRepository) Create(ctx context.Context, log domainusage.AuditLog) error {
	query := `
		INSERT INTO usage_audit_logs (
			id, request_id, model, provider, credential_id, credential_type, credential_account_id,
			prompt_tokens, cached_tokens, completion_tokens, total_tokens,
			input_price_micros, output_price_micros, total_price_micros,
			status, error_message, started_at, completed_at, duration_ms, stream, created_at
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, NOW()
		)
	`

	_, err := r.pool.Exec(ctx, query,
		log.ID,
		log.RequestID,
		log.Model,
		log.Provider,
		log.CredentialID,
		log.CredentialType,
		log.CredentialAccountID,
		log.PromptTokens,
		log.CachedTokens,
		log.CompletionTokens,
		log.TotalTokens,
		log.InputPriceMicros,
		log.OutputPriceMicros,
		log.TotalPriceMicros,
		string(log.Status),
		log.ErrorMessage,
		log.StartedAt,
		log.CompletedAt,
		log.DurationMs,
		log.Stream,
	)
	if err != nil {
		return fmt.Errorf("insert usage audit log: %w", err)
	}

	return nil
}

// List returns audit logs within a time range and optional filters.
func (r *PostgresRepository) List(ctx context.Context, filter usecaseusage.AuditLogFilter) ([]domainusage.AuditLog, error) {
	query := `
		SELECT id, request_id, model, provider, credential_id, credential_type, credential_account_id,
			prompt_tokens, cached_tokens, completion_tokens, total_tokens,
			input_price_micros, output_price_micros, total_price_micros,
			status, error_message, started_at, completed_at, duration_ms, stream, created_at
		FROM usage_audit_logs
		WHERE created_at >= $1 AND created_at < $2
		  AND ($3 = '' OR model = $3)
		  AND ($4 = '' OR provider = $4)
		  AND ($5 = '' OR credential_id = $5)
		  AND ($6 = '' OR status = $6)
		ORDER BY created_at DESC
		LIMIT $7 OFFSET $8
	`

	rows, err := r.pool.Query(
		ctx,
		query,
		filter.StartTime,
		filter.EndTime,
		filter.Model,
		filter.Provider,
		filter.CredentialID,
		filter.Status,
		filter.Limit,
		filter.Offset,
	)
	if err != nil {
		return nil, fmt.Errorf("query usage audit logs: %w", err)
	}
	defer rows.Close()

	return r.scanLogs(rows)
}

// Count returns the count of audit logs within a time range and optional filters.
func (r *PostgresRepository) Count(ctx context.Context, filter usecaseusage.AuditLogFilter) (int64, error) {
	query := `
		SELECT COUNT(*) FROM usage_audit_logs
		WHERE created_at >= $1 AND created_at < $2
		  AND ($3 = '' OR model = $3)
		  AND ($4 = '' OR provider = $4)
		  AND ($5 = '' OR credential_id = $5)
		  AND ($6 = '' OR status = $6)
	`

	var count int64
	err := r.pool.QueryRow(
		ctx,
		query,
		filter.StartTime,
		filter.EndTime,
		filter.Model,
		filter.Provider,
		filter.CredentialID,
		filter.Status,
	).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("count usage audit logs: %w", err)
	}

	return count, nil
}

// GetByRequestID returns the audit log for a request.
func (r *PostgresRepository) GetByRequestID(ctx context.Context, requestID string) (*domainusage.AuditLog, error) {
	query := `
		SELECT id, request_id, model, provider, credential_id, credential_type, credential_account_id,
			prompt_tokens, cached_tokens, completion_tokens, total_tokens,
			input_price_micros, output_price_micros, total_price_micros,
			status, error_message, started_at, completed_at, duration_ms, stream, created_at
		FROM usage_audit_logs
		WHERE request_id = $1
		LIMIT 1
	`

	row := r.pool.QueryRow(ctx, query, requestID)
	log, err := r.scanLog(row)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("get usage audit log by request_id: %w", err)
	}

	return log, nil
}

// DeleteBefore deletes logs older than the given time.
func (r *PostgresRepository) DeleteBefore(ctx context.Context, before time.Time) (int64, error) {
	result, err := r.pool.Exec(ctx, `DELETE FROM usage_audit_logs WHERE created_at < $1`, before)
	if err != nil {
		return 0, fmt.Errorf("delete usage audit logs: %w", err)
	}

	return result.RowsAffected(), nil
}

// AggregateByModel returns aggregated stats by model.
func (r *PostgresRepository) AggregateByModel(ctx context.Context, startTime, endTime time.Time) ([]domainusage.ModelStats, error) {
	query := `
		SELECT
			model,
			COUNT(*) as request_count,
			COALESCE(SUM(prompt_tokens), 0) as total_prompt_tokens,
			COALESCE(SUM(cached_tokens), 0) as total_cached_tokens,
			COALESCE(SUM(completion_tokens), 0) as total_completion_tokens,
			COALESCE(SUM(total_tokens), 0) as total_tokens,
			COALESCE(SUM(total_price_micros), 0) as total_price_micros,
			COUNT(*) FILTER (WHERE status = 'done') as success_count,
			COUNT(*) FILTER (WHERE status = 'failed') as failed_count,
			COUNT(*) FILTER (WHERE status = 'canceled') as canceled_count
		FROM usage_audit_logs
		WHERE created_at >= $1 AND created_at < $2
		GROUP BY model
		ORDER BY request_count DESC
	`

	rows, err := r.pool.Query(ctx, query, startTime, endTime)
	if err != nil {
		return nil, fmt.Errorf("aggregate by model: %w", err)
	}
	defer rows.Close()

	var results []domainusage.ModelStats
	for rows.Next() {
		var s domainusage.ModelStats
		err := rows.Scan(
			&s.Model,
			&s.RequestCount,
			&s.PromptTokens,
			&s.CachedTokens,
			&s.CompletionTokens,
			&s.TotalTokens,
			&s.TotalPriceMicros,
			&s.SuccessCount,
			&s.FailedCount,
			&s.CanceledCount,
		)
		if err != nil {
			return nil, fmt.Errorf("scan model stats: %w", err)
		}
		results = append(results, s)
	}

	return results, nil
}

// AggregateByCredential returns aggregated stats by credential.
func (r *PostgresRepository) AggregateByCredential(ctx context.Context, startTime, endTime time.Time) ([]domainusage.CredentialStats, error) {
	query := `
		SELECT
			credential_id,
			credential_type,
			credential_account_id,
			COUNT(*) as request_count,
			COALESCE(SUM(prompt_tokens), 0) as total_prompt_tokens,
			COALESCE(SUM(cached_tokens), 0) as total_cached_tokens,
			COALESCE(SUM(completion_tokens), 0) as total_completion_tokens,
			COALESCE(SUM(total_tokens), 0) as total_tokens,
			COALESCE(SUM(total_price_micros), 0) as total_price_micros,
			COUNT(*) FILTER (WHERE status = 'done') as success_count,
			COUNT(*) FILTER (WHERE status = 'failed') as failed_count,
			COUNT(*) FILTER (WHERE status = 'canceled') as canceled_count
		FROM usage_audit_logs
		WHERE created_at >= $1 AND created_at < $2
		GROUP BY credential_id, credential_type, credential_account_id
		ORDER BY request_count DESC
	`

	rows, err := r.pool.Query(ctx, query, startTime, endTime)
	if err != nil {
		return nil, fmt.Errorf("aggregate by credential: %w", err)
	}
	defer rows.Close()

	var results []domainusage.CredentialStats
	for rows.Next() {
		var s domainusage.CredentialStats
		err := rows.Scan(
			&s.CredentialID,
			&s.CredentialType,
			&s.CredentialAccountID,
			&s.RequestCount,
			&s.PromptTokens,
			&s.CachedTokens,
			&s.CompletionTokens,
			&s.TotalTokens,
			&s.TotalPriceMicros,
			&s.SuccessCount,
			&s.FailedCount,
			&s.CanceledCount,
		)
		if err != nil {
			return nil, fmt.Errorf("scan credential stats: %w", err)
		}
		results = append(results, s)
	}

	return results, nil
}

// AggregateHourly returns hourly aggregated stats.
func (r *PostgresRepository) AggregateHourly(ctx context.Context, startTime, endTime time.Time) ([]domainusage.HourlyStats, error) {
	query := `
		SELECT
			date_trunc('hour', created_at) as hour,
			COUNT(*) as request_count,
			COALESCE(SUM(total_tokens), 0) as total_tokens,
			COALESCE(SUM(total_price_micros), 0) as total_price_micros,
			COUNT(*) FILTER (WHERE status = 'done') as success_count,
			COUNT(*) FILTER (WHERE status = 'failed') as failed_count
		FROM usage_audit_logs
		WHERE created_at >= $1 AND created_at < $2
		GROUP BY date_trunc('hour', created_at)
		ORDER BY hour
	`

	rows, err := r.pool.Query(ctx, query, startTime, endTime)
	if err != nil {
		return nil, fmt.Errorf("aggregate hourly: %w", err)
	}
	defer rows.Close()

	var results []domainusage.HourlyStats
	for rows.Next() {
		var s domainusage.HourlyStats
		err := rows.Scan(
			&s.Hour,
			&s.RequestCount,
			&s.TotalTokens,
			&s.TotalPriceMicros,
			&s.SuccessCount,
			&s.FailedCount,
		)
		if err != nil {
			return nil, fmt.Errorf("scan hourly stats: %w", err)
		}
		results = append(results, s)
	}

	return results, nil
}

// AggregateDaily returns daily aggregated stats.
func (r *PostgresRepository) AggregateDaily(ctx context.Context, startTime, endTime time.Time) ([]domainusage.DailyStats, error) {
	query := `
		SELECT
			date_trunc('day', created_at) as day,
			COUNT(*) as request_count,
			COALESCE(SUM(total_tokens), 0) as total_tokens,
			COALESCE(SUM(total_price_micros), 0) as total_price_micros,
			COUNT(*) FILTER (WHERE status = 'done') as success_count,
			COUNT(*) FILTER (WHERE status = 'failed') as failed_count
		FROM usage_audit_logs
		WHERE created_at >= $1 AND created_at < $2
		GROUP BY date_trunc('day', created_at)
		ORDER BY day
	`

	rows, err := r.pool.Query(ctx, query, startTime, endTime)
	if err != nil {
		return nil, fmt.Errorf("aggregate daily: %w", err)
	}
	defer rows.Close()

	var results []domainusage.DailyStats
	for rows.Next() {
		var s domainusage.DailyStats
		err := rows.Scan(
			&s.Day,
			&s.RequestCount,
			&s.TotalTokens,
			&s.TotalPriceMicros,
			&s.SuccessCount,
			&s.FailedCount,
		)
		if err != nil {
			return nil, fmt.Errorf("scan daily stats: %w", err)
		}
		results = append(results, s)
	}

	return results, nil
}

// GetOverview returns overall stats for a time range.
func (r *PostgresRepository) GetOverview(ctx context.Context, startTime, endTime time.Time) (*domainusage.Overview, error) {
	query := `
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
		WHERE created_at >= $1 AND created_at < $2
	`

	var o domainusage.Overview
	err := r.pool.QueryRow(ctx, query, startTime, endTime).Scan(
		&o.TotalRequests,
		&o.TotalPromptTokens,
		&o.TotalCachedTokens,
		&o.TotalCompletionTokens,
		&o.TotalTokens,
		&o.TotalPriceMicros,
		&o.SuccessCount,
		&o.FailedCount,
		&o.CanceledCount,
		&o.AvgDurationMs,
	)
	if err != nil {
		return nil, fmt.Errorf("get overview: %w", err)
	}

	return &o, nil
}

func (r *PostgresRepository) scanLogs(rows pgx.Rows) ([]domainusage.AuditLog, error) {
	logs := make([]domainusage.AuditLog, 0)
	for rows.Next() {
		log, err := r.scanLogFromRows(rows)
		if err != nil {
			return nil, err
		}
		logs = append(logs, *log)
	}
	return logs, nil
}

func (r *PostgresRepository) scanLogFromRows(rows pgx.Rows) (*domainusage.AuditLog, error) {
	var log domainusage.AuditLog
	var status string
	var errorMessage pgtype.Text
	var startedAt, completedAt, createdAt pgtype.Timestamptz

	err := rows.Scan(
		&log.ID,
		&log.RequestID,
		&log.Model,
		&log.Provider,
		&log.CredentialID,
		&log.CredentialType,
		&log.CredentialAccountID,
		&log.PromptTokens,
		&log.CachedTokens,
		&log.CompletionTokens,
		&log.TotalTokens,
		&log.InputPriceMicros,
		&log.OutputPriceMicros,
		&log.TotalPriceMicros,
		&status,
		&errorMessage,
		&startedAt,
		&completedAt,
		&log.DurationMs,
		&log.Stream,
		&createdAt,
	)
	if err != nil {
		return nil, fmt.Errorf("scan log: %w", err)
	}

	log.Status = domainusage.Status(status)
	if errorMessage.Valid {
		log.ErrorMessage = errorMessage.String
	}
	if startedAt.Valid {
		log.StartedAt = startedAt.Time
	}
	if completedAt.Valid {
		log.CompletedAt = completedAt.Time
	}
	if createdAt.Valid {
		log.CreatedAt = createdAt.Time
	}

	return &log, nil
}

func (r *PostgresRepository) scanLog(row pgx.Row) (*domainusage.AuditLog, error) {
	var log domainusage.AuditLog
	var status string
	var errorMessage pgtype.Text
	var startedAt, completedAt, createdAt pgtype.Timestamptz

	err := row.Scan(
		&log.ID,
		&log.RequestID,
		&log.Model,
		&log.Provider,
		&log.CredentialID,
		&log.CredentialType,
		&log.CredentialAccountID,
		&log.PromptTokens,
		&log.CachedTokens,
		&log.CompletionTokens,
		&log.TotalTokens,
		&log.InputPriceMicros,
		&log.OutputPriceMicros,
		&log.TotalPriceMicros,
		&status,
		&errorMessage,
		&startedAt,
		&completedAt,
		&log.DurationMs,
		&log.Stream,
		&createdAt,
	)
	if err != nil {
		return nil, err
	}

	log.Status = domainusage.Status(status)
	if errorMessage.Valid {
		log.ErrorMessage = errorMessage.String
	}
	if startedAt.Valid {
		log.StartedAt = startedAt.Time
	}
	if completedAt.Valid {
		log.CompletedAt = completedAt.Time
	}
	if createdAt.Valid {
		log.CreatedAt = createdAt.Time
	}

	return &log, nil
}
