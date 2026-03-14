// Package usage provides usecase interfaces for usage tracking.
package usage

import (
	"context"
	"time"

	domainusage "github.com/duchoang/llmpool/internal/domain/usage"
)

// UsagePublisher is used by completion service to publish usage records.
type UsagePublisher interface {
	// Publish queues a usage record for async processing.
	// Returns immediately (at-most-once delivery).
	Publish(record domainusage.UsageRecord)
}

// UsageManager manages usage tracking operations.
type UsageManager interface {
	UsagePublisher

	// Start starts the background processing.
	Start(ctx context.Context)

	// Stop gracefully stops the manager.
	Stop()
}

// AuditRepository provides persistence for usage audit logs.
type AuditRepository interface {
	// Create stores a new audit log.
	Create(ctx context.Context, log domainusage.AuditLog) error

	// List returns audit logs within a time range.
	List(ctx context.Context, filter AuditLogFilter) ([]domainusage.AuditLog, error)

	// Count returns the count of audit logs within a time range.
	Count(ctx context.Context, filter AuditLogFilter) (int64, error)

	// GetByRequestID returns the audit log for a request.
	GetByRequestID(ctx context.Context, requestID string) (*domainusage.AuditLog, error)

	// DeleteBefore deletes logs older than the given time.
	DeleteBefore(ctx context.Context, before time.Time) (int64, error)

	// AggregateByModel returns aggregated stats by model.
	AggregateByModel(ctx context.Context, startTime, endTime time.Time) ([]domainusage.ModelStats, error)

	// AggregateByCredential returns aggregated stats by credential.
	AggregateByCredential(ctx context.Context, startTime, endTime time.Time) ([]domainusage.CredentialStats, error)

	// AggregateHourly returns hourly aggregated stats.
	AggregateHourly(ctx context.Context, startTime, endTime time.Time) ([]domainusage.HourlyStats, error)

	// AggregateDaily returns daily aggregated stats.
	AggregateDaily(ctx context.Context, startTime, endTime time.Time) ([]domainusage.DailyStats, error)

	// GetOverview returns overall stats for a time range.
	GetOverview(ctx context.Context, startTime, endTime time.Time) (*domainusage.Overview, error)
}

// AuditLogFilter describes query options for audit log listing/counting.
type AuditLogFilter struct {
	StartTime    time.Time
	EndTime      time.Time
	Limit        int
	Offset       int
	Model        string
	Provider     string
	CredentialID string
	Status       string
}

// DashboardStatsQuery describes query options for dashboard analytics.
type DashboardStatsQuery struct {
	Period    string
	StartDate *time.Time
	EndDate   *time.Time
}

// StatsService provides dashboard statistics.
type StatsService interface {
	// GetDashboardStats returns dashboard stats for the given period or explicit range.
	// Period can be: "today", "7d", "30d", "90d", "365d".
	// When StartDate and EndDate are provided, they take precedence over Period.
	GetDashboardStats(ctx context.Context, query DashboardStatsQuery) (*domainusage.DashboardStats, error)

	// GetAuditLogs returns paginated audit logs.
	GetAuditLogs(ctx context.Context, filter AuditLogFilter) ([]domainusage.AuditLog, int64, error)

	// GetAuditLogByRequestID returns a single audit log by request ID.
	GetAuditLogByRequestID(ctx context.Context, requestID string) (*domainusage.AuditLog, error)
}

// RetentionService handles cleanup of old audit logs.
type RetentionService interface {
	// Cleanup deletes audit logs older than retention period.
	Cleanup(ctx context.Context) (int64, error)
}
