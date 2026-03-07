package usage

import (
	"context"
	"fmt"
	"time"

	domainusage "github.com/duchoang/llmpool/internal/domain/usage"
	"go.uber.org/zap"
)

// StatsServiceConfig holds configuration for the stats service.
type StatsServiceConfig struct {
	CacheTTL time.Duration // How long to cache stats
}

// DefaultStatsServiceConfig returns sensible defaults.
func DefaultStatsServiceConfig() StatsServiceConfig {
	return StatsServiceConfig{
		CacheTTL: 5 * time.Minute,
	}
}

// statsService implements StatsService.
type statsService struct {
	repo   AuditRepository
	cache  StatsCache
	config StatsServiceConfig
	logger *zap.Logger
}

// NewStatsService creates a new stats service.
func NewStatsService(
	repo AuditRepository,
	cache StatsCache,
	config StatsServiceConfig,
	logger *zap.Logger,
) StatsService {
	return &statsService{
		repo:   repo,
		cache:  cache,
		config: config,
		logger: logger,
	}
}

// periodToTimeRange converts a period string to start/end times.
func periodToTimeRange(period string) (start, end time.Time, err error) {
	now := time.Now().UTC()
	end = now

	switch period {
	case "today":
		start = time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	case "7d":
		start = now.AddDate(0, 0, -7)
	case "30d":
		start = now.AddDate(0, 0, -30)
	case "90d":
		start = now.AddDate(0, 0, -90)
	case "365d":
		start = now.AddDate(0, 0, -365)
	default:
		return time.Time{}, time.Time{}, fmt.Errorf("invalid period: %s", period)
	}

	return start, end, nil
}

// GetDashboardStats returns dashboard stats for the given period.
func (s *statsService) GetDashboardStats(ctx context.Context, period string) (*domainusage.DashboardStats, error) {
	// Try cache first
	if cached, err := s.cache.GetDashboardStats(ctx, period); err == nil && cached != nil {
		s.logger.Debug("returning cached stats", zap.String("period", period))
		return cached, nil
	}

	// Build from database
	startTime, endTime, err := periodToTimeRange(period)
	if err != nil {
		return nil, err
	}

	stats, err := s.buildStats(ctx, startTime, endTime)
	if err != nil {
		return nil, err
	}

	stats.Period = domainusage.StatsPeriod{
		StartTime: startTime,
		EndTime:   endTime,
	}

	// Cache the result
	if cacheErr := s.cache.SetDashboardStats(ctx, period, *stats, s.config.CacheTTL); cacheErr != nil {
		s.logger.Warn("failed to cache stats", zap.Error(cacheErr))
	}

	return stats, nil
}

// RebuildStats rebuilds cached stats from PostgreSQL.
func (s *statsService) RebuildStats(ctx context.Context) error {
	s.logger.Info("rebuilding stats from PostgreSQL")

	// Invalidate existing cache
	if err := s.cache.InvalidateStats(ctx); err != nil {
		s.logger.Warn("failed to invalidate cache", zap.Error(err))
	}

	// Rebuild for all standard periods
	periods := []string{"today", "7d", "30d", "90d", "365d"}
	for _, period := range periods {
		startTime, endTime, err := periodToTimeRange(period)
		if err != nil {
			continue
		}

		stats, err := s.buildStats(ctx, startTime, endTime)
		if err != nil {
			s.logger.Error("failed to build stats",
				zap.String("period", period),
				zap.Error(err),
			)
			continue
		}

		stats.Period = domainusage.StatsPeriod{
			StartTime: startTime,
			EndTime:   endTime,
		}

		if cacheErr := s.cache.SetDashboardStats(ctx, period, *stats, s.config.CacheTTL); cacheErr != nil {
			s.logger.Warn("failed to cache stats",
				zap.String("period", period),
				zap.Error(cacheErr),
			)
		}
	}

	s.logger.Info("stats rebuild complete")
	return nil
}

func (s *statsService) buildStats(ctx context.Context, startTime, endTime time.Time) (*domainusage.DashboardStats, error) {
	// Get overview
	overview, err := s.repo.GetOverview(ctx, startTime, endTime)
	if err != nil {
		return nil, fmt.Errorf("get overview: %w", err)
	}

	// Get hourly stats
	hourlyStats, err := s.repo.AggregateHourly(ctx, startTime, endTime)
	if err != nil {
		return nil, fmt.Errorf("aggregate hourly: %w", err)
	}
	if hourlyStats == nil {
		hourlyStats = []domainusage.HourlyStats{}
	}

	// Get daily stats
	dailyStats, err := s.repo.AggregateDaily(ctx, startTime, endTime)
	if err != nil {
		return nil, fmt.Errorf("aggregate daily: %w", err)
	}
	if dailyStats == nil {
		dailyStats = []domainusage.DailyStats{}
	}

	// Get model stats
	modelStats, err := s.repo.AggregateByModel(ctx, startTime, endTime)
	if err != nil {
		return nil, fmt.Errorf("aggregate by model: %w", err)
	}
	if modelStats == nil {
		modelStats = []domainusage.ModelStats{}
	}

	// Get credential stats
	credentialStats, err := s.repo.AggregateByCredential(ctx, startTime, endTime)
	if err != nil {
		return nil, fmt.Errorf("aggregate by credential: %w", err)
	}
	if credentialStats == nil {
		credentialStats = []domainusage.CredentialStats{}
	}

	return &domainusage.DashboardStats{
		Overview:        *overview,
		HourlyStats:     hourlyStats,
		DailyStats:      dailyStats,
		ModelStats:      modelStats,
		CredentialStats: credentialStats,
		GeneratedAt:     time.Now().UTC(),
	}, nil
}

// GetAuditLogs returns paginated audit logs.
func (s *statsService) GetAuditLogs(ctx context.Context, filter AuditLogFilter) ([]domainusage.AuditLog, int64, error) {
	logs, err := s.repo.List(ctx, filter)
	if err != nil {
		return nil, 0, fmt.Errorf("list audit logs: %w", err)
	}

	count, err := s.repo.Count(ctx, filter)
	if err != nil {
		return nil, 0, fmt.Errorf("count audit logs: %w", err)
	}

	return logs, count, nil
}

// GetAuditLogByRequestID returns a single audit log by request ID.
func (s *statsService) GetAuditLogByRequestID(ctx context.Context, requestID string) (*domainusage.AuditLog, error) {
	return s.repo.GetByRequestID(ctx, requestID)
}
