package usage

import (
	"context"
	"fmt"
	"time"

	domainusage "github.com/duchoang/llmpool/internal/domain/usage"
	"go.uber.org/zap"
)

const dailyKeyLayout = "2006-01-02"

// statsService implements StatsService.
type statsService struct {
	repo   AuditRepository
	logger *zap.Logger
}

// NewStatsService creates a new stats service.
func NewStatsService(
	repo AuditRepository,
	logger *zap.Logger,
) StatsService {
	return &statsService{
		repo:   repo,
		logger: logger,
	}
}

func periodToTimeRangeFromNow(now time.Time, period string) (start, end time.Time, err error) {
	now = now.UTC()
	end = now
	todayStart := beginningOfUTCDay(now)

	switch period {
	case "today":
		start = todayStart
	case "7d":
		start = todayStart.AddDate(0, 0, -6)
	case "30d":
		start = todayStart.AddDate(0, 0, -29)
	case "90d":
		start = todayStart.AddDate(0, 0, -89)
	case "365d":
		start = todayStart.AddDate(0, 0, -364)
	default:
		return time.Time{}, time.Time{}, fmt.Errorf("invalid period: %s", period)
	}

	return start, end, nil
}

func queryToTimeRange(now time.Time, query DashboardStatsQuery) (time.Time, time.Time, error) {
	if query.StartDate != nil || query.EndDate != nil {
		if query.StartDate == nil || query.EndDate == nil {
			return time.Time{}, time.Time{}, fmt.Errorf("startDate and endDate must both be provided")
		}

		startTime := query.StartDate.UTC()
		endTime := query.EndDate.UTC()
		if !startTime.Before(endTime) {
			return time.Time{}, time.Time{}, fmt.Errorf("startDate must be before endDate")
		}

		return startTime, endTime, nil
	}

	period := query.Period
	if period == "" {
		period = "today"
	}

	return periodToTimeRangeFromNow(now, period)
}

// GetDashboardStats returns dashboard stats for the given period.
func (s *statsService) GetDashboardStats(ctx context.Context, query DashboardStatsQuery) (*domainusage.DashboardStats, error) {
	startTime, endTime, err := queryToTimeRange(time.Now().UTC(), query)
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

	return stats, nil
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
	dailyStats = fillMissingDailyStats(startTime, endTime, dailyStats)

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

func beginningOfUTCDay(t time.Time) time.Time {
	t = t.UTC()
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
}

func fillMissingDailyStats(startTime, endTime time.Time, dailyStats []domainusage.DailyStats) []domainusage.DailyStats {
	startDay := beginningOfUTCDay(startTime)
	endDay := beginningOfUTCDay(endTime)

	if endTime.Equal(endDay) && endTime.After(startTime) {
		endDay = endDay.AddDate(0, 0, -1)
	}

	if endDay.Before(startDay) {
		return []domainusage.DailyStats{}
	}

	statsByDay := make(map[string]domainusage.DailyStats, len(dailyStats))
	for _, stat := range dailyStats {
		day := beginningOfUTCDay(stat.Day)
		stat.Day = day
		statsByDay[day.Format(dailyKeyLayout)] = stat
	}

	totalDays := int(endDay.Sub(startDay).Hours()/24) + 1
	filled := make([]domainusage.DailyStats, 0, totalDays)
	for i := 0; i < totalDays; i++ {
		day := startDay.AddDate(0, 0, i)
		if stat, ok := statsByDay[day.Format(dailyKeyLayout)]; ok {
			stat.Day = day
			filled = append(filled, stat)
			continue
		}

		filled = append(filled, domainusage.DailyStats{Day: day})
	}

	return filled
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
