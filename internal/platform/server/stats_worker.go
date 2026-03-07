package server

import (
	"context"
	"time"

	usecaseusage "github.com/duchoang/llmpool/internal/usecase/usage"
	"go.uber.org/zap"
)

// StatsWorkerConfig holds configuration for the stats aggregation worker.
type StatsWorkerConfig struct {
	RebuildInterval time.Duration // How often to rebuild cached aggregated stats
}

// DefaultStatsWorkerConfig returns default worker configuration.
func DefaultStatsWorkerConfig() StatsWorkerConfig {
	return StatsWorkerConfig{
		RebuildInterval: 15 * time.Minute,
	}
}

// StatsWorker runs periodic dashboard stats aggregation and cache rebuild.
type StatsWorker struct {
	service usecaseusage.StatsService
	logger  *zap.Logger
	cfg     StatsWorkerConfig
}

// NewStatsWorker creates a new stats worker.
func NewStatsWorker(
	service usecaseusage.StatsService,
	logger *zap.Logger,
	cfg StatsWorkerConfig,
) *StatsWorker {
	return &StatsWorker{
		service: service,
		logger:  logger,
		cfg:     cfg,
	}
}

// Start begins the background worker loop.
func (w *StatsWorker) Start(ctx context.Context) {
	w.logger.Info("stats worker starting",
		zap.Duration("rebuild_interval", w.cfg.RebuildInterval),
	)

	// Rebuild once on startup.
	w.runRebuild(ctx)

	rebuildTicker := time.NewTicker(w.cfg.RebuildInterval)
	defer rebuildTicker.Stop()

	for {
		select {
		case <-ctx.Done():
			w.logger.Info("stats worker stopping")
			return

		case <-rebuildTicker.C:
			w.runRebuild(ctx)
		}
	}
}

// runRebuild rebuilds cached dashboard stats from PostgreSQL.
func (w *StatsWorker) runRebuild(ctx context.Context) {
	start := time.Now()
	if err := w.service.RebuildStats(ctx); err != nil {
		w.logger.Error("stats rebuild failed", zap.Error(err))
		return
	}
	w.logger.Info("stats rebuild completed", zap.Duration("duration", time.Since(start)))
}
