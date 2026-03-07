package server

import (
	"context"
	"time"

	usecaseusage "github.com/duchoang/llmpool/internal/usecase/usage"
	"go.uber.org/zap"
)

// RetentionWorkerConfig holds configuration for the retention worker.
type RetentionWorkerConfig struct {
	CleanupInterval time.Duration // How often to run retention cleanup
}

// DefaultRetentionWorkerConfig returns default worker configuration.
func DefaultRetentionWorkerConfig() RetentionWorkerConfig {
	return RetentionWorkerConfig{
		CleanupInterval: 24 * time.Hour,
	}
}

// RetentionWorker runs background retention cleanup for usage audit logs.
type RetentionWorker struct {
	service usecaseusage.RetentionService
	logger  *zap.Logger
	cfg     RetentionWorkerConfig
}

// NewRetentionWorker creates a new retention worker.
func NewRetentionWorker(
	service usecaseusage.RetentionService,
	logger *zap.Logger,
	cfg RetentionWorkerConfig,
) *RetentionWorker {
	return &RetentionWorker{
		service: service,
		logger:  logger,
		cfg:     cfg,
	}
}

// Start begins the background worker loop.
func (w *RetentionWorker) Start(ctx context.Context) {
	w.logger.Info("retention worker starting",
		zap.Duration("cleanup_interval", w.cfg.CleanupInterval),
	)

	// Run cleanup on startup
	w.runCleanup(ctx)

	cleanupTicker := time.NewTicker(w.cfg.CleanupInterval)
	defer cleanupTicker.Stop()

	for {
		select {
		case <-ctx.Done():
			w.logger.Info("retention worker stopping")
			return

		case <-cleanupTicker.C:
			w.runCleanup(ctx)
		}
	}
}

// runCleanup runs the retention cleanup.
func (w *RetentionWorker) runCleanup(ctx context.Context) {
	start := time.Now()
	deleted, err := w.service.Cleanup(ctx)
	if err != nil {
		w.logger.Error("retention cleanup failed", zap.Error(err))
		return
	}
	w.logger.Info("retention cleanup completed",
		zap.Int64("deleted", deleted),
		zap.Duration("duration", time.Since(start)),
	)
}
