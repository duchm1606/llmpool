package server

import (
	"context"
	"time"

	usecasequota "github.com/duchoang/llmpool/internal/usecase/quota"
	"go.uber.org/zap"
)

// QuotaWorkerConfig holds configuration for the quota worker.
type QuotaWorkerConfig struct {
	SampleInterval    time.Duration // 5m tick sample cycle
	FullSweepInterval time.Duration // 60m full sweep
}

// DefaultQuotaWorkerConfig returns default worker configuration.
func DefaultQuotaWorkerConfig() QuotaWorkerConfig {
	return QuotaWorkerConfig{
		SampleInterval:    5 * time.Minute,
		FullSweepInterval: 60 * time.Minute,
	}
}

// QuotaWorker runs background liveness checks.
type QuotaWorker struct {
	service usecasequota.LivenessService
	logger  *zap.Logger
	cfg     QuotaWorkerConfig
}

// NewQuotaWorker creates a new quota worker.
func NewQuotaWorker(
	service usecasequota.LivenessService,
	logger *zap.Logger,
	cfg QuotaWorkerConfig,
) *QuotaWorker {
	return &QuotaWorker{
		service: service,
		logger:  logger,
		cfg:     cfg,
	}
}

// Start begins the background worker loop.
func (w *QuotaWorker) Start(ctx context.Context) {
	w.logger.Info("quota worker starting",
		zap.Duration("sample_interval", w.cfg.SampleInterval),
		zap.Duration("full_sweep_interval", w.cfg.FullSweepInterval),
	)

	// Check for rehydration on startup
	w.checkRehydration(ctx)

	sampleTicker := time.NewTicker(w.cfg.SampleInterval)
	fullSweepTicker := time.NewTicker(w.cfg.FullSweepInterval)
	defer sampleTicker.Stop()
	defer fullSweepTicker.Stop()

	for {
		select {
		case <-ctx.Done():
			w.logger.Info("quota worker stopping")
			return

		case <-sampleTicker.C:
			w.runSampleCheck(ctx)

		case <-fullSweepTicker.C:
			w.runFullSweep(ctx)
		}
	}
}

// checkRehydration checks if cache needs rehydration and runs full sweep if needed.
func (w *QuotaWorker) checkRehydration(ctx context.Context) {
	needsRehydration, err := w.service.NeedsRehydration(ctx)
	if err != nil {
		w.logger.Error("rehydration check failed", zap.Error(err))
		// On error, assume rehydration needed
		needsRehydration = true
	}

	if needsRehydration {
		w.logger.Info("cache needs rehydration, running full sweep")
		w.runFullSweep(ctx)
	}
}

// runSampleCheck runs a sample-based liveness check.
func (w *QuotaWorker) runSampleCheck(ctx context.Context) {
	// First check if rehydration is needed (Redis may have been restarted)
	needsRehydration, _ := w.service.NeedsRehydration(ctx)
	if needsRehydration {
		w.logger.Info("cache empty during sample check, running full sweep instead")
		w.runFullSweep(ctx)
		return
	}

	start := time.Now()
	if err := w.service.CheckSample(ctx); err != nil {
		w.logger.Error("sample check failed", zap.Error(err))
		return
	}
	w.logger.Info("sample check completed", zap.Duration("duration", time.Since(start)))
}

// runFullSweep runs a full sweep of all credentials.
func (w *QuotaWorker) runFullSweep(ctx context.Context) {
	start := time.Now()
	if err := w.service.CheckAll(ctx); err != nil {
		w.logger.Error("full sweep failed", zap.Error(err))
		return
	}
	w.logger.Info("full sweep completed", zap.Duration("duration", time.Since(start)))
}
