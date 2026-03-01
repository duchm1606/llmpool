package server

import (
	"context"
	"time"

	usecasecredential "github.com/duchoang/llmpool/internal/usecase/credential"
	"go.uber.org/zap"
)

type RefreshWorker struct {
	service  usecasecredential.RefreshService
	logger   *zap.Logger
	interval time.Duration
}

func NewRefreshWorker(service usecasecredential.RefreshService, logger *zap.Logger, interval time.Duration) *RefreshWorker {
	return &RefreshWorker{service: service, logger: logger, interval: interval}
}

func (w *RefreshWorker) Start(ctx context.Context) {
	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := w.service.RefreshDue(ctx); err != nil {
				w.logger.Error("refresh worker cycle failed", zap.Error(err))
			}
		}
	}
}
