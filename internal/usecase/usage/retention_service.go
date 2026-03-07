package usage

import (
	"context"
	"time"

	"go.uber.org/zap"
)

// RetentionConfig holds configuration for the retention service.
type RetentionConfig struct {
	RetentionDays int // Number of days to retain audit logs
}

// DefaultRetentionConfig returns sensible defaults.
func DefaultRetentionConfig() RetentionConfig {
	return RetentionConfig{
		RetentionDays: 90,
	}
}

// retentionService implements RetentionService.
type retentionService struct {
	repo   AuditRepository
	config RetentionConfig
	logger *zap.Logger
}

// NewRetentionService creates a new retention service.
func NewRetentionService(
	repo AuditRepository,
	config RetentionConfig,
	logger *zap.Logger,
) RetentionService {
	return &retentionService{
		repo:   repo,
		config: config,
		logger: logger,
	}
}

// Cleanup deletes audit logs older than retention period.
func (s *retentionService) Cleanup(ctx context.Context) (int64, error) {
	cutoff := time.Now().UTC().AddDate(0, 0, -s.config.RetentionDays)

	s.logger.Info("running retention cleanup",
		zap.Int("retention_days", s.config.RetentionDays),
		zap.Time("cutoff", cutoff),
	)

	deleted, err := s.repo.DeleteBefore(ctx, cutoff)
	if err != nil {
		s.logger.Error("retention cleanup failed", zap.Error(err))
		return 0, err
	}

	s.logger.Info("retention cleanup complete",
		zap.Int64("deleted_count", deleted),
	)

	return deleted, nil
}
